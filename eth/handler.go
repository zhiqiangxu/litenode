package eth

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	eth3 "github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/fetcher"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/panjf2000/ants"
	common2 "github.com/zhiqiangxu/litenode/eth/common"
	"github.com/zhiqiangxu/litenode/eth/internal"
)

func NewHandler(config common2.HandlerConfig, txPool *internal.TxPool, syncChallengeHeaderPool *internal.SyncChallengeHeaderPool, snapEnabled bool, maxPeers int) *Handler {
	h := &Handler{
		HandlerConfig:           config,
		peers:                   eth3.NewPeerSet(),
		snapEnabled:             snapEnabled,
		maxPeers:                maxPeers,
		txPool:                  txPool,
		syncChallengeHeaderPool: syncChallengeHeaderPool,
	}

	fetchTx := func(peer string, hashes []common.Hash) error {
		p := h.peers.Peer(peer)
		if p == nil {
			return errors.New("unknown peer")
		}
		return p.RequestTxs(hashes)
	}

	h.txFetcher = fetcher.NewTxFetcher(h.txPool.Has, h.txPool.AddRemotes, fetchTx)

	return h
}

type Handler struct {
	common2.HandlerConfig
	peers  *eth3.PeerSet
	peerWG sync.WaitGroup
	wg     sync.WaitGroup

	txsCh                   chan core.NewTxsEvent
	pout                    chan types.Transactions
	txsSub                  event.Subscription
	txFetcher               *fetcher.TxFetcher
	txPool                  *internal.TxPool
	syncChallengeHeaderPool *internal.SyncChallengeHeaderPool

	blockFeed event.Feed
	snapFeed  event.Feed
	scope     event.SubscriptionScope

	snapEnabled bool
	maxPeers    int
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
	h.pout = make(chan types.Transactions)
	h.txsSub = h.txPool.SubscribeNewTxsEvent(h.txsCh)
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

func (h *Handler) SubscribeChainHeadsEvent(ch chan<- common2.ChainHeadEvent) event.Subscription {
	return h.scope.Track(h.blockFeed.Subscribe(ch))
}

func (h *Handler) SubscribeSnapSyncMsg(ch chan<- common2.SnapSyncPacket) event.Subscription {
	return h.scope.Track(h.snapFeed.Subscribe(ch))
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

func (h *Handler) BroadcastLocalTransactions(txs types.Transactions) {
	select {
	case h.pout <- txs:
	case <-h.txsSub.Err():
	}
}

// txBroadcastLoop announces new transactions to connected peers.
func (h *Handler) txBroadcastLoop() {
	defer h.wg.Done()

	tk := time.NewTicker(time.Second * 3)
	var peersCache []*eth3.EthPeer

	for {
		select {
		case <-tk.C:
			peersCache = h.peers.AllPeers()
		case txs := <-h.pout:
			h.BroadcastTransactions(peersCache, txs, true)
		case event := <-h.txsCh:
			h.BroadcastTransactions(nil, event.Txs, false)
		case <-h.txsSub.Err():
			return
		}
	}
}

func (h *Handler) PeerCount() int {
	return h.peers.Len()
}

func (h *Handler) AllPeers() []*eth3.EthPeer {
	return h.peers.AllPeers()
}

var antPool *ants.PoolWithFunc

type poolSendIns struct {
	tar []*eth3.EthPeer
	//txs types.Transactions
	data []byte
}

func init() {
	var err error

	antPool, err = ants.NewPoolWithFunc(runtime.NumCPU(), func(i interface{}) {
		ins := i.(*poolSendIns)
		for _, t := range ins.tar {
			_ = t.RW.(p2p.PriorityMsgWriter).PWriteMsg(
				p2p.Msg{
					Code:    eth.TransactionsMsg,
					Size:    uint32(len(ins.data)),
					Payload: bytes.NewReader(ins.data),
				},
			)
		}
	})
	if err != nil {
		panic(fmt.Sprintf("NewPoolWithFunc:%v", err))
	}
}

// BroadcastTransactions will propagate a batch of transactions
// - To a square root of all peers
// - And, separately, as announcements to all peers which are not known to
// already have the given transaction.
func (h *Handler) BroadcastTransactions(allPeers []*eth3.EthPeer, txs types.Transactions, local bool) {
	var (
		annoCount   int // Count of announcements made
		annoPeers   int
		directCount int // Count of the txs sent directly to peers
		directPeers int // Count of the peers that were sent transactions directly

		txset = make(map[*eth3.EthPeer][]common.Hash) // Set peer->hash to transfer directly
		annos = make(map[*eth3.EthPeer][]common.Hash) // Set peer->hash to announce

	)

	if local {

		data, err := rlp.EncodeToBytes(txs)
		if err != nil {
			log.Error("Transaction broadcast", "err", err)
			return
		}

		unit := len(allPeers) / runtime.NumCPU()
		if unit == 0 {
			unit = 1
		}

		for i := 0; i*unit < len(allPeers); i++ {
			to := (i + 1) * unit
			if to > len(allPeers) {
				to = len(allPeers)
			}
			peers := allPeers[i*unit : to]
			_ = antPool.Invoke(&poolSendIns{
				peers,
				data,
			})
		}
		return
	}

	// Broadcast transactions to a batch of peers not knowing about it
	for _, tx := range txs {
		peers := h.peers.PeersWithoutTransaction(tx.Hash())
		// Send the tx unconditionally to a subset of our peers
		var numDirect int
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
