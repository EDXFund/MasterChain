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
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/log"
	"math"
	"reflect"
	"sync"

	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/event"
	"github.com/hashicorp/golang-lru"
)

const (
	sheaderCacheLimit = 5120
	stdCacheLimit     = 10240
	snumberCacheLimit = 20480
)

type PendingShard map[uint64]types.ShardBlockInfo
type ShardChainPool struct {
	bc     *core.BlockChain
	db     ethdb.Database
	shards map[uint16]*HeaderTreeManager

	shardFeed event.Feed //new valid shardblock has arrived
	shardCh   chan *core.ChainsShardEvent
	shardSub  event.Subscription

	masterBlockFeed event.Feed //new masterblock has arrived
	masterBlockCh   chan core.ChainHeadEvent
	masterBlockSub  event.Subscription

	quitCh           chan struct{}
	mu               sync.RWMutex
	shardHeaderInfos *lru.Cache
	shardHeaders     *lru.Cache
	shardBlocks      *lru.Cache
	pending          map[uint16]PendingShard

	currenMasterBlock types.BlockIntf

	newShardFeed event.Feed
	scope        event.SubscriptionScope
}

func NewShardChainPool(bc *core.BlockChain, database ethdb.Database) *ShardChainPool {

	pool := &ShardChainPool{
		bc:     bc,
		db:     database,
		shards: make(map[uint16]*HeaderTreeManager),
		//shardFeed :=
		shardCh:       make(chan *core.ChainsShardEvent),
		masterBlockCh: make(chan core.ChainHeadEvent),
		pending:       make(map[uint16]PendingShard),
		quitCh:        make(chan struct{}),
	}
	pool.shardHeaderInfos, _ = lru.New(sheaderCacheLimit)
	pool.shardHeaders, _ = lru.New(stdCacheLimit)
	pool.shardBlocks, _ = lru.New(snumberCacheLimit)
	pool.shardSub = bc.SubscribeChainShardsEvent(pool.shardCh)
	pool.masterBlockSub = bc.SubscribeChainHeadEvent(pool.masterBlockCh)

	//defer pool.headSub.Unsubscribe()

	go pool.loop()
	return pool
}
func (scp *ShardChainPool) Stop() {
	scp.shardSub.Unsubscribe()
	scp.masterBlockSub.Unsubscribe()
	close(scp.quitCh)
	scp.scope.Close()

}
func (scp *ShardChainPool) loop() {
	//waiting for shard blocks
	for {
		select {
		case headers := <-scp.shardCh: //this event only occurs on shard block
			scp.InsertChain(headers.Block)
		case masterBlock := <-scp.masterBlockCh: //block use chainHeadEvent feed to notify chain head info
			scp.reset(scp.currenMasterBlock, masterBlock.Block)
		case <-scp.quitCh:
			return
		}
	}

}
func (scp *ShardChainPool) InsertChain(blocks types.BlockIntfs) error {
	scp.mu.Lock()
	defer scp.mu.Unlock()
	if blocks[0].ShardId() == types.ShardMaster {

		log.Crit(" master block should not arrive here!")
		return core.ErrInvalidBlocks
	} else {
		toblocks := []*types.SBlock{}

		for _, block := range blocks {
			toblocks = append(toblocks, block.ToSBlock())
		}
		return scp.insertShardChain(toblocks)
	}

}
func (scp *ShardChainPool) SubscribeChainShardsEvent(newShardCh chan *core.ChainsShardEvent) event.Subscription {
	return scp.newShardFeed.Subscribe(newShardCh)
}
func (scp *ShardChainPool) GetBlock(hash common.Hash, number uint64) *types.SBlock {
	if block, ok := scp.shardBlocks.Get(hash); ok {
		result := (block).(*types.SBlock)
		return result
	}
	return rawdb.ReadBlock(scp.db, hash, number).ToSBlock()
}

/**
	insert shardchain update each shard chain tree
 */
func (scp *ShardChainPool) insertShardChain(blocks []*types.SBlock) error {

	shardId := blocks[0].ShardId()
	qchain, ok := scp.shards[shardId]
	if !ok {
		qchain = NewHeaderTreeManager(shardId)
		scp.shards[shardId] = qchain
	}
	header := []types.HeaderIntf{}
	for _, item := range blocks {
		scp.shardBlocks.Add(item.Hash(), item)
		header = append(header, item.Header())
	}

	newHeaders := qchain.AddNewHeads(header)
	log.Debug(" Add Headers:","header count:",len(header),"confirmed:", len(newHeaders))
	if len(newHeaders) > 0 {
		newBlocks := types.BlockIntfs{}
		for _, val := range newHeaders {
			block := scp.GetBlock(val.Hash(), val.NumberU64())
			if block != nil && !reflect.ValueOf(block).IsNil() {
				newBlocks = append(newBlocks, block)
			}
		}
		scp.newShardFeed.Send(&core.ChainsShardEvent{Block: newBlocks})

	}
	return nil;
}

//miner's worker uses Pending to retrieve shardblockInfos which could be packed into master block
func (scp *ShardChainPool) Pending() (map[uint16]PendingShard, error) {
	scp.mu.Lock()
	defer scp.mu.Unlock()
	results := make(map[uint16]PendingShard)
	for shardId, shards := range scp.shards {

		pendings := shards.Pending()
		log.Trace(" Shards Pending:","shardId:",shardId," len:",len(pendings))
		if len(pendings) > 0 {
			results[shardId] = make(PendingShard)
			for _, head := range pendings {
				results[shardId][head.NumberU64()] = types.ShardBlockInfo{shardId, head.NumberU64(), head.Hash(), head.ParentHash(), head.Coinbase(), head.Difficulty().Uint64()}

			}
		}
	}
	log.Trace("Pending:"," len:",len(results))
	return results, nil
}

/**
reset would be called on master block's incoming
 */
func (scp *ShardChainPool) reset(oldHead, newHead types.BlockIntf) {

	scp.mu.Lock()
	defer scp.mu.Unlock()
	shards := make(map[uint16]types.ShardBlockInfos)
	//find common anscentor
	if oldHead != nil && oldHead.Hash() != newHead.Hash() {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.NumberU64()
		newNum := newHead.NumberU64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {

			var (
				rem = scp.bc.GetBlock(oldHead.Hash(), oldHead.NumberU64())
				add = scp.bc.GetBlock(newHead.Hash(), newHead.NumberU64())
			)
			for rem.NumberU64() > add.NumberU64() {

				if rem = scp.bc.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil || reflect.ValueOf(rem).IsNil() {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				for _, shard := range add.ShardBlocks() {
					shardId := shard.ShardId
					shardToStore, ok := shards[shardId]
					if !ok {
						shardToStore = types.ShardBlockInfos{}
						shards[shardId] = shardToStore
					}
					//insert into the front
					shards[shardId] = append(types.ShardBlockInfos{shard}, shardToStore...)
				}
				if add = scp.bc.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil || reflect.ValueOf(add).IsNil() {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			for rem.Hash() != add.Hash() {

				if rem = scp.bc.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil || reflect.ValueOf(rem).IsNil() {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
				for _, shard := range add.ShardBlocks() {
					shardId := shard.ShardId
					shardToStore, ok := shards[shardId]
					if !ok {
						shardToStore = types.ShardBlockInfos{}
						shards[shardId] = shardToStore
					}
					//insert into the front
					shardToStore = append(types.ShardBlockInfos{shard}, shardToStore...)
				}
				if add = scp.bc.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil || reflect.ValueOf(add).IsNil() {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}

		}

	}
	//those shard block infos from ancestor to new should be reinject to Headertreemanager

	mostRecentBlock := make(map[uint16]types.BlockIntf)
	for shardId, shardInfos := range shards {
		blocks := []*types.SBlock{}
		for _, shardInfo := range shardInfos {
			block := scp.bc.GetBlock(shardInfo.Hash, shardInfo.BlockNumber)
			if block != nil && !reflect.ValueOf(block).IsNil() {
				blocks = append(blocks, block.ToSBlock())
				recent, ok := mostRecentBlock[shardId]
				if !ok || recent.NumberU64() < block.NumberU64() {
					mostRecentBlock[shardId] = block
				}
			}
		}
		scp.insertShardChain(blocks)
	}

	for shardId, mostRecent := range mostRecentBlock {
		qchain, ok := scp.shards[shardId]
		if !ok {
			log.Crit("qchain does not exist of:", shardId)
		} else {
			qchain.SetConfirmed(mostRecent.Header())
		}
	}
	scp.currenMasterBlock = newHead
}
