package eth

import (
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/zhiqiangxu/util/concurrent"
	"github.com/zhiqiangxu/zcache"
)

type TxPoolConfig struct {
	HashCap int
	TxCap   int
}

type TxPool struct {
	hashCache *concurrent.Bucket[common.Hash, *zcache.RoundRobin[common.Hash, struct{}]]
	txCache   *concurrent.Bucket[common.Hash, *zcache.RoundRobin[common.Hash, *types.Transaction]]
	txFeed    event.Feed
	scope     event.SubscriptionScope
	TxPoolConfig
}

var _ txPool = (*TxPool)(nil)

func NewTxPool(config TxPoolConfig) *TxPool {
	if config.HashCap < config.TxCap {
		panic("HashCap < TxCap")
	}

	return &TxPool{
		hashCache: zcache.NewBucketRoundRobin[common.Hash, struct{}](config.HashCap, 32, func(h common.Hash) uint32 {
			return binary.BigEndian.Uint32(h[:])
		}),
		txCache: zcache.NewBucketRoundRobin[common.Hash, *types.Transaction](config.TxCap, 32, func(h common.Hash) uint32 {
			return binary.BigEndian.Uint32(h[:])
		}),
		TxPoolConfig: config}
}

func (pool *TxPool) Has(hash common.Hash) bool {
	_, ok := pool.hashCache.Element(hash).Get(hash)
	return ok
}

func (pool *TxPool) Get(hash common.Hash) *types.Transaction {
	tx, ok := pool.txCache.Element(hash).Get(hash)
	if !ok {
		return nil
	}

	return tx
}

var (
	ErrIsCreateContract = errors.New("is create contract tx")
)

func (pool *TxPool) AddRemotes(txs []*types.Transaction) []error {
	errs := make([]error, len(txs))
	out := make([]*types.Transaction, 0, len(txs))
	for i := range txs {
		tx := txs[i]

		if tx.To() == nil {
			errs[i] = ErrIsCreateContract
			continue
		}

		hash := tx.Hash()
		isNew := pool.hashCache.Element(hash).Set(hash, struct{}{})

		if isNew {
			out = append(out, tx)
			pool.txCache.Element(hash).Set(hash, tx)
		} else {
			errs[i] = core.ErrAlreadyKnown
		}

	}

	if len(out) > 0 {
		pool.txFeed.Send(core.NewTxsEvent{
			Txs: out,
		})
	}

	return errs
}

func (pool *TxPool) Pending(enforceTips bool) map[common.Address]types.Transactions {
	return nil
}

func (pool *TxPool) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

func (pool *TxPool) Stop() {
	pool.scope.Close()
}
