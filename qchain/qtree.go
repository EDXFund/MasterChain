// Copyright 2018 The EDX Authors
// This file is part of the EDX library.
//
// The edx library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The edx library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package qchain

import (
	"container/list"
	"fmt"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/log"
	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"math/big"
	"reflect"
	"sort"
	"sync"
)

type HeaderTree struct {
	self     types.HeaderIntf
	children *list.List
	//for quick search
	parent *HeaderTree
	wg     sync.RWMutex
	owner  *HeaderTreeManager
}

func NewHeaderTree(root types.HeaderIntf, owner *HeaderTreeManager) *HeaderTree {
	return &HeaderTree{
		self:     root,
		children: list.New(),
		owner:    owner,
	}
}

func (t *HeaderTree) Header() types.HeaderIntf { return t.self }
func (t *HeaderTree) Children() *list.List     { return t.children }
func (t *HeaderTree) Parent() *HeaderTree      { return t.parent }
func (t *HeaderTree) FindHeader(node types.HeaderIntf, compare func(n1, n2 types.HeaderIntf) bool) *HeaderTree {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.findHeader(node, compare)
}
func (t *HeaderTree) findHeader(node types.HeaderIntf, compare func(n1, n2 types.HeaderIntf) bool) *HeaderTree {
	if compare(t.self, node) {
		return t
	} else {
		var res *HeaderTree
		for i := t.children.Front(); i != nil; i = i.Next() {
			res = (i.Value).(*HeaderTree).findHeader(node, compare)
			if res != nil {
				break
			}
		}
		return res
	}
}
func (t *HeaderTree) AddHeader(node types.HeaderIntf) bool {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.addHeader(node)
}

//insert node , whose parent
func (t *HeaderTree) addHeader(node types.HeaderIntf) bool {

	parent := t.findHeader(node, func(n1, n2 types.HeaderIntf) bool {
		return n1.Hash() == n2.ParentHash()
	})
	if parent != nil {

		exist := false
		for i := parent.children.Front(); i != nil; i = i.Next() {
			if i.Value.(*HeaderTree).self.Hash() == node.Hash() {
				exist = true
				break
			}
		}
		if !exist {
			parent.children.PushBack(&HeaderTree{self: node, children: list.New(), parent: parent, owner: t.owner})
		}

		//make a HeaderTree
		return true
	} else {
		return false
	}
}

//cut all branch's except root by node ,return a HeaderTree with node as root
func (t *HeaderTree) ShrinkToBranch(node types.HeaderIntf) *HeaderTree {
	t.wg.Lock()
	defer t.wg.Unlock()
	parent := t.findHeader(node, func(n1, n2 types.HeaderIntf) bool {
		if n1.Hash() == n2.ParentHash() {
			return true
		} else {
			return false
		}
	})
	if parent != nil {
		var result *HeaderTree = nil
		for it := parent.Children().Front(); it != nil; it = it.Next() {
			if it.Value.(*HeaderTree).self.Hash() == node.Hash() {
				result = it.Value.(*HeaderTree)
				parent.Children().Remove(it)
				break
			}
		}
		return result

	} else {
		if t.self.Hash() == node.Hash() {
			return t
		} else {
			return nil
		}
	}
}

//found max td return (td, the longest tree node)
func (t *HeaderTree) getMaxTdPath() (uint64, *HeaderTree) {
	td := t.owner.GetTd(t.self)
	fmt.Println("td test:", "shardId:", t.self.ShardId(), " blockNo:", t.self.NumberU64(), " td:", td)
	maxTd := uint64(0)
	var maxHeader *HeaderTree
	maxHeader = nil
	for i := t.children.Front(); i != nil; i = i.Next() {
		curTd, node := i.Value.(*HeaderTree).getMaxTdPath()
		//fmt.Println("node hash: %V, td: %V",node.self.Hash(),curTd)
		if curTd > maxTd {
			maxHeader = node
			maxTd = curTd
		}
	}
	if maxHeader != nil {
		return maxTd, maxHeader
	} else {
		return td, t
	}
}

// find a max td path on node's branch, if node is nil find max of all
func (t *HeaderTree) GetMaxTdPath(node types.HeaderIntf) *HeaderTree {
	t.wg.Lock()
	defer t.wg.Unlock()
	if node != nil {
		nodeTree := t.findHeader(node, func(n1, n2 types.HeaderIntf) bool {
			return n1.Hash() == n2.Hash()
		})
		if nodeTree != nil {
			_, node := nodeTree.getMaxTdPath()
			return node
		} else {
			return nil
		}

	} else {
		_, node := t.getMaxTdPath()
		return node
	}

}
func (t *HeaderTree) MergeTree(newT *HeaderTree) bool {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.mergeTree(newT)
}
func (t *HeaderTree) mergeTree(newT *HeaderTree) bool {
	parent := t.findHeader(newT.self, func(n1, n2 types.HeaderIntf) bool {
		return n1.Hash() == n2.ParentHash()
	})
	if parent != nil {
		//check for duplication
		duplication := false
		for i := t.children.Front(); i != nil; i = i.Next() {
			if i.Value.(*HeaderTree).self.Hash() == newT.self.Hash() {
				duplication = true
			}
		}
		if !duplication {
			parent.children.PushBack(newT)
		}

		return !duplication
	} else {
		return false
	}
}

func (t *HeaderTree) dfIterator(check func(node types.HeaderIntf) bool) bool {
	if check(t.self) {
		return true
	} else {
		for i := t.children.Front(); i != nil; i = i.Next() {
			res := i.Value.(*HeaderTree).dfIterator(check)
			if res {
				return true
			}
		}
		return false
	}
}
func (t *HeaderTree) wfIterator(check func(node types.HeaderIntf) bool) bool {
	if check(t.self) {
		return true
	} else {
		res := false
		for i := t.children.Front(); i != nil; i = i.Next() {
			if check(i.Value.(*HeaderTree).self) {
				res = true
				break
			}

		}
		if !res {
			for i := t.children.Front(); i != nil; i = i.Next() {
				if i.Value.(*HeaderTree).wfIterator(check) {
					res = true
					break
				}

			}
		}
		return res
	}
}

// using routine "proc" to iterate all node, it breaks when "proc" return true
func (t *HeaderTree) Iterator(deepFirst bool, proc func(node types.HeaderIntf) bool) {
	t.wg.Lock()
	defer t.wg.Unlock()
	if deepFirst {
		t.dfIterator(proc)
	} else {
		t.wfIterator(proc)
	}
}
func (t *HeaderTree) RemoveByHash(hash common.Hash, deleteSelf bool) {
	var nodeFound types.HeaderIntf = nil
	t.wfIterator(func(node types.HeaderIntf) bool {
		if node.Hash() == hash {
			nodeFound = node
			return true
		} else {
			return false
		}
	})

	if nodeFound != nil {
		t.Remove(nodeFound, deleteSelf)
	}
}
func (t *HeaderTree) Remove(node types.HeaderIntf, removeHeader bool) {
	t.wg.Lock()
	defer t.wg.Unlock()
	t.remove(node, removeHeader)
}
func (t *HeaderTree) remove(node types.HeaderIntf, removeHeader bool) {
	var target *HeaderTree
	if node != nil {
		target = t.findHeader(node, func(n1, n2 types.HeaderIntf) bool {
			return n1.Hash() == n2.Hash()
		})
	} else {
		target = t
	}
	if target != nil {

		for i := target.children.Front(); i != nil; i = i.Next() {
			i.Value.(*HeaderTree).Remove(nil, false)
		}
		target.children.Init()
		if node != nil && removeHeader {
			ls := target.parent.children
			for it := ls.Front(); it != nil; it = it.Next() {
				if it.Value.(*HeaderTree).self.Hash() == node.Hash() {
					ls.Remove(it)
					break
				}
			}
		}
	}

}
func (t *HeaderTree) GetCount() int {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.getCount()
}
func (t *HeaderTree) getCount() int {
	count := 1
	//if  {
	for it := t.children.Front(); it != nil; it = it.Next() {
		count += it.Value.(*HeaderTree).getCount()
	}
	//	}
	return count
}

type HeaderTreeManager struct {
	shardId   uint16
	trees     map[common.Hash]*HeaderTree
	rootHash  common.Hash
	maxTd     types.HeaderIntf
	confirmed []types.HeaderIntf

	confirmedHash common.Hash
	db            ethdb.Database
	tdCache       *lru.Cache
}

func NewHeaderTreeManager(shardId uint16, database ethdb.Database) *HeaderTreeManager {
	htm := &HeaderTreeManager{
		shardId:       shardId,
		trees:         make(map[common.Hash]*HeaderTree),
		rootHash:      common.Hash{},
		confirmed:     make([]types.HeaderIntf, 0),
		confirmedHash: common.Hash{},
		db:            database,
	}
	htm.tdCache, _ = lru.New(sheaderCacheLimit)
	confirmedHash, confirmedNumber, maxHash, maxNumber := rawdb.ReadLatestShardInfo(database, shardId)
	headerToInsert := []types.HeaderIntf{}
	if maxNumber != 0 {

		hash := maxHash
		number := maxNumber
		for (hash != common.Hash{} && number > 0) {
			headerInfo := rawdb.ReadHeader(database, hash, number)
			if headerInfo != nil {
				headerToInsert = append(headerToInsert, headerInfo)
				if hash == confirmedHash || number == confirmedNumber {
					break
				}
			} else {
				break
			}

		}
	}
	sort.Sort(SortHead(headerToInsert))

	htm.AddNewHeads(headerToInsert)
	return htm

}

func (t *HeaderTreeManager) Trees() map[common.Hash]*HeaderTree  { return t.trees }
func (t *HeaderTreeManager) TreeOf(hash common.Hash) *HeaderTree { return t.trees[hash] }
func (t *HeaderTreeManager) SetRootHash(hash common.Hash)        { t.rootHash = hash }
func (t *HeaderTreeManager) GetTd(header types.HeaderIntf) uint64 {
	td, ok := t.tdCache.Get(header.Hash())
	if ok {
		return td.(*big.Int).Uint64()
	}
	td = rawdb.ReadTd(t.db, t.shardId, header.Hash(), header.NumberU64())
	if td == nil || reflect.ValueOf(td).IsNil() {
		return 0
	} else {
		t.tdCache.Add(header.Hash(), td)
		return td.(*big.Int).Uint64()
	}
}

//add new BLock to tree, if more than 6 blocks has reached, a new block will popup to shard_pool
//if the node can not be add to  any existing tree, a new tree will be established
func (t *HeaderTreeManager) AddNewHeads(nodes []types.HeaderIntf) []types.HeaderIntf {
	info := make([]uint64, 0, len(nodes))
	for _, val := range nodes {
		info = append(info, val.NumberU64())
	}
	log.Trace("Add New Headers", "count:", len(nodes), "value:", info)
	for _, val := range nodes {
		t.AddNewHead(val)
	}

	t.confirmed = []types.HeaderIntf{}
	//寻找最长链
	if t.trees[t.rootHash] != nil {
		node := t.trees[t.rootHash].GetMaxTdPath(nil)
		t.maxTd = node.self
		//fmt.Println(" new node,", node.self.Number().Uint64() )
		if node.self.Number().Uint64()-t.trees[t.rootHash].self.Number().Uint64() > 5 {
			//	fmt.Println(" new node" )
			i := 0
			for node.self != t.trees[t.rootHash].self {
				node = node.parent
				i++
				if i > 5 {
					t.confirmed = append(t.confirmed, node.self)
				}
			}
		}
		if len(t.confirmed) > 0 {
			rawdb.WriteLatestShardInfo(t.db, t.confirmed[0], t.maxTd)
		} else {
			rawdb.WriteLatestShardInfo(t.db, t.trees[t.rootHash].self, t.maxTd)
		}
	}

	sort.Sort(SortHead(t.confirmed))

	return t.confirmed
}

func (t *HeaderTreeManager) GetMaxTd() (*types.ShardBlockInfo, error) {
	if (t.rootHash != common.Hash{}) {
		head := t.maxTd
		if head != nil {
			return &types.ShardBlockInfo{head.ShardId(), head.NumberU64(), head.Hash(), head.ParentHash(), head.Coinbase(), t.GetTd(head)}, nil
		} else {
			return nil, errors.New("no max td node found")
		}
	} else {
		return nil, errors.New("no tree node found")
	}

}
func (t *HeaderTreeManager) AddNewHead(node types.HeaderIntf) {

	var found *HeaderTree = nil
	for _, tree := range t.trees {
		if tree.AddHeader(node) {
			found = tree
			break
		}
	}
	if (t.rootHash == common.Hash{}) {
		t.rootHash = node.Hash()
	}
	if found == nil {
		found = NewHeaderTree(node, t)
		t.trees[node.Hash()] = found
	}
	//do possible tree merge
	for _, value := range t.trees {
		if value.Header().ParentHash() == node.Hash() {
			found.MergeTree(value)
			break
		}
	}

}

// remove shard block with given hash, do nothing if the block does not exist
func (t *HeaderTreeManager) RemoveHead(node types.HeaderIntf) {
	for _, value := range t.trees {
		value.Remove(node, true)
	}
}

func (t *HeaderTreeManager) RemoveHeadByHash(hash common.Hash) {
	for _, value := range t.trees {
		value.RemoveByHash(hash, true)
	}
}

func (t *HeaderTreeManager) GetPendingCount() int {
	count := 0
	for _, val := range t.trees {
		count += val.GetCount()
	}
	return count
}

//cut all node, only tree from node survived
func (t *HeaderTreeManager) ReduceTo(node types.HeaderIntf) error {
	invalidHashes := make([]common.Hash, 0, 1)
	var newTree *HeaderTree = nil
	for hash, val := range t.trees {
		if val.self.Number().Uint64() < node.Number().Uint64() {
			invalidHashes = append(invalidHashes, hash)
			newHeader := val.ShrinkToBranch(node)
			if newHeader != nil {
				newTree = newHeader
			}
		}
	}

	for _, val := range invalidHashes {
		delete(t.trees, val)
	}
	if newTree != nil {
		t.trees[newTree.self.Hash()] = newTree
		t.rootHash = node.Hash()
		return nil
	} else {
		return core.ErrInvalidBlocks
	}

}

func (t *HeaderTreeManager) SetConfirmed(head types.HeaderIntf) []types.HeaderIntf {

	if t.rootHash == head.Hash() {
		return nil
	}
	val := head.NumberU64()

	reduceIndex := 0
	for index, item := range t.confirmed {
		if item.NumberU64() >= val {
			reduceIndex = index
			break
		}
	}
	//cut of prcessed
	t.confirmed = t.confirmed[reduceIndex:]
	t.ReduceTo(head)

	t.confirmedHash = head.Hash()
	//发送事件
	return t.Pending()

}

func (t *HeaderTreeManager) Pending() []types.HeaderIntf {
	if len(t.confirmed) > 0 {
		result := make([]types.HeaderIntf, 0, len(t.confirmed))
		for _, val := range t.confirmed {
			if val.Hash() != t.rootHash && val.Hash() != t.confirmedHash {
				result = append(result, val)
			}

		}
		return result
	} else {
		return nil
	}
}

type SortHead []types.HeaderIntf

func (a SortHead) Len() int           { return len(a) }
func (a SortHead) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortHead) Less(i, j int) bool { return a[i].NumberU64() < a[j].NumberU64() }
