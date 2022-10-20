package common

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
)

// Constants to match up protocol versions and messages
const (
	ETH65 = 65
	ETH66 = 66
	ETH67 = 67
)

// Constants to match up protocol versions and messages
const (
	SNAP1 = 1
)

type ProtocolVersions struct {
	Name     string          `json:"name"`
	Versions []uint          `json:"versions"`
	Lengths  map[uint]uint64 `json:"lengths"`
}

type NodeConfig struct {
	P2P                     p2p.Config
	Handler                 HandlerConfig
	TxPool                  TxPoolConfig
	SyncChallengeHeaderPool *SyncChallengeHeaderPoolConfig
	LogLevel                log.Lvl
	EthProtocolVersions     ProtocolVersions
	SnapProtocolVersions    *ProtocolVersions
}

type HandlerConfig struct {
	NetworkID   uint64
	GenesisHash common.Hash

	Upgrade bool

	SnapSyncer SnapSyncer
}

type SnapSyncer interface {
	Register(peer *snap.Peer) error
	Unregister(peer *snap.Peer) error
}

type TxPoolConfig struct {
	HashCap int
	TxCap   int
}

type SyncChallengeHeaderPoolConfig struct {
	Cap    int
	Expire int
}
