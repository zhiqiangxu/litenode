package litenode

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/zhiqiangxu/litenode/eth"
	common2 "github.com/zhiqiangxu/litenode/eth/common"
	"gotest.tools/v3/assert"
)

func TestHello(t *testing.T) {
	config := Config{
		Eth: &common2.NodeConfig{
			P2P: p2p.Config{
				MaxPeers: 999,
				BootstrapNodes: eth.Nodes{
					"enode://d860a01f9722d78051619d1e2351aba3f43f943f6f00718d1b9baa4101932a1f5011f16bb2b1bb35db20d6fe28fa0bf09636d26a87d31de9ec6203eeedb1f666@18.138.108.67:30303",
				}.Convert(),
				StaticNodes:  eth.Nodes{}.Convert(),
				TrustedNodes: eth.Nodes{}.Convert(),
				// EnableMsgEvents: true,
			},
			Handler: common2.HandlerConfig{
				NetworkID:   1,
				GenesisHash: common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
			},
			TxPool:   common2.TxPoolConfig{HashCap: 10000, TxCap: 1000},
			LogLevel: log.LvlDebug,
			EthProtocolVersions: common2.ProtocolVersions{
				Versions: []uint{common2.ETH67, common2.ETH66},
				Lengths:  map[uint]uint64{common2.ETH67: 17, common2.ETH66: 17},
			},
		},
	}
	node := New(config)

	err := node.Start()
	assert.Assert(t, err == nil)

	for i := 0; i < 15; i++ {
		t.Log("#peers", node.Eth.PeerCount())
		time.Sleep(time.Second)
	}
}
