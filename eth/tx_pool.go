package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/zhiqiangxu/lru"
)

type TxPoolConfig struct {
	Cap    int
	Expire int
}

type TxPool struct {
	cache  lru.Cache
	txFeed event.Feed
	scope  event.SubscriptionScope
	TxPoolConfig
}

var _ txPool = (*TxPool)(nil)

func NewTxPool(config TxPoolConfig) *TxPool {
	return &TxPool{cache: lru.NewCache(config.Cap, 0, nil), TxPoolConfig: config}
}

func (pool *TxPool) Has(hash common.Hash) bool {
	_, ok := pool.cache.Get(hash)
	return ok
}

func (pool *TxPool) Get(hash common.Hash) *types.Transaction {
	tx, ok := pool.cache.RGet(hash)
	if !ok {
		return nil
	}

	return tx.(*types.Transaction)
}

var (
	ErrIsCreateContract = errors.New("is create contract tx")
)

func (pool *TxPool) AddRemotes(txs []*types.Transaction) []error {
	errs := make([]error, len(txs))
	out := make([]*types.Transaction, 0)
	for i := range txs {
		tx := txs[i]

		if tx.To() == nil {
			errs[i] = ErrIsCreateContract
			continue
		}

		new := pool.cache.Add(tx.Hash(), tx, pool.Expire)

		if new {
			out = append(out, tx)
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
