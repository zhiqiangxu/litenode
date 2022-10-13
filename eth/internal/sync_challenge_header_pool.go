package internal

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/zhiqiangxu/litenode/eth/common"
	"github.com/zhiqiangxu/lru"
)

// this module only stores headers for sync chanllenge
type SyncChallengeHeaderPool struct {
	cache lru.Cache
	common.SyncChallengeHeaderPoolConfig
}

func NewSyncChallengeHeaderool(config common.SyncChallengeHeaderPoolConfig) *SyncChallengeHeaderPool {
	return &SyncChallengeHeaderPool{cache: lru.NewCache(config.Cap, 0, nil), SyncChallengeHeaderPoolConfig: config}
}

func (pool *SyncChallengeHeaderPool) GetHeader(height uint64) *types.Header {
	header, ok := pool.cache.Get(height)
	if !ok {
		return nil
	}

	return header.(*types.Header)
}

func (pool *SyncChallengeHeaderPool) AddHeaderIfNotExists(header *types.Header) {

	pool.cache.CompareAndSet(header.Number.Uint64(), func(value interface{}, exists bool, t lru.Txn) {
		if exists {
			return
		}

		t.Add(header.Number.Uint64(), header, pool.Expire)
	})
	pool.cache.Add(header.Number.Uint64(), header, pool.Expire)
}

// ------------ only used for old protocols ------

func (pool *SyncChallengeHeaderPool) pendingRequestKey(height uint64) string {
	return fmt.Sprintf("p:%d", height)
}

type ChanllengeCB struct {
	Peer *eth.Peer
}

func (pool *SyncChallengeHeaderPool) RememberChallenge(height uint64, req *ChanllengeCB) {
	key := pool.pendingRequestKey(height)
	pool.cache.Txn(func(t lru.Txn) {
		pending, ok := t.Get(key)
		if !ok {
			t.Add(key, []*ChanllengeCB{req}, pool.Expire)
			return
		}

		newPending := append(pending.([]*ChanllengeCB), req)
		t.Add(key, newPending, pool.Expire)
	})
}

func (pool *SyncChallengeHeaderPool) ClearChallenge(height uint64) []*ChanllengeCB {
	key := pool.pendingRequestKey(height)
	pending, ok := pool.cache.RGet(key)
	if !ok {
		return nil
	}

	// the racing is intentionally ignored here.
	pool.cache.Remove(key)

	return pending.([]*ChanllengeCB)
}
