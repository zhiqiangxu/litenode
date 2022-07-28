package eth

import (
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
)

type Node struct {
	handler *Handler
	txPool  *TxPool
	server  *p2p.Server
}

type NodeConfig struct {
	P2P              p2p.Config
	Handler          HandlerConfig
	TxPool           TxPoolConfig
	LogLevel         log.Lvl
	ProtocolVersions ProtocolVersions
}

func NewNode(config *NodeConfig) *Node {
	txPool := NewTxPool(config.TxPool)

	config.Handler.TxPool = txPool
	handler := NewHandler(config.Handler, config.P2P.MaxPeers)
	config.P2P.Protocols = MakeProtocols(handler, config.ProtocolVersions)
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
	n.handler.BroadcastTransactions(txs, true)
}

func (n *Node) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return n.txPool.SubscribeNewTxsEvent(ch)
}

func (n *Node) SubscribeChainHeadEvent(ch chan<- ChainHeadEvent) event.Subscription {
	return n.handler.SubscribeChainHeadsEvent(ch)
}

func (n *Node) PeerCount() int {
	return n.handler.peers.Len()
}

func (n *Node) AllPeers() []*eth.EthPeer {
	return n.handler.peers.AllPeers()
}
