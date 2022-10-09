package eth

import (
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/zhiqiangxu/litenode/eth/common"
)

// snapHandler implements the snap.Backend interface to handle the various network
// packets that are sent as replies or broadcasts.
type snapHandler Handler

func (h *snapHandler) Chain() *core.BlockChain { return nil }

// RunPeer is invoked when a peer joins on the `snap` protocol.
func (h *snapHandler) RunPeer(peer *snap.Peer, handler snap.Handler) error {
	h.peerWG.Add(1)
	defer h.peerWG.Done()

	if err := h.peers.RegisterSnapExtension(peer); err != nil {
		peer.Log().Warn("Snapshot extension registration failed", "err", err)
		return err
	}
	return handler(peer)

}

// PeerInfo retrieves all known `snap` information about a peer.
func (h *snapHandler) PeerInfo(id enode.ID) interface{} {
	if p := h.peers.Peer(id.String()); p != nil {
		if p.SnapExt != nil {
			return p.SnapExt.Info()
		}
	}
	return nil
}

// Handle is invoked from a peer's message handler when it receives a new remote
// message that the handler couldn't consume and serve itself.
func (h *snapHandler) Handle(peer *snap.Peer, packet snap.Packet) error {
	h.snapFeed.Send(common.SnapSyncPacket{Peer: peer, Packet: packet})
	return nil
}
