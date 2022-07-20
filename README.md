# litenode, 一个开箱即用的轻节点模块

目前仅支持以太系，目标是支持异构链，所以`Config`和`Node`都是底层结构的wrapper。


## 使用

```golang
package main

import "github.com/zhiqiangxu/litenode"

config := Config{
    Eth: &eth.NodeConfig{
        P2P: p2p.Config{
            MaxPeers: 999,
            BootstrapNodes: eth.Nodes{
                "enode://d860a01f9722d78051619d1e2351aba3f43f943f6f00718d1b9baa4101932a1f5011f16bb2b1bb35db20d6fe28fa0bf09636d26a87d31de9ec6203eeedb1f666@18.138.108.67:30303",
            }.Convert(),
            StaticNodes:  eth.Nodes{}.Convert(),
            TrustedNodes: eth.Nodes{}.Convert(),
            // EnableMsgEvents: true,
        },
        Handler: eth.HandlerConfig{
            NetworkID:   1,
            GenesisHash: common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
        },
        TxPool:   eth.TxPoolConfig{Cap: 10000, Expire: 10 * 60},
        LogLevel: log.LvlDebug,
        ProtocolVersions: eth.ProtocolVersions{
            Versions: []uint{eth.ETH67, eth.ETH66},
            Lengths:  map[uint]uint64{eth.ETH67: 17, eth.ETH66: 17},
        },
    },
}
node := New(config)

node.Start()

// wait for p2p connecrtions
time.Sleep(time.Second*10)

node.Eth.BroadcastTransactions(txs)

txsCh := make(chan core.NewTxsEvent, txChanSize)
sub := node.Eth.SubscribeNewTxsEvent(txsCh)
defer sub.Unsubscribe()

for {
    tx := <-txsCh
    // process tx
}
```

目前核心提供的能力是交易广播(`BroadcastTransactions`)和交易订阅(`SubscribeNewTxsEvent`)。