package eth

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	zlog "github.com/rs/zerolog/log"
	common2 "github.com/zhiqiangxu/litenode/eth/common"
	"github.com/zhiqiangxu/litenode/eth/internal"
	eth2 "github.com/zhiqiangxu/litenode/eth/protocols/eth"
)

// ethHandler implements the eth.Backend interface to handle the various network
// packets that are sent as replies or broadcasts.
type ethHandler Handler

func (h *ethHandler) Chain() *core.BlockChain {
	return nil
}

func (h *ethHandler) TxPool() eth.TxPool {
	return h.txPool
}

func (h *ethHandler) AcceptTxs() bool {
	return true
}

func (h *ethHandler) RunPeer(peer *eth.Peer, handler eth.Handler) error {

	var snapPeer *snap.Peer
	if h.snapEnabled {
		var err error
		// If the peer has a `snap` extension, wait for it to connect so we can have
		// a uniform initialization/teardown mechanism
		snapPeer, err = h.peers.WaitSnapExtension(peer)
		if err != nil {
			peer.Log().Error("Snapshot extension barrier failed", "err", err)
			return err
		}
	}
	h.peerWG.Add(1)
	defer h.peerWG.Done()

	ms, err := peer.HandshakeLite(h.NetworkID, h.GenesisHash, h.Upgrade)
	if err != nil {
		// zlog.Error().Err(err).
		// 	Str("peer.id", peer.ID()).
		// 	Str("peer.ip", peer.RemoteAddr().String()).
		// 	Str("status", "failed").
		// 	Msg("ethereum:handshake")
		return err
	}
	if h.StatusFeed {
		h.statusFeed.Send(ms)
	}

	// Ignore maxPeers if this is a trusted peer
	if !peer.Peer.Info().Network.Trusted {
		if h.peers.Len() >= h.maxPeers {
			return p2p.DiscTooManyPeers
		}
	}
	peer.Log().Debug("Ethereum peer connected", "name", peer.Name())
	// Register the peer locally
	if err := h.peers.RegisterPeer(peer, snapPeer); err != nil {
		peer.Log().Error("Ethereum peer registration failed", "err", err)
		return err
	}
	defer (*Handler)(h).unregisterPeer(peer.ID())

	p := h.peers.Peer(peer.ID())
	if p == nil {
		return errors.New("peer dropped during handling")
	}

	if snapPeer != nil {
		if h.SnapSyncer != nil {
			if err := h.SnapSyncer.Register(snapPeer); err != nil {
				peer.Log().Error("Failed to register peer in snap syncer", "err", err)
				return err
			}
		}

	}

	return handler(peer)
}

func (h *ethHandler) PeerInfo(id enode.ID) interface{} {
	if p := h.peers.Peer(id.String()); p != nil {
		return p.Info()
	}
	return nil
}

var (
	zeroHash             common.Hash
	syncChallengeTimeout = 7 * time.Second
)

var (
	// errPeerNotRegistered is returned if a peer is attempted to be removed from
	// a peer set, but no peer with the given id exists.
	errPeerNotRegistered = errors.New("peer not registered")
)

func (h *ethHandler) handleSyncChallenge(peer *eth.Peer, query *eth2.GetBlockHeadersPacket) error {
	if query.Origin.Hash != zeroHash {
		zlog.Info().Str("id", peer.ID()).Str("hash", query.Origin.Hash.Hex()).Msg("ignored GetBlockHeadersPacket for non challenge")
		return nil
	}

	if query.Amount != 1 {
		zlog.Info().Str("id", peer.ID()).Uint64("amount", query.Amount).Msg("ignored GetBlockHeadersPacket for non challenge")
		return nil
	}

	zlog.Info().Str("id", peer.ID()).Msg("handle sync challenge for old protocol")

	err := p2p.Send(peer.RW, eth2.GetBlockHeadersMsg, query)
	if err != nil {
		return err
	}
	h.syncChallengeHeaderPool.RememberChallenge(query.Origin.Number, &internal.ChanllengeCB{Peer: peer})
	return nil
}

func (h *ethHandler) handleSyncChallenge66(peer *eth.Peer, query *eth2.GetBlockHeadersPacket66) error {

	if query.Origin.Hash != zeroHash {
		return nil
	}

	if query.Amount != 1 {
		return nil
	}

	zlog.Info().Str("id", peer.ID()).Msg("handle sync challenge")

	if h.syncChallengeHeaderPool != nil {
		header := h.syncChallengeHeaderPool.GetHeader(query.Origin.Number)
		if header != nil {
			// fast path

			rlpData, _ := rlp.EncodeToBytes(header)
			response := []rlp.RawValue{rlpData}
			return peer.ReplyBlockHeadersRLP(query.RequestId, response)
		}
	}

	// slow path

	resCh := make(chan *eth.Response)
	req, err := peer.RequestHeadersByNumber(query.Origin.Number, 1, 0, false, resCh)
	if err != nil {
		return err
	}
	go func() {
		timeout := time.NewTimer(syncChallengeTimeout)
		defer func() {
			req.Close()
			timeout.Stop()
		}()

		select {
		case res := <-resCh:
			headers := ([]*types.Header)(*res.Res.(*eth2.BlockHeadersPacket))
			if len(headers) != 1 {
				res.Done <- errors.New("#headers != 1")
				return
			}

			if h.syncChallengeHeaderPool != nil {
				h.syncChallengeHeaderPool.AddHeaderIfNotExists(headers[0])
			}

			rlpData, _ := rlp.EncodeToBytes(headers[0])
			response := []rlp.RawValue{rlpData}
			err := peer.ReplyBlockHeadersRLP(query.RequestId, response)
			if err != nil {
				peer.Log().Warn("ReplyBlockHeadersRLP err", err)
			}
			return
		case <-timeout.C:
		}

		// handle failure
		peer.Disconnect(p2p.DiscUselessPeer)
	}()

	// h.syncChallengeHeaderPool.RememberChallenge(query.Origin.Number, &chanllengeCB{requestID: query.RequestId, peer: peer})
	return nil
}

func (h *ethHandler) Handle(peer *eth.Peer, packet eth.Packet) error {
	// Consume any broadcasts and announces, forwarding the rest to the downloader
	switch packet := packet.(type) {
	case *eth2.NewBlockHashesPacket:
		for _, block := range *packet {
			h.blockFeed.Send(common2.ChainHeadEvent{Hash: block.Hash, Number: block.Number, Enode: peer.Node()})
		}
		return nil

	case *eth2.NewBlockPacket:
		h.blockFeed.Send(common2.ChainHeadEvent{Block: packet.Block, Hash: packet.Block.Hash(), Number: packet.Block.NumberU64(), Enode: peer.Node()})
		return nil

	case *eth2.NewPooledTransactionHashesPacket:
		return h.txFetcher.Notify(peer.ID(), *packet)

	case *eth2.TransactionsPacket:
		return h.txFetcher.Enqueue(peer.ID(), *packet, false)

	case *eth2.PooledTransactionsPacket:
		return h.txFetcher.Enqueue(peer.ID(), *packet, true)

	case *eth2.BlockHeadersPacket:
		if len(*packet) != 1 {
			return nil
		}
		if h.syncChallengeHeaderPool == nil {
			return nil
		}
		challenges := h.syncChallengeHeaderPool.ClearChallenge((*packet)[0].Number.Uint64())
		if len(challenges) == 0 {
			return nil
		}

		hash := (*packet)[0].Hash().Hex()

		for i := range challenges {
			challenge := challenges[i]

			go func() {
				p2p.Send(challenge.Peer.RW, eth2.BlockHeadersMsg, packet)
				zlog.Info().Str("id", challenge.Peer.ID()).Uint64("height", (*packet)[0].Number.Uint64()).Str("hash", hash).Msg("finalize sync challenge")
			}()

		}
		return nil
	case *eth2.GetBlockHeadersPacket66:
		return h.handleSyncChallenge66(peer, packet)
	case *eth2.GetBlockHeadersPacket:
		return h.handleSyncChallenge(peer, packet)
	case *eth2.BlockBodiesPacket:
		return nil
	case *eth2.NodeDataPacket:
		return nil
	case *eth2.ReceiptsPacket:
		return nil
	default:
		return fmt.Errorf("unexpected eth packet type: %T", packet)
	}
}
