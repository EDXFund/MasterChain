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

package fetcher

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/consensus/ethash"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/crypto"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/params"
)

var (
	testdb        = ethdb.NewMemDatabase()
	testKey, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddress   = crypto.PubkeyToAddress(testKey.PublicKey)
	genesis       = core.GenesisBlockForTesting(testdb, testAddress, big.NewInt(1000000000), types.ShardMaster)
	genesis_shard = core.GenesisBlockForTesting(testdb, testAddress, big.NewInt(1000000000), 0)
	unknownBlock  = types.NewBlock(&types.Header{}, nil, nil, nil)
)

// makeChain creates a chain of n blocks starting at and including parent.
// the returned hash chain is ordered head->parent. In addition, every 3rd block
// contains a transaction and every 5th an uncle to allow testing correct block
// reassembly.
func makeChain(n int, seed byte, parent types.BlockIntf) ([]common.Hash, map[common.Hash]types.BlockIntf) {
	blocks, _ := core.GenerateChain(params.TestChainConfig, parent, ethash.NewFaker(), testdb, n, func(i int, block *core.BlockGen) {
		block.SetCoinbase(common.Address{seed})

		// If the block number is multiple of 3, send a bonus transaction to the miner
		if parent == genesis && i%3 == 0 {
			signer := types.MakeSigner(params.TestChainConfig, block.Number())
			tx, err := types.SignTx(types.NewTransaction(block.TxNonce(testAddress), common.Address{seed}, big.NewInt(1000), params.TxGas, nil, nil, 0), signer, testKey)
			if err != nil {
				panic(err)
			}
			block.AddTx(tx)
		}
		// If the block number is a multiple of 5, add a bonus uncle to the block
		/*if i%5 == 0 {
			block.AddUncle(&types.Header{ParentHash: block.PrevBlock(i - 1).Hash(), Number: big.NewInt(int64(i - 1))})
		}*/
	})
	hashes := make([]common.Hash, n+1)
	hashes[len(hashes)-1] = parent.Hash()
	blockm := make(map[common.Hash]types.BlockIntf, n+1)
	blockm[parent.Hash()] = parent
	for i, b := range blocks {
		hashes[len(hashes)-i-2] = b.Hash()
		if b.ShardId() == types.ShardMaster {
			blockm[b.Hash()] = b.ToBlock()
		} else {
			blockm[b.Hash()] = b.ToSBlock()
		}

	}
	return hashes, blockm
}

// fetcherTester is a test simulator for mocking out local block chain.
type fetcherTester struct {
	fetcher *Fetcher

	hashes []common.Hash                   // Hash chain belonging to the tester
	blocks map[common.Hash]types.BlockIntf // Blocks belonging to the tester
	drops  map[string]bool                 // Map of peers dropped by the fetcher

	lock sync.RWMutex
}

// newTester creates a new fetcher test mocker.
func newTester(shardId uint16) *fetcherTester {
	if shardId == types.ShardMaster {
		tester := &fetcherTester{
			hashes: []common.Hash{genesis.Hash()},
			blocks: map[common.Hash]types.BlockIntf{genesis.Hash(): genesis},
			drops:  make(map[string]bool),
		}
		tester.fetcher = New(tester.getBlock, tester.verifyHeader, tester.broadcastBlock, tester.chainHeight, tester.insertChain, tester.dropPeer)
		tester.fetcher.Start()

		return tester
	} else {
		tester := &fetcherTester{
			hashes: []common.Hash{genesis_shard.Hash()},
			blocks: map[common.Hash]types.BlockIntf{genesis_shard.Hash(): genesis_shard},
			drops:  make(map[string]bool),
		}
		tester.fetcher = New(tester.getBlock, tester.verifyHeader, tester.broadcastBlock, tester.chainHeight, tester.insertChain, tester.dropPeer)
		tester.fetcher.Start()

		return tester
	}

}

// getBlock retrieves a block from the tester's block chain.
func (f *fetcherTester) getBlock(hash common.Hash) types.BlockIntf {
	f.lock.RLock()
	defer f.lock.RUnlock()

	return f.blocks[hash]
}

// verifyHeader is a nop placeholder for the block header verification.
func (f *fetcherTester) verifyHeader(header types.HeaderIntf) error {
	return nil
}

// broadcastBlock is a nop placeholder for the block broadcasting.
func (f *fetcherTester) broadcastBlock(block types.BlockIntf, propagate bool) {
}

// chainHeight retrieves the current height (block number) of the chain.
func (f *fetcherTester) chainHeight(shardId uint16) uint64 {
	f.lock.RLock()
	defer f.lock.RUnlock()

	return f.blocks[f.hashes[len(f.hashes)-1]].NumberU64()
}

// insertChain injects a new blocks into the simulated chain.
func (f *fetcherTester) insertChain(blocks types.BlockIntfs) (int, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for i, block := range blocks {
		// Make sure the parent in known
		if _, ok := f.blocks[block.ParentHash()]; !ok {
			return i, errors.New("unknown parent")
		}
		// Discard any new blocks if the same height already exists
		if block.NumberU64() <= f.blocks[f.hashes[len(f.hashes)-1]].NumberU64() {
			return i, nil
		}
		// Otherwise build our current chain
		f.hashes = append(f.hashes, block.Hash())
		f.blocks[block.Hash()] = block
	}
	return 0, nil
}

// dropPeer is an emulator for the peer removal, simply accumulating the various
// peers dropped by the fetcher.
func (f *fetcherTester) dropPeer(peer string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.drops[peer] = true
}

// makeHeaderFetcher retrieves a block header fetcher associated with a simulated peer.
func (f *fetcherTester) makeHeaderFetcher(peer string, blocks map[common.Hash]types.BlockIntf, drift time.Duration) headerRequesterFn {
	closure := make(map[common.Hash]types.BlockIntf)
	for hash, block := range blocks {
		closure[hash] = block
	}
	// Create a function that return a header from the closure
	return func(hash common.Hash, shardId uint16) error {
		// Gather the blocks to return
		headers := make([]types.HeaderIntf, 0, 1)
		if block, ok := closure[hash]; ok {
			headers = append(headers, block.Header())
		}
		// Return on a new thread
		go f.fetcher.FilterHeaders(peer, headers, time.Now().Add(drift))

		return nil
	}
}

// makeBodyFetcher retrieves a block body fetcher associated with a simulated peer.
func (f *fetcherTester) makeBodyFetcher(peer string, blocks map[common.Hash]types.BlockIntf, drift time.Duration, shardId uint16) bodyRequesterFn {
	closure := make(map[common.Hash]types.BlockIntf)
	for hash, block := range blocks {
		closure[hash] = block
	}
	// Create a function that returns blocks from the closure
	return func(hashes []common.Hash, shardId uint16) error {
		// Gather the block bodies to return
		shardBlocks := make([][]*types.ShardBlockInfo, 0, len(hashes))
		transactions := make([][]*types.Transaction, 0, len(hashes))
		results := make([][]*types.ContractResult, 0, len(hashes))
		//uncles := make([][]types.HeaderIntf, 0, len(hashes))

		for _, hash := range hashes {
			if block, ok := closure[hash]; ok {
				if shardId == types.ShardMaster {
					shardBlocks = append(shardBlocks, block.ShardBlocks())
				} else {
					transactions = append(transactions, block.Transactions())
					results = append(results, block.Results())
				}

				//uncles = append(uncles, block.Uncles())
			}
		}
		// Return on a new thread
		go f.fetcher.FilterBodies(peer, shardBlocks, nil, transactions, results, time.Now().Add(drift), shardId)

		return nil
	}
}

// verifyFetchingEvent verifies that one single event arrive on a fetching channel.
func verifyFetchingEvent(t *testing.T, fetching chan []common.Hash, arrive bool) {
	if arrive {
		select {
		case <-fetching:
		case <-time.After(time.Second):
			t.Fatalf("fetching timeout")
		}
	} else {
		select {
		case <-fetching:
			t.Fatalf("fetching invoked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// verifyCompletingEvent verifies that one single event arrive on an completing channel.
func verifyCompletingEvent(t *testing.T, completing chan []common.Hash, arrive bool) {
	if arrive {
		select {
		case <-completing:
		case <-time.After(time.Second):
			t.Fatalf("completing timeout")
		}
	} else {
		select {
		case <-completing:
			t.Fatalf("completing invoked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// verifyImportEvent verifies that one single event arrive on an import channel.
func verifyImportEvent(t *testing.T, imported chan types.BlockIntf, arrive bool) {
	if arrive {
		select {
		case <-imported:
		case <-time.After(time.Second):
			t.Fatalf("import timeout")
		}
	} else {
		select {
		case <-imported:
			t.Fatalf("import invoked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// verifyImportCount verifies that exactly count number of events arrive on an
// import hook channel.
func verifyImportCount(t *testing.T, imported chan types.BlockIntf, count int) {
	for i := 0; i < count; i++ {
		select {
		case <-imported:
		case <-time.After(time.Second):
			t.Fatalf("block %d: import timeout", i+1)
		}
	}
	verifyImportDone(t, imported)
}

// verifyImportDone verifies that no more events are arriving on an import channel.
func verifyImportDone(t *testing.T, imported chan types.BlockIntf) {
	select {
	case <-imported:
		t.Fatalf("extra block imported")
	case <-time.After(50 * time.Millisecond):
	}
}

// Tests that a fetcher accepts block announcements and initiates retrievals for
// them, successfully importing into the local chain.
func TestSequentialAnnouncements62M(t *testing.T) {
	testSequentialAnnouncements(t, 62, types.ShardMaster)
}
func TestSequentialAnnouncements63M(t *testing.T) {
	testSequentialAnnouncements(t, 63, types.ShardMaster)
}
func TestSequentialAnnouncements64M(t *testing.T) {
	testSequentialAnnouncements(t, 64, types.ShardMaster)
}
func TestSequentialAnnouncements62S(t *testing.T) { testSequentialAnnouncements(t, 62, 0) }
func TestSequentialAnnouncements63S(t *testing.T) { testSequentialAnnouncements(t, 63, 0) }
func TestSequentialAnnouncements64S(t *testing.T) { testSequentialAnnouncements(t, 64, 0) }
func testSequentialAnnouncements(t *testing.T, protocol int, shardId uint16) {
	// Create a chain of blocks to import
	targetBlocks := 4 * hashLimit
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}
	hashes, blocks := makeChain(targetBlocks, 0, parent)

	tester := newTester(shardId)
	headerFetcher := tester.makeHeaderFetcher("valid", blocks, -gatherSlack)

	bodyFetcher := tester.makeBodyFetcher("valid", blocks, 0, shardId)

	// Iteratively announce blocks until all are imported
	imported := make(chan types.BlockIntf)
	tester.fetcher.importedHook = func(block types.BlockIntf) {
		imported <- block
		fmt.Println("imported:", block.NumberU64())
	}

	for i := len(hashes) - 2; i >= 0; i-- {
		tester.fetcher.Notify("valid", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
		verifyImportEvent(t, imported, true)
	}
	verifyImportDone(t, imported)
}

// Tests that if blocks are announced by multiple peers (or even the same buggy
// peer), they will only get downloaded at most once.
func TestConcurrentAnnouncements62(t *testing.T) {
	testConcurrentAnnouncements(t, 62, types.ShardMaster)
}
func TestConcurrentAnnouncements63(t *testing.T) {
	testConcurrentAnnouncements(t, 63, types.ShardMaster)
}
func TestConcurrentAnnouncements64(t *testing.T) {
	testConcurrentAnnouncements(t, 64, types.ShardMaster)
}

func TestConcurrentAnnouncements62S(t *testing.T) { testConcurrentAnnouncements(t, 62, 0) }
func TestConcurrentAnnouncements63S(t *testing.T) { testConcurrentAnnouncements(t, 63, 0) }
func TestConcurrentAnnouncements64S(t *testing.T) { testConcurrentAnnouncements(t, 64, 0) }
func testConcurrentAnnouncements(t *testing.T, protocol int, shardId uint16) {
	// Create a chain of blocks to import
	targetBlocks := 4 * hashLimit
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}
	hashes, blocks := makeChain(targetBlocks, 0, parent)

	// Assemble a tester with a built in counter for the requests
	tester := newTester(shardId)
	firstHeaderFetcher := tester.makeHeaderFetcher("first", blocks, -gatherSlack)
	firstBodyFetcher := tester.makeBodyFetcher("first", blocks, 0, shardId)
	secondHeaderFetcher := tester.makeHeaderFetcher("second", blocks, -gatherSlack)
	secondBodyFetcher := tester.makeBodyFetcher("second", blocks, 0, shardId)

	counter := uint32(0)
	firstHeaderWrapper := func(hash common.Hash, shardId uint16) error {
		atomic.AddUint32(&counter, 1)
		return firstHeaderFetcher(hash, shardId)
	}
	secondHeaderWrapper := func(hash common.Hash, shardId uint16) error {
		atomic.AddUint32(&counter, 1)
		return secondHeaderFetcher(hash, shardId)
	}
	// Iteratively announce blocks until all are imported
	imported := make(chan types.BlockIntf)
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }

	for i := len(hashes) - 2; i >= 0; i-- {
		tester.fetcher.Notify("first", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout), firstHeaderWrapper, firstBodyFetcher)
		tester.fetcher.Notify("second", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout+time.Millisecond), secondHeaderWrapper, secondBodyFetcher)
		tester.fetcher.Notify("second", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout-time.Millisecond), secondHeaderWrapper, secondBodyFetcher)
		verifyImportEvent(t, imported, true)
	}
	verifyImportDone(t, imported)

	// Make sure no blocks were retrieved twice
	if int(counter) != targetBlocks {
		t.Fatalf("retrieval count mismatch: have %v, want %v", counter, targetBlocks)
	}
}

// Tests that announcements arriving while a previous is being fetched still
// results in a valid import.
func TestOverlappingAnnouncements62M(t *testing.T) {
	testOverlappingAnnouncements(t, 62, types.ShardMaster)
}
func TestOverlappingAnnouncements63M(t *testing.T) {
	testOverlappingAnnouncements(t, 63, types.ShardMaster)
}
func TestOverlappingAnnouncements64M(t *testing.T) {
	testOverlappingAnnouncements(t, 64, types.ShardMaster)
}
func TestOverlappingAnnouncements62S(t *testing.T) { testOverlappingAnnouncements(t, 62, 0) }
func TestOverlappingAnnouncements63S(t *testing.T) { testOverlappingAnnouncements(t, 63, 0) }
func TestOverlappingAnnouncements64S(t *testing.T) { testOverlappingAnnouncements(t, 64, 0) }
func testOverlappingAnnouncements(t *testing.T, protocol int, shardId uint16) {
	// Create a chain of blocks to import
	targetBlocks := 4 * hashLimit
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}
	hashes, blocks := makeChain(targetBlocks, 0, parent)

	tester := newTester(shardId)
	headerFetcher := tester.makeHeaderFetcher("valid", blocks, -gatherSlack)
	var bodyFetcher bodyRequesterFn
	bodyFetcher = tester.makeBodyFetcher("valid", blocks, 0, shardId)

	// Iteratively announce blocks, but overlap them continuously
	overlap := 16
	imported := make(chan types.BlockIntf, len(hashes)-1)
	for i := 0; i < overlap; i++ {
		imported <- nil
	}
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }

	for i := len(hashes) - 2; i >= 0; i-- {
		tester.fetcher.Notify("valid", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
		select {
		case <-imported:
		case <-time.After(time.Second):
			t.Fatalf("block %d: import timeout", len(hashes)-i)
		}
	}
	// Wait for all the imports to complete and check count
	verifyImportCount(t, imported, overlap)
}

// Tests that announces already being retrieved will not be duplicated.
func TestPendingDeduplication62M(t *testing.T) { testPendingDeduplication(t, 62, types.ShardMaster) }
func TestPendingDeduplication63M(t *testing.T) { testPendingDeduplication(t, 63, types.ShardMaster) }
func TestPendingDeduplication64M(t *testing.T) { testPendingDeduplication(t, 64, types.ShardMaster) }
func TestPendingDeduplication62S(t *testing.T) { testPendingDeduplication(t, 62, 0) }
func TestPendingDeduplication63S(t *testing.T) { testPendingDeduplication(t, 63, 0) }
func TestPendingDeduplication64S(t *testing.T) { testPendingDeduplication(t, 64, 0) }
func testPendingDeduplication(t *testing.T, protocol int, shardId uint16) {
	// Create a hash and corresponding block
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}
	hashes, blocks := makeChain(1, 0, parent)

	// Assemble a tester with a built in counter and delayed fetcher
	tester := newTester(shardId)
	headerFetcher := tester.makeHeaderFetcher("repeater", blocks, -gatherSlack)

	bodyFetcher := tester.makeBodyFetcher("repeater", blocks, 0, shardId)

	delay := 50 * time.Millisecond
	counter := uint32(0)
	headerWrapper := func(hash common.Hash, shardId uint16) error {
		atomic.AddUint32(&counter, 1)

		// Simulate a long running fetch
		go func() {
			time.Sleep(delay)
			headerFetcher(hash, shardId)
		}()
		return nil
	}
	// Announce the same block many times until it's fetched (wait for any pending ops)
	for tester.getBlock(hashes[0]) == nil {
		tester.fetcher.Notify("repeater", shardId, hashes[0], 1, time.Now().Add(-arriveTimeout), headerWrapper, bodyFetcher)
		time.Sleep(time.Millisecond)
	}
	time.Sleep(delay)

	// Check that all blocks were imported and none fetched twice
	if imported := len(tester.blocks); imported != 2 {
		t.Fatalf("synchronised block mismatch: have %v, want %v", imported, 2)
	}
	if int(counter) != 1 {
		t.Fatalf("retrieval count mismatch: have %v, want %v", counter, 1)
	}
}

// Tests that announcements retrieved in a random order are cached and eventually
// imported when all the gaps are filled in.
func TestRandomArrivalImport62M(t *testing.T) { testRandomArrivalImport(t, 62, types.ShardMaster) }
func TestRandomArrivalImport63M(t *testing.T) { testRandomArrivalImport(t, 63, types.ShardMaster) }
func TestRandomArrivalImport64M(t *testing.T) { testRandomArrivalImport(t, 64, types.ShardMaster) }
func TestRandomArrivalImport62S(t *testing.T) { testRandomArrivalImport(t, 62, 0) }
func TestRandomArrivalImport63S(t *testing.T) { testRandomArrivalImport(t, 63, 0) }
func TestRandomArrivalImport64S(t *testing.T) { testRandomArrivalImport(t, 64, 0) }
func testRandomArrivalImport(t *testing.T, protocol int, shardId uint16) {
	// Create a chain of blocks to import, and choose one to delay
	targetBlocks := maxQueueDist
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}
	hashes, blocks := makeChain(targetBlocks, 0, parent)
	skip := targetBlocks / 2

	tester := newTester(shardId)
	headerFetcher := tester.makeHeaderFetcher("valid", blocks, -gatherSlack)
	bodyFetcher := tester.makeBodyFetcher("valid", blocks, 0, shardId)

	// Iteratively announce blocks, skipping one entry
	imported := make(chan types.BlockIntf, len(hashes)-1)
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }

	for i := len(hashes) - 1; i >= 0; i-- {
		if i != skip {
			tester.fetcher.Notify("valid", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
			time.Sleep(time.Millisecond)
		}
	}
	// Finally announce the skipped entry and check full import
	tester.fetcher.Notify("valid", shardId, hashes[skip], uint64(len(hashes)-skip-1), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
	verifyImportCount(t, imported, len(hashes)-1)
}

// Tests that direct block enqueues (due to block propagation vs. hash announce)
// are correctly schedule, filling and import queue gaps.
func TestQueueGapFill62M(t *testing.T) { testQueueGapFill(t, 62, types.ShardMaster) }
func TestQueueGapFill63M(t *testing.T) { testQueueGapFill(t, 63, types.ShardMaster) }
func TestQueueGapFill64M(t *testing.T) { testQueueGapFill(t, 64, types.ShardMaster) }
func TestQueueGapFill62S(t *testing.T) { testQueueGapFill(t, 62, 0) }
func TestQueueGapFill63S(t *testing.T) { testQueueGapFill(t, 63, 0) }
func TestQueueGapFill64S(t *testing.T) { testQueueGapFill(t, 64, 0) }
func testQueueGapFill(t *testing.T, protocol int, shardId uint16) {
	// Create a chain of blocks to import, and choose one to not announce at all
	targetBlocks := maxQueueDist
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}
	hashes, blocks := makeChain(targetBlocks, 0, parent)
	skip := targetBlocks / 2

	tester := newTester(shardId)
	headerFetcher := tester.makeHeaderFetcher("valid", blocks, -gatherSlack)
	bodyFetcher := tester.makeBodyFetcher("valid", blocks, 0, shardId)

	// Iteratively announce blocks, skipping one entry
	imported := make(chan types.BlockIntf, len(hashes)-1)
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }

	for i := len(hashes) - 1; i >= 0; i-- {
		if i != skip {
			tester.fetcher.Notify("valid", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
			time.Sleep(time.Millisecond)
		}
	}
	// Fill the missing block directly as if propagated
	tester.fetcher.Enqueue("valid", blocks[hashes[skip]])
	verifyImportCount(t, imported, len(hashes)-1)
}

// Tests that blocks arriving from various sources (multiple propagations, hash
// announces, etc) do not get scheduled for import multiple times.
func TestImportDeduplication62M(t *testing.T) { testImportDeduplication(t, 62, types.ShardMaster) }
func TestImportDeduplication63M(t *testing.T) { testImportDeduplication(t, 63, types.ShardMaster) }
func TestImportDeduplication64M(t *testing.T) { testImportDeduplication(t, 64, types.ShardMaster) }
func TestImportDeduplication62S(t *testing.T) { testImportDeduplication(t, 62, 0) }
func TestImportDeduplication63S(t *testing.T) { testImportDeduplication(t, 63, 0) }
func TestImportDeduplication64S(t *testing.T) { testImportDeduplication(t, 64, 0) }
func testImportDeduplication(t *testing.T, protocol int, shardId uint16) {
	// Create two blocks to import (one for duplication, the other for stalling)
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}
	hashes, blocks := makeChain(2, 0, parent)

	// Create the tester and wrap the importer with a counter
	tester := newTester(shardId)
	headerFetcher := tester.makeHeaderFetcher("valid", blocks, -gatherSlack)
	bodyFetcher := tester.makeBodyFetcher("valid", blocks, 0, shardId)

	counter := uint32(0)
	tester.fetcher.insertChain = func(blocks types.BlockIntfs) (int, error) {
		atomic.AddUint32(&counter, uint32(len(blocks)))
		return tester.insertChain(blocks)
	}
	// Instrument the fetching and imported events
	fetching := make(chan []common.Hash)
	imported := make(chan types.BlockIntf, len(hashes)-1)
	tester.fetcher.fetchingHook = func(hashes []common.Hash, shardId uint16) { fetching <- hashes }
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }

	// Announce the duplicating block, wait for retrieval, and also propagate directly
	tester.fetcher.Notify("valid", shardId, hashes[0], 1, time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
	<-fetching

	tester.fetcher.Enqueue("valid", blocks[hashes[0]])
	tester.fetcher.Enqueue("valid", blocks[hashes[0]])
	tester.fetcher.Enqueue("valid", blocks[hashes[0]])

	// Fill the missing block directly as if propagated, and check import uniqueness
	tester.fetcher.Enqueue("valid", blocks[hashes[1]])
	verifyImportCount(t, imported, 2)

	if counter != 2 {
		t.Fatalf("import invocation count mismatch: have %v, want %v", counter, 2)
	}
}

// Tests that blocks with numbers much lower or higher than out current head get
// discarded to prevent wasting resources on useless blocks from faulty peers.
func TestDistantPropagationDiscarding(t *testing.T) {
	// Create a long chain to import and define the discard boundaries
	hashes, blocks := makeChain(3*maxQueueDist, 0, genesis)
	head := hashes[len(hashes)/2]

	low, high := len(hashes)/2+maxUncleDist+1, len(hashes)/2-maxQueueDist-1

	// Create a tester and simulate a head block being the middle of the above chain
	tester := newTester(types.ShardMaster)

	tester.lock.Lock()
	tester.hashes = []common.Hash{head}
	tester.blocks = map[common.Hash]types.BlockIntf{head: blocks[head]}
	tester.lock.Unlock()

	// Ensure that a block with a lower number than the threshold is discarded
	tester.fetcher.Enqueue("lower", blocks[hashes[low]])
	time.Sleep(10 * time.Millisecond)
	if !tester.fetcher.queue.Empty() {
		t.Fatalf("fetcher queued stale block")
	}
	// Ensure that a block with a higher number than the threshold is discarded
	tester.fetcher.Enqueue("higher", blocks[hashes[high]])
	time.Sleep(10 * time.Millisecond)
	if !tester.fetcher.queue.Empty() {
		t.Fatalf("fetcher queued future block")
	}
}

// Tests that announcements with numbers much lower or higher than out current
// head get discarded to prevent wasting resources on useless blocks from faulty
// peers.
func TestDistantAnnouncementDiscarding62M(t *testing.T) {
	testDistantAnnouncementDiscarding(t, 62, types.ShardMaster)
}
func TestDistantAnnouncementDiscarding63M(t *testing.T) {
	testDistantAnnouncementDiscarding(t, 63, types.ShardMaster)
}
func TestDistantAnnouncementDiscarding64M(t *testing.T) {
	testDistantAnnouncementDiscarding(t, 64, types.ShardMaster)
}

func TestDistantAnnouncementDiscarding62S(t *testing.T) { testDistantAnnouncementDiscarding(t, 62, 0) }
func TestDistantAnnouncementDiscarding63S(t *testing.T) { testDistantAnnouncementDiscarding(t, 63, 0) }
func TestDistantAnnouncementDiscarding64S(t *testing.T) { testDistantAnnouncementDiscarding(t, 64, 0) }

func testDistantAnnouncementDiscarding(t *testing.T, protocol int, shardId uint16) {
	// Create a long chain to import and define the discard boundaries
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}

	hashes, blocks := makeChain(3*maxQueueDist, 0, parent)
	head := hashes[len(hashes)/2]

	low, high := len(hashes)/2+maxUncleDist+1, len(hashes)/2-maxQueueDist-1

	// Create a tester and simulate a head block being the middle of the above chain
	tester := newTester(shardId)

	tester.lock.Lock()
	tester.hashes = []common.Hash{head}
	tester.blocks = map[common.Hash]types.BlockIntf{head: blocks[head]}
	tester.lock.Unlock()

	headerFetcher := tester.makeHeaderFetcher("lower", blocks, -gatherSlack)
	bodyFetcher := tester.makeBodyFetcher("lower", blocks, 0, shardId)

	fetching := make(chan struct{}, 2)
	tester.fetcher.fetchingHook = func(hashes []common.Hash, shardId uint16) { fetching <- struct{}{} }

	// Ensure that a block with a lower number than the threshold is discarded
	tester.fetcher.Notify("lower", shardId, hashes[low], blocks[hashes[low]].NumberU64(), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
	select {
	case <-time.After(50 * time.Millisecond):
	case <-fetching:
		t.Fatalf("fetcher requested stale header")
	}
	// Ensure that a block with a higher number than the threshold is discarded
	tester.fetcher.Notify("higher", shardId, hashes[high], blocks[hashes[high]].NumberU64(), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)
	select {
	case <-time.After(50 * time.Millisecond):
	case <-fetching:
		t.Fatalf("fetcher requested future header")
	}
}

// Tests that peers announcing blocks with invalid numbers (i.e. not matching
// the headers provided afterwards) get dropped as malicious.
func TestInvalidNumberAnnouncement62M(t *testing.T) {
	testInvalidNumberAnnouncement(t, 62, types.ShardMaster)
}
func TestInvalidNumberAnnouncement63M(t *testing.T) {
	testInvalidNumberAnnouncement(t, 63, types.ShardMaster)
}
func TestInvalidNumberAnnouncement64M(t *testing.T) {
	testInvalidNumberAnnouncement(t, 64, types.ShardMaster)
}

func TestInvalidNumberAnnouncement62S(t *testing.T) { testInvalidNumberAnnouncement(t, 62, 0) }
func TestInvalidNumberAnnouncement63S(t *testing.T) { testInvalidNumberAnnouncement(t, 63, 0) }
func TestInvalidNumberAnnouncement64S(t *testing.T) { testInvalidNumberAnnouncement(t, 64, 0) }
func testInvalidNumberAnnouncement(t *testing.T, protocol int, shardId uint16) {
	// Create a single block to import and check numbers against
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}

	hashes, blocks := makeChain(1, 0, parent)

	tester := newTester(shardId)
	badHeaderFetcher := tester.makeHeaderFetcher("bad", blocks, -gatherSlack)
	badBodyFetcher := tester.makeBodyFetcher("bad", blocks, 0, shardId)

	imported := make(chan types.BlockIntf)
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }

	// Announce a block with a bad number, check for immediate drop
	tester.fetcher.Notify("bad", shardId, hashes[0], 2, time.Now().Add(-arriveTimeout), badHeaderFetcher, badBodyFetcher)
	verifyImportEvent(t, imported, false)

	tester.lock.RLock()
	dropped := tester.drops["bad"]
	tester.lock.RUnlock()

	if !dropped {
		t.Fatalf("peer with invalid numbered announcement not dropped")
	}

	goodHeaderFetcher := tester.makeHeaderFetcher("good", blocks, -gatherSlack)
	goodBodyFetcher := tester.makeBodyFetcher("good", blocks, 0, shardId)
	// Make sure a good announcement passes without a drop
	tester.fetcher.Notify("good", shardId, hashes[0], 1, time.Now().Add(-arriveTimeout), goodHeaderFetcher, goodBodyFetcher)
	verifyImportEvent(t, imported, true)

	tester.lock.RLock()
	dropped = tester.drops["good"]
	tester.lock.RUnlock()

	if dropped {
		t.Fatalf("peer with valid numbered announcement dropped")
	}
	verifyImportDone(t, imported)
}

// Tests that if a block is empty (i.e. header only), no body request should be
// made, and instead the header should be assembled into a whole block in itself.
func TestEmptyBlockShortCircuit62M(t *testing.T) { testEmptyBlockShortCircuit(t, 62, types.ShardMaster) }
func TestEmptyBlockShortCircuit63M(t *testing.T) { testEmptyBlockShortCircuit(t, 63, types.ShardMaster) }
func TestEmptyBlockShortCircuit64M(t *testing.T) { testEmptyBlockShortCircuit(t, 64, types.ShardMaster) }
func TestEmptyBlockShortCircuit62S(t *testing.T) { testEmptyBlockShortCircuit(t, 62, 0) }
func TestEmptyBlockShortCircuit63S(t *testing.T) { testEmptyBlockShortCircuit(t, 63, 0) }
func TestEmptyBlockShortCircuit64S(t *testing.T) { testEmptyBlockShortCircuit(t, 64, 0) }
func testEmptyBlockShortCircuit(t *testing.T, protocol int, shardId uint16) {
	// Create a chain of blocks to import
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}

	hashes, blocks := makeChain(33, 0, parent)

	tester := newTester(shardId)
	headerFetcher := tester.makeHeaderFetcher("valid", blocks, -gatherSlack)
	bodyFetcher := tester.makeBodyFetcher("valid", blocks, 0, shardId)

	// Add a monitoring hook for all internal events
	fetching := make(chan []common.Hash)
	tester.fetcher.fetchingHook = func(hashes []common.Hash, shardId uint16) { fetching <- hashes }

	completing := make(chan []common.Hash)
	tester.fetcher.completingHook = func(hashes []common.Hash, shardId uint16) { completing <- hashes }

	imported := make(chan types.BlockIntf)
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }

	// Iteratively announce blocks until all are imported
	for i := len(hashes) - 2; i >= 0; i-- {
		tester.fetcher.Notify("valid", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout), headerFetcher, bodyFetcher)

		// All announces should fetch the header
		verifyFetchingEvent(t, fetching, true)

		// Only blocks with data contents should request bodies
		verifyCompletingEvent(t, completing, len(blocks[hashes[i]].Transactions()) > 0 || len(blocks[hashes[i]].Uncles()) > 0)

		// Irrelevant of the construct, import should succeed
		verifyImportEvent(t, imported, true)
	}
	verifyImportDone(t, imported)
}

// Tests that a peer is unable to use unbounded memory with sending infinite
// block announcements to a node, but that even in the face of such an attack,
// the fetcher remains operational.
func TestHashMemoryExhaustionAttack62M(t *testing.T) {
	testHashMemoryExhaustionAttack(t, 62, types.ShardMaster)
}
func TestHashMemoryExhaustionAttack63M(t *testing.T) {
	testHashMemoryExhaustionAttack(t, 63, types.ShardMaster)
}
func TestHashMemoryExhaustionAttack64M(t *testing.T) {
	testHashMemoryExhaustionAttack(t, 64, types.ShardMaster)
}

func TestHashMemoryExhaustionAttack62S(t *testing.T) { testHashMemoryExhaustionAttack(t, 62, 0) }
func TestHashMemoryExhaustionAttack63S(t *testing.T) { testHashMemoryExhaustionAttack(t, 63, 0) }
func TestHashMemoryExhaustionAttack64S(t *testing.T) { testHashMemoryExhaustionAttack(t, 64, 0) }
func testHashMemoryExhaustionAttack(t *testing.T, protocol int, shardId uint16) {
	// Create a tester with instrumented import hooks
	tester := newTester(shardId)

	imported, announces := make(chan types.BlockIntf), int32(0)
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }
	tester.fetcher.announceChangeHook = func(hash common.Hash, added bool) {
		if added {
			atomic.AddInt32(&announces, 1)
		} else {
			atomic.AddInt32(&announces, -1)
		}
	}
	// Create a valid chain and an infinite junk chain
	targetBlocks := hashLimit + 2*maxQueueDist
	var parent types.BlockIntf
	if shardId == types.ShardMaster {
		parent = genesis
	} else {
		parent = genesis_shard
	}

	hashes, blocks := makeChain(targetBlocks, 0, parent)
	validHeaderFetcher := tester.makeHeaderFetcher("valid", blocks, -gatherSlack)
	validBodyFetcher := tester.makeBodyFetcher("valid", blocks, 0, shardId)

	attack, _ := makeChain(targetBlocks, 0, unknownBlock)
	attackerHeaderFetcher := tester.makeHeaderFetcher("attacker", nil, -gatherSlack)
	attackerBodyFetcher := tester.makeBodyFetcher("attacker", nil, 0, shardId)

	// Feed the tester a huge hashset from the attacker, and a limited from the valid peer
	for i := 0; i < len(attack); i++ {
		if i < maxQueueDist {
			tester.fetcher.Notify("valid", shardId, hashes[len(hashes)-2-i], uint64(i+1), time.Now(), validHeaderFetcher, validBodyFetcher)
		}
		tester.fetcher.Notify("attacker", shardId, attack[i], 1 /* don't distance drop */, time.Now(), attackerHeaderFetcher, attackerBodyFetcher)
	}
	//if count := atomic.LoadInt32(&announces); count < (hashLimit+maxQueueDist) {
	//	t.Fatalf("queued announce count mismatch: have %d, want %d", count, hashLimit+maxQueueDist)
	//}
	// Wait for fetches to complete
	verifyImportCount(t, imported, maxQueueDist)

	// Feed the remaining valid hashes to ensure DOS protection state remains clean
	for i := len(hashes) - maxQueueDist - 2; i >= 0; i-- {
		tester.fetcher.Notify("valid", shardId, hashes[i], uint64(len(hashes)-i-1), time.Now().Add(-arriveTimeout), validHeaderFetcher, validBodyFetcher)
		verifyImportEvent(t, imported, true)
	}
	verifyImportDone(t, imported)
}

// Tests that blocks sent to the fetcher (either through propagation or via hash
// announces and retrievals) don't pile up indefinitely, exhausting available
// system memory.
func TestBlockMemoryExhaustionAttack(t *testing.T) {
	// Create a tester with instrumented import hooks
	tester := newTester(types.ShardMaster)

	imported, enqueued := make(chan types.BlockIntf), int32(0)
	tester.fetcher.importedHook = func(block types.BlockIntf) { imported <- block }
	tester.fetcher.queueChangeHook = func(hash common.Hash, added bool) {
		if added {
			atomic.AddInt32(&enqueued, 1)
		} else {
			atomic.AddInt32(&enqueued, -1)
		}
	}
	// Create a valid chain and a batch of dangling (but in range) blocks
	targetBlocks := hashLimit + 2*maxQueueDist

	hashes, blocks := makeChain(targetBlocks, 0, genesis)
	attack := make(map[common.Hash]types.BlockIntf)
	for i := byte(0); len(attack) < blockLimit+2*maxQueueDist; i++ {
		hashes, blocks := makeChain(maxQueueDist-1, i, unknownBlock)
		for _, hash := range hashes[:maxQueueDist-2] {
			attack[hash] = blocks[hash]
		}
	}
	// Try to feed all the attacker blocks make sure only a limited batch is accepted
	for _, block := range attack {
		tester.fetcher.Enqueue("attacker", block)
	}
	time.Sleep(200 * time.Millisecond)
	if queued := atomic.LoadInt32(&enqueued); queued != blockLimit {
		t.Fatalf("queued block count mismatch: have %d, want %d", queued, blockLimit)
	}
	// Queue up a batch of valid blocks, and check that a new peer is allowed to do so
	for i := 0; i < maxQueueDist-1; i++ {
		tester.fetcher.Enqueue("valid", blocks[hashes[len(hashes)-3-i]])
	}
	time.Sleep(100 * time.Millisecond)
	if queued := atomic.LoadInt32(&enqueued); queued != blockLimit+maxQueueDist-1 {
		t.Fatalf("queued block count mismatch: have %d, want %d", queued, blockLimit+maxQueueDist-1)
	}
	// Insert the missing piece (and sanity check the import)
	tester.fetcher.Enqueue("valid", blocks[hashes[len(hashes)-2]])
	verifyImportCount(t, imported, maxQueueDist)

	// Insert the remaining blocks in chunks to ensure clean DOS protection
	for i := maxQueueDist; i < len(hashes)-1; i++ {
		tester.fetcher.Enqueue("valid", blocks[hashes[len(hashes)-2-i]])
		verifyImportEvent(t, imported, true)
	}
	verifyImportDone(t, imported)
}
