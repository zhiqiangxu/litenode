package eth

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	eth2 "github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/fetcher"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type ProtocolVersions struct {
	Versions []uint          `json:"versions"`
	Lengths  map[uint]uint64 `json:"lengths"`
}

func MakeProtocols(backend eth.Backend, protocolVersions ProtocolVersions) []p2p.Protocol {
	protocols := make([]p2p.Protocol, len(protocolVersions.Versions))

	for i, version := range protocolVersions.Versions {
		version := version // Closure

		protocols[i] = p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  protocolVersions.Lengths[version],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := eth.NewPeer(version, p, rw, backend.TxPool())
				defer peer.Close()

				return backend.RunPeer(peer, func(peer *eth.Peer) error {
					return Handle(backend, peer)
				})
			},
			NodeInfo: func() interface{} {
				return nil
			},
			PeerInfo: func(id enode.ID) interface{} {
				return backend.PeerInfo(id)
			},
		}
	}
	return protocols
}

type txPool interface {
	// Has returns an indicator whether txpool has a transaction
	// cached with the given hash.
	Has(hash common.Hash) bool

	// Get retrieves the transaction from local txpool with given
	// tx hash.
	Get(hash common.Hash) *types.Transaction

	// AddRemotes should add the given transactions to the pool.
	AddRemotes([]*types.Transaction) []error

	// Pending should return pending transactions.
	// The slice should be modifiable by the caller.
	Pending(enforceTips bool) map[common.Address]types.Transactions

	// SubscribeNewTxsEvent should return an event subscription of
	// NewTxsEvent and send events to the given channel.
	SubscribeNewTxsEvent(chan<- core.NewTxsEvent) event.Subscription
}

type HandlerConfig struct {
	NetworkID   uint64
	GenesisHash common.Hash

	TxPool txPool

	Upgrade bool
}

func NewHandler(config HandlerConfig, maxPeers int) *Handler {
	h := &Handler{
		HandlerConfig: config,
		peers:         eth2.NewPeerSet(),
		maxPeers:      maxPeers,
		txpool:        config.TxPool,
	}

	fetchTx := func(peer string, hashes []common.Hash) error {
		p := h.peers.Peer(peer)
		if p == nil {
			return errors.New("unknown peer")
		}
		return p.RequestTxs(hashes)
	}
	h.txFetcher = fetcher.NewTxFetcher(h.txpool.Has, h.txpool.AddRemotes, fetchTx)

	return h
}

type Handler struct {
	HandlerConfig
	peers  *eth2.PeerSet
	peerWG sync.WaitGroup
	wg     sync.WaitGroup

	txsCh     chan core.NewTxsEvent
	txsSub    event.Subscription
	txFetcher *fetcher.TxFetcher
	txpool    txPool

	blockFeed event.Feed
	scope     event.SubscriptionScope

	maxPeers int
}

func (h *Handler) Chain() *core.BlockChain {
	return nil
}

func (h *Handler) TxPool() eth.TxPool {
	return h.txpool
}

func (h *Handler) AcceptTxs() bool {
	return true
}

func (h *Handler) RunPeer(peer *eth.Peer, handler eth.Handler) error {
	h.peerWG.Add(1)
	defer h.peerWG.Done()

	if err := peer.HandshakeLite(h.NetworkID, h.GenesisHash, h.Upgrade); err != nil {
		// zlog.Error().Err(err).
		// 	Str("peer.id", peer.ID()).
		// 	Str("peer.ip", peer.RemoteAddr().String()).
		// 	Str("status", "failed").
		// 	Msg("ethereum:handshake")
		return err
	}

	// Ignore maxPeers if this is a trusted peer
	if !peer.Peer.Info().Network.Trusted {
		if h.peers.Len() >= h.maxPeers {
			return p2p.DiscTooManyPeers
		}
	}
	peer.Log().Debug("Ethereum peer connected", "name", peer.Name())
	// Register the peer locally
	if err := h.peers.RegisterPeer(peer, nil); err != nil {
		peer.Log().Error("Ethereum peer registration failed", "err", err)
		return err
	}
	defer h.unregisterPeer(peer.ID())

	p := h.peers.Peer(peer.ID())
	if p == nil {
		return errors.New("peer dropped during handling")
	}

	return handler(peer)
}

func (h *Handler) PeerInfo(id enode.ID) interface{} {
	if p := h.peers.Peer(id.String()); p != nil {
		return p.Info()
	}
	return nil
}

func (h *Handler) Handle(peer *eth.Peer, packet eth.Packet) error {
	// Consume any broadcasts and announces, forwarding the rest to the downloader
	switch packet := packet.(type) {
	case *NewBlockHashesPacket:
		for _, block := range *packet {
			h.blockFeed.Send(ChainHeadEvent{Hash: block.Hash, Number: block.Number})
		}
		return nil

	case *NewBlockPacket:
		h.blockFeed.Send(ChainHeadEvent{Block: packet.Block, Hash: packet.Block.Hash(), Number: packet.Block.NumberU64()})
		return nil

	case *NewPooledTransactionHashesPacket:
		return h.txFetcher.Notify(peer.ID(), *packet)

	case *TransactionsPacket:
		return h.txFetcher.Enqueue(peer.ID(), *packet, false)

	case *PooledTransactionsPacket:
		return h.txFetcher.Enqueue(peer.ID(), *packet, true)

	case *BlockHeadersPacket:
		return nil
	case *BlockBodiesPacket:
		return nil
	case *NodeDataPacket:
		return nil
	case *ReceiptsPacket:
		return nil
	default:
		return fmt.Errorf("unexpected eth packet type: %T", packet)
	}
}

const (
	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096
)

func (h *Handler) Start() {
	// broadcast transactions
	h.wg.Add(1)
	h.txsCh = make(chan core.NewTxsEvent, txChanSize)
	h.txsSub = h.txpool.SubscribeNewTxsEvent(h.txsCh)
	go h.txBroadcastLoop()

	h.txFetcher.Start()
}

func (h *Handler) Stop() {
	h.txFetcher.Stop()

	h.scope.Close()

	h.txsSub.Unsubscribe() // quits txBroadcastLoop

	h.wg.Wait()

	// Disconnect existing sessions.
	// This also closes the gate for any new registrations on the peer set.
	// sessions which are already established but not added to h.peers yet
	// will exit when they try to register.
	h.peers.Close()
	h.peerWG.Wait()

	log.Info("Ethereum protocol stopped")
}

type ChainHeadEvent struct {
	Block  *types.Block // may be empty
	Hash   common.Hash
	Number uint64
}

func (h *Handler) SubscribeChainHeadsEvent(ch chan<- ChainHeadEvent) event.Subscription {
	return h.scope.Track(h.blockFeed.Subscribe(ch))
}

func (h *Handler) unregisterPeer(id string) {
	// Create a custom logger to avoid printing the entire id
	var logger log.Logger
	if len(id) < 16 {
		// Tests use short IDs, don't choke on them
		logger = log.New("peer", id)
	} else {
		logger = log.New("peer", id[:8])
	}
	// Abort if the peer does not exist
	peer := h.peers.Peer(id)
	if peer == nil {
		logger.Error("Ethereum peer removal failed", "err", errPeerNotRegistered)
		return
	}
	// Remove the `eth` peer if it exists
	logger.Debug("Removing Ethereum peer", "id", id)

	h.txFetcher.Drop(id)

	if err := h.peers.UnregisterPeer(id); err != nil {
		logger.Error("Ethereum peer removal failed", "err", err)
	}
}

// txBroadcastLoop announces new transactions to connected peers.
func (h *Handler) txBroadcastLoop() {
	defer h.wg.Done()
	for {
		select {
		case event := <-h.txsCh:
			h.BroadcastTransactions(event.Txs, false)
		case <-h.txsSub.Err():
			return
		}
	}
}

// BroadcastTransactions will propagate a batch of transactions
// - To a square root of all peers
// - And, separately, as announcements to all peers which are not known to
// already have the given transaction.
func (h *Handler) BroadcastTransactions(txs types.Transactions, local bool) {
	var (
		annoCount   int // Count of announcements made
		annoPeers   int
		directCount int // Count of the txs sent directly to peers
		directPeers int // Count of the peers that were sent transactions directly

		txset = make(map[*eth2.EthPeer][]common.Hash) // Set peer->hash to transfer directly
		annos = make(map[*eth2.EthPeer][]common.Hash) // Set peer->hash to announce

	)
	// Broadcast transactions to a batch of peers not knowing about it
	for _, tx := range txs {
		peers := h.peers.PeersWithoutTransaction(tx.Hash())
		// Send the tx unconditionally to a subset of our peers
		var numDirect int
		if local {
			numDirect = len(peers)
		}
		// numDirect := int(math.Sqrt(float64(len(peers))))

		for _, peer := range peers[:numDirect] {
			txset[peer] = append(txset[peer], tx.Hash())
		}
		// For the remaining peers, send announcement only
		for _, peer := range peers[numDirect:] {
			annos[peer] = append(annos[peer], tx.Hash())
		}
	}
	for peer, hashes := range txset {
		directPeers++
		directCount += len(hashes)
		peer.AsyncSendTransactions(hashes)
	}
	for peer, hashes := range annos {
		annoPeers++
		annoCount += len(hashes)
		peer.AsyncSendPooledTransactionHashes(hashes)
	}
	log.Debug("Transaction broadcast", "txs", len(txs),
		"announce packs", annoPeers, "announced hashes", annoCount,
		"tx packs", directPeers, "broadcast txs", directCount)
}

func Handle(backend eth.Backend, peer *eth.Peer) error {
	for {
		if err := handleMessage(backend, peer); err != nil {
			peer.Log().Debug("Message handling failed in `eth`", "err", err)
			return err
		}
	}
}

type msgHandler func(backend eth.Backend, msg Decoder, peer *eth.Peer) error
type Decoder interface {
	Decode(val interface{}) error
	Time() time.Time
}

var eth65 = map[uint64]msgHandler{
	GetBlockHeadersMsg:            handleGetBlockHeaders66,
	BlockHeadersMsg:               handleBlockHeaders,
	GetBlockBodiesMsg:             handleGetBlockBodies66,
	BlockBodiesMsg:                handleBlockBodies,
	GetNodeDataMsg:                handleGetNodeData66,
	NodeDataMsg:                   handleNodeData,
	GetReceiptsMsg:                handleGetReceipts66,
	ReceiptsMsg:                   handleReceipts,
	NewBlockHashesMsg:             handleNewBlockhashes,
	NewBlockMsg:                   handleNewBlock,
	TransactionsMsg:               handleTransactions,
	NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes,
	GetPooledTransactionsMsg:      handleGetPooledTransactions66,
	PooledTransactionsMsg:         handlePooledTransactions,
}

var eth66 = map[uint64]msgHandler{
	NewBlockHashesMsg:             handleNewBlockhashes,
	NewBlockMsg:                   handleNewBlock,
	TransactionsMsg:               handleTransactions,
	NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes,
	GetBlockHeadersMsg:            handleGetBlockHeaders66,
	BlockHeadersMsg:               handleBlockHeaders66,
	GetBlockBodiesMsg:             handleGetBlockBodies66,
	BlockBodiesMsg:                handleBlockBodies66,
	GetNodeDataMsg:                handleGetNodeData66,
	NodeDataMsg:                   handleNodeData66,
	GetReceiptsMsg:                handleGetReceipts66,
	ReceiptsMsg:                   handleReceipts66,
	GetPooledTransactionsMsg:      handleGetPooledTransactions66,
	PooledTransactionsMsg:         handlePooledTransactions66,
}

var eth67 = map[uint64]msgHandler{
	NewBlockHashesMsg:             handleNewBlockhashes,
	NewBlockMsg:                   handleNewBlock,
	TransactionsMsg:               handleTransactions,
	NewPooledTransactionHashesMsg: handleNewPooledTransactionHashes,
	GetBlockHeadersMsg:            handleGetBlockHeaders66,
	BlockHeadersMsg:               handleBlockHeaders66,
	GetBlockBodiesMsg:             handleGetBlockBodies66,
	BlockBodiesMsg:                handleBlockBodies66,
	GetReceiptsMsg:                handleGetReceipts66,
	ReceiptsMsg:                   handleReceipts66,
	GetPooledTransactionsMsg:      handleGetPooledTransactions66,
	PooledTransactionsMsg:         handlePooledTransactions66,
}

func handleMessage(backend eth.Backend, peer *eth.Peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := peer.RW.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > maxMessageSize {
		return fmt.Errorf("%w: %v > %v", errMsgTooLarge, msg.Size, maxMessageSize)
	}
	defer msg.Discard()

	var handlers = eth65
	if peer.Version() >= ETH67 {
		handlers = eth67
	} else if peer.Version() >= ETH66 {
		handlers = eth66
	}

	// Track the amount of time it takes to serve the request and run the handler
	if metrics.Enabled {
		h := fmt.Sprintf("%s/%s/%d/%#02x", p2p.HandleHistName, ProtocolName, peer.Version(), msg.Code)
		defer func(start time.Time) {
			sampler := func() metrics.Sample {
				return metrics.ResettingSample(
					metrics.NewExpDecaySample(1028, 0.015),
				)
			}
			metrics.GetOrRegisterHistogramLazy(h, nil, sampler).Update(time.Since(start).Microseconds())
		}(time.Now())
	}
	if handler := handlers[msg.Code]; handler != nil {
		return handler(backend, msg, peer)
	}
	return fmt.Errorf("%w: %v", errInvalidMsgCode, msg.Code)
}
