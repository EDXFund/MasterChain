// Copyright 2015 The go-ethereum Authors
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
/***
  如果是子链分片节点，关心txsch和chainHead通道（Header的ShardId和自身的一致）
  如果是主链节点，关心的是shardInfo和chainHead通道（Header的ShardId和自身的一致）
*/
package miner

import (
	"fmt"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/vm"
	"github.com/hashicorp/golang-lru"
	"math"

	//"github.com/golang/dep/gps"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/consensus"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/state"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/event"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/params"
	"github.com/deckarep/golang-set"
)

const (
	//cache size for recently caculated txs, about for one block
	txCacheSize = 2560
	// resultQueueSize is the size of channel listening to sealing result.
	resultQueueSize = 10

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 40960

	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 50

	// chainSideChanSize is the size of channel listening to ChainSideEvent.
	chainSideChanSize = 100

	// resubmitAdjustChanSize is the size of resubmitting interval adjustment channel.
	resubmitAdjustChanSize = 10

	// miningLogAtDepth is the number of confirmations before logging successful mining.
	miningLogAtDepth = 7

	// minRecommitInterval is the minimal time interval to recreate the mining block with
	// any newly arrived transactions.
	minRecommitInterval = 1 * time.Second

	// maxRecommitInterval is the maximum time interval to recreate the mining block with
	// any newly arrived transactions.
	maxRecommitInterval = 15 * time.Second

	// intervalAdjustRatio is the impact a single interval adjustment has on sealing work
	// resubmitting interval.
	intervalAdjustRatio = 0.1

	// intervalAdjustBias is applied during the new resubmit interval calculation in favor of
	// increasing upper limit or decreasing lower limit so that the limit can be reachable.
	intervalAdjustBias = 200 * 1000.0 * 1000.0

	// staleThreshold is the maximum depth of the acceptable stale block.
	staleThreshold = 7
)

// environment is the worker's current environment and holds all of the current state information.
type environment struct {
	signer types.Signer

	state     *state.StateDB // apply state changes here
	ancestors mapset.Set     // ancestor set (used for checking uncle parent validity)
	family    mapset.Set     // family set (used for checking uncle invalidity)
	uncles    mapset.Set     // uncle set
	tcount    int            // tx count in cycle
	gasPool   *core.GasPool  // available gas used to pack transactions

	header       types.HeaderIntf
	txs          []*types.Transaction
	shards       []*types.ShardBlockInfo
	results      []*types.ContractResult
	receipts     []*types.Receipt
	prevSealHash common.Hash
	prevTxsHash  common.Hash
	stopEngineCh chan struct{}
}

// task contains all information for consensus engine sealing and result submitting.
type task struct {
	shards    []*types.ShardBlockInfo
	results   []*types.ContractResult
	receipts  []*types.Receipt
	state     *state.StateDB
	block     types.BlockIntf
	createdAt time.Time
}

const (
	commitInterruptNone int32 = iota
	commitInterruptNewHead
	commitInterruptResubmit
)

// newWorkReq represents a request for new sealing work submitting with relative interrupt notifier.
type newWorkReq struct {
	interrupt *int32
	noempty   bool
	timestamp int64
}

const (
	ST_IDLE      = uint8(0)
	ST_MASTER    = 1
	ST_SHARD     = 2
	ST_RESETING  = 3
	ST_INSERTING = 4
)

// intervalAdjust represents a resubmitting interval adjustment.
type intervalAdjust struct {
	ratio float64
	inc   bool
}

type E_EFuncs func()

// worker is the main object which takes care of submitting new work to consensus engine
// and gathering the sealing result.
type worker struct {
	config *params.ChainConfig
	engine consensus.Engine
	eth    Backend
	chain  *core.BlockChain

	gasFloor uint64
	gasCeil  uint64

	shardId uint16
	// Subscriptions
	mux               *event.TypeMux
	txsCh             chan core.NewTxsEvent
	txsSub            event.Subscription
	chainHeadCh       chan core.ChainHeadEvent
	chainHeadSub      event.Subscription
	chainShardCh      chan *core.ChainsShardEvent
	chainShardSub     event.Subscription
	masterHeadProcCh  chan core.ChainHeadEvent
	masterHeadProcSub event.Subscription

	chainErrorCh  chan core.ChainHeadEvent
	chainErrorSub event.Subscription

	// Channels
	newWorkCh          chan *newWorkReq
	taskCh             chan *task
	resultCh           chan types.BlockIntf
	startCh            chan struct{}
	exitCh             chan struct{}
	resubmitIntervalCh chan time.Duration
	resubmitAdjustCh   chan *intervalAdjust

	current      *environment                    // An environment for current running cycle.
	localUncles  map[common.Hash]types.BlockIntf // A set of side blocks generated locally as the possible uncle blocks.
	remoteUncles map[common.Hash]types.BlockIntf // A set of side blocks as the possible uncle blocks.
	unconfirmed  *unconfirmedBlocks              // A set of locally mined blocks pending canonicalness confirmations.

	mu       sync.RWMutex // The lock used to protect the coinbase and extra fields
	coinbase common.Address
	extra    []byte

	pendingMu    sync.RWMutex
	pendingTasks map[common.Hash]types.BlockIntf

	snapshotMu    sync.RWMutex // The lock used to protect the block snapshot and state snapshot
	snapshotBlock types.BlockIntf
	snapshotState *state.StateDB

	// atomic status counters
	running   int32 // The indicator whether the consensus engine is running or not.
	newTxs    int32 // New arrival transaction count since last sealing work submitting.
	newShards int32
	// External functions, this will be dropped next
	isLocalBlock func(block types.BlockIntf) bool // Function used to determine whether the specified block is mined by local miner.

	// Test hooks
	newTaskHook  func(block types.BlockIntf)        // Method to call upon receiving a new sealing task.
	skipSealHook func(block types.BlockIntf) bool   // Method to decide whether skipping the sealing.
	fullTaskHook func()                             // Method to call before pushing the full sealing task.
	resubmitHook func(time.Duration, time.Duration) // Method to call upon updating resubmitting interval.

	state       uint8
	exitFuncs   []E_EFuncs
	enterFuncs  []E_EFuncs
	timer       *time.Timer
	txsCache    *lru.Cache //cache for recent caculated txs
	recommit    time.Duration
	timedelay   time.Duration
	nextExp     uint16
	nextEnabled [32]byte
}

func newWorker(config *params.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux, recommit time.Duration, gasFloor, gasCeil uint64, isLocalBlock func(types.BlockIntf) bool, shardId uint16) *worker {
	worker := &worker{
		shardId:          shardId,
		config:           config,
		engine:           engine,
		eth:              eth,
		mux:              mux,
		chain:            eth.BlockChain(),
		gasFloor:         gasFloor,
		gasCeil:          gasCeil,
		isLocalBlock:     isLocalBlock,
		localUncles:      make(map[common.Hash]types.BlockIntf),
		remoteUncles:     make(map[common.Hash]types.BlockIntf),
		unconfirmed:      newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
		pendingTasks:     make(map[common.Hash]types.BlockIntf),
		txsCh:            make(chan core.NewTxsEvent, txChanSize),
		chainHeadCh:      make(chan core.ChainHeadEvent, chainHeadChanSize),
		masterHeadProcCh: make(chan core.ChainHeadEvent, chainHeadChanSize),
		chainShardCh:     make(chan *core.ChainsShardEvent, chainSideChanSize),
		chainErrorCh:     make(chan core.ChainHeadEvent, chainSideChanSize),
		newWorkCh:        make(chan *newWorkReq),
		//taskCh:             make(chan *task),
		resultCh:           make(chan types.BlockIntf, resultQueueSize),
		exitCh:             make(chan struct{}),
		startCh:            make(chan struct{}, 1),
		resubmitIntervalCh: make(chan time.Duration),
		resubmitAdjustCh:   make(chan *intervalAdjust, resubmitAdjustChanSize),
		state:              ST_IDLE,
		timer:              time.NewTimer(0),
		recommit:           10000000,
		timedelay:          10000000,
		nextExp:            0,
		nextEnabled:        [32]byte{1},
	}
	// Subscribe NewTxsEvent for tx pool
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainErrorSub = eth.BlockChain().SubscribeChainInsertErrorEvent(worker.chainErrorCh)
	if shardId == types.ShardMaster {
		worker.chainShardSub = eth.ShardPool().SubscribeChainShardsEvent(worker.chainShardCh)
		worker.masterHeadProcSub = eth.ShardPool().SubscribeMasterHeadProcsEvent(worker.masterHeadProcCh)
	} else {
		worker.masterHeadProcSub = eth.TxPool().SubscribeBlockTxsProcsEvent(worker.masterHeadProcCh)
		//worker.masterHeadProcSub = eth.ShardPool().SubscribeMasterHeadProcsEvent(worker.masterHeadProcCh)

	}

	worker.txsCache, _ = lru.New(txCacheSize)
	// Sanitize recommit interval if the user-specified one is too short.
	if recommit < minRecommitInterval {
		log.Warn("Sanitizing miner recommit interval", "provided", recommit, "updated", minRecommitInterval)
		recommit = minRecommitInterval
	}
	worker.exitFuncs = []E_EFuncs{nil, worker.stopEngineSeal, worker.stopEngineSeal, nil, nil}
	worker.enterFuncs = []E_EFuncs{nil, worker.enterMaster, worker.enterShard, worker.enterResume, worker.enterInserting}
	/*go worker.mainLoop()
	go worker.newWorkLoop(recommit)
	go worker.resultLoop()
	go worker.taskLoop()

	*/
	// Submit first work to initialize pending state.
	//worker.startCh <- struct{}{}
	go worker.mainStateLoop()
	return worker
}

// setEtherbase sets the etherbase used to initialize the block coinbase field.
func (w *worker) setEtherbase(addr common.Address) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.coinbase = addr
}

// setExtra sets the content used to initialize the block extra field.
func (w *worker) setExtra(extra []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.extra = extra
}

// setRecommitInterval updates the interval for miner sealing work recommitting.
func (w *worker) setRecommitInterval(interval time.Duration) {
	w.resubmitIntervalCh <- interval
}

// pending returns the pending state and corresponding block.
func (w *worker) pending() (types.BlockIntf, *state.StateDB) {
	// return a snapshot to avoid contention on currentMu mutex
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	if w.snapshotState == nil {
		return nil, nil
	}
	return w.snapshotBlock, w.snapshotState.Copy()
}

// pendingBlock returns pending block.
func (w *worker) pendingBlock() types.BlockIntf {
	// return a snapshot to avoid contention on currentMu mutex
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	return w.snapshotBlock
}

// start sets the running status as 1 and triggers new work submitting.
func (w *worker) start() {
	atomic.StoreInt32(&w.running, 1)
	w.startCh <- struct{}{}
}

// stop sets the running status as 0.
func (w *worker) stop() {
	atomic.StoreInt32(&w.running, 0)
}

// isRunning returns an indicator whether worker is running or not.
func (w *worker) isRunning() bool {
	return atomic.LoadInt32(&w.running) == 1
}

// close terminates all background threads maintained by the worker.
// Note the worker does not support being closed multiple times.
func (w *worker) close() {
	close(w.exitCh)
}

func (w *worker) clearPendingTask(number uint64) {
	// clearPending cleans the stale pending tasks.
	w.pendingMu.Lock()
	for h, t := range w.pendingTasks {
		if t.NumberU64()+staleThreshold <= number {
			delete(w.pendingTasks, h)
		}
	}
	w.pendingMu.Unlock()
}
func (w *worker) shardBuildEnvironment() types.BlockIntf {

	w.mu.RLock()
	defer w.mu.RUnlock()

	w.clearPendingTask(w.chain.CurrentBlock().NumberU64())
	w.stopEngineSeal()

	parent := w.chain.CurrentBlock()
	timestamp := time.Now().Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(timestamp)) >= 0 {
		timestamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); timestamp > now+1 {
		wait := time.Duration(timestamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	var header types.HeaderIntf
	sheader := new(types.SHeader)
	sheader.FillBy(&types.SHeaderStruct{
		ShardId:    w.chain.ShardId(),
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent, w.gasFloor, w.gasCeil),
		Extra:      w.extra,
		Time:       big.NewInt(timestamp),
	})

	header = sheader

	pending, err := w.eth.TxPool().Pending()
	log.Trace("mining shard ", " shardId:", w.shardId, " after:", "number", parent.NumberU64())
	// Only set the coinbase if our consensus engine is running (avoid spurious block rewards)
	if w.isRunning() {
		if w.coinbase == (common.Address{}) {
			log.Error("Refusing to mine without etherbase")
			return nil
		}
		header.SetCoinbase(w.coinbase)
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return nil
	}

	// Could potentially happen if starting to mine in an odd state.
	err = w.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return nil
	}
	// Fill the block with all available pending transactions.

	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return nil
	}
	// Short circuit if there is no available pending transactions
	if len(pending) != 0 {

		// Split the pending transactions into locals and remotes

		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, pending)
		interrupt := int32(0)
		w.newTxs = 0
		if w.shardCommitTransactions(txs, w.coinbase, &interrupt) {

			return nil
		}

	}

	block, err := w.commit(w.fullTaskHook, true, time.Now())

	if err != nil {
		return nil
	} else {
		log.Trace("block commit:", " block number:", block.NumberU64(), " shardId:", block.ShardId(), " txs len:", len(block.Results()))
		return block
	}
}
func (self *worker) SetupShardExps(exp uint16, enabled [32]byte) {
	self.nextExp = exp
	self.nextEnabled = enabled
}
func (w *worker) masterBuildEnvironment() types.BlockIntf {
	w.mu.RLock()
	defer w.mu.RUnlock()

	fmt.Println("master build env")
	w.clearPendingTask(w.chain.CurrentBlock().NumberU64())
	w.stopEngineSeal()

	parent := w.chain.CurrentBlock()

	timestamp := time.Now().Unix()

	if parent.Time().Cmp(new(big.Int).SetInt64(timestamp)) >= 0 {
		timestamp = parent.Time().Int64() + 1
	}

	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); timestamp > now+1 {
		wait := time.Duration(timestamp-now) * time.Second
		log.Debug("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	var header types.HeaderIntf
	header_ := new(types.Header)
	header_.FillBy(&types.HeaderStruct{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent, w.gasFloor, w.gasCeil),
		Extra:      w.extra,
		Time:       big.NewInt(timestamp),
	})
	header = header_

	shardExp := parent.ShardExp()
	if w.nextExp > shardExp {
		shardExp = w.nextExp
	}
	shardEnabled := parent.ShardEnabled()
	for i := 0; i < 32; i++ {
		shardEnabled[i] |= w.nextEnabled[i]
	}

	type SHARD struct {
		shardId uint16
		number  uint64
	}
	shardInfo, err := w.eth.ShardPool().Pending()

	//build shard enabled
	for shardId, _ := range shardInfo {
		seg := shardId >> 3
		offset := shardId % 8
		//if shardBlock
		shardEnabled[seg] |= 0x01 << offset
		if shardId > uint16(math.Pow(2, float64(shardExp))) {
			shardExp = shardExp + 1
		}
	}

	//setup shardExp
	header_.SetShardExp(shardExp)
	header_.SetShardEnabled(shardEnabled)

	// Only set the coinbase if our consensus engine is running (avoid spurious block rewards)
	if w.isRunning() {
		if w.coinbase == (common.Address{}) {
			log.Error("Refusing to mine without etherbase")
			return nil
		}
		header.SetCoinbase(w.coinbase)
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return nil
	}

	// Could potentially happen if starting to mine in an odd state.
	err = w.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return nil
	}

	if err != nil {
		log.Error("Failed to create get shards ", "err", err)
		return nil
	}

	log.Trace("Shards Before:", "count:", len(shardInfo))
	blocks := types.BlockIntfs{}
	shards := make([]*types.ShardBlockInfo, 0, len(shardInfo))

	for _, pendingShard := range shardInfo {
		for _, shard := range pendingShard {
			newShard := shard
			shards = append(shards, &newShard)
			blocks = append(blocks, rawdb.ReadBlock(w.eth.ChainDb(), shard.Hash, shard.BlockNumber))
			if len(blocks) > 512 {
				break
			}
		}
		if len(blocks) > 512 {
			break
		}

	}
	interrupt := int32(0)
	w.newShards = 0
	log.Trace("Shards:", "count:", len(shards))
	w.current.shards = shards

	//statedb transition

	w.masterProcessShards(blocks, w.coinbase, &interrupt)
	//ask for seal
	block, err := w.commit(w.fullTaskHook, true, time.Now())

	if err != nil {
		return nil
	} else {
		return block
	}

}

func (w *worker) enterMaster() {
	w.timer.Reset(1000 * time.Second)
	block := w.masterBuildEnvironment()
	w.timer.Reset(w.recommit)
	w.startEngineSeal(block)

}
func (w *worker) enterShard() {
	w.timer.Reset(1000 * time.Second)
	block := w.shardBuildEnvironment()
	w.timer.Reset(w.recommit)
	if block != nil {
		w.startEngineSeal(block)
	}

}
func (w *worker) enterInserting() {
	w.stopEngineSeal()
	w.timer.Reset(100 * time.Second)
}
func (w *worker) enterResume() {

	w.stopEngineSeal()
	w.timer.Reset(10 * time.Millisecond)
	//w.timedelay  = 10*time.Millisecond
}
func (w *worker) enterState(newState uint8) {
	//fmt.Println("State Transition","shardId:",w.shardId, "cur state:",w.state," new State:",newState)
	if w.exitFuncs[w.state] != nil {
		w.exitFuncs[w.state]()
	}
	w.state = newState
	if w.enterFuncs[w.state] != nil {
		w.enterFuncs[w.state]()
	}
}
func (w *worker) handleTxsLoop() {
	for {
		select {
		case newTxs := <-w.txsCh:
			log.Trace("Event Transition", "evt_newtx:", w.shardId)
			w.handleNewTxs(newTxs.Txs)
		}
	}

}
func (w *worker) mainStateLoop() {
	minRecommit := time.Duration(10 * time.Second)

	timer := time.NewTimer(0)
	<-timer.C // discard the initial tick

	// commit aborts in-flight transaction execution with given signal and resubmits a new one.
	/*	commit := func(noempty bool, s int32) {
		if interrupt != nil {
			atomic.StoreInt32(interrupt, s)
		}
		interrupt = new(int32)
		w.newWorkCh <- &newWorkReq{interrupt: interrupt, noempty: noempty, timestamp: timestamp}
		timer.Reset(recommit)
		atomic.StoreInt32(&w.newTxs, 0)
	}*/
	// recalcRecommit recalculates the resubmitting interval upon feedback.
	recalcRecommit := func(target float64, inc bool) {
		var (
			prev = float64(w.recommit.Nanoseconds())
			next float64
		)
		if inc {
			next = prev*(1-intervalAdjustRatio) + intervalAdjustRatio*(target+intervalAdjustBias)
			// Recap if interval is larger than the maximum time interval
			if next > float64(maxRecommitInterval.Nanoseconds()) {
				next = float64(maxRecommitInterval.Nanoseconds())
			}
		} else {
			next = prev*(1-intervalAdjustRatio) + intervalAdjustRatio*(target-intervalAdjustBias)
			// Recap if interval is less than the user specified minimum
			if next < float64(minRecommit.Nanoseconds()) {
				next = float64(minRecommit.Nanoseconds())
			}
		}
		w.recommit = time.Duration(int64(next))
	}
	defer w.txsSub.Unsubscribe()
	defer w.chainHeadSub.Unsubscribe()
	if w.chainShardSub != nil {
		defer w.chainShardSub.Unsubscribe()
	}

	w.stopEngineSeal()
	<-w.timer.C
	go w.handleTxsLoop()
	for {
		select {
		case <-w.startCh:
			log.Trace("Event Transition", "evt_start:", w.shardId)
			if w.state == ST_IDLE {
				if w.shardId == types.ShardMaster {
					w.enterState(ST_MASTER)
				} else {
					w.enterState(ST_SHARD)
				}
			}

		case <-w.timer.C:
			log.Trace("Event Transition", "evt_timer:", w.shardId)
			w.handleTimer(w.timer)
		case newHead := <-w.chainHeadCh:
			log.Trace("Event Transition", "evt_newChain:", w.shardId, "block shard:", newHead.Block.ShardId(), " number:", newHead.Block.NumberU64(), " hash:", newHead.Block.Hash())
			w.handleNewHead(newHead.Block)
		case newHead := <-w.masterHeadProcCh:
			log.Trace("Event Transition", "evt_headProc:", w.shardId, "block shard:", newHead.Block.ShardId(), " number:", newHead.Block.NumberU64(), " blocks:", len(newHead.Block.ShardBlocks()))
			w.handleMasterHeadProc(newHead.Block)
		case newHead := <-w.chainErrorCh:
			log.Trace("Event Transition", "evt_InsertError:", w.shardId, "block shard:", newHead.Block.ShardId(), " number:", newHead.Block.NumberU64(), " blocks:", len(newHead.Block.ShardBlocks()))
			w.handleInsertErrorProc(newHead.Block)
		case newShards := <-w.chainShardCh:
			log.Trace("Event Transition", "evt_shard:", w.shardId, "block shard:", newShards.Block[0].ShardId(), " number:", newShards.Block[0].NumberU64(), " hash:", newShards.Block[0].Hash())
			w.handleShardChain(newShards.Block)
		case newBlock := <-w.resultCh:
			log.Trace("Event Transition", "evt_newblock:", w.shardId, "number:", newBlock.NumberU64(), "hash", newBlock.Hash(), " stateRoot:", newBlock.Root())
			w.handleNewBlock(newBlock)
		case <-w.exitCh:
			return
		case interval := <-w.resubmitIntervalCh:
			// Adjust resubmit interval explicitly by user.
			if interval < minRecommitInterval {
				log.Warn("Sanitizing miner recommit interval", "provided", interval, "updated", minRecommitInterval)
				interval = minRecommitInterval
			}
			log.Trace("Miner recommit interval update", "from", minRecommit, "to", interval)
			minRecommit, w.recommit = interval, interval

			if w.resubmitHook != nil {
				w.resubmitHook(minRecommit, w.recommit)
			}

		case adjust := <-w.resubmitAdjustCh:
			// Adjust resubmit interval by feedback.
			if adjust.inc {
				before := w.recommit
				recalcRecommit(float64(w.recommit.Nanoseconds())/adjust.ratio, true)
				log.Trace("Increase miner recommit interval", "from", before, "to", w.recommit)
			} else {
				before := w.recommit
				recalcRecommit(float64(minRecommit.Nanoseconds()), false)
				log.Trace("Decrease miner recommit interval", "from", before, "to", w.recommit)
			}

			if w.resubmitHook != nil {
				w.resubmitHook(minRecommit, w.recommit)
			}
		}
	}
}

func (w *worker) handleTimer(timer *time.Timer) {
	switch w.state {
	case ST_IDLE:
	case ST_MASTER:
		if w.newShards > 0 {
			w.enterState(ST_RESETING)
			timer.Reset(0)
		} else {
			timer.Reset(minRecommitInterval)
		}

	case ST_SHARD:
		if w.newTxs > 0 {
			w.enterState(ST_RESETING)
		} else {
			timer.Reset(minRecommitInterval)
		}

	case ST_RESETING:

		if w.shardId == types.ShardMaster {
			w.enterState(ST_MASTER)
		} else {
			w.enterState(ST_SHARD)
		}

	}
}

func (w *worker) handleInsertErrorProc(block types.BlockIntf) {
	switch w.state {

	case ST_IDLE:
		if w.shardId == types.ShardMaster {
			w.masterBuildEnvironment()
		} else {
			//w.shardBuildEnvironment()
		}
	case ST_INSERTING:
		fallthrough
	case ST_RESETING:
		if block.ShardId() == w.shardId {
			if w.shardId == types.ShardMaster {
				w.enterState(ST_MASTER)
			} else {
				w.enterState(ST_SHARD)
			}
		}

	case ST_MASTER:
		if w.current == nil || w.chain.CurrentHeader().Hash() != w.current.header.ParentHash() {

			w.enterState(ST_RESETING)
		}

	case ST_SHARD:
		if w.current == nil || w.chain.CurrentHeader().Hash() != w.current.header.ParentHash() {

			w.enterState(ST_RESETING)
		}

	}
}
func (w *worker) handleMasterHeadProc(block types.BlockIntf) {
	switch w.state {

	case ST_IDLE:
		if w.shardId == types.ShardMaster {
			w.masterBuildEnvironment()
		} else {
			w.shardBuildEnvironment()
		}
	case ST_INSERTING:
		fallthrough
	case ST_RESETING:
		if block.ShardId() == w.shardId {
			if w.shardId == types.ShardMaster {
				w.enterState(ST_MASTER)
			} else {
				w.enterState(ST_SHARD)
			}
		}

	case ST_MASTER:
		if w.current == nil || w.chain.CurrentHeader().Hash() != w.current.header.ParentHash() {

			w.enterState(ST_RESETING)
		}

	case ST_SHARD:
		if w.current == nil || w.chain.CurrentHeader().Hash() != w.current.header.ParentHash() {

			w.enterState(ST_RESETING)
		}

	}
}

func (w *worker) handleNewHead(block types.BlockIntf) {
	switch w.state {

	case ST_IDLE:
		if w.shardId == types.ShardMaster {
			//w.masterBuildEnvironment()
		} else {
			//w.shardBuildEnvironment()
		}
	case ST_INSERTING:
		fallthrough
	case ST_RESETING:
		if block.ShardId() == w.shardId {
			if w.shardId == types.ShardMaster {
				//	w.enterState(ST_MASTER)
			} else {
				//w.enterState(ST_SHARD)
			}
		}

	case ST_MASTER:
		if w.current == nil || w.chain.CurrentHeader().Hash() != w.current.header.ParentHash() {

			//	w.enterState(ST_RESETING)
		}

	case ST_SHARD:
		if w.current == nil || w.chain.CurrentHeader().Hash() != w.current.header.ParentHash() {

			w.enterState(ST_RESETING)
		}

	}
}

func (w *worker) handleShardChain(blocks types.BlockIntfs) {
	w.mux.Post(core.NewMinedBlockEvent{Block: blocks[0]})
	switch w.state {
	case ST_IDLE:
		if w.shardId == types.ShardMaster {
			w.masterBuildEnvironment()
		} else {
			//ignore shard
		}
	case ST_MASTER:
		w.newShards += int32(len(blocks))
	case ST_RESETING:
		w.newShards += int32(len(blocks))
	case ST_SHARD:
		//ignore
	}
}

func (w *worker) handleNewTxs(txs types.Transactions) {
	switch w.state {
	case ST_IDLE:
		if w.shardId != types.ShardMaster {
			w.shardBuildEnvironment()
		} else {
			//ignore on master worker
		}
	case ST_MASTER:
		//ignore
	case ST_SHARD:
		//ignore
		w.newTxs += int32(len(txs))
	}
}
func (w *worker) handleNewBlock(block types.BlockIntf) {

	w.enterState(ST_INSERTING)
	w.chain.InsertChain(types.BlockIntfs{block})
	w.mux.Post(core.NewMinedBlockEvent{Block: block})
	//w.updateSnapshot()
	/*if w.state == ST_INSERTING {
		if w.shardId == types.ShardMaster {
			w.enterState(ST_MASTER)
		}else {
			w.enterState(ST_SHARD)
		}
	} //else new block insert  ok
	*/

	//w.enterState(ST_RESETING)
	//insert chain will trigger newHeadEvent normally
}

// makeCurrent creates a new environment for the current cycle.
func (w *worker) makeCurrent(parent types.BlockIntf, header types.HeaderIntf) error {
	state, err := w.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	env := &environment{
		signer:    types.NewEIP155Signer(w.config.ChainID),
		state:     state,
		ancestors: mapset.NewSet(),
		family:    mapset.NewSet(),
		header:    header,
	}

	// when 08 is processed ancestors contain 07 (quick block)
	for _, ancestor := range w.chain.GetBlocksFromHash(parent.Hash(), 7) {
		env.family.Add(ancestor.Hash())
		env.ancestors.Add(ancestor.Hash())
	}

	// Keep track of transactions which return errors so they can be removed
	env.tcount = 0
	env.receipts = nil

	w.current = env
	//fmt.Printf("current :%v",w.current.header.NumberU64(),w.current.txs,w.current.receipts,w.current.gasPool,w.current.)
	//fmt.Println("")
	return nil
}

/*
// updateSnapshot updates pending snapshot block and state.
// Note this function assumes the current variable is thread safe.
func (w *worker) updateSnapshot() {
	w.snapshotMu.Lock()
	defer w.snapshotMu.Unlock()

	if w.chain.ShardId() == types.ShardMaster {
		w.snapshotBlock = types.NewBlock(
			w.current.header,
			w.current.shards,

			nil,
			w.current.receipts)

	} else {
		w.snapshotBlock = types.NewSBlock(
			w.current.header,
			w.current.results)
	}
	resultHashes := make([]common.Hash,0,1)
	for _,result := range w.snapshotBlock.Results() {
		resultHashes = append(resultHashes,result.TxHash)
	}

	w.snapshotState = w.current.state.Copy()
}
*/
func (w *worker) shardCommitTransaction(tx *types.Transaction, coinbase common.Address, gasPool *core.GasPool, gasUsed *uint64) (*types.ContractResult, error) {

	result, err := core.ApplyToInstruction(w.config, w.current.header, tx, gasPool, gasUsed)
	if err != nil {

		return nil, err
	}
	//	w.current.txs = append(w.current.txs, tx)
	if w.current.results == nil {
		w.current.results = make([]*types.ContractResult, 0, 2)
	}
	w.current.results = append(w.current.results, result)

	return result, nil
}

func (w *worker) shardCommitTransactions(txs *types.TransactionsByPriceAndNonce, coinbase common.Address, interrupt *int32) bool {
	// Short circuit if current is nil
	if w.current == nil {
		return true
	}

	if w.current.gasPool == nil {
		w.current.gasPool = new(core.GasPool).AddGas(w.current.header.GasLimit())
	}
	gasUsed := uint64(0)
	var coalescedResults []*types.ContractResult
	for {
		// In the following three cases, we will interrupt the execution of the transaction.
		// (1) new head block event arrival, the interrupt signal is 1
		// (2) worker start or restart, the interrupt signal is 1
		// (3) worker recreate the mining block with any newly arrived transactions, the interrupt signal is 2.
		// For the first two cases, the semi-finished work will be discarded.
		// For the third case, the semi-finished work will be submitted to the consensus engine.
		if interrupt != nil && atomic.LoadInt32(interrupt) != commitInterruptNone {
			// Notify resubmit loop to increase resubmitting interval due to too frequent commits.
			if atomic.LoadInt32(interrupt) == commitInterruptResubmit {
				ratio := float64(w.current.header.GasLimit()-w.current.gasPool.Gas()) / float64(w.current.header.GasLimit())
				if ratio < 0.1 {
					ratio = 0.1
				}
				w.resubmitAdjustCh <- &intervalAdjust{
					ratio: ratio,
					inc:   true,
				}
			}
			return atomic.LoadInt32(interrupt) == commitInterruptNewHead
		}
		// If we don't have enough gas for any further transactions then we're done
		if w.current.gasPool.Gas() < params.TxGas {
			fmt.Println("Not enough gas for further transactions", "have", w.current.gasPool, "want", params.TxGas)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		//
		if txResult, ok := w.txsCache.Get(tx.Hash()); ok {
			coalescedResults = append(coalescedResults, txResult.(*types.ContractResult))
			txs.Shift()
		} else { //recaculate
			// Error may be ignored here. The error has already been checked
			// during transaction acceptance is the transaction pool.
			//
			// We use the eip155 signer regardless of the current hf.
			from, _ := types.Sender(w.current.signer, tx)
			// Check whether the tx is replay protected. If we're not in the EIP155 hf
			// phase, start ignoring the sender until we do.
			if tx.Protected() && !w.config.IsEIP155(w.current.header.Number()) {
				log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", w.config.EIP155Block)

				txs.Pop()
				continue
			}
			// Start executing the transaction
			w.current.state.Prepare(tx.Hash(), common.Hash{}, w.current.tcount)

			logs, err := w.shardCommitTransaction(tx, coinbase, w.current.gasPool, &gasUsed)
			switch err {
			case core.ErrGasLimitReached:
				// Pop the current out-of-gas transaction without shifting in the next from the account
				log.Trace("Gas limit exceeded for current block", "sender", from)
				txs.Pop()

			case core.ErrNonceTooLow:
				// New head notification data race between the transaction pool and miner, shift
				log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
				txs.Shift()

			case core.ErrNonceTooHigh:
				// Reorg notification data race between the transaction pool and miner, skip account =
				log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
				txs.Pop()

			case nil:
				// Everything ok, collect the logs and shift in the next transaction from the same account
				coalescedResults = append(coalescedResults, logs)
				w.current.tcount++
				w.txsCache.Add(tx.Hash(), logs)
				txs.Shift()

			default:
				// Strange error, discard the transaction and get the next in line (note, the
				// nonce-too-high clause will prevent us from executing in vain).
				log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
				txs.Shift()
			}
		}

	}
	w.current.header.SetGasUsed(gasUsed)
	// Notify resubmit loop to decrease resubmitting interval if current interval is larger
	// than the user-specified one.
	if interrupt != nil {
		w.resubmitAdjustCh <- &intervalAdjust{inc: false}
	}
	w.current.results = coalescedResults
	return false
}

func (w *worker) stopEngineSeal() {
	if w.current != nil && w.current.stopEngineCh != nil {
		close(w.current.stopEngineCh)
		w.current.stopEngineCh = nil
	}
}
func (w *worker) startEngineSeal(block types.BlockIntf) {
	log.Trace("start Seal", " mine:", block.ShardId(), "\twith results:", len(block.Results()), "\t with root", block.Root())
	if w.newTaskHook != nil {
		w.newTaskHook(block)
	}
	// Reject duplicate sealing work due to resubmitting.
	sealHash := w.engine.SealHash(block.Header())
	if sealHash == w.current.prevSealHash {
		return
	}
	// Interrupt previous sealing operation
	//interrupt()

	if w.skipSealHook != nil && w.skipSealHook(block) {
		return
	}
	w.current.stopEngineCh, w.current.prevSealHash = make(chan struct{}), sealHash
	w.pendingMu.Lock()
	w.pendingTasks[w.engine.SealHash(block.Header())] = block
	w.pendingMu.Unlock()

	//curTime := time.Now()

	if err := w.engine.Seal(w.chain, block, w.resultCh, w.current.stopEngineCh); err != nil {
		log.Warn("Block sealing failed", "err", err)
	}

	//fmt.Println(" seal time :", "duration ", time.Now().Sub(curTime))
}
func (w *worker) masterProcessShards(blocks types.BlockIntfs, coinbase common.Address, interrupt *int32) bool {
	coalescedLogs := make([]*types.Log, 0, 10)
	gasUsed := uint64(0)
	//gasUsed = gasUsed;// + w.current.header.GasUsed()
	txs_proc := 0
	/*fname := "./work_proc.txt"
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_RDWR|os.O_APPEND, os.ModeAppend|os.ModePerm)
	if err != nil {
		fmt.Println(err)
	}

	defer f.Close()
	*/
	//log.Trace("Process shard blocks:"," count:",blocks)
	for _, block := range blocks {
		//   if len(blocks) > 0 {
		fmt.Println("proc instr: ", "shardId:", block.ShardId(), "number:", block.NumberU64(), " txs:", len(block.Results()))

		//	f.WriteString(str)
		for _, instruction := range block.Results() {

			if instruction.TxType == core.TT_COMMON {

				tx := w.eth.TxPool().Get(instruction.TxHash)
				w.current.state.Prepare(tx.Hash(), common.Hash{}, w.current.tcount)
				shardBase := block.Coinbase()
				logs, err := w.masterCommitTransaction(tx, coinbase, &shardBase, &gasUsed)
				switch err {
				case core.ErrGasLimitReached:
					// Pop the current out-of-gas transaction without shifting in the next from the account
					log.Trace("Gas limit exceeded for current block", "sender")

				case core.ErrNonceTooLow:
					// New head notification data race between the transaction pool and miner, shift
					log.Trace("Skipping transaction with low nonce", "sender", "nonce", tx.Nonce())

				case core.ErrNonceTooHigh:
					// Reorg notification data race between the transaction pool and miner, skip account =
					log.Trace("Skipping account with hight nonce", "sender", "nonce", tx.Nonce())

				case nil:
					// Everything ok, collect the logs and shift in the next transaction from the same account
					coalescedLogs = append(coalescedLogs, logs...)
					w.current.tcount++

				default:
					// Strange error, discard the transaction and get the next in line (note, the
					// nonce-too-high clause will prevent us from executing in vain).
					log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)

				}
				txs_proc++
			}

		}

	}
	w.current.header.SetGasUsed(gasUsed)
	if !w.isRunning() && len(coalescedLogs) > 0 {
		// We don't push the pendingLogsEvent while we are mining. The reason is that
		// when we are mining, the worker will regenerate a mining block every 3 seconds.
		// In order to avoid pushing the repeated pendingLog, we disable the pending log pushing.

		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}

		go w.mux.Post(core.PendingLogsEvent{Logs: cpy})
	}

	// Notify resubmit loop to decrease resubmitting interval if current interval is larger
	// than the user-specified one.
	if interrupt != nil {
		w.resubmitAdjustCh <- &intervalAdjust{inc: false}
	}

	fmt.Println(" Transactions processed:", txs_proc, " from :", len(blocks))
	return false
}

func (w *worker) masterCommitTransaction(tx *types.Transaction, coinbase common.Address, shardBase *common.Address, gasUsed *uint64) ([]*types.Log, error) {

	snap := w.current.state.Snapshot()
	if w.current.gasPool == nil {
		w.current.gasPool = new(core.GasPool).AddGas(w.current.header.GasLimit())
	}

	receipt, _, err := core.ApplyTransaction(w.config, w.chain, &coinbase, w.current.gasPool, shardBase, w.current.state, w.current.header, tx, gasUsed, vm.Config{})
	if err != nil {
		w.current.state.RevertToSnapshot(snap)
		return nil, err
	}
	w.current.txs = append(w.current.txs, tx)
	w.current.receipts = append(w.current.receipts, receipt)
	/*	str := fmt.Sprintf("%v,%v\r\n",tx.Hash(),*receipt)
		f.WriteString(str)
	*/
	return receipt.Logs, nil
}

// commit runs any post-transaction state modifications, assembles the final block
// and commits new work if consensus engine is running.
func (w *worker) commit(interval func(), update bool, start time.Time) (types.BlockIntf, error) {

	s := w.current.state.Copy()

	block, err := w.engine.Finalize(w.chain, w.current.header, s, w.current.shards, w.current.results, nil, w.current.receipts)
	if err != nil {
		return nil, err
	}

	return block, nil
}
