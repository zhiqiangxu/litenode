package eth

import "github.com/ethereum/go-ethereum/p2p/enode"

type Nodes []string

func (n Nodes) Convert() []*enode.Node {
	out := make([]*enode.Node, 0)
	for _, url := range n {
		out = append(out, enode.MustParseV4(url))
	}
	return out
}
