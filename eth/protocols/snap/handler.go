package snap

import (
	"bytes"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	common2 "github.com/zhiqiangxu/litenode/eth/common"
)

func MakeProtocols(backend snap.Backend, protocolVersions common2.ProtocolVersions) []p2p.Protocol {
	protocols := make([]p2p.Protocol, len(protocolVersions.Versions))

	for i, version := range protocolVersions.Versions {
		version := version // Closure

		protocols[i] = p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  protocolVersions.Lengths[version],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {

				return backend.RunPeer(snap.NewPeer(version, p, rw), func(peer *snap.Peer) error {
					return Handle(backend, peer)
				})
			},
			NodeInfo: func() interface{} {
				return nil
			},
			PeerInfo: func(id enode.ID) interface{} {
				return backend.PeerInfo(id)
			},
			Attributes: []enr.Entry{&enrEntry{}},
		}
	}
	return protocols
}

// Handle is the callback invoked to manage the life cycle of a `snap` peer.
// When this function terminates, the peer is disconnected.
func Handle(backend snap.Backend, peer *snap.Peer) error {
	for {
		if err := HandleMessage(backend, peer); err != nil {
			peer.Log().Debug("Message handling failed in `snap`", "err", err)
			return err
		}
	}
}

// HandleMessage is invoked whenever an inbound message is received from a
// remote peer on the `snap` protocol. The remote connection is torn down upon
// returning any error.
func HandleMessage(backend snap.Backend, peer *snap.Peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := peer.RW.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > maxMessageSize {
		return fmt.Errorf("%w: %v > %v", errMsgTooLarge, msg.Size, maxMessageSize)
	}
	defer msg.Discard()
	start := time.Now()
	// Track the emount of time it takes to serve the request and run the handler
	if metrics.Enabled {
		h := fmt.Sprintf("%s/%s/%d/%#02x", p2p.HandleHistName, ProtocolName, peer.Version(), msg.Code)
		defer func(start time.Time) {
			sampler := func() metrics.Sample {
				return metrics.ResettingSample(
					metrics.NewExpDecaySample(1028, 0.015),
				)
			}
			metrics.GetOrRegisterHistogramLazy(h, nil, sampler).Update(time.Since(start).Microseconds())
		}(start)
	}
	// Handle the message depending on its contents
	switch {
	case msg.Code == GetAccountRangeMsg:
		return nil

	case msg.Code == AccountRangeMsg:
		// A range of accounts arrived to one of our previous requests
		res := new(AccountRangePacket)
		if err := msg.Decode(res); err != nil {
			return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
		}
		// Ensure the range is monotonically increasing
		for i := 1; i < len(res.Accounts); i++ {
			if bytes.Compare(res.Accounts[i-1].Hash[:], res.Accounts[i].Hash[:]) >= 0 {
				return fmt.Errorf("accounts not monotonically increasing: #%d [%x] vs #%d [%x]", i-1, res.Accounts[i-1].Hash[:], i, res.Accounts[i].Hash[:])
			}
		}
		requestTracker.Fulfil(peer.ID(), peer.Version(), AccountRangeMsg, res.ID)

		return backend.Handle(peer, res)

	case msg.Code == GetStorageRangesMsg:
		return nil

	case msg.Code == StorageRangesMsg:
		// A range of storage slots arrived to one of our previous requests
		res := new(StorageRangesPacket)
		if err := msg.Decode(res); err != nil {
			return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
		}
		// Ensure the ranges are monotonically increasing
		for i, slots := range res.Slots {
			for j := 1; j < len(slots); j++ {
				if bytes.Compare(slots[j-1].Hash[:], slots[j].Hash[:]) >= 0 {
					return fmt.Errorf("storage slots not monotonically increasing for account #%d: #%d [%x] vs #%d [%x]", i, j-1, slots[j-1].Hash[:], j, slots[j].Hash[:])
				}
			}
		}
		requestTracker.Fulfil(peer.ID(), peer.Version(), StorageRangesMsg, res.ID)

		return backend.Handle(peer, res)

	case msg.Code == GetByteCodesMsg:
		return nil

	case msg.Code == ByteCodesMsg:
		// A batch of byte codes arrived to one of our previous requests
		res := new(ByteCodesPacket)
		if err := msg.Decode(res); err != nil {
			return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
		}
		requestTracker.Fulfil(peer.ID(), peer.Version(), ByteCodesMsg, res.ID)

		return backend.Handle(peer, res)

	case msg.Code == GetTrieNodesMsg:
		return nil

	case msg.Code == TrieNodesMsg:
		// A batch of trie nodes arrived to one of our previous requests
		res := new(TrieNodesPacket)
		if err := msg.Decode(res); err != nil {
			return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
		}
		requestTracker.Fulfil(peer.ID(), peer.Version(), TrieNodesMsg, res.ID)

		return backend.Handle(peer, res)

	default:
		return fmt.Errorf("%w: %v", errInvalidMsgCode, msg.Code)
	}
}
