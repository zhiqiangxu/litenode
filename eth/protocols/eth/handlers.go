package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/trie"
)

func handleNewBlockhashes(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of new block announcements just arrived
	ann := new(NewBlockHashesPacket)
	if err := msg.Decode(ann); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	// Mark the hashes as present at the remote node
	for _, block := range *ann {
		peer.MarkBlock(block.Hash)
	}
	// Deliver them all to the backend for queuing
	return backend.Handle(peer, ann)
}

func handleNewBlock(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// Retrieve and decode the propagated block
	ann := new(NewBlockPacket)
	if err := msg.Decode(ann); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	if err := ann.sanityCheck(); err != nil {
		return err
	}
	if hash := types.CalcUncleHash(ann.Block.Uncles()); hash != ann.Block.UncleHash() {
		log.Warn("Propagated block has invalid uncles", "have", hash, "exp", ann.Block.UncleHash())
		return nil // TODO(karalabe): return error eventually, but wait a few releases
	}
	if hash := types.DeriveSha(ann.Block.Transactions(), trie.NewStackTrie(nil)); hash != ann.Block.TxHash() {
		log.Warn("Propagated block has invalid body", "have", hash, "exp", ann.Block.TxHash())
		return nil // TODO(karalabe): return error eventually, but wait a few releases
	}
	ann.Block.ReceivedAt = msg.Time()
	ann.Block.ReceivedFrom = peer

	// Mark the peer as owning the block
	peer.MarkBlock(ann.Block.Hash())

	return backend.Handle(peer, ann)
}

func handleTransactions(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// Transactions arrived, make sure we have a valid and fresh chain to handle them
	if !backend.AcceptTxs() {
		return nil
	}
	// Transactions can be processed, parse all of them and deliver to the pool
	var txs TransactionsPacket
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	for i, tx := range txs {
		// Validate and mark the remote transaction
		if tx == nil {
			return fmt.Errorf("%w: transaction %d is nil", errDecode, i)
		}
		peer.MarkTransaction(tx.Hash())
	}
	return backend.Handle(peer, &txs)
}

func handleNewPooledTransactionHashes(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// New transaction announcement arrived, make sure we have
	// a valid and fresh chain to handle them
	if !backend.AcceptTxs() {
		return nil
	}
	ann := new(NewPooledTransactionHashesPacket)
	if err := msg.Decode(ann); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	// Schedule all the unknown hashes for retrieval
	for _, hash := range *ann {
		peer.MarkTransaction(hash)
	}
	return backend.Handle(peer, ann)
}

// handleGetBlockHeaders66 is the eth/66 version of handleGetBlockHeaders
func handleGetBlockHeaders66(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// Decode the complex header query
	query := new(GetBlockHeadersPacket66)
	if err := msg.Decode(query); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	return backend.Handle(peer, query)
}

func handleBlockHeaders(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of headers arrived to one of our previous requests
	res := new(BlockHeadersPacket)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	return backend.Handle(peer, res)
}

func handleBlockHeaders66(_ eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of headers arrived to one of our previous requests
	res := new(BlockHeadersPacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	metadata := func() interface{} {
		hashes := make([]common.Hash, len(res.BlockHeadersPacket))
		for i, header := range res.BlockHeadersPacket {
			hashes[i] = header.Hash()
		}
		return hashes
	}
	return peer.DispatchResponse(&eth.Response{
		ID:   res.RequestId,
		Code: BlockHeadersMsg,
		Res:  &res.BlockHeadersPacket,
	}, metadata)
}

func handleGetBlockBodies66(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	return nil
}

func handleBlockBodies(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of block bodies arrived to one of our previous requests
	res := new(BlockBodiesPacket)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	return backend.Handle(peer, res)
}

func handleBlockBodies66(_ eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of block bodies arrived to one of our previous requests
	res := new(BlockBodiesPacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	metadata := func() interface{} {
		var (
			txsHashes   = make([]common.Hash, len(res.BlockBodiesPacket))
			uncleHashes = make([]common.Hash, len(res.BlockBodiesPacket))
		)
		hasher := trie.NewStackTrie(nil)
		for i, body := range res.BlockBodiesPacket {
			txsHashes[i] = types.DeriveSha(types.Transactions(body.Transactions), hasher)
			uncleHashes[i] = types.CalcUncleHash(body.Uncles)
		}
		return [][]common.Hash{txsHashes, uncleHashes}
	}
	return peer.DispatchResponse(&eth.Response{
		ID:   res.RequestId,
		Code: BlockBodiesMsg,
		Res:  &res.BlockBodiesPacket,
	}, metadata)
}

func handleNodeData(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of node state data arrived to one of our previous requests
	res := new(NodeDataPacket)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	return backend.Handle(peer, res)
}

func handleGetNodeData66(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	return nil
}

func handleNodeData66(_ eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of node state data arrived to one of our previous requests
	res := new(NodeDataPacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	return peer.DispatchResponse(&eth.Response{
		ID:   res.RequestId,
		Code: NodeDataMsg,
		Res:  &res.NodeDataPacket,
	}, nil) // No post-processing, we're not using this packet anymore
}

func handleGetReceipts66(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	return nil
}

func handleReceipts(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of receipts arrived to one of our previous requests
	res := new(ReceiptsPacket)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	return backend.Handle(peer, res)
}

func handleReceipts66(_ eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// A batch of receipts arrived to one of our previous requests
	res := new(ReceiptsPacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	metadata := func() interface{} {
		hasher := trie.NewStackTrie(nil)
		hashes := make([]common.Hash, len(res.ReceiptsPacket))
		for i, receipt := range res.ReceiptsPacket {
			hashes[i] = types.DeriveSha(types.Receipts(receipt), hasher)
		}
		return hashes
	}
	return peer.DispatchResponse(&eth.Response{
		ID:   res.RequestId,
		Code: ReceiptsMsg,
		Res:  &res.ReceiptsPacket,
	}, metadata)
}

func handleGetPooledTransactions66(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	return nil
}

func handlePooledTransactions(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// Transactions can be processed, parse all of them and deliver to the pool
	var txs PooledTransactionsPacket
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	for i, tx := range txs {
		// Validate and mark the remote transaction
		if tx == nil {
			return fmt.Errorf("%w: transaction %d is nil", errDecode, i)
		}
		peer.MarkTransaction(tx.Hash())
	}
	return backend.Handle(peer, &txs)
}

func handlePooledTransactions66(backend eth.Backend, msg eth.Decoder, peer *eth.Peer) error {
	// Transactions arrived, make sure we have a valid and fresh chain to handle them
	if !backend.AcceptTxs() {
		return nil
	}
	// Transactions can be processed, parse all of them and deliver to the pool
	var txs PooledTransactionsPacket66
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	for i, tx := range txs.PooledTransactionsPacket {
		// Validate and mark the remote transaction
		if tx == nil {
			return fmt.Errorf("%w: transaction %d is nil", errDecode, i)
		}
		peer.MarkTransaction(tx.Hash())
	}
	requestTracker.Fulfil(peer.ID(), peer.Version(), PooledTransactionsMsg, txs.RequestId)

	return backend.Handle(peer, &txs.PooledTransactionsPacket)
}
