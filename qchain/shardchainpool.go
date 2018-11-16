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
	"github.com/EDXFund/MasterChain/ethdb"
	"sync"

	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/event"
	"github.com/hashicorp/golang-lru"
)

const (
	sheaderCacheLimit = 5120
	stdCacheLimit     = 10240
	snumberCacheLimit = 20480
)
type  PendingShard map[uint64]types.ShardBlockInfo
type ShardChainPool  struct {
	bc                  *core.BlockChain
	db					ethdb.Database
	shards 				map[uint16]*HeaderTreeManager
	ShardFeed      		event.Feed 			//new valid shardblock has emerged
	scope          		event.SubscriptionScope
	headCh              chan  core.ChainsShardEvent
	headSub				 event.Subscription
	mu           		sync.RWMutex
	shardHeaderInfos  *lru.Cache
	shardHeaders      *lru.Cache
	shardBlocks       *lru.Cache
	pending             map[uint16]PendingShard
}

func NewShardChainPool(bc *core.BlockChain,database ethdb.Database) *ShardChainPool{

	pool := &ShardChainPool{
		bc:bc,
		db:database,
		shards:make(map[uint16]*HeaderTreeManager),
		//shardFeed :=
		headCh:make(chan   core.ChainsShardEvent),
		pending:make(map[uint16]PendingShard),
	}
	pool.shardHeaderInfos, _ = lru.New(sheaderCacheLimit)
	pool.shardHeaders, _ = lru.New(stdCacheLimit)
	pool.shardBlocks, _ = lru.New(snumberCacheLimit)
	pool.headSub = bc.SubscribeChainShardsEvent(pool.headCh)

	defer pool.headSub.Unsubscribe()

	return pool
}

func (scp *ShardChainPool) loop() {
	//waiting for shard blocks
	select {
	case headers := <- scp.headCh:
		scp.InsertChain(headers.Block)
		break;
	}
}
func (scp *ShardChainPool) InsertChain(blocks types.BlockIntfs) error {
	scp.mu.Lock()
	defer scp.mu.Unlock()
	if blocks[0].ShardId() == types.ShardMaster {
		toblocks := make([]*types.Block,len(blocks))

		for _,block := range blocks {
			toblocks = append(toblocks,block.ToBlock())
		}
		return scp.insertMasterChain(toblocks)
	}else {
		toblocks := make([]*types.SBlock,len(blocks))

		for _,block := range blocks {
			toblocks = append(toblocks,block.ToSBlock())
		}
		return scp.insertShardChain(toblocks)
	}

}

/**
   insert master chain, the corresponding shard blocks should have been synchronised already
 */
func (scp *ShardChainPool) insertMasterChain(blocks []*types.Block) error {
	//从blocks中找到每个shard的最新区块
	headers := make(map[uint16]*types.ShardBlockInfo,1)
	for _,body := range blocks {
		for _, shardInfo :=range body.ToBlock().ShardBlocks() {
			shardId := shardInfo.ShardId()
			info, ok := headers[shardId]
			if !ok || info.Number().Cmp(shardInfo.Number()) < 0 {
				headers[shardId] = shardInfo
			}
		}
	}

	results := make([]types.HeaderIntf,1)
	for shardId,shardInfo := range headers {
		qchain,ok := scp.shards[shardId]
		if !ok {
			return core.ErrInvalidBlocks
		} else {
			head := rawdb.ReadHeader(scp.db,shardInfo.Hash(),shardInfo.NumberU64())
			result := qchain.SetConfirmed(head)
			if len(result) > 0 {
				results = append(results,result...)
			}
		}
	}
	if len(results) > 0 {
		scp.ShardFeed.Send(results)
	}
	return nil
}
/**
	insert shardchain update each shard chain tree
 */
func (scp *ShardChainPool) insertShardChain(blocks []*types.SBlock) error {
	shardId := blocks[0].ShardId()
	 qchain,ok := scp.shards[shardId]
	 if !ok {
	 	qchain = NewHeaderTreeManager(shardId)
		scp.shards[shardId] = qchain
	 }
	 header := make([]types.HeaderIntf,len(blocks))
	 for _,item := range blocks {
	 	header = append(header,item.Header())
	 }
	 newBlocks := qchain.AddNewHeads(header)
	 if len(newBlocks) > 0 {
		scp.ShardFeed.Send(newBlocks)

	 }
	return nil;
}
//miner's worker uses Pending to retrieve shardblockInfos which could be packed into master block
func  (scp *ShardChainPool) Pending() (map[uint16]PendingShard,error){
	results:=make(map[uint16]PendingShard)
	for shardId,shards := range scp.shards {
		pendings := shards.Pending()
		if len(pendings)  > 0 {
			results[shardId] = make(PendingShard)
			for _,head := range pendings {
				sb := types.ShardBlockInfo{}
				sb.FillBy(&types.ShardBlockInfoStruct{shardId,head.NumberU64(),head.Hash(),head.ParentHash(),head.Difficulty().Uint64()})
				results[shardId][head.NumberU64()] = sb
				}
		}
	}
	return results,nil
}
/*
func (scp *ShardChainPool) reset(oldHead,newHead types.HeaderIntf){
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
				rem = scp.chain.GetBlock(oldHead.Hash(), oldHead.NumberU64())
				add = scp.chain.GetBlock(newHead.Hash(), newHead.NumberU64())
			)
			for rem.NumberU64() > add.NumberU64() {
				discarded = append(discarded, rem.ShardBlocks()...)
				if rem = scp.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil  || reflect.ValueOf(rem).IsNil() {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				included = append(included, add.ShardBlocks()...)
				if add = scp.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil  || reflect.ValueOf(add).IsNil(){
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			for rem.Hash() != add.Hash() {
				discarded = append(discarded, rem.ShardBlocks()...)
				if rem = scp.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil  || reflect.ValueOf(rem).IsNil(){
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
				included = append(included, add.ShardBlocks()...)
				if add = scp.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil  || reflect.ValueOf(add).IsNil() {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			reinject = types.ShardInfoDifference(discarded, included)
			for _,val := range discarded {

			}
		}
	}
	// Initialize the internal state to the current head
	if newHead == nil || reflect.ValueOf(newHead).IsNil() {
		newHead = scp.chain.CurrentBlock().Header() // Special case during testing
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
	senderCacher.recover(pool.signer, reinject)
	pool.addTxsLocked(reinject, false)

	// validate the pool of pending transactions, this will remove
	// any transactions that have been included in the block or
	// have been invalidated because of another transaction (e.g.
	// higher gas price)
	pool.demoteUnexecutables()

	// Update all accounts to the latest known pending nonce
	for addr, list := range pool.pending {
		txs := list.Flatten() // Heavy but will be cached and is needed by the miner anyway
		pool.pendingState.SetNonce(addr, txs[len(txs)-1].Nonce()+1)
	}
	// Check the queue and move transactions over to the pending if possible
	// or remove those that have become invalid
	pool.promoteExecutables(nil)
}
*/