package common

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type ChainHeadEvent struct {
	Block  *types.Block // may be empty
	Hash   common.Hash
	Number uint64
	Enode  *enode.Node
}

type SnapSyncPacket struct {
	Peer   *snap.Peer
	Packet snap.Packet
}
