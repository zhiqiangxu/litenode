package eth

import (
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/zhiqiangxu/litenode/eth/common"
	"github.com/zhiqiangxu/litenode/eth/internal"
	eth2 "github.com/zhiqiangxu/litenode/eth/protocols/eth"
	"github.com/zhiqiangxu/litenode/eth/protocols/snap"
)

type Node struct {
	handler *Handler
	txPool  *internal.TxPool
	server  *p2p.Server
}

func NewNode(config *common.NodeConfig) *Node {
	txPool := internal.NewTxPool(config.TxPool)

	var syncChallengeHeaderPool *internal.SyncChallengeHeaderPool
	if config.SyncChallengeHeaderPool != nil {
		syncChallengeHeaderPool = internal.NewSyncChallengeHeaderool(*config.SyncChallengeHeaderPool)
	}
	// ensure syncChallengeHeaderPool is created for old protocol since sync challenge will use it
	if syncChallengeHeaderPool == nil {
		hasOldProtocol := false
		for _, version := range config.EthProtocolVersions.Versions {
			if version < common.ETH65 {
				hasOldProtocol = true
				break
			}
		}
		if hasOldProtocol {
			syncChallengeHeaderPool = internal.NewSyncChallengeHeaderool(common.SyncChallengeHeaderPoolConfig{Cap: 100, Expire: 0})
		}
	}

	handler := NewHandler(config.Handler, txPool, syncChallengeHeaderPool, config.SnapProtocolVersions != nil && len(config.SnapProtocolVersions.Versions) > 0, config.P2P.MaxPeers)
	config.P2P.Protocols = eth2.MakeProtocols((*ethHandler)(handler), config.EthProtocolVersions)
	if config.SnapProtocolVersions != nil {
		config.P2P.Protocols = append(config.P2P.Protocols, snap.MakeProtocols((*snapHandler)(handler), *config.SnapProtocolVersions)...)
	}
	if config.P2P.Logger == nil {
		config.P2P.Logger = log.New()
	}
	config.P2P.Logger.SetHandler(log.LvlFilterHandler(config.LogLevel, log.StdoutHandler))

	if config.P2P.PrivateKey == nil {
		key, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		config.P2P.PrivateKey = key
	}

	server := &p2p.Server{Config: config.P2P}

	node := &Node{handler: handler, txPool: txPool, server: server}

	return node
}

func (n *Node) Start() error {
	n.handler.Start()
	return n.server.Start()
}

func (n *Node) Stop() {
	n.handler.Stop()
	n.server.Stop()
	n.txPool.Stop()
}

func (n *Node) BroadcastTransactions(txs types.Transactions) {
	n.handler.BroadcastLocalTransactions(txs)
}

func (n *Node) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return n.txPool.SubscribeNewTxsEvent(ch)
}

func (n *Node) SubscribeChainHeadEvent(ch chan<- common.ChainHeadEvent) event.Subscription {
	return n.handler.SubscribeChainHeadsEvent(ch)
}

func (n *Node) SubscribeSnapSyncMsg(ch chan<- common.SnapSyncPacket) event.Subscription {
	return n.handler.SubscribeSnapSyncMsg(ch)
}

func (n *Node) PeerCount() int {
	return n.handler.PeerCount()
}

func (n *Node) AllPeers() []*eth.EthPeer {
	return n.handler.AllPeers()
}
