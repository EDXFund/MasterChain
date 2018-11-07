// Copyright 2016 The go-ethereum Authors
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

package light

import (
	"context"
	"math/big"
	"testing"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/consensus/ethash"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/params"
)

// So we can deterministically seed different blockchains
var (
	canonicalSeed = 1
	forkSeed      = 2
)

// makeHeaderChain creates a deterministic chain of headers rooted at parent.
func makeHeaderChain(parent types.HeaderIntf, n int, db ethdb.Database, seed int) []types.HeaderIntf {
	var blocks []types.BlockIntf
	if parent.ShardId() == types.ShardMaster{
		blocks, _ = core.GenerateChain(params.TestChainConfig, types.NewBlockWithHeader(parent), ethash.NewFaker(), db, n, func(i int, b *core.BlockGen) {
			b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
		})
	}else {
		blocks, _ = core.GenerateChain(params.TestChainConfig, types.NewSBlockWithHeader(parent), ethash.NewFaker(), db, n, func(i int, b *core.BlockGen) {
			b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
		})
	}

	headers := make([]types.HeaderIntf, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	return headers
}

// newCanonical creates a chain database, and injects a deterministic canonical
// chain. Depending on the full flag, if creates either a full block chain or a
// header only chain.
func newCanonical(n int,shardId uint16) (ethdb.Database, *LightChain, error) {
	db := ethdb.NewMemDatabase()
	gspec := core.Genesis{Config: params.TestChainConfig}
	genesis := gspec.MustCommit(db,shardId)
	blockchain, _ := NewLightChain(&dummyOdr{db: db, indexerConfig: TestClientIndexerConfig}, gspec.Config, ethash.NewFaker(),shardId)

	// Create and inject the requested chain
	if n == 0 {
		return db, blockchain, nil
	}
	// Header-only chain requested
	headers := makeHeaderChain(genesis.Header(), n, db, canonicalSeed)
	_, err := blockchain.InsertHeaderChain(headers, 1)
	return db, blockchain, err

}

// newTestLightChain creates a LightChain that doesn't validate anything.
func newTestLightChain(shardId uint16) *LightChain {
	db := ethdb.NewMemDatabase()
	gspec := &core.Genesis{
		Difficulty: big.NewInt(1),
		Config:     params.TestChainConfig,
	}
	gspec.MustCommit(db,shardId)
	lc, err := NewLightChain(&dummyOdr{db: db}, gspec.Config, ethash.NewFullFaker(),shardId)
	if err != nil {
		panic(err)
	}
	return lc
}

// Test fork of length N starting from block i
func testFork(t *testing.T, LightChain *LightChain, i, n int, shardId uint16,comparator func(td1, td2 *big.Int)) {
	// Copy old chain up to #i into a new db
	db, LightChain2, err := newCanonical(i,shardId)
	if err != nil {
		t.Fatal("could not make new canonical in testFork", err)
	}
	// Assert the chains have the same header/block at #i
	var hash1, hash2 common.Hash
	hash1 = LightChain.GetHeaderByNumber(uint64(i)).Hash()
	hash2 = LightChain2.GetHeaderByNumber(uint64(i)).Hash()
	if hash1 != hash2 {
		t.Errorf("chain content mismatch at %d: have hash %v, want hash %v", i, hash2, hash1)
	}
	// Extend the newly created chain
	headerChainB := makeHeaderChain(LightChain2.CurrentHeader(), n, db, forkSeed)
	if _, err := LightChain2.InsertHeaderChain(headerChainB, 1); err != nil {
		t.Fatalf("failed to insert forking chain: %v", err)
	}
	// Sanity check that the forked chain can be imported into the original
	var tdPre, tdPost *big.Int

	tdPre = LightChain.GetTdByHash(LightChain.CurrentHeader().Hash())
	if err := testHeaderChainImport(headerChainB, LightChain); err != nil {
		t.Fatalf("failed to import forked header chain: %v", err)
	}
	tdPost = LightChain.GetTdByHash(headerChainB[len(headerChainB)-1].Hash())
	// Compare the total difficulties of the chains
	comparator(tdPre, tdPost)
}

// testHeaderChainImport tries to process a chain of header, writing them into
// the database if successful.
func testHeaderChainImport(chain []types.HeaderIntf, lightchain *LightChain) error {
	for _, header := range chain {
		// Try and validate the header
		if err := lightchain.engine.VerifyHeader(lightchain.hc, header, true); err != nil {
			return err
		}
		// Manually insert the header into the database, but don't reorganize (allows subsequent testing)
		lightchain.mu.Lock()
		rawdb.WriteTd(lightchain.chainDb, header.Hash(), header.NumberU64(), new(big.Int).Add(header.Difficulty(), lightchain.GetTdByHash(header.ParentHash())))
		rawdb.WriteHeader(lightchain.chainDb, header)
		lightchain.mu.Unlock()
	}
	return nil
}
func TestExtendCanonicalHeadersMaster(t *testing.T) {  testExtendCanonicalHeaders(t,types.ShardMaster) }
func TestExtendCanonicalHeadersShard(t *testing.T) {  testExtendCanonicalHeaders(t,0) }
// Tests that given a starting canonical chain of a given size, it can be extended
// with various length chains.
func testExtendCanonicalHeaders(t *testing.T,shardId uint16) {
	length := 5

	// Make first chain starting from genesis
	_, processor, err := newCanonical(length,shardId)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	// Define the difficulty comparator
	better := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) <= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected more than %v", td2, td1)
		}
	}
	// Start fork from current height
	testFork(t, processor, length, 1, shardId,better)
	testFork(t, processor, length, 2, shardId,better)
	testFork(t, processor, length, 5, shardId,better)
	testFork(t, processor, length, 10, shardId,better)
}
func TestShorterForkHeadersMaster(t *testing.T) {
	 testShorterForkHeaders(t,types.ShardMaster)
}
func TestShorterForkHeadersShard(t *testing.T) {testShorterForkHeaders(t ,0)}

	// Tests that given a starting canonical chain of a given size, creating shorter
// forks do not take canonical ownership.
func testShorterForkHeaders(t *testing.T,shardId uint16) {
	length := 10

	// Make first chain starting from genesis
	_, processor, err := newCanonical(length,shardId)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	// Define the difficulty comparator
	worse := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) >= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected less than %v", td2, td1)
		}
	}
	// Sum of numbers must be less than `length` for this to be a shorter fork
	testFork(t, processor, 0, 3,shardId, worse)
	testFork(t, processor, 0, 7,shardId, worse)
	testFork(t, processor, 1, 1,shardId, worse)
	testFork(t, processor, 1, 7,shardId, worse)
	testFork(t, processor, 5, 3,shardId, worse)
	testFork(t, processor, 5, 4,shardId, worse)
}

func TestLongerForkHeadersMaster(t *testing.T) {testLongerForkHeaders(t ,types.ShardMaster)}
func TestLongerForkHeadersShard(t *testing.T){testLongerForkHeaders(t , 0)}

// Tests that given a starting canonical chain of a given size, creating longer
// forks do take canonical ownership.
func testLongerForkHeaders(t *testing.T,shard uint16) {
	length := 10

	// Make first chain starting from genesis
	_, processor, err := newCanonical(length,shard)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	// Define the difficulty comparator
	better := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) <= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected more than %v", td2, td1)
		}
	}
	// Sum of numbers must be greater than `length` for this to be a longer fork
	testFork(t, processor, 0, 11,shard, better)
	testFork(t, processor, 0, 15,shard, better)
	testFork(t, processor, 1, 10,shard, better)
	testFork(t, processor, 1, 12,shard, better)
	testFork(t, processor, 5, 6,shard, better)
	testFork(t, processor, 5, 8,shard, better)
}

func TestEqualForkHeadersMaster(t *testing.T) {
	testEqualForkHeaders(t,types.ShardMaster)
}

func TestEqualForkHeadersShard(t *testing.T) {

	testEqualForkHeaders(t, 0)
}


// Tests that given a starting canonical chain of a given size, creating equal
// forks do take canonical ownership.
func testEqualForkHeaders(t *testing.T,shardId uint16) {
	length := 10

	// Make first chain starting from genesis
	_, processor, err := newCanonical(length, shardId)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	// Define the difficulty comparator
	equal := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) != 0 {
			t.Errorf("total difficulty mismatch: have %v, want %v", td2, td1)
		}
	}
	// Sum of numbers must be equal to `length` for this to be an equal fork
	testFork(t, processor, 0, 10, shardId, equal)
	testFork(t, processor, 1, 9, shardId, equal)
	testFork(t, processor, 2, 8, shardId, equal)
	testFork(t, processor, 5, 5, shardId, equal)
	testFork(t, processor, 6, 4, shardId, equal)
	testFork(t, processor, 9, 1, shardId, equal)
}
func TestBrokenHeaderChainMaster(t *testing.T) {
	testBrokenHeaderChain(t,types.ShardMaster)
}
func TestBrokenHeaderChainShard(t *testing.T) {
	testBrokenHeaderChain(t,0)
}
// Tests that chains missing links do not get accepted by the processor.
func testBrokenHeaderChain(t *testing.T,shardId uint16) {
	// Make chain starting from genesis
	db, LightChain, err := newCanonical(10, shardId)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	// Create a forked chain, and try to insert with a missing link
	chain := makeHeaderChain(LightChain.CurrentHeader(), 5, db, forkSeed)[1:]
	if err := testHeaderChainImport(chain, LightChain); err == nil {
		t.Errorf("broken header chain not reported")
	}
}

func makeHeaderChainWithDiff(genesis types.BlockIntf, d []int, seed byte) []types.HeaderIntf {
	var chain []types.HeaderIntf
	for i, difficulty := range d {
		if genesis.ShardId() == types.ShardMaster {
			headerSt := &types.HeaderStruct{
				Coinbase:    common.Address{seed},
				Number:      big.NewInt(int64(i + 1)),
				Difficulty:  big.NewInt(int64(difficulty)),
				ShardTxsHash:      types.EmptyRootHash,
				ReceiptHash: types.EmptyRootHash,
			}
			if i == 0 {
				headerSt.ParentHash = genesis.Hash()
			} else {
				headerSt.ParentHash = chain[i-1].Hash()
			}
			header := &types.Header{}
			header.FillBy(headerSt)
			chain = append(chain, header)
		}else{
			headerSt := &types.SHeaderStruct{
				ShardId:genesis.ShardId(),
				Coinbase:    common.Address{seed},
				Number:      big.NewInt(int64(i + 1)),
				Difficulty:  big.NewInt(int64(difficulty)),
				TxHash:      types.EmptyRootHash,
				ReceiptHash: types.EmptyRootHash,
			}
			if i == 0 {
				headerSt.ParentHash = genesis.Hash()
			} else {
				headerSt.ParentHash = chain[i-1].Hash()
			}
			header := &types.SHeader{}
			header.FillBy(headerSt)
			chain = append(chain, header)
		}

	}
	return chain
}

type dummyOdr struct {
	OdrBackend
	db            ethdb.Database
	indexerConfig *IndexerConfig
}

func (odr *dummyOdr) Database() ethdb.Database {
	return odr.db
}

func (odr *dummyOdr) Retrieve(ctx context.Context, req OdrRequest) error {
	return nil
}

func (odr *dummyOdr) IndexerConfig() *IndexerConfig {
	return odr.indexerConfig
}
func TestReorgLongHeadersMaster(t *testing.T) {
	testReorgLongHeaders(t,types.ShardMaster)
}
func TestReorgLongHeadersShard(t *testing.T) {
	testReorgLongHeaders(t,0)
}
// Tests that reorganizing a long difficult chain after a short easy one
// overwrites the canonical numbers and links in the database.
func testReorgLongHeaders(t *testing.T,shardId uint16) {
	testReorg(t, []int{1, 2, 4}, []int{1, 2, 3, 4}, 10,shardId)
}


func TestReorgShortHeadersMaster(t *testing.T){
	testReorgShortHeaders(t,types.ShardMaster)
}
func TestReorgShortHeadersShard(t *testing.T){
	testReorgShortHeaders(t,0)
}
// Tests that reorganizing a short difficult chain after a long easy one
// overwrites the canonical numbers and links in the database.
func testReorgShortHeaders(t *testing.T,shardId uint16) {
	testReorg(t, []int{1, 2, 3, 4}, []int{1, 10}, 11,shardId)
}

func testReorg(t *testing.T, first, second []int, td int64,shardId uint16) {
	bc := newTestLightChain(shardId)

	// Insert an easy and a difficult chain afterwards
	bc.InsertHeaderChain(makeHeaderChainWithDiff(bc.genesisBlock, first, 11), 1)
	bc.InsertHeaderChain(makeHeaderChainWithDiff(bc.genesisBlock, second, 22), 1)
	// Check that the chain is valid number and link wise
	prev := bc.CurrentHeader()
	for header := bc.GetHeaderByNumber(bc.CurrentHeader().NumberU64() - 1); header.NumberU64() != 0; prev, header = header, bc.GetHeaderByNumber(header.NumberU64()-1) {
		if prev.ParentHash() != header.Hash() {
			t.Errorf("parent header hash mismatch: have %x, want %x", prev.ParentHash(), header.Hash())
		}
	}
	// Make sure the chain total difficulty is the correct one
	want := new(big.Int).Add(bc.genesisBlock.Difficulty(), big.NewInt(td))
	if have := bc.GetTdByHash(bc.CurrentHeader().Hash()); have.Cmp(want) != 0 {
		t.Errorf("total difficulty mismatch: have %v, want %v", have, want)
	}
}
func TestBadHeaderHashesMaster(t *testing.T) { testBadHeaderHashes(t,types.ShardMaster)}
func TestBadHeaderHashesShard(t *testing.T) { testBadHeaderHashes(t,0)}
// Tests that the insertion functions detect banned hashes.
func testBadHeaderHashes(t *testing.T,shardId uint16) {
	bc := newTestLightChain(shardId)

	// Create a chain, ban a hash and try to import
	var err error
	headers := makeHeaderChainWithDiff(bc.genesisBlock, []int{1, 2, 4}, 10)
	core.BadHashes[headers[2].Hash()] = true
	if _, err = bc.InsertHeaderChain(headers, 1); err != core.ErrBlacklistedHash {
		t.Errorf("error mismatch: have: %v, want %v", err, core.ErrBlacklistedHash)
	}
}
func TestReorgBadHeaderHashesMaster(t *testing.T) {
	testReorgBadHeaderHashes(t,types.ShardMaster)
}
func TestReorgBadHeaderHashesShard(t *testing.T) {
	testReorgBadHeaderHashes(t,0)
}
// Tests that bad hashes are detected on boot, and the chan rolled back to a
// good state prior to the bad hash.
func testReorgBadHeaderHashes(t *testing.T,shardId uint16) {
	bc := newTestLightChain(shardId)

	// Create a chain, import and ban afterwards
	headers := makeHeaderChainWithDiff(bc.genesisBlock, []int{1, 2, 3, 4}, 10)

	if _, err := bc.InsertHeaderChain(headers, 1); err != nil {
		t.Fatalf("failed to import headers: %v", err)
	}
	if bc.CurrentHeader().Hash() != headers[3].Hash() {
		t.Errorf("last header hash mismatch: have: %x, want %x", bc.CurrentHeader().Hash(), headers[3].Hash())
	}
	core.BadHashes[headers[3].Hash()] = true
	defer func() { delete(core.BadHashes, headers[3].Hash()) }()

	// Create a new LightChain and check that it rolled back the state.
	ncm, err := NewLightChain(&dummyOdr{db: bc.chainDb}, params.TestChainConfig, ethash.NewFaker(),shardId)
	if err != nil {
		t.Fatalf("failed to create new chain manager: %v", err)
	}
	if ncm.CurrentHeader().Hash() != headers[2].Hash() {
		t.Errorf("last header hash mismatch: have: %x, want %x", ncm.CurrentHeader().Hash(), headers[2].Hash())
	}
}
