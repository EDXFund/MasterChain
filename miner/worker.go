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
	"github.com/hashicorp/golang-lru"

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
	"github.com/EDXFund/MasterChain/core/vm"
	"github.com/EDXFund/MasterChain/event"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/params"
	"github.com/deckarep/golang-set"
)

const (
	//cache size for recently caculated txs, about for one block
	txCacheSize     = 2560
	// resultQueueSize is the size of channel listening to sealing result.
	resultQueueSize = 10

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096

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
	ST_IDLE     = uint8(0)
	ST_MASTER   = 1
	ST_SHARD    = 2
	ST_RESETING = 3
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
	mux           *event.TypeMux
	txsCh         chan core.NewTxsEvent
	txsSub        event.Subscription
	chainHeadCh   chan core.ChainHeadEvent
	chainHeadSub  event.Subscription
	chainShardCh  chan *core.ChainsShardEvent
	chainShardSub event.Subscription

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
	newTaskHook  func(block types.BlockIntf)                        // Method to call upon receiving a new sealing task.
	skipSealHook func(block types.BlockIntf) bool                   // Method to decide whether skipping the sealing.
	fullTaskHook func()                             // Method to call before pushing the full sealing task.
	resubmitHook func(time.Duration, time.Duration) // Method to call upon updating resubmitting interval.

	state      uint8
	exitFuncs  []E_EFuncs
	enterFuncs []E_EFuncs
	timer      *time.Timer
	txsCache   *lru.Cache							//cache for recent caculated txs
}

func newWorker(config *params.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux, recommit time.Duration, gasFloor, gasCeil uint64, isLocalBlock func(types.BlockIntf) bool, shardId uint16) *worker {
	worker := &worker{
		shardId:            shardId,
		config:             config,
		engine:             engine,
		eth:                eth,
		mux:                mux,
		chain:              eth.BlockChain(),
		gasFloor:           gasFloor,
		gasCeil:            gasCeil,
		isLocalBlock:       isLocalBlock,
		localUncles:        make(map[common.Hash]types.BlockIntf),
		remoteUncles:       make(map[common.Hash]types.BlockIntf),
		unconfirmed:        newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
		pendingTasks:       make(map[common.Hash]types.BlockIntf),
		txsCh:              make(chan core.NewTxsEvent, txChanSize),
		chainHeadCh:        make(chan core.ChainHeadEvent, chainHeadChanSize),
		chainShardCh:       make(chan *core.ChainsShardEvent, chainSideChanSize),
		newWorkCh:          make(chan *newWorkReq),
		//taskCh:             make(chan *task),
		resultCh:           make(chan types.BlockIntf, resultQueueSize),
		exitCh:             make(chan struct{}),
		startCh:            make(chan struct{}, 1),
		resubmitIntervalCh: make(chan time.Duration),
		resubmitAdjustCh:   make(chan *intervalAdjust, resubmitAdjustChanSize),
		state:              ST_IDLE,
		timer:				time.NewTimer(0),

	}
	// Subscribe NewTxsEvent for tx pool
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainShardSub = eth.ShardPool().SubscribeChainShardsEvent(worker.chainShardCh)
	worker.txsCache,_   =        lru.New(txCacheSize)
	// Sanitize recommit interval if the user-specified one is too short.
	if recommit < minRecommitInterval {
		log.Warn("Sanitizing miner recommit interval", "provided", recommit, "updated", minRecommitInterval)
		recommit = minRecommitInterval
	}
	worker.exitFuncs = []E_EFuncs{nil,worker.stopEngineSeal,worker.stopEngineSeal,nil}
	worker.enterFuncs = []E_EFuncs{nil,worker.enterMaster,worker.enterShard,worker.enterResume}
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

func (w *worker)clearPendingTask(number uint64) {
	// clearPending cleans the stale pending tasks.
	w.pendingMu.Lock()
	for h, t := range w.pendingTasks {
		if t.NumberU64()+staleThreshold <= number {
			delete(w.pendingTasks, h)
		}
	}
	w.pendingMu.Unlock()
}
func (w *worker) shardBuildEnvironment() types.BlockIntf{

	w.mu.RLock()
	defer w.mu.RUnlock()
	w.clearPendingTask(w.chain.CurrentBlock().NumberU64())
	w.stopEngineSeal()
	fmt.Print("1")
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
	fmt.Print("2")
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
	fmt.Print("3")
	// Only set the coinbase if our consensus engine is running (avoid spurious block rewards)
	if w.isRunning() {
		if w.coinbase == (common.Address{}) {
			log.Error("Refusing to mine without etherbase")
			return  nil
		}
		header.SetCoinbase(w.coinbase)
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return nil
	}
	fmt.Print("4")
	// Could potentially happen if starting to mine in an odd state.
	err := w.makeCurrent(parent, header)
	if err != nil {
		log.Error( "Failed to create mining context", "err", err)
		return  nil
	}
	// Fill the block with all available pending transactions.
	pending, err := w.eth.TxPool().Pending()
	fmt.Printf("5:%v,",len(pending))
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return  nil
	}
	// Short circuit if there is no available pending transactions
	if len(pending) != 0 {

		// Split the pending transactions into locals and remotes
		fmt.Print("5A")
		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, pending)
		interrupt := int32(0)
		w.newTxs = 0
		if w.shardCommitTransactions(txs, w.coinbase, &interrupt) {
			fmt.Print("5B")
			return nil
		}
	}
	fmt.Print("6")
	block,err := w.commit(w.fullTaskHook,true,time.Now())
	fmt.Print("7")
	if err != nil {
		return nil
	}else {
		return block
	}
}

func (w *worker)masterBuildEnvironment() types.BlockIntf{
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
	header_ := new(types.Header)
	header_.FillBy(&types.HeaderStruct{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent, w.gasFloor, w.gasCeil),
		Extra:      w.extra,
		Time:       big.NewInt(timestamp),
	})
	header = header_

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
	err := w.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return nil
	}
	shardInfo,err := w.eth.ShardPool().Pending()
	if err != nil{
		log.Error("Failed to create get shards ", "err", err)
		return nil
	}
	blocks := types.BlockIntfs{}
	for _,pendingShard := range shardInfo {
		for _, shard := range pendingShard{
			blocks = append(blocks,rawdb.ReadBlock(w.eth.ChainDb(),shard.Hash(),shard.NumberU64()))
		}

	}
	interrupt := int32(0)
	w.newShards = 0
	w.masterProcessShards(blocks,w.coinbase,&interrupt)
	block,err := w.commit(w.fullTaskHook,true,time.Now())
	if err != nil {
		return nil
	}else {
		return block
	}

}

func (w *worker) enterMaster() {
	block := w.masterBuildEnvironment();
	w.timer.Reset(1*time.Second)
	w.startEngineSeal(block)

}
func (w *worker) enterShard() {
	fmt.Println("build env")
	block := w.shardBuildEnvironment()
	w.timer.Reset(1*time.Second)
	if block != nil {
		w.startEngineSeal(block)
	}

}
func (w *worker) enterResume() {
	w.timer.Reset(10*time.Millisecond)
}
func (w *worker) enterState(newState uint8) {
	fmt.Println("cur state:",w.state," new State:",newState)
	if w.exitFuncs[w.state] != nil {
		w.exitFuncs[w.state]()
	}
	w.state = newState;
	if w.enterFuncs[w.state] != nil {
		w.enterFuncs[w.state]()
	}
}
func (w *worker) mainStateLoop() {
	defer w.txsSub.Unsubscribe()
	defer w.chainHeadSub.Unsubscribe()
	defer w.chainShardSub.Unsubscribe()
	w.stopEngineSeal()
	<- w.timer.C
	for {
		select {
		case <-w.startCh:
			fmt.Println("evt_start")
			if w.state == ST_IDLE{
				if w.shardId == types.ShardMaster {
					w.enterState(ST_MASTER)
				}else {
					w.enterState(ST_SHARD)
				}
			}

		case <- w.timer.C:
			fmt.Println("evt_timer")
			w.handleTimer(w.timer)
		case newHead := <- w.chainHeadCh:
			fmt.Println("evt_newChain")
			w.handleNewHead(newHead.Block)
		case newShards := <- w.chainShardCh:
			fmt.Println("evt_shard")
			w.handleShardChain(newShards.Block)
		case newTxs := <- w.txsCh:
			fmt.Println("evt_newtx")
			w.handleNewTxs(newTxs.Txs)
		case newBlock := <- w.resultCh:
			fmt.Println("evt_newblock")
			w.handleNewBlock(newBlock)
		case <- w.exitCh:
			return
		}
	}
}

func (w *worker)handleTimer(timer *time.Timer){
	switch w.state {
	case ST_IDLE:
	case ST_MASTER:
		if w.newShards > 0 {
			w.enterState(ST_RESETING)
			timer.Reset(0)
		}else{
			timer.Reset(minRecommitInterval)
		}

	case ST_SHARD:
		if w.newTxs > 0 {
			w.enterState(ST_RESETING)
		}else{
			timer.Reset(minRecommitInterval)
		}

	case ST_RESETING:

			if w.shardId == types.ShardMaster {
				w.enterState(ST_MASTER)
			}else {
				w.enterState(ST_SHARD)
			}


	}
}

func(w *worker) handleNewHead(block types.BlockIntf){
	switch w.state {
	case ST_IDLE:
		if w.shardId== types.ShardMaster {
			w.masterBuildEnvironment()
		}else {
			w.shardBuildEnvironment()
		}
	case ST_MASTER:
		w.enterState(ST_RESETING)
	case ST_SHARD:
		w.enterState(ST_RESETING)
	}
}

func (w *worker)handleShardChain(blocks types.BlockIntfs){
	switch w.state {
	case ST_IDLE:
		if w.shardId== types.ShardMaster {
			w.masterBuildEnvironment()
		}else {
			//ignore shard
		}
	case ST_MASTER:
		w.newShards += int32(len(blocks))
	case ST_SHARD:
		//ignore
	}
}

func (w *worker)handleNewTxs(txs types.Transactions){
	switch w.state {
	case ST_IDLE:
		if w.shardId != types.ShardMaster {
			w.shardBuildEnvironment()
		}else {
			//ignore on master worker
		}
	case ST_MASTER:
		//ignore
	case ST_SHARD:
		//ignore
		w.newTxs += int32(len(txs))
	}
}
func (w *worker)handleNewBlock(block types.BlockIntf){
	w.chain.InsertChain(types.BlockIntfs{block})
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
	w.current = env
	return nil
}

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
	fmt.Println(" results:", resultHashes)
	w.snapshotState = w.current.state.Copy()
}
func (w *worker) shardCommitTransaction(tx *types.Transaction, coinbase common.Address) (*types.ContractResult, uint64, error) {

	result, gasUsed, err := core.ApplyToInstruction(w.config, w.current.header, tx)
	if err != nil {

		return nil, 0, err
	}
	//	w.current.txs = append(w.current.txs, tx)
	if w.current.results == nil {
		w.current.results = make([]*types.ContractResult, 0, 2)
	}
	w.current.results = append(w.current.results, result)

	return result, gasUsed, nil
}

func (w *worker) shardCommitTransactions(txs *types.TransactionsByPriceAndNonce, coinbase common.Address, interrupt *int32) bool {
	// Short circuit if current is nil
	if w.current == nil {
		return true
	}

	if w.current.gasPool == nil {
		w.current.gasPool = new(core.GasPool).AddGas(w.current.header.GasLimit())
	}

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
			log.Trace("Not enough gas for further transactions", "have", w.current.gasPool, "want", params.TxGas)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		//
		if txResult,ok := w.txsCache.Get(tx.Hash());  ok  {
			coalescedResults = append(coalescedResults,txResult.(*types.ContractResult))
			txs.Shift()
		}else{  //recaculate
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

			logs, _, err := w.shardCommitTransaction(tx, coinbase)
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
				w.txsCache.Add(tx.Hash(),logs)
				txs.Shift()

			default:
				// Strange error, discard the transaction and get the next in line (note, the
				// nonce-too-high clause will prevent us from executing in vain).
				log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
				txs.Shift()
			}
		}

	}

	// Notify resubmit loop to decrease resubmitting interval if current interval is larger
	// than the user-specified one.
	if interrupt != nil {
		w.resubmitAdjustCh <- &intervalAdjust{inc: false}
	}
	w.current.results = coalescedResults
	return false
}


func (w *worker) stopEngineSeal() {
	if w.current !=nil && w.current.stopEngineCh != nil {
		close(w.current.stopEngineCh)
		w.current.stopEngineCh = nil
	}
}
func (w *worker) startEngineSeal(block types.BlockIntf) {
	fmt.Println(" mine....")
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

	if err := w.engine.Seal(w.chain, block, w.resultCh, w.current.stopEngineCh); err != nil {
		log.Warn("Block sealing failed", "err", err)
	}
}
func (w *worker) masterProcessShards(blocks types.BlockIntfs, coinbase common.Address, interrupt *int32) bool {
	coalescedLogs := make([]*types.Log,0,10)
	gasUsed := w.current.header.GasUsed()
	for _, block := range blocks {
		for _, instruction := range block.Results() {
			tx := w.eth.TxPool().Get(instruction.TxHash)
			w.current.state.Prepare(tx.Hash(), common.Hash{}, w.current.tcount)

			logs, err := w.masterCommitTransaction(tx, coinbase, &gasUsed)
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
		}
	}

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
	return false
}

func (w *worker) masterCommitTransaction(tx *types.Transaction, coinbase common.Address, gasUsed *uint64) ([]*types.Log, error) {
	snap := w.current.state.Snapshot()

	receipt, _, err := core.ApplyTransaction(w.config, w.chain, &coinbase, w.current.gasPool, w.current.state, w.current.header, tx, gasUsed, vm.Config{})
	if err != nil {
		w.current.state.RevertToSnapshot(snap)
		return nil, err
	}
	w.current.txs = append(w.current.txs, tx)
	w.current.receipts = append(w.current.receipts, receipt)

	return receipt.Logs, nil
}
// commit runs any post-transaction state modifications, assembles the final block
// and commits new work if consensus engine is running.
func (w *worker) commit(interval func(), update bool, start time.Time) (types.BlockIntf,error) {

	s := w.current.state.Copy()
	fmt.Println("do transaction:", w.current.results)
	block, err := w.engine.Finalize(w.chain, w.current.header, s, nil, w.current.results, nil, nil)
	if err != nil {
		return nil,err
	}

	if update {
		w.updateSnapshot()
	}
	return block,nil
}
