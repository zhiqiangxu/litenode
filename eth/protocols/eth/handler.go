package eth

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	common2 "github.com/zhiqiangxu/litenode/eth/common"
)

func MakeProtocols(backend eth.Backend, protocolVersions common2.ProtocolVersions) []p2p.Protocol {
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

func Handle(backend eth.Backend, peer *eth.Peer) error {
	for {
		if err := handleMessage(backend, peer); err != nil {
			peer.Log().Debug("Message handling failed in `eth`", "err", err)
			return err
		}
	}
}

type msgHandler func(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error

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
	if peer.Version() >= common2.ETH67 {
		handlers = eth67
	} else if peer.Version() >= common2.ETH66 {
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
