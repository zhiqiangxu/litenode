package litenode

import "github.com/zhiqiangxu/litenode/eth"

type Node struct {
	Eth *eth.Node
}

func New(config Config) *Node {
	node := &Node{}
	if config.Eth != nil {
		node.Eth = eth.NewNode(config.Eth)
	} else {
		panic("config.Eth is nil")
	}
	return node
}

func (n *Node) Start() error {
	if n.Eth != nil {
		return n.Eth.Start()
	}

	return nil
}

func (n *Node) Stop() {
	if n.Eth != nil {
		n.Eth.Stop()
	}
}
