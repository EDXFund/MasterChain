// Copyright 2018 The go-ethereum Authors
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

package miner

import (
	"fmt"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/qchain"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"io"
	"math"
	"math/big"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/consensus"
	"github.com/EDXFund/MasterChain/consensus/clique"
	"github.com/EDXFund/MasterChain/consensus/ethash"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/core/vm"
	"github.com/EDXFund/MasterChain/crypto"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/event"
	"github.com/EDXFund/MasterChain/params"
)

var (
	// Test chain configurations
	testTxPoolConfig  core.TxPoolConfig
	ethashChainConfig *params.ChainConfig
	cliqueChainConfig *params.ChainConfig

	// Test accounts
	testBankKey, _  = crypto.GenerateKey()
	testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
	testBankFunds   = big.NewInt(1000000000000000000)

	testUserKey, _  = crypto.GenerateKey()
	testUserAddress = crypto.PubkeyToAddress(testUserKey.PublicKey)

	// Test transactions
	pendingTxs []*types.Transaction
	newTxs     []*types.Transaction
)

func init() {
	testTxPoolConfig = core.DefaultTxPoolConfig
	testTxPoolConfig.Journal = ""
	ethashChainConfig = params.TestChainConfig
	cliqueChainConfig = params.TestChainConfig
	cliqueChainConfig.Clique = &params.CliqueConfig{
		Period: 100,
		Epoch:  30000,
	}
	tx1, _ := types.SignTx(types.NewTransaction(0, testUserAddress, big.NewInt(1000), params.TxGas, nil, nil,0), types.HomesteadSigner{}, testBankKey)
	pendingTxs = append(pendingTxs, tx1)
	tx2, _ := types.SignTx(types.NewTransaction(1, testUserAddress, big.NewInt(1000), params.TxGas, nil, nil,0), types.HomesteadSigner{}, testBankKey)
	newTxs = append(newTxs, tx2)
}

// testWorkerBackend implements worker.Backend interfaces and wraps all information needed during the testing.
type testWorkerBackend struct {
	db         ethdb.Database
	txPool     core.TxPoolIntf
	shardPool  *qchain.ShardChainPool
	chain      *core.BlockChain
	testTxFeed event.Feed
	uncleBlock types.BlockIntf
}
func newTestWorkerBackend(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, n int, shardId uint16) *testWorkerBackend {
	db    := ethdb.NewMemDatabase()
	return newTestWorkerBackendWithAccount(t,db,chainConfig,engine,n,shardId,core.GenesisAlloc{testBankAddress: {Balance: testBankFunds}})
}
func newTestWorkerBackendWithDb(t *testing.T, db ethdb.Database,chainConfig *params.ChainConfig, engine consensus.Engine, n int, shardId uint16) *testWorkerBackend {

	return newTestWorkerBackendWithAccount(t,db,chainConfig,engine,n,shardId,core.GenesisAlloc{testBankAddress: {Balance: testBankFunds}})
}

func newTestWorkerBackendWithAccount(t *testing.T, db ethdb.Database, chainConfig *params.ChainConfig, engine consensus.Engine, n int, shardId uint16, accountsInfos core.GenesisAlloc) *testWorkerBackend {
	var (

		gspec = core.Genesis{
			Config: chainConfig,
			Alloc:  accountsInfos,
		}
	)

	switch engine.(type) {
	case *clique.Clique:
		gspec.ExtraData = make([]byte, 32+common.AddressLength+65)
		copy(gspec.ExtraData[32:], testBankAddress[:])
	case *ethash.Ethash:
	default:
		t.Fatalf("unexpected consensus engine type: %T", engine)
	}
	genesis := gspec.MustCommit(db, shardId)

	chain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{}, nil,shardId)
	var txpool core.TxPoolIntf
	var shardpool *qchain.ShardChainPool
	if shardId == types.ShardMaster {
		txpool = core.NewTxPoolMaster(testTxPoolConfig, chainConfig, chain, shardId)
		shardpool = qchain.NewShardChainPool(chain,db)
	}else {
		txpool = core.NewTxPoolShard(*testTxPoolConfig.ToShardConfig(), chainConfig, chain, shardId)
	}

	chain.SetupProcessor(gspec.Config,engine,txpool)
	// Generate a small n-block chain and an uncle block for it
	if n > 0 {
		blocks, _ := core.GenerateChain(chainConfig, genesis, engine, db, n, func(i int, gen *core.BlockGen) {
			gen.SetCoinbase(testBankAddress)
		})
		if _, err := chain.InsertChain(blocks); err != nil {
			t.Fatalf("failed to insert origin chain: %v", err)
		}
	}
	parent := genesis
	if n > 0 {
		parent = chain.GetBlockByHash(chain.CurrentBlock().ParentHash())
	}
	blocks, _ := core.GenerateChain(chainConfig, parent, engine, db, 1, func(i int, gen *core.BlockGen) {
		gen.SetCoinbase(testUserAddress)
	})

	return &testWorkerBackend{
		db:         db,
		chain:      chain,
		txPool:     txpool,
		shardPool:  shardpool,
		uncleBlock: blocks[0],
	}
}
func createChain(chainConfig *params.ChainConfig, number int,shardId uint16) []types.BlockIntf {
	var (
		db    = ethdb.NewMemDatabase()
		gspec = core.Genesis{
			Config: chainConfig,
			Alloc:  core.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
		}
	)

	genesis := gspec.MustCommit(db, shardId)
	//chain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{}, nil, shardId)
	// overwrite the old chain
	chainblock, _ := core.GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, number, nil)
	return chainblock
}

func (b *testWorkerBackend) BlockChain() *core.BlockChain { return b.chain }
func (b *testWorkerBackend) TxPool() core.TxPoolIntf         { return b.txPool }
func (b *testWorkerBackend) ShardPool() *qchain.ShardChainPool         { return b.shardPool }
func (b *testWorkerBackend) ChainDb()  ethdb.Database         { return b.db }
func (b *testWorkerBackend) PostChainEvents(events []interface{}) {
	b.chain.PostChainEvents(events, nil)
}
func newTestWorker(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, blocks int, shardId uint16) (*worker, *testWorkerBackend) {
	backend := newTestWorkerBackend(t, chainConfig, engine, blocks, shardId)
	backend.txPool.AddLocals(pendingTxs)
	w := newWorker(chainConfig, engine, backend, new(event.TypeMux), time.Second, params.GenesisGasLimit, params.GenesisGasLimit, nil,shardId)
	w.setEtherbase(testBankAddress)
	w.start()
	return w, backend
}
type TestWorker struct {
	worker *worker
	backend *testWorkerBackend
	channel chan core.ChainHeadEvent
}
func newMasterShardTestWorker(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, blocks int, shardExp uint16,accounts core.GenesisAlloc) (*TestWorker,[]*TestWorker) {
	 master := TestWorker{}
	//create master chain
	db    := ethdb.NewMemDatabase()
	master.backend = newTestWorkerBackendWithAccount(t, db,chainConfig, engine, blocks, types.ShardMaster,accounts)

	master.worker = newWorker(chainConfig, engine, master.backend, new(event.TypeMux), time.Second, params.GenesisGasLimit, params.GenesisGasLimit, nil,types.ShardMaster)
	master.worker.setEtherbase(testBankAddress)
	master.worker.start()

	shards := int(math.Exp2(float64(shardExp)))

	shardBackends := make([]*TestWorker,shards)
	//create shard chain
	for i := 0; i < shards; i++ {
		shardBackends[i] = &TestWorker{}
		shardBackends[i].backend = newTestWorkerBackendWithDb(t,db,chainConfig,engine,0,uint16(i))
		shardBackends[i].worker = newWorker(chainConfig, engine, shardBackends[i].backend, new(event.TypeMux), time.Second, params.GenesisGasLimit, params.GenesisGasLimit, nil,uint16(i))
		shardBackends[i].channel = make(chan core.ChainHeadEvent)
		shardBackends[i].backend.chain.SubscribeChainHeadEvent(shardBackends[i].channel)


		shardBackends[i] .worker.setEtherbase(testBankAddress)
		shardBackends[i] .worker.start()
		go func(chain *core.BlockChain){
			ch := make(chan core.ChainHeadEvent)
			master.backend.chain.SubscribeChainHeadEvent(ch)
			for {
				select {
				case value := <- ch:

					chain.InsertChain(types.BlockIntfs{value.Block})
				}
			}
		}(shardBackends[i].backend.chain)
	}
	go func(){
		cases := make([]reflect.SelectCase, shards)
		for i, ch := range shardBackends {
			cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch.channel)}
		}
		for {
			_, value, ok := reflect.Select(cases)
			// ok will be true if the channel has not been closed.
			if ok {
				block := value.Interface().(core.ChainHeadEvent).Block
				master.backend.chain.InsertChain(types.BlockIntfs{block})
			}
		}


	}()
	return &master,shardBackends
}
func TestSingleTransaction(t *testing.T) {
	cfg := ethash.Config{
		CacheDir:       "ethash",
		CachesInMem:    2,
		CachesOnDisk:   3,
		DatasetsInMem:  1,
		DatasetsOnDisk: 2,
	}
	testSingleTransaction(t, ethashChainConfig, ethash.New(cfg,nil,false))
}
func testSingleTransaction(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine) {

	defer engine.Close()

	state := make(chan  struct{})

	testKey, _  := crypto.GenerateKey()
	testAddress := crypto.PubkeyToAddress(testKey.PublicKey)
	testFunds   := big.NewInt(1000000000000000000)

	testRecvKey, _  := crypto.GenerateKey()
	testRecvAddress := crypto.PubkeyToAddress(testRecvKey.PublicKey)

	testRecvKey2, _  := crypto.GenerateKey()
	testRecvAddress2 := crypto.PubkeyToAddress(testRecvKey2.PublicKey)
	tx2, _ := types.SignTx(types.NewTransaction(0, testRecvAddress2, big.NewInt(1000), params.TxGas, nil, nil,0), types.HomesteadSigner{}, testKey)
	tx3, _ := types.SignTx(types.NewTransaction(100, testRecvAddress2, big.NewInt(3000), params.TxGas, nil, nil,0), types.HomesteadSigner{}, testKey)
	//tx4, _ := types.SignTx(types.NewTransaction(100, testRecvAddress2, big.NewInt(3000), params.TxGas, nil, nil,0), types.HomesteadSigner{}, testKey)

	fmt.Printf(" transfer %v from %v to %v\r\n",big.NewInt(1000),testAddress.Bytes(),testRecvAddress.Bytes());

	fmt.Printf(" transfer %v from %v to %v\r\n",big.NewInt(3000),testAddress.Bytes(),testRecvAddress2.Bytes());
	master,shards := newMasterShardTestWorker(t, chainConfig, engine, 0,0,core.GenesisAlloc{testAddress:{Balance:testFunds}})
	defer func(){
		master.worker.close();
		for _,shard := range shards {
			close(shard.channel)
			shard.worker.close()
		}
	}()

	go func() {
		time.Sleep(1400 * time.Millisecond)
		//create a new send Txs
		fmt.Println("create txs")
		master.backend.txPool.AddLocals([]*types.Transaction{tx2,tx3})
		shard2 := master.backend.chain.TxShardByHash(tx2.Hash())
		shard3 := master.backend.chain.TxShardByHash(tx3.Hash())
		//shard4 := master.backend.chain.TxShardByHash(tx4.Hash())
		shards[shard2].backend.txPool.AddLocals([]*types.Transaction{tx2})
		shards[shard3].backend.txPool.AddLocals([]*types.Transaction{tx3})
		time.Sleep(10*time.Second)
		master.backend.txPool.AddLocals([]*types.Transaction{tx3})
		shards[shard3].backend.txPool.AddLocals([]*types.Transaction{tx3})


	}()

	go func(){
		fmt.Println("waiting for master block")
		// Ensure the new tx events has been processed

		ch := make(chan core.ChainHeadEvent)
		master.backend.chain.SubscribeChainHeadEvent(ch)
		count := 0
		for {
			select {
			case head := <-ch:
				fmt.Println(head)
				if count++; count > 30 {
					state <- struct{}{}
				}
			}
		}


	}()

	<- state
}



/*func TestPendingStateAndBlockEthash(t *testing.T) {
	testPendingStateAndBlock(t, ethashChainConfig, ethash.NewFaker(),types.ShardMaster)
}
func TestPendingStateAndBlockClique(t *testing.T) {
	testPendingStateAndBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),types.ShardMaster)
}*/
func TestPendingStateAndBlockEthash0(t *testing.T) {
	testPendingStateAndBlock(t, ethashChainConfig, ethash.NewFaker(),0)
}
func TestPendingStateAndBlockClique0(t *testing.T) {
	testPendingStateAndBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),0)
}

func TestPendingStateAndBlockEthash10(t *testing.T) {
	testPendingStateAndBlock(t, ethashChainConfig, ethash.NewFaker(),10)
}
func TestPendingStateAndBlockClique10(t *testing.T) {
	testPendingStateAndBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),10)
}
func TestShardBlock(t *testing.T) {
	testShardBlock(t, ethashChainConfig, ethash.NewFaker())
}

func testPendingStateAndBlock(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine,shardId uint16) {
	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine, 0, shardId)
	w.start()
	defer w.close()

	// Ensure snapshot has been updated.
	time.Sleep(100 * time.Millisecond)
	block, state := w.pending()
	if block.NumberU64() != 1 {
		t.Errorf("block number mismatch: have %d, want %d", block.NumberU64(), 1)
	}
	if balance := state.GetBalance(testUserAddress); balance.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("account balance mismatch: have %d, want %d", balance, 1000)
	}
	b.txPool.AddLocals(newTxs)

	// Ensure the new tx events has been processed
	time.Sleep(100 * time.Millisecond)
	block, state = w.pending()
	if balance := state.GetBalance(testUserAddress); balance.Cmp(big.NewInt(2000)) != 0 {
		t.Errorf("account balance mismatch: have %d, want %d", balance, 2000)
	}
}
var (
	ostream log.Handler
	glogger *log.GlogHandler
)

func init() {
	usecolor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	output := io.Writer(os.Stderr)
	if usecolor {
		output = colorable.NewColorableStderr()
	}
	ostream = log.StreamHandler(output, log.TerminalFormat(usecolor))
	glogger = log.NewGlogHandler(ostream)

	log.PrintOrigins(true)
	glogger.Verbosity(log.Lvl(5))
	//glogger.Vmodule(ctx.GlobalString(vmoduleFlag.Name))
	//glogger.BacktraceAt(ctx.GlobalString(backtraceAtFlag.Name))
	log.Root().SetHandler(glogger)
}



func testShardBlock(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine) {
	log.PrintOrigins(true)
	glogger.Verbosity(log.Lvl(4))
	//glogger.Vmodule(ctx.GlobalString(vmoduleFlag.Name))
	//glogger.BacktraceAt(ctx.GlobalString(backtraceAtFlag.Name))
	log.Root().SetHandler(glogger)

	defer engine.Close()

	state := make(chan  struct{})
	w, b := newTestWorker(t, chainConfig, engine, 0,10)
	defer w.close()


	// Ensure snapshot has been updated.
	/*time.Sleep(100 * time.Millisecond)
	block, _ := w.pending()
	if block.NumberU64() != 1 {
		t.Errorf("block number mismatch: have %d, want %d", block.NumberU64(), 1)
	}
	if len(block.Results()) != 0 {
		t.Errorf("shard results error balance mismatch: have %d, want %d", len(block.Results()), 0)
	}*/
	go func() {
		time.Sleep(1500 * time.Millisecond)
		fmt.Println("add tx")
		b.txPool.AddLocals(newTxs)
	}()




	go func(){
		fmt.Println("waiting")
		// Ensure the new tx events has been processed
		time.Sleep(60000 * time.Millisecond)
		block:= w.pendingBlock()
		if block == nil {
			t.Errorf("shard results error: block is nil")
		}else{
			if len(block.Results()) != len(newTxs) {
				t.Errorf("shard results error balance mismatch: have %d, want %d", len(block.Results()),len(newTxs))
			}
		}
		state <- struct{}{}
	}()

	<- state
}


/*func TestEmptyWorkEthash(t *testing.T) {
	testEmptyWork(t, ethashChainConfig, ethash.NewFaker(),types.ShardMaster)
}
func TestEmptyWorkClique(t *testing.T) {
	testEmptyWork(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),types.ShardMaster)
}*/
func TestEmptyWorkEthash0(t *testing.T) {
	testEmptyWork(t, ethashChainConfig, ethash.NewFaker(),0)
}
func TestEmptyWorkClique0(t *testing.T) {
	testEmptyWork(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),0)
}
func TestEmptyWorkEthash10(t *testing.T) {
	testEmptyWork(t, ethashChainConfig, ethash.NewFaker(),10)
}
func TestEmptyWorkClique10(t *testing.T) {
	testEmptyWork(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),10)
}
func testEmptyWork(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, shardId uint16) {
	defer engine.Close()

	w, _ := newTestWorker(t, chainConfig, engine, 0,shardId)
	defer w.close()

	var (
		taskCh    = make(chan struct{}, 2)
		taskIndex int
	)

	checkEqual := func(t *testing.T, block types.BlockIntf, index int) {
		receiptLen, _ := 0, big.NewInt(0)
		if index == 1 {
			receiptLen, _ = 1, big.NewInt(1000)
		}
		if len(block.Receipts()) != receiptLen {
			t.Errorf("receipt number mismatch: have %d, want %d", len(block.Receipts()), receiptLen)
		}
		/*if task.state.GetBalance(testUserAddress).Cmp(balance) != 0 {
			t.Errorf("account balance mismatch: have %d, want %d", task.state.GetBalance(testUserAddress), balance)
		}*/
	}

	w.newTaskHook = func(block types.BlockIntf) {
		if block.NumberU64() == 1 {
			checkEqual(t, block, taskIndex)
			taskIndex += 1
			taskCh <- struct{}{}
		}
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}

	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()
	for i := 0; i < 2; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}
}

/*func TestStreamUncleBlock(t *testing.T) {
	ethash := ethash.NewFaker()
	defer ethash.Close()

	w, b := newTestWorker(t, ethashChainConfig, ethash, 1,types.ShardMaster)
	defer w.close()

	var taskCh = make(chan struct{})

	taskIndex := 0
	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 2 {
			if taskIndex == 2 {
				have := task.block.Header().UncleHash()
				want := types.CalcUncleHash([]types.HeaderIntf{b.uncleBlock.Header()})
				if have != want {
					t.Errorf("uncle hash mismatch: have %s, want %s", have.Hex(), want.Hex())
				}
			}
			taskCh <- struct{}{}
			taskIndex += 1
		}
	}
	w.skipSealHook = func(task *task) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}

	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 2 {
			break
		}
	}
	w.start()

	// Ignore the first two works
	for i := 0; i < 2; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}
	b.PostChainEvents([]interface{}{core.ChainSideEvent{Block: b.uncleBlock}})

	select {
	case <-taskCh:
	case <-time.NewTimer(time.Second).C:
		t.Error("new task timeout")
	}
}

func TestRegenerateMiningBlockEthash(t *testing.T) {
	testRegenerateMiningBlock(t, ethashChainConfig, ethash.NewFaker(),types.ShardMaster)
}

func TestRegenerateMiningBlockClique(t *testing.T) {
	testRegenerateMiningBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),types.ShardMaster)
}
*/
func TestRegenerateMiningBlockEthash0(t *testing.T) {
	testRegenerateMiningBlock(t, ethashChainConfig, ethash.NewFaker(),0)
}

func TestRegenerateMiningBlockClique0(t *testing.T) {
	testRegenerateMiningBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),0)
}
func TestRegenerateMiningBlockEthash10(t *testing.T) {
	testRegenerateMiningBlock(t, ethashChainConfig, ethash.NewFaker(),10)
}

func TestRegenerateMiningBlockClique10(t *testing.T) {
	testRegenerateMiningBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),10)
}
func testRegenerateMiningBlock(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, shardId uint16) {
	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine, 0,shardId)
	defer w.close()

	var taskCh = make(chan struct{})

	//taskIndex := 0
	/*w.newTaskHook = func(block types.BlockIntf) {
		if block.NumberU64() == 1 {
			if taskIndex == 2 {
				receiptLen, balance := 2, big.NewInt(2000)
				if len(task.receipts) != receiptLen {
					t.Errorf("receipt number mismatch: have %d, want %d", len(task.receipts), receiptLen)
				}
				if task.state.GetBalance(testUserAddress).Cmp(balance) != 0 {
					t.Errorf("account balance mismatch: have %d, want %d", task.state.GetBalance(testUserAddress), balance)
				}
			}
			taskCh <- struct{}{}
			taskIndex += 1
		}
	}*/
	w.skipSealHook = func(block types.BlockIntf) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}
	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()
	// Ignore the first two works
	for i := 0; i < 2; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}
	b.txPool.AddLocals(newTxs)
	time.Sleep(time.Second)

	select {
	case <-taskCh:
	case <-time.NewTimer(time.Second).C:
		t.Error("new task timeout")
	}
}
/*
func TestAdjustIntervalEthash(t *testing.T) {
	testAdjustInterval(t, ethashChainConfig, ethash.NewFaker(),types.ShardMaster)
}

func TestAdjustIntervalClique0(t *testing.T) {
	testAdjustInterval(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),types.ShardMaster)
}*/
func TestAdjustIntervalEthash0(t *testing.T) {
	testAdjustInterval(t, ethashChainConfig, ethash.NewFaker(),0)
}

func TestAdjustIntervalClique10(t *testing.T) {
	testAdjustInterval(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),0)
}

func TestAdjustIntervalEthash10(t *testing.T) {
	testAdjustInterval(t, ethashChainConfig, ethash.NewFaker(),10)
}

func TestAdjustIntervalClique(t *testing.T) {
	testAdjustInterval(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()),10)
}


func testAdjustInterval(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, shardId uint16) {
	defer engine.Close()

	w, _ := newTestWorker(t, chainConfig, engine, 0,shardId)
	defer w.close()

	w.skipSealHook = func(block types.BlockIntf) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}
	var (
		progress = make(chan struct{}, 100)
		result   = make([]float64, 0, 100)
		index    = 0
		start    = false
	)
	w.resubmitHook = func(minInterval time.Duration, recommitInterval time.Duration) {
		// Short circuit if interval checking hasn't started.
		if !start {
			return
		}
		var wantMinInterval, wantRecommitInterval time.Duration

		switch index {
		case 0:
			wantMinInterval, wantRecommitInterval = 3*time.Second, 3*time.Second
		case 1:
			origin := float64(3 * time.Second.Nanoseconds())
			estimate := origin*(1-intervalAdjustRatio) + intervalAdjustRatio*(origin/0.8+intervalAdjustBias)
			wantMinInterval, wantRecommitInterval = 3*time.Second, time.Duration(estimate)*time.Nanosecond
		case 2:
			estimate := result[index-1]
			min := float64(3 * time.Second.Nanoseconds())
			estimate = estimate*(1-intervalAdjustRatio) + intervalAdjustRatio*(min-intervalAdjustBias)
			wantMinInterval, wantRecommitInterval = 3*time.Second, time.Duration(estimate)*time.Nanosecond
		case 3:
			wantMinInterval, wantRecommitInterval = time.Second, time.Second
		}

		// Check interval
		if minInterval != wantMinInterval {
			t.Errorf("resubmit min interval mismatch: have %v, want %v ", minInterval, wantMinInterval)
		}
		if recommitInterval != wantRecommitInterval {
			t.Errorf("resubmit interval mismatch: have %v, want %v", recommitInterval, wantRecommitInterval)
		}
		result = append(result, float64(recommitInterval.Nanoseconds()))
		index += 1
		progress <- struct{}{}
	}
	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()

	time.Sleep(time.Second)

	start = true
	w.setRecommitInterval(3 * time.Second)
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}

	w.resubmitAdjustCh <- &intervalAdjust{inc: true, ratio: 0.8}
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}

	w.resubmitAdjustCh <- &intervalAdjust{inc: false}
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}

	w.setRecommitInterval(500 * time.Millisecond)
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}
}
