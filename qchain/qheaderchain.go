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

package qchain

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"github.com/EDXFund/MasterChain/core"
	"math"
	"math/big"
	mrand "math/rand"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/consensus"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/params"

	"github.com/hashicorp/golang-lru"
)

const (
	headerCacheLimit = 512
	tdCacheLimit     = 1024
	numberCacheLimit = 2048
)

// QHeaderChain implements the basic block header chain logic that is shared by
// core.BlockChain and light.LightChain. It is not usable in itself, only as
// a part of either structure.
// It is not thread safe either, the encapsulating chain structures should do
// the necessary mutex locking/unlocking.
type QHeaderChain struct {
	config *params.ChainConfig
	shardId       uint16
	chainDb       ethdb.Database
	genesisHeader types.HeaderIntf

	currentHeader     atomic.Value // Current head of the header chain (may be above the block chain!)
	currentHeaderHash common.Hash  // Hash of the current head of the header chain (prevent recomputing all the time)

	headerCache *lru.Cache // Cache for the most recent block headers
	tdCache     *lru.Cache // Cache for the most recent block total difficulties
	numberCache *lru.Cache // Cache for the most recent block numbers

	procInterrupt func() bool

	headerManager *HeaderTreeManager
	//rootHash common.Hash		//current block
	blockCh     chan types.HeaderIntf
	quitCh      chan interface{}
	rand   *mrand.Rand
	engine consensus.Engine
}

// NewQHeaderChain creates a new QHeaderChain structure.
//  getValidator should return the parent's validator
//  procInterrupt points to the parent's interrupt semaphore
//  wg points to the parent's shutdown wait group
func NewQHeaderChain(chainDb ethdb.Database, config *params.ChainConfig, engine consensus.Engine, procInterrupt func() bool,shardId uint16) (*QHeaderChain, error) {
	headerCache, _ := lru.New(headerCacheLimit)
	tdCache, _ := lru.New(tdCacheLimit)
	numberCache, _ := lru.New(numberCacheLimit)

	// Seed a fast but crypto originating random generator
	seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	hc := &QHeaderChain{
		config:        config,
		shardId:	   shardId,
		chainDb:       chainDb,
		headerCache:   headerCache,
		tdCache:       tdCache,
		numberCache:   numberCache,
		procInterrupt: procInterrupt,
		rand:          mrand.New(mrand.NewSource(seed.Int64())),
		engine:        engine,

		blockCh:	   make(chan types.HeaderIntf),
		quitCh:		   make(chan interface{}),

	}

	hc.headerManager = NewHeaderTreeManager(shardId,hc.chainDb)

	hc.genesisHeader = hc.GetHeaderByNumber(0)
	if  hc.genesisHeader == nil  || reflect.ValueOf(hc.genesisHeader).IsNil() {
		return nil, core.ErrNoGenesis
	}

	hc.currentHeader.Store(hc.genesisHeader)
	if headHash := rawdb.ReadHeadBlockHash(chainDb, hc.shardId); headHash != (common.Hash{}) {
		if chead := hc.GetHeaderByHash(headHash); chead != nil {
			hc.currentHeader.Store(chead)
		}
	}
	hc.currentHeaderHash = hc.CurrentHeader().Hash()
	go     hc.loop()
	return hc, nil
}
func (hc *QHeaderChain) ShardId()  uint16 {return hc.shardId}

// GetBlockNumber retrieves the block number belonging to the given hash
// from the cache or database
func (hc *QHeaderChain) GetBlockNumber(hash common.Hash) *uint64 {
	if cached, ok := hc.numberCache.Get(hash); ok {
		number := cached.(uint64)
		return &number
	}
	number := rawdb.ReadHeaderNumber(hc.chainDb, hash)
	if number != nil {
		hc.numberCache.Add(hash, *number)
	}
	return number
}
// GetHeader retrieves a block header from the database by hash and number,
// caching it if found.
func (hc *QHeaderChain)SetCacheHeader(header types.HeaderIntf) {



	// Cache the found header for next time and return
	hc.headerCache.Add(header.Hash(), header)

}

func (hc *QHeaderChain) loop()  {
	for {
		select {
		case ahead := <-hc.blockCh:
			hc.WriteConfirmedHeader(ahead)
		case <-hc.quitCh:
				return
		}
	}
}
func (hc *QHeaderChain) WriteHeaderToDb(header types.HeaderIntf)   error{
	// Cache some values to prevent constant recalculation
	var (
		hash   = header.Hash()
		number = header.NumberU64()
	)
	// Calculate the total difficulty of the header
	ptd := hc.GetTd(header.ParentHash(), number-1)
	if ptd == nil {
		return  consensus.ErrUnknownAncestor
	}
	//localTd := hc.GetTd(hc.currentHeaderHash, hc.CurrentHeader().NumberU64())
	externTd := new(big.Int).Add(header.Difficulty(), ptd)

	// Irrelevant of the canonical status, write the td and header to the database
	if err := hc.WriteTd(hash, number, externTd); err != nil {
		log.Crit("Failed to write header total difficulty", "err", err)
	}
	rawdb.WriteHeader(hc.chainDb, header)
	hc.headerCache.Add(hash, header)
	hc.numberCache.Add(hash, number)
	return nil
}
// WriteHeader writes a header into the local chain, given that its parent is
// already known. If the total difficulty of the newly inserted header becomes
// greater than the current known TD, the canonical chain is re-routed.
//
// Note: This method is not concurrent-safe with inserting blocks simultaneously
// into the chain, as side effects caused by reorganisations cannot be emulated
// without the real blocks. Hence, writing headers directly should only be done
// in two scenarios: pure-header mode of operation (light clients), or properly
// separated header/block phases (non-archive clients).
func (hc *QHeaderChain) WriteConfirmedHeader(header types.HeaderIntf)   error{
	// Cache some values to prevent constant recalculation
	var (
		hash   = header.Hash()
		number = header.NumberU64()
	)
	// Calculate the total difficulty of the header
	ptd := hc.GetTd(header.ParentHash(), number-1)
	if ptd == nil {
		return  consensus.ErrUnknownAncestor
	}
	localTd := hc.GetTd(hc.currentHeaderHash, hc.CurrentHeader().NumberU64())
	externTd := new(big.Int).Add(header.Difficulty(), ptd)

	rawdb.WriteHeader(hc.chainDb, header)

	// If the total difficulty is higher than our known, add it to the canonical chain
	// Second clause in the if statement reduces the vulnerability to selfish mining.
	// Please refer to http://www.cs.cornell.edu/~ie53/publications/btcProcFC.pdf
	if externTd.Cmp(localTd) > 0  {
		// Delete any canonical number assignments above the new head
		batch := hc.chainDb.NewBatch()
		for i := number + 1; ; i++ {
			hash := rawdb.ReadCanonicalHash(hc.chainDb, hc.shardId, i)
			if hash == (common.Hash{}) {
				break
			}
			rawdb.DeleteCanonicalHash(batch, hc.shardId, i)
		}
		batch.Write()

		// Overwrite any stale canonical number assignments
		var (
			headHash= header.ParentHash()
			headNumber= header.NumberU64() - 1
			headHeader= hc.GetHeader(headHash, headNumber)
		)
		for rawdb.ReadCanonicalHash(hc.chainDb, hc.shardId, headNumber) != headHash {
			rawdb.WriteCanonicalHash(hc.chainDb, hc.shardId, headHash, headNumber)

			headHash = headHeader.ParentHash()
			headNumber = headHeader.NumberU64() - 1
			headHeader = hc.GetHeader(headHash, headNumber)
		}
		// Extend the canonical chain with the new header
		rawdb.WriteCanonicalHash(hc.chainDb, hc.shardId, hash, number)
		rawdb.WriteHeadHeaderHash(hc.chainDb, hc.shardId, hash)

		hc.currentHeaderHash = hash
		hc.currentHeader.Store(types.CopyHeaderIntf(header))
	}

	return nil
}

// WhCallback is a callback function for inserting individual headers.
// A callback is used for two reasons: first, in a LightChain, status should be
// processed and light chain events sent, while in a BlockChain this is not
// necessary since chain events are sent after inserting blocks. Second, the
// header writes should be protected by the parent chain mutex individually.
type WhCallback func(types.HeaderIntf) error

func (hc *QHeaderChain) ValidateHeader(chain []types.HeaderIntf, checkFreq int) (int, error) {
	// Do a sanity check that the provided chain is actually ordered and linked
	for i := 1; i < len(chain); i++ {
		if chain[i].NumberU64() != chain[i-1].NumberU64()+1 || chain[i].ParentHash() != chain[i-1].Hash() {
			// Chain broke ancestry, log a message (programming error) and skip insertion
			log.Error("Non contiguous header insert", "number", chain[i].Number, "hash", chain[i].Hash(),
				"parent", chain[i].ParentHash, "prevnumber", chain[i-1].Number, "prevhash", chain[i-1].Hash())
	        chaini_1_hash := chain[i-1].Hash().Bytes()
	        chaini_hash := chain[i].Hash().Bytes()
	        chaini_p_hash := chain[i].ParentHash()
			return 0, fmt.Errorf("non contiguous insert: item %d is #%d [%x…], item %d is #%d [%x…] (parent [%x…])", i-1, chain[i-1].Number(),
				chaini_1_hash[:4], i, chain[i].Number(), chaini_hash[:4], chaini_p_hash[:4])
		}
	}

	// Generate the list of seal verification requests, and start the parallel verifier
	seals := make([]bool, len(chain))
	for i := 0; i < len(seals)/checkFreq; i++ {
		index := i*checkFreq + hc.rand.Intn(checkFreq)
		if index >= len(seals) {
			index = len(seals) - 1
		}
		seals[index] = true
	}
	seals[len(seals)-1] = true // Last should always be verified to avoid junk

	abort, results := hc.engine.VerifyHeaders(hc, chain, seals)
	defer close(abort)

	// Iterate over the headers and ensure they all check out
	for i, header := range chain {
		// If the chain is terminating, stop processing blocks
		if hc.procInterrupt() {
			log.Debug("Premature abort during headers verification")
			return 0, errors.New("aborted")
		}
		// If the header is a banned one, straight out abort
		if core.BadHashes[header.Hash()] {
			return i, core.ErrBlacklistedHash
		}
		// Otherwise wait for headers checks and ensure they pass
		if err := <-results; err != nil {
			return i, err
		}
	}

	return 0, nil
}
func (hc *QHeaderChain) InsertHeaderChain(chain []types.HeaderIntf, start time.Time) ([]types.HeaderIntf, error) {
	//simple put headers into qztree
	for _,val := range chain {
		hc.WriteHeaderToDb(val)
	}
	results := hc.headerManager.AddNewHeads(chain)
	if results != nil {
		for i:= len(results)  ; i >0; i-- {
			hc.WriteConfirmedHeader(results[i-1])
			fmt.Println(" new node:",results[i-1].NumberU64() )
		}

	}

	hc.headerManager.ReduceTo(results[len(results)-1])
	return results,nil
}
// InsertQHeaderChain attempts to insert the given header chain in to the local
// chain, possibly creating a reorg. If an error is returned, it will return the
// index number of the failing header as well an error describing what went wrong.
//
// The verify parameter can be used to fine tune whether nonce verification
// should be done or not. The reason behind the optional check is because some
// of the header retrieval mechanisms already need to verfy nonces, as well as
// because nonces can be verified sparsely, not needing to check each.
func (hc *QHeaderChain) onValidChainBlocks(chain []types.HeaderIntf, writeHeader WhCallback, start time.Time) (int, error) {
	// Collect some import statistics to report on
	stats := struct{ processed, ignored int }{}
	// All headers passed verification, import them into the database
	for i, header := range chain {
		// Short circuit insertion if shutting down
		if hc.procInterrupt() {
			log.Debug("Premature abort during headers import")
			return i, errors.New("aborted")
		}
		// If the header's already known, skip it, otherwise store
		if hc.HasHeader(header.Hash(), header.NumberU64()) {
			stats.ignored++
			continue
		}
		if err := writeHeader(header); err != nil {
			return i, err
		}
		stats.processed++
	}
	// Report some public statistics so the user has a clue what's going on
	last := chain[len(chain)-1]

	context := []interface{}{
		"count", stats.processed, "elapsed", common.PrettyDuration(time.Since(start)),
		"number", last.Number, "hash", last.Hash(),
	}
	if timestamp := time.Unix(last.Time().Int64(), 0); time.Since(timestamp) > time.Minute {
		context = append(context, []interface{}{"age", common.PrettyAge(timestamp)}...)
	}
	if stats.ignored > 0 {
		context = append(context, []interface{}{"ignored", stats.ignored}...)
	}
	log.Info("Imported new block headers", context...)

	return 0, nil
}

// GetBlockHashesFromHash retrieves a number of block hashes starting at a given
// hash, fetching towards the genesis block.
func (hc *QHeaderChain) GetBlockHashesFromHash(hash common.Hash, max uint64) []common.Hash {
	// Get the origin header from which to fetch
	header := hc.GetHeaderByHash(hash)
	if header == nil  || reflect.ValueOf(header).IsNil() {
		return nil
	}
	// Iterate the headers until enough is collected or the genesis reached
	chain := make([]common.Hash, 0, max)
	for i := uint64(0); i < max; i++ {
		next := header.ParentHash()
		if header = hc.GetHeader(next, header.NumberU64()-1); header == nil  || reflect.ValueOf(header).IsNil() {
			break
		}
		chain = append(chain, next)
		if header.Number().Sign() == 0 {
			break
		}
	}
	return chain
}

// GetAncestor retrieves the Nth ancestor of a given block. It assumes that either the given block or
// a close ancestor of it is canonical. maxNonCanonical points to a downwards counter limiting the
// number of blocks to be individually checked before we reach the canonical chain.
//
// Note: ancestor == 0 returns the same block, 1 returns its parent and so on.
func (hc *QHeaderChain) GetAncestor(hash common.Hash, number, ancestor uint64, maxNonCanonical *uint64) (common.Hash, uint64) {
	if ancestor > number {
		return common.Hash{}, 0
	}
	if ancestor == 1 {
		// in this case it is cheaper to just read the header
		if header := hc.GetHeader(hash, number); header != nil {
			return header.ParentHash(), number - 1
		} else {
			return common.Hash{}, 0
		}
	}
	for ancestor != 0 {
		if rawdb.ReadCanonicalHash(hc.chainDb,hc.shardId,  number) == hash {
			number -= ancestor
			return rawdb.ReadCanonicalHash(hc.chainDb, hc.shardId, number), number
		}
		if *maxNonCanonical == 0 {
			return common.Hash{}, 0
		}
		*maxNonCanonical--
		ancestor--
		header := hc.GetHeader(hash, number)
		if header == nil  || reflect.ValueOf(header).IsNil() {
			return common.Hash{}, 0
		}
		hash = header.ParentHash()
		number--
	}
	return hash, number
}

// GetTd retrieves a block's total difficulty in the canonical chain from the
// database by hash and number, caching it if found.
func (hc *QHeaderChain) GetTd(hash common.Hash, number uint64) *big.Int {
	// Short circuit if the td's already in the cache, retrieve otherwise
	if cached, ok := hc.tdCache.Get(hash); ok {
		return cached.(*big.Int)
	}
	td := rawdb.ReadTd(hc.chainDb, hc.shardId,hash, number)
	if td == nil || reflect.ValueOf(td).IsNil() {
		return big.NewInt(0)
	}
	// Cache the found body for next time and return
	hc.tdCache.Add(hash, td)
	return td
}

// GetTdByHash retrieves a block's total difficulty in the canonical chain from the
// database by hash, caching it if found.
func (hc *QHeaderChain) GetTdByHash(hash common.Hash) *big.Int {
	number := hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	return hc.GetTd(hash, *number)
}

// WriteTd stores a block's total difficulty into the database, also caching it
// along the way.
func (hc *QHeaderChain) WriteTd(hash common.Hash, number uint64, td *big.Int) error {
	rawdb.WriteTd(hc.chainDb, hc.shardId,hash, number, td)
	hc.tdCache.Add(hash, new(big.Int).Set(td))
	return nil
}

// GetHeader retrieves a block header from the database by hash and number,
// caching it if found.
func (hc *QHeaderChain) GetHeader(hash common.Hash, number uint64) types.HeaderIntf {
	// Short circuit if the header's already in the cache, retrieve otherwise
	if header, ok := hc.headerCache.Get(hash); ok {
		return header.(types.HeaderIntf)
	}
	header := rawdb.ReadHeader(hc.chainDb,hash, number)
	if header == nil {
		return nil
	}


	// Cache the found header for next time and return
	hc.headerCache.Add(hash, header)
	return header
}

// GetHeaderByHash retrieves a block header from the database by hash, caching it if
// found.
func (hc *QHeaderChain) GetHeaderByHash(hash common.Hash) types.HeaderIntf {
	number := hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	return hc.GetHeader(hash, *number)
}

// HasHeader checks if a block header is present in the database or not.
func (hc *QHeaderChain) HasHeader(hash common.Hash, number uint64) bool {
	if hc.numberCache.Contains(hash) || hc.headerCache.Contains(hash) {
		return true
	}
	return rawdb.HasHeader(hc.chainDb,hash, number)
}

// GetHeaderByNumber retrieves a block header from the database by number,
// caching it (associated with its hash) if found.
func (hc *QHeaderChain) GetHeaderByNumber(number uint64) types.HeaderIntf {
	hash := rawdb.ReadCanonicalHash (hc.chainDb, hc.shardId,number)
	if hash == (common.Hash{}) {
		return nil
	}
	return hc.GetHeader(hash, number)
}

// CurrentHeader retrieves the current head header of the canonical chain. The
// header is retrieved from the QHeaderChain's internal cache.
func (hc *QHeaderChain) CurrentHeader() types.HeaderIntf {
	return hc.currentHeader.Load().(types.HeaderIntf)
}

// SetCurrentHeader sets the current head header of the canonical chain.
func (hc *QHeaderChain) SetCurrentHeader(head types.HeaderIntf) {
	rawdb.WriteHeadHeaderHash(hc.chainDb,hc.shardId, head.Hash())

	hc.currentHeader.Store(head)
	hc.currentHeaderHash = head.Hash()

	hc.headerManager.SetRootHash(head.Hash())
	hc.headerManager.AddNewHead(head)

}

// DeleteCallback is a callback function that is called by SetHead before
// each header is deleted.
type DeleteCallback func(rawdb.DatabaseDeleter, common.Hash, uint64)

// SetHead rewinds the local chain to a new head. Everything above the new head
// will be deleted and the new one set.
func (hc *QHeaderChain) SetHead(head uint64, delFn DeleteCallback) {
	height := uint64(0)

	if hdr := hc.CurrentHeader(); hdr != nil {
		height = hdr.NumberU64()
	}
	batch := hc.chainDb.NewBatch()
	for hdr := hc.CurrentHeader(); hdr != nil && hdr.NumberU64() > head; hdr = hc.CurrentHeader() {
		hash := hdr.Hash()
		num := hdr.NumberU64()
		if delFn != nil {
			delFn(batch, hash, num)
		}
		rawdb.DeleteHeader(batch, hc.shardId, hash,  num)
		rawdb.DeleteTd(batch, hc.shardId, hash,  num)

		hc.currentHeader.Store(hc.GetHeader(hdr.ParentHash(), hdr.NumberU64()-1))
	}
	// Roll back the canonical chain numbering
	for i := height; i > head; i-- {
		rawdb.DeleteCanonicalHash(batch, hc.shardId, i)
	}
	batch.Write()

	// Clear out any stale content from the caches
	hc.headerCache.Purge()
	hc.tdCache.Purge()
	hc.numberCache.Purge()
	hch := hc.CurrentHeader()
	if hch == nil  ||  reflect.ValueOf(hch).IsNil() {
		hc.currentHeader.Store(hc.genesisHeader)
	}
	hc.currentHeaderHash = hc.CurrentHeader().Hash()

	rawdb.WriteHeadHeaderHash(hc.chainDb,hc.shardId, hc.currentHeaderHash)

}

// SetGenesis sets a new genesis block header for the chain
func (hc *QHeaderChain) SetGenesis(head types.HeaderIntf) {
	hc.genesisHeader = head
}

// Config retrieves the header chain's chain configuration.
func (hc *QHeaderChain) Config() *params.ChainConfig { return hc.config }

// Engine retrieves the header chain's consensus engine.
func (hc *QHeaderChain) Engine() consensus.Engine { return hc.engine }

// GetBlock implements consensus.ChainReader, and returns nil for every input as
// a header chain does not have blocks available for retrieval.
func (hc *QHeaderChain) GetBlock(hash common.Hash, number uint64) types.BlockIntf {
	return nil
}
