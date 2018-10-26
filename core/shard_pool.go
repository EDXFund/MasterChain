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
// shard pool represents all blocks of shard
package core

import (
	"errors"
	"github.com/EDXFund/MasterChain/ethdb"

	"math"
	"math/big"

	"sync"
	"time"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/state"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/event"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/metrics"
	"github.com/EDXFund/MasterChain/params"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainShardHeadChanSize = 10
	ShardsCount		       =32    //32*8 = 512 shards
)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrShardDisabled = errors.New("shard is disabled")

	// ErrNonceTooLow is returned if the nonce of a transaction is lower than the
	// one present in the local chain.
	ErrShardAlreadyExist = errors.New("shard already exist")

	// ErrUnderpriced is returned if a transaction's gas price is below the minimum
	// configured for the transaction pool.
	ErrShardUnderpriced = errors.New("transaction underpriced")

	// ErrReplaceUnderpriced is returned if a transaction is attempted to be replaced
	// with a different one without the required price bump.
	ErrShardReplaceUnderpriced = errors.New("replacement transaction underpriced")

	// ErrInsufficientFunds is returned if the total cost of executing a transaction
	// is higher than the balance of the user's account.
	ErrShardInsufficientFunds = errors.New("insufficient funds for gas * price + value")

	// ErrIntrinsicGas is returned if the transaction is specified to use less gas
	// than required to start the invocation.
	ErrShardIntrinsicGas = errors.New("intrinsic gas too low")

	// ErrGasLimit is returned if a transaction's requested gas limit exceeds the
	// maximum allowance of the current block.
	ErrShardGasLimit = errors.New("exceeds block gas limit")

	// ErrNegativeValue is a sanity error to ensure noone is able to specify a
	// transaction with a negative value.
	ErrShardNegativeValue = errors.New("negative value")

	// ErrOversizedData is returned if the input data of a transaction is greater
	// than some meaningful limit a user might use. This is not a consensus error
	// making the transaction invalid, rather a DOS protection.
	ErrShardOversizedData = errors.New("oversized data")
)

var (
	evictionShardInterval    = time.Minute     // Time interval to check for evictable Shard
	statsShardReportInterval = 8 * time.Second // Time interval to report Shard pool stats
)

var (
	// Metrics for the pending pool
	pendingShardDiscardCounter   = metrics.NewRegisteredCounter("shardpool/pending/discard", nil)
	pendingShardReplaceCounter   = metrics.NewRegisteredCounter("shardpool/pending/replace", nil)
	pendingShardRateLimitCounter = metrics.NewRegisteredCounter("shardpool/pending/ratelimit", nil) // Dropped due to rate limiting
	pendingShardNofundsCounter   = metrics.NewRegisteredCounter("shardpool/pending/nofunds", nil)   // Dropped due to out-of-funds

	// Metrics for the queued pool
	queuedShardDiscardCounter   = metrics.NewRegisteredCounter("shardpool/queued/discard", nil)
	queuedShardReplaceCounter   = metrics.NewRegisteredCounter("shardpool/queued/replace", nil)
	queuedShardRateLimitCounter = metrics.NewRegisteredCounter("shardpool/queued/ratelimit", nil) // Dropped due to rate limiting
	queuedShardNofundsCounter   = metrics.NewRegisteredCounter("shardpool/queued/nofunds", nil)   // Dropped due to out-of-funds

	// General tx metrics
	invalidShardCounter     = metrics.NewRegisteredCounter("shardpool/invalid", nil)
	underpricedShardCounter = metrics.NewRegisteredCounter("shardpool/underpriced", nil)
)

// TxStatus is the current status of a transaction as seen by the pool.
type ShardStatus uint

const (
	ShardStatusUnknown TxStatus = iota
	ShardStatusQueued
	ShardStatusPending
	ShardStatusIncluded
)

// blockChain provides the state of blockchain and current gas limit to do
// some pre checks in tx pool and event subscribers.
type blockShardChain interface {
	CurrentBlock() *types.Block
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash) (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- ChainHeadEvent) event.Subscription
}

// TxPoolConfig are the configuration parameters of the transaction pool.
type ShardPoolConfig struct {

	Journal   string           // Journal of local transactions to survive node restarts
	Rejournal time.Duration    // Time interval to regenerate the local transaction journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts


	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

// DefaultTxPoolConfig contains the default configurations for the transaction
// pool.
var DefaultShardPoolConfig = ShardPoolConfig{
	Journal:   "shardblocks.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}

// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (config *ShardPoolConfig) sanitize() ShardPoolConfig {
	conf := *config
	if conf.Rejournal < time.Second {
		log.Warn("Sanitizing invalid txpool journal time", "provided", conf.Rejournal, "updated", time.Second)
		conf.Rejournal = time.Second
	}
	if conf.PriceLimit < 1 {
		log.Warn("Sanitizing invalid txpool price limit", "provided", conf.PriceLimit, "updated", DefaultTxPoolConfig.PriceLimit)
		conf.PriceLimit = DefaultTxPoolConfig.PriceLimit
	}
	if conf.PriceBump < 1 {
		log.Warn("Sanitizing invalid txpool price bump", "provided", conf.PriceBump, "updated", DefaultTxPoolConfig.PriceBump)
		conf.PriceBump = DefaultTxPoolConfig.PriceBump
	}
	return conf
}

// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
type ShardPool struct {
	config       ShardPoolConfig
	chainconfig  *params.ChainConfig
	chain        blockChain
	gasPrice     *big.Int
	chaindb      ethdb.Database
	shardblockFeed       event.Feed
	scope        event.SubscriptionScope
	chainHeadCh  chan ChainHeadEvent
	chainHeadSub event.Subscription
	shardChainHeadCh  chan ShardChainHeadEvent
	shardChainHeadSub event.Subscription
	mu           sync.RWMutex

	//controller of shards
	shardExp     uint16
	shardEnabled [ShardsCount]byte

	currentState  *state.StateDB      // Current state in the blockchain head
	currentMaxGas uint64              // Current gas limit for transaction caps

	//journal *txJournal  // Journal of local transaction to back up to disk


	beats   map[uint16]time.Time // Last heartbeat from each known account
	all     *shardLookup                    // All transactions to allow lookups
	//priced  *txPricedList                // All transactions sorted by price

	pendingBlocks map[uint16]*QZTreeManager		   //all pending shard blocks in pool orgnized as tree
	executableBlocks map[common.Hash]types.ShardBlockInfo
	lastShards map[uint16]*types.LastShardInfo   //most recent ShardBlock of each  shard chain
	wg sync.WaitGroup // for shutdown sync

	homestead bool
}

// NewTxPool creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewShardPool(config ShardPoolConfig, chainconfig *params.ChainConfig, chain blockChain) *ShardPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	config = (&config).sanitize()

	// Create the transaction pool with its initial settings
	pool := &ShardPool{
		config:      config,
		chainconfig: chainconfig,
		chain:       chain,

		pendingBlocks:     make(map[uint16]*QZTreeManager),
		executableBlocks:make(map[common.Hash]types.ShardBlockInfo),
		lastShards:       make( map[uint16]*types.LastShardInfo),
		beats:       make(map[uint16]time.Time),
		all:         newShardLookup(),
		chainHeadCh: make(chan ChainHeadEvent, chainShardHeadChanSize),
		shardChainHeadCh: make(chan ShardChainHeadEvent, chainShardHeadChanSize),
		gasPrice:    new(big.Int).SetUint64(config.PriceLimit),
		shardExp:    0,
		//shardEnabled:make([32]byte), //most 256 shards
	}


	pool.reset(nil, chain.CurrentBlock().Header().ToHeader())

/*	// If local transactions and journaling is enabled, load from disk
	if !config.NoLocals && config.Journal != "" {
		pool.journal = newTxJournal(config.Journal)

		if err := pool.journal.load(pool.AddLocals); err != nil {
			log.Warn("Failed to load transaction journal", "err", err)
		}
		if err := pool.journal.rotate(pool.local()); err != nil {
			log.Warn("Failed to rotate transaction journal", "err", err)
		}
	}
*/
	// Subscribe events from blockchain
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()

	return pool
}

// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *ShardPool) loop() {
	defer pool.wg.Done()

	// Start the stats reporting and transaction eviction tickers
	var prevPending, prevQueued int

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	journal := time.NewTicker(pool.config.Rejournal)
	defer journal.Stop()

	// Track the previous head headers for transaction reorgs
	head := pool.chain.CurrentBlock()

	// Keep waiting for and reacting to the various events
	for {
		select {
		case ev := <-pool.shardChainHeadCh:
			pool.addShardBlock(ev.Block)
		// Handle ShardChainHeadEvent
		case ev := <-pool.chainHeadCh: //block of master
			if ev.Block != nil {
				pool.mu.Lock()
				if pool.chainconfig.IsHomestead(ev.Block.Number()) {
					pool.homestead = true
				}
				pool.reset(head.Header().ToHeader(), ev.Block.Header().ToHeader())
				head = ev.Block.ToBlock()

				//rebuild lashshards
				pool.rebuildExecutedBlock()
				pool.SetShardState(ev.Block.ShardExp(),ev.Block.ShardEnabled())
				pool.mu.Unlock()
			}
		// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return

		// Handle stats reporting ticks
		case <-report.C:
			pool.mu.RLock()
			pending, queued := pool.stats()
		//	stales := pool.priced.stales
			pool.mu.RUnlock()

			if pending != prevPending || queued != prevQueued  {
				log.Debug("Transaction pool status report", "executable", pending, "queued", queued)
				prevPending, prevQueued = pending, queued
			}



		// Handle local transaction journal rotation
		////MUST TODO rebuild jounal
		case <-journal.C:
			/*if pool.journal != nil {
				pool.mu.Lock()
				if err := pool.journal.rotate(pool.local()); err != nil {
					log.Warn("Failed to rotate local tx journal", "err", err)
				}
				pool.mu.Unlock()
			}*/
		}
	}
}

func (pool *ShardPool)GetLastestBlockInfo() map[uint16]*types.LastShardInfo{
	return pool.lastShards
}
func (pool *ShardPool)SetLastestBlockInfo(value map[uint16]*types.LastShardInfo) {
	pool.lastShards = value
}

func (pool *ShardPool)GetShardState()  (uint16, [ShardsCount]byte){
	return pool.shardExp,pool.shardEnabled
}

func (pool *ShardPool)SetShardState(shardEx uint16,shardEnabled [ShardsCount]byte) {
	if shardEnabled != pool.shardEnabled || shardEx != pool.shardExp {

	}
}
//rebuild executed block with current block
func (pool *ShardPool)rebuildExecutedBlock(){
	//find all lastest block of current header
	head := pool.chain.CurrentBlock()
	temp := make(map[uint16]*types.LastShardInfo)
	//find most recent blocks of shard in this block there may be multiple shard blocks in one master block
	for _,shardInfo := range head.ShardBlocks() {
		value,ok := temp[shardInfo.ShardId()]
		if ok {
			if value.BlockNumber < shardInfo.BlockNumber() {
				temp[shardInfo.ShardId()] = value
			}
		}
	}
	for shardId,shardInfo := range temp {
		pool.lastShards[shardId] = shardInfo
	}
	//those shardInfo not in temp would not change
}
// lockedReset is a wrapper around reset to allow calling it in a thread safe
// manner. This method is only ever used in the tester!
func (pool *ShardPool) lockedReset(oldHead, newHead types.HeaderIntf) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.reset(oldHead, newHead)
}

// reset retrieves the current state of the blockchain and ensures the content
// of the transaction pool is valid with regard to the chain state.
func (pool *ShardPool) reset(oldHead, newHead types.HeaderIntf) {
	// If we're reorging an old state, reinject all dropped transactions
	var reinject types.ShardBlockInfos

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash() {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.NumberU64()
		newNum := newHead.NumberU64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all transactions into memory
			var discarded, included types.ShardBlockInfos

			var (
				rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.NumberU64())
				add = pool.chain.GetBlock(newHead.Hash(), newHead.NumberU64())
			)
			for rem.NumberU64() > add.NumberU64() {
				discarded = append(discarded, rem.ShardBlocks()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				included = append(included, add.ShardBlocks()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			for rem.Hash() != add.Hash() {
				discarded = append(discarded, rem.ShardBlocks()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
				included = append(included, add.ShardBlocks()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			reinject = types.ShardBlockDifference(discarded, included)
		}
	}
	// Initialize the internal state to the current head
	if newHead == nil {
		newHead = pool.chain.CurrentBlock().Header().ToHeader() // Special case during testing
	}
	statedb, err := pool.chain.StateAt(newHead.Root())
	if err != nil {
		log.Error("Failed to reset txpool state", "err", err)
		return
	}
	pool.currentState = statedb
	//pool.pendingState = state.ManageState(statedb)
	pool.currentMaxGas = newHead.GasLimit()

	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))


	//rebuild latest shard info
	latestShardInfo,err := rawdb.ReadShardLatestEntry(pool.chaindb,newHead.ToHeader().ShardBlockHash())
	if err == nil {
		pool.lastShards = latestShardInfo
	}else {
		pool.lastShards = make(map[uint16]*types.LastShardInfo)
		log.Crit("Last shard info fetch failed")
	}


	//reinject is the shardblocks sould be reinjected
	for _,value := range reinject {
		pool.AddShardBlockInfo(value)
	}

	pool.purgeExpired()
}

// Stop terminates the transaction pool.
func (pool *ShardPool) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

/*	if pool.journal != nil {
		pool.journal.close()
	}
*/
	log.Info("Transaction pool stopped")
}

// SubscribeNewTxsEvent registers a subscription of NewTxsEvent and
// starts sending event to the given channel.
func (pool *ShardPool) SubscribeNewShardBlockEvent(ch chan<- NewShardBlockEvent) event.Subscription {
	return pool.scope.Track(pool.shardblockFeed.Subscribe(ch))
}

// GasPrice returns the current gas price enforced by the transaction pool.
func (pool *ShardPool) GasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return new(big.Int).Set(pool.gasPrice)
}



// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *ShardPool) Stats() (int, int) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.stats()
}

// stats retrieves the current pool stats, namely the number of executable blocks and the
// number of queued (waiting for confirm) blocks.
func (pool *ShardPool) stats() (int, int) {
	pending := 0
	for _, list := range pool.pendingBlocks {
		pending += list.GetPendingCount()
	}
	queued := len(pool.executableBlocks)
	return pending, queued
}


func (pool *ShardPool)AddShardBlock(blk *types.SBlock) (bool,error){
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.addShardBlock(blk)
}

// add validates a transaction and inserts it into the non-executable queue for
// later pending promotion and execution. If the transaction is a replacement for
// an already pending or queued one, it overwrites the previous and returns this
// so outer code doesn't uselessly call promote.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of
// the pool due to pricing constraints.
func (pool *ShardPool) addShardBlock(blk *types.SBlock) (bool, error) {
	// If the transaction is already known, discard it
	//hash := blk.Hash()
	blockInfo :=  new(types.ShardBlockInfo)
	blockInfo.FillBy(&types.ShardBlockInfoStruct{ShardId:blk.ShardId(),BlockNumber:blk.NumberU64(),BlockHash:blk.Hash(),Difficulty:blk.Difficulty().Uint64()})
	return pool.addShardBlockInfo(blockInfo)

}

func (pool *ShardPool)AddShardBlockInfo(blkInfo *types.ShardBlockInfo) (bool,error){
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.addShardBlockInfo(blkInfo)
}

// add validates a transaction and inserts it into the non-executable queue for
// later pending promotion and execution. If the transaction is a replacement for
// an already pending or queued one, it overwrites the previous and returns this
// so outer code doesn't uselessly call promote.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of
// the pool due to pricing constraints.
func (pool *ShardPool) addShardBlockInfo(blkInfo *types.ShardBlockInfo) (bool, error) {
	// If the transaction is already known, discard it
	//hash := blk.Hash()
	if shard := pool.all.Get(blkInfo.Hash()) ; shard == nil {
		newShardBlockInfo :=blkInfo
		shardId := newShardBlockInfo.ShardId()
		// shard block will b accepted only when this shard is enabled
		if pool.shardEnabled [int(shardId/8)] & (1 << (shardId &0x07)) != 0 {
			pool.all.Add(newShardBlockInfo)
			//find a shard manager or create one
			shardManager,ok := pool.pendingBlocks[shardId]
			if !ok {
				lastBlock, exist := pool.lastShards[shardId]
				if  !exist  {  //First for this shard?
					lastBlock = &types.LastShardInfo{ShardId:shardId,BlockNumber:0,Hash:pool.chain.CurrentBlock().Hash(),Td:0}
				}
				shardManager =  &QZTreeManager{shardId:blkInfo.ShardId(),trees:make(map[common.Hash]*QZTree),rootHash:lastBlock.Hash}
			}
			shardManager.AddNewBlock(blkInfo)
			return true,nil
		}else {
			return false, ErrShardDisabled
		}
	}
	return false,ErrShardAlreadyExist

}


// journalTx adds the specified transaction to the local disk journal if it is
// deemed to have been sent from a local account.
/*func (pool *ShardPool) journalTx(from common.Address, tx *types.ShardBlockInfo) {
	// Only journal if it's enabled and the transaction is local
	if pool.journal == nil || !pool.locals.contains(from) {
		return
	}
	if err := pool.journal.insert(tx); err != nil {
		log.Warn("Failed to journal local transaction", "err", err)
	}
}
*/

// Get returns a transaction if it is contained in the pool
// and nil otherwise.
func (pool *ShardPool) Get(hash common.Hash) *types.ShardBlockInfo {
	return pool.all.Get(hash)
}

// removeTx removes a single ShardBlock from the queue, moving all subsequent
// transactions back to the future queue.
func (pool *ShardPool) removeShardBlock(hash common.Hash) {
	result := pool.Get(hash)
	if result != nil {
		pool.pendingBlocks[result.ShardId()].RemoveBlockByHash(hash)
		pool.all.Remove(hash)
	}
}

//purge shard blocks whose blocknumber is older than latest shard info
func (pool *ShardPool)purgeExpired() {
	for shardId,manager := range pool.pendingBlocks {
		if val,ok := pool.lastShards[shardId]; ok {
			shardBlock := new(types.ShardBlockInfo)
			shardBlock.FillBy (&types.ShardBlockInfoStruct{ShardId:val.ShardId,BlockNumber:val.BlockNumber,BlockHash:val.Hash,Difficulty:val.Td})
			manager.ReduceTo(shardBlock)
		}

	}
}

// txLookup is used internally by TxPool to track transactions while allowing lookup without
// mutex contention.
//
// Note, although this type is properly protected against concurrent access, it
// is **not** a type that should ever be mutated or even exposed outside of the
// transaction pool, since its internal state is tightly coupled with the pools
// internal mechanisms. The sole purpose of the type is to permit out-of-bound
// peeking into the pool in TxPool.Get without having to acquire the widely scoped
// TxPool.mu mutex.
type shardLookup struct {
	all  map[common.Hash]*types.ShardBlockInfo
	lock sync.RWMutex
}

// newTxLookup returns a new txLookup structure.
func newShardLookup() *shardLookup {
	return &shardLookup{
		all: make(map[common.Hash]*types.ShardBlockInfo),
	}
}

// Range calls f on each key and value present in the map.
func (t *shardLookup) Range(f func(hash common.Hash, tx *types.ShardBlockInfo) bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for key, value := range t.all {
		if !f(key, value) {
			break
		}
	}
}

// Get returns a transaction if it exists in the lookup, or nil if not found.
func (t *shardLookup) Get(hash common.Hash) *types.ShardBlockInfo {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.all[hash]
}

// Count returns the current number of items in the lookup.
func (t *shardLookup) Count() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return len(t.all)
}

// Add adds a transaction to the lookup.
func (t *shardLookup) Add(tx *types.ShardBlockInfo) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.all[tx.Hash()] = tx
}

// Remove removes a transaction from the lookup.
func (t *shardLookup) Remove(hash common.Hash) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.all, hash)
}
