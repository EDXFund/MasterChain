// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
/**
  TxPoolShard中除了具有txs外，还有shards，用于管理主链区块里的内容
  shard有三种状态：
  unconfirmed:由shardManager管理，保存在shardManager中
  pending:    在目前的pending队列中
  confirmed:  被存入到数据库中，在TxPoolShard中不再存在

  除了addTx之外，TxPoolShard通过reset函数来进行管理，reset在收到区块头信息，并且区块经过校验和存储后，通过ChainHeadCh来通知
  在TxPoolShard中，收到新的区块头时，有三种情况：
  1.当前是子链，收到了主链的区块头:从主链的区块头中找到最新的子链分片信息，使用这个最新的头来进行reset
  2.当前是子链，收到了子链分片的区块头：使用这个区块头来reset

  3.当前是主链，收到了主链的区块头：更新所有的shard信息，更新所有的txs信息
  4.当前是主链，收到了子链的区块头：交给shardManager处理
*/
package core

import (
	"github.com/EDXFund/MasterChain/core/rawdb"
	"math"
	"math/big"
	"reflect"
	"sync"
	"time"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/state"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/event"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/params"
)



type TxPoolIntf interface {
	Pending() (map[common.Address]types.Transactions, error)
	validateTx(tx *types.Transaction, local bool) error
	AddLocals(txs []*types.Transaction) []error
	SubscribeNewTxsEvent(ch chan<- NewTxsEvent) event.Subscription
	Get(hash common.Hash) *types.Transaction
	SetGasPrice(*big.Int)
	AddLocal(tx *types.Transaction) error
	State() *state.ManagedState
	Stats() (int, int)
	AddRemotes([]*types.Transaction) []error
	Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions)
	Stop()
}
// TxPoolShardConfig are the configuration parameters of the transaction pool.
type TxPoolShardConfig struct {
	Locals    []common.Address // Addresses that should be treated by default as local
	NoLocals  bool             // Whether local transaction handling should be disabled
	Journal   string           // Journal of local transactions to survive node restarts

	AccountSlots uint64 // Number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}
func (tpc *TxPoolConfig)ToShardConfig() *TxPoolShardConfig {
	 return  &TxPoolShardConfig{
	 	tpc.Locals,tpc.NoLocals,tpc.Journal,tpc.AccountSlots,tpc.GlobalSlots,tpc.AccountQueue,tpc.GlobalQueue,tpc.Lifetime}

}

// DefaultTxPoolShardConfig contains the default configurations for the transaction
// pool.
var DefaultTxPoolShardConfig = TxPoolShardConfig{
	Journal:   "transactions.rlp",
	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}


// TxPoolShard contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
type TxPoolShard struct {
	config       TxPoolShardConfig
	chainconfig  *params.ChainConfig
	chain        blockChain

	shardId      uint16
	gasPrice     *big.Int
	txFeed       event.Feed
	scope        event.SubscriptionScope
	chainHeadCh  chan ChainHeadEvent
	chainHeadSub event.Subscription
	signer       types.Signer
	mu           sync.RWMutex

	currentState  *state.StateDB      // Current state in the blockchain head
	pendingState  *state.ManagedState // Pending state tracking virtual nonces
	currentMaxGas uint64              // Current gas limit for transaction caps


	//queue   map[common.Address]*txList   // Queued but non-processable transactions
	all     *txLookup                    // All transactions to allow lookups


	wg sync.WaitGroup // for shutdown sync

	homestead bool
}

// NewTxPoolShard creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewTxPoolShard(config TxPoolShardConfig, chainconfig *params.ChainConfig, chain blockChain,shardId uint16) *TxPoolShard {
	// Sanitize the input to ensure no vulnerable gas prices are set


	// Create the transaction pool with its initial settings
	pool := &TxPoolShard{
		config:      config,
		chainconfig: chainconfig,
		chain:       chain,
		shardId:     shardId,
		signer:      types.NewEIP155Signer(chainconfig.ChainID),

		//queue:       make(map[common.Address]*txList),
		all:         newTxLookup(),
		chainHeadCh: make(chan ChainHeadEvent, chainHeadChanSize),

	}

	if shardId == types.ShardMaster {
		panic(" no master should be here")
	}else {
		pool.resetOfSHeader(nil,chain.CurrentBlock().Header().ToSHeader())
	}


	// Subscribe events from blockchain
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)


	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()

	return pool
}
func (pool *TxPoolShard)AddRemotes(txs []*types.Transaction) []error{
pool.addTxs(txs,false)
return nil
}
func (pool *TxPoolShard)AddLocals(txs []*types.Transaction) []error {
	pool.addTxs(txs,false)
	return nil
}
func (pool *TxPoolShard)AddLocal(tx *types.Transaction) error {
	pool.AddTx(tx,false)
	return nil
}
// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *TxPoolShard) loop() {
	defer pool.wg.Done()

	// Start the stats reporting and transaction eviction tickers
	var prevPending, prevQueued int

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	//reload pending txs
	hashes,err := rawdb.ReadPendingTransactions(pool.chain.DB())
	txs := make([]*types.Transaction,0,len(hashes))
	if err != nil && hashes != nil {
		for _,hash := range hashes {
			tx,err := rawdb.ReadRawTransaction(pool.chain.DB(),*hash)
			if err != nil {
				txs = append(txs,tx)
			}
		}
	}

	pool.addTxs(txs,false)
	// Track the previous head headers for transaction reorgs
	head := pool.chain.CurrentBlock()

	// Keep waiting for and reacting to the various events
	for {
		select {
		// Handle ChainHeadEvent
		case ev := <-pool.chainHeadCh:
		//	fmt.Println("new Header shard")
			if ev.Block != nil {
				pool.mu.Lock()
				if pool.chainconfig.IsHomestead(ev.Block.Number()) {
					pool.homestead = true
				}
				if ev.Block.ShardId() == types.ShardMaster {

					//shard tx pool ignores master block, because shard block would be updated before master block

				} else { //this is shard Id, new shard block arrived
					if head.ShardId() == ev.Block.ShardId() {
						temp := ev.Block.Header().ToSHeader()
						pool.resetOfSHeader(head.Header().ToSHeader(), temp)
					}
					//on master node,  when shard block arrives, it will be send to shardManager and popuped
				}

				head = ev.Block

				pool.mu.Unlock()
			}
			// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return

			// Handle stats reporting ticks
		case <-report.C:
			pool.mu.RLock()
			pending, queued := pool.stats()
			pool.mu.RUnlock()

			if pending != prevPending || queued != prevQueued {
				log.Debug("Transaction pool status report", "executable", pending, "queued", queued)
				prevPending, prevQueued = pending, queued
			}

			// Handle inactive account transaction eviction
		case <-evict.C:
			pool.mu.Lock()
			/*for addr := range pool.queue {
				// Skip local transactions from the eviction mechanism
				if pool.locals.contains(addr) {
					continue
				}
				// Any non-locals old enough should be removed
				if time.Since(pool.beats[addr]) > pool.config.Lifetime {
					for _, tx := range pool.queue[addr].Flatten() {
						pool.removeTx(tx.Hash(), true)
					}
				}
			}*/
			pool.mu.Unlock()

			// Handle local transaction journal rotation
		}
	}
}

// lockedResetOfSHeader is a wrapper around resetOfSHeader to allow calling it in a thread safe
// manner. This method is only ever used in the tester!
func (pool *TxPoolShard) lockedReset(oldHead, newHead types.HeaderIntf) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	if pool.shardId == types.ShardMaster {
		log.Crit(" pool should not reach here")
	}else {
		pool.resetOfSHeader(oldHead, newHead)
	}

}

// resetOfSHeader is used by shard Chain to deal with txs, it should promote those txs to
func (pool *TxPoolShard) resetOfSHeader(oldHead, newHead types.HeaderIntf) {
	// If we're reorging an old state, reinject all dropped transactions
	var reinject types.Transactions
	var discarded, included types.Transactions
	if newHead == nil ||  reflect.ValueOf(newHead).IsNil() {
		newHead = pool.chain.CurrentBlock().Header()
	}
	if oldHead != nil &&  !reflect.ValueOf(oldHead).IsNil() && oldHead.Hash() != newHead.Hash() {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.NumberU64()
		newNum := newHead.NumberU64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all transactions into memory


			var (
				rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.NumberU64())
				add = pool.chain.GetBlock(newHead.Hash(), newHead.NumberU64())
			)
			for rem.NumberU64() > add.NumberU64() {

				for _,item := range rem.Results() {
					discarded = append(discarded, pool.Get(item.TxHash))
				}

				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil  || reflect.ValueOf(rem).IsNil() {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				for _,item := range add.Results() {
					included = append(included, pool.Get(item.TxHash))
				}
				included = append(included, add.Transactions()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil  || reflect.ValueOf(add).IsNil(){
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			//向前寻找共同的祖先
			for rem.Hash() != add.Hash() {

				for _,item := range rem.Results() {
					discarded = append(discarded, pool.Get(item.TxHash))
				}
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil  || reflect.ValueOf(rem).IsNil(){
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
				for _,item := range add.Results() {
					included = append(included, pool.Get(item.TxHash))
				}
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil  || reflect.ValueOf(add).IsNil() {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			reinject = types.TxDifference(discarded, included)
		}
	}
	// Initialize the internal state to the current head
	if newHead == nil || reflect.ValueOf(newHead).IsNil() {
		newHead = pool.chain.CurrentBlock().Header().ToSHeader() // Special case during testing
	}
	if newHead == nil || reflect.ValueOf(newHead).IsNil() {
		log.Error("Failed to reset txpool state as one")
		return
	}
	statedb, err := pool.chain.StateAt(newHead.Root())
	if err != nil {
		log.Error("Failed to reset txpool state", "err", err)
		return
	}
	pool.currentState = statedb
	pool.pendingState = state.ManageState(statedb)
	pool.currentMaxGas = newHead.GasLimit()

	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))

	pool.addTxsLocked(reinject, false)

	for _,tx := range included {

		pool.all.Remove(tx.Hash())

	}


}


// Stop terminates the transaction pool.
func (pool *TxPoolShard) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()


	log.Info("Transaction pool stopped")
}

// SubscribeNewTxsEvent registers a subscription of NewTxsEvent and
// starts sending event to the given channel.
func (pool *TxPoolShard) SubscribeNewTxsEvent(ch chan<- NewTxsEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

// GasPrice returns the current gas price enforced by the transaction pool.
func (pool *TxPoolShard) GasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return new(big.Int).Set(pool.gasPrice)
}

// SetGasPrice updates the minimum price required by the transaction pool for a
// new transaction, and drops all transactions below this threshold.
func (pool *TxPoolShard) SetGasPrice(price *big.Int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()


	log.Info("Transaction pool price threshold updated", "price", price)
}

// State returns the virtual managed state of the transaction pool.
func (pool *TxPoolShard) State() *state.ManagedState {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.pendingState
}

// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPoolShard) Stats() (int, int) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.stats()
}

// stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPoolShard) stats() (int, int) {

	return pool.all.Count(), 0
}

// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (pool *TxPoolShard) Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	pool.all.Range(func(hash common.Hash, tx *types.Transaction) bool {
		msg,err := tx.AsMessage(pool.signer)
		if err == nil {
			txs,ok := pending[msg.From()]
			if(!ok ) {
				txs = make(types.Transactions,0,1)
				pending[msg.From()] = txs
			}
			txs = append(txs,tx)
		}
		return true

	})


	return pending, nil
}

// Pending retrieves all currently processable transactions, grouped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPoolShard) Pending() (map[common.Address]types.Transactions, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()


	result := make(map[common.Address]types.Transactions)

    var errs error
	pool.all.Range(func(hash common.Hash, tx *types.Transaction) bool {
		msg,err := tx.AsMessage(pool.signer)
		if err == nil {
			result[msg.From()] = append(result[msg.From()],tx)
		}else {
			errs = err
		}

		return true
	})
	return result,errs
}



// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
func (pool *TxPoolShard) validateTx(tx *types.Transaction, local bool) error {
	// Heuristic limit, reject transactions over 32KB to prevent DOS attacks
	if tx.Size() > 32*1024 {
		return ErrOversizedData
	}
	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	if tx.Value().Sign() < 0 {
		return ErrNegativeValue
	}
	// Ensure the transaction doesn't exceed the current block limit gas.
	if pool.currentMaxGas < tx.Gas() {
		return ErrGasLimit
	}
	// Make sure the transaction is signed properly
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return ErrInvalidSender
	}

	// Ensure the transaction adheres to nonce ordering
	if pool.currentState.GetNonce(from) > tx.Nonce() {
		return ErrNonceTooLow
	}
	// Transactor should have enough funds to cover the costs
	// cost == V + GP * GL
	if pool.currentState.GetBalance(from).Cmp(tx.Cost()) < 0 {
		return ErrInsufficientFunds
	}
	intrGas, err := IntrinsicGas(tx.Data(), tx.To() == nil, pool.homestead)
	if err != nil {
		return err
	}
	if tx.Gas() < intrGas {
		return ErrIntrinsicGas
	}

	return nil
}


// addTx enqueues a single transaction into the pool if it is valid.
func (pool *TxPoolShard) AddTx(tx *types.Transaction, local bool) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Try to inject the transaction and update any state
	_tx,err := rawdb.ReadRawTransaction(pool.chain.DB(),tx.Hash())
	if _tx == nil || err != nil {
		rawdb.WriteRawTransaction(pool.chain.DB(), tx.Hash(), tx)
		pool.all.Add(tx);

	}else {
		log.Debug("Try to add duplicated tx:","hash:",tx.Hash())
	}
	return nil

}

// addTxs attempts to queue a batch of transactions if they are valid.
func (pool *TxPoolShard) addTxs(txs []*types.Transaction, local bool) []error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	return pool.addTxsLocked(txs, local)
}

// addTxsLocked attempts to queue a batch of transactions if they are valid,
// whilst assuming the transaction pool lock is already held.
func (pool *TxPoolShard) addTxsLocked(txs []*types.Transaction, local bool) []error {
	// Add the batch of transaction, tracking the accepted ones

	for _, tx := range txs {
		 _tx,err := rawdb.ReadRawTransaction(pool.chain.DB(),tx.Hash())
		 if _tx == nil || err != nil {
			 rawdb.WriteRawTransaction(pool.chain.DB(),tx.Hash(),tx)
			 pool.all.Add(tx)
		 }else {
			 log.Debug("Try to add duplicated tx:","hash:",tx.Hash())
		 }

	}
	keys := make([]*common.Hash,0,pool.all.Count())
	pool.all.Range(func(hash common.Hash, tx *types.Transaction) bool {
		keys = append(keys,&hash)
		return true
	})
	rawdb.WritePendingTransactions(pool.chain.DB(),keys)
	return nil
}

// Status returns the status (unknown/pending/queued) of a batch of transactions
// identified by their hashes.
func (pool *TxPoolShard) Status(hashes []common.Hash) []TxStatus {
	pool.mu.RLock()
	defer pool.mu.RUnlock()


	return nil
}

// Get returns a transaction if it is contained in the pool
// and nil otherwise.
func (pool *TxPoolShard) Get(hash common.Hash) *types.Transaction {
	tx := pool.all.Get(hash)
	if tx == nil {
		tx2,err := rawdb.ReadRawTransaction(pool.chain.DB(),hash)
		if err != nil {
			log.Error("Error read tx:","hash",hash)
		}else {
			tx = tx2
		}
	}
	return tx
}

// removeTx removes a single transaction from the queue, moving all subsequent
// transactions back to the future queue.
func (pool *TxPoolShard) removeTx(hash common.Hash, outofbound bool) {
	// Fetch the transaction we wish to delete
	pool.all.Remove(hash)
}

// txLookup is used internally by TxPoolShard to track transactions while allowing lookup without
// mutex contention.
//
// Note, although this type is properly protected against concurrent access, it
// is **not** a type that should ever be mutated or even exposed outside of the
// transaction pool, since its internal state is tightly coupled with the pools
// internal mechanisms. The sole purpose of the type is to permit out-of-bound
// peeking into the pool in TxPoolShard.Get without having to acquire the widely scoped
// TxPoolShard.mu mutex.
type txLookup struct {
	all  map[common.Hash]*types.Transaction
	lock sync.RWMutex
}

// newTxLookup returns a new txLookup structure.
func newTxLookup() *txLookup {
	return &txLookup{
		all: make(map[common.Hash]*types.Transaction),
	}
}

// Range calls f on each key and value present in the map.
func (t *txLookup) Range(f func(hash common.Hash, tx *types.Transaction) bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for key, value := range t.all {
		if !f(key, value) {
			break
		}
	}
}

// Get returns a transaction if it exists in the lookup, or nil if not found.
func (t *txLookup) Get(hash common.Hash) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()
	tx := t.all[hash]

	return  tx
}

// Count returns the current number of items in the lookup.
func (t *txLookup) Count() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return len(t.all)
}

// Add adds a transaction to the lookup.
func (t *txLookup) Add(tx *types.Transaction) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.all[tx.Hash()] = tx

}

// Remove removes a transaction from the lookup.
func (t *txLookup) Remove(hash common.Hash) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.all, hash)
}
