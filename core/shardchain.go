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

package core

import (
	"container/list"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/event"
	"math/big"
	"sync"
)

type Node interface {
	Hash() common.Hash
	Number() *big.Int
	ParentHash() common.Hash
	Difficulty() *big.Int
}

type ShardChain struct {
	self     Node
	children *list.List
	//for quick search
	parent *ShardChain
	wg     sync.RWMutex

}

func NewShardChain(root Node) *ShardChain {
	return &ShardChain{
		self:     root,
		children: list.New(),
	}
}

func (t *ShardChain) Node() Node           { return t.self }
func (t *ShardChain) Children() *list.List { return t.children }
func (t *ShardChain) Parent() *ShardChain      { return t.parent }
func (t *ShardChain) FindNode(node Node, compare func(n1, n2 Node) bool) *ShardChain {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.findNode(node,compare)
}
func (t *ShardChain) findNode(node Node, compare func(n1, n2 Node) bool) *ShardChain {
	if compare(t.self, node) {
		return t
	} else {
		var res *ShardChain
		for i := t.children.Front(); i != nil; i = i.Next() {
			res = (i.Value).(*ShardChain).findNode(node, compare)
			if res != nil {
				break
			}
		}
		return res
	}
}
func (t *ShardChain) AddNode(node Node) bool {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.addNode(node)
}
//insert node , whose parent
func (t *ShardChain) addNode(node Node) bool {

	parent := t.findNode(node, func(n1, n2 Node) bool {
		return n1.Hash() == n2.ParentHash()
	})
	if parent != nil {

		exist := false
		for i := parent.children.Front(); i != nil; i = i.Next() {
			if i.Value.(*ShardChain).self.Hash() == node.Hash() {
				exist = true
				break
			}
		}
		if !exist {
			parent.children.PushBack(&ShardChain{self: node, children: list.New(), parent: parent})
		}

		//make a ShardChain
		return true
	} else {
		return false
	}
}
//cut all branch's except root by node ,return a ShardChain with node as root
func (t *ShardChain) ShrinkToBranch(node Node) *ShardChain{
	t.wg.Lock()
	defer t.wg.Unlock()
	parent := t.findNode(node, func(n1, n2 Node) bool {
		if n1.Hash() == n2.ParentHash() {
			return true
		}else {
			return false
		}
	})
	if parent != nil{
		var result *ShardChain = nil
		for it:=parent.Children().Front(); it != nil; it = it.Next() {
			if it.Value.(*ShardChain).self.Hash() == node.Hash() {
				result = it.Value.(*ShardChain)
				parent.Children().Remove(it)
				break;
			}
		}
		return result

	} else {
		if t.self.Hash() == node.Hash() {
			return t
		}else {
			return nil
		}
	}
}
func (t *ShardChain) getMaxTdPath() (uint64, *ShardChain) {
	td := t.self.Difficulty().Uint64()
	maxTd := uint64(0)
	var maxNode *ShardChain
	maxNode = nil
	for i := t.children.Front(); i != nil; i = i.Next() {
		curTd, node := i.Value.(*ShardChain).getMaxTdPath()
		//fmt.Println("node hash: %V, td: %V",node.self.Hash(),curTd)
		if curTd > maxTd {
			maxNode = node
			maxTd = curTd
		}
	}
	if maxNode != nil {
		return maxTd, maxNode
	} else {
		return td, t
	}
}

// find a max td path on node's branch, if node is nil find max of all
func (t *ShardChain) GetMaxTdPath(node Node) *ShardChain {
	t.wg.Lock()
	defer t.wg.Unlock()
	if node != nil {
		nodeTree := t.findNode(node, func(n1, n2 Node) bool {
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
func (t *ShardChain) MergeTree(newT *ShardChain) bool {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.mergeTree(newT)
}
func (t *ShardChain) mergeTree(newT *ShardChain) bool {
	parent := t.findNode(newT.self, func(n1, n2 Node) bool {
		return n1.Hash() == n2.ParentHash()
	})
	if parent != nil {
		//check for duplication
		duplication := false
		for i := t.children.Front(); i != nil; i = i.Next() {
			if i.Value.(*ShardChain).self.Hash() == newT.self.Hash() {
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

func (t *ShardChain) dfIterator(check func(node Node) bool) bool {
	if check(t.self) {
		return true
	} else {
		for i := t.children.Front(); i != nil; i = i.Next() {
			res := i.Value.(*ShardChain).dfIterator(check)
			if res {
				return true
			}
		}
		return false
	}
}
func (t *ShardChain) wfIterator(check func(node Node) bool) bool {
	if check(t.self) {
		return true
	} else {
		res := false
		for i := t.children.Front(); i != nil; i = i.Next() {
			if check(i.Value.(*ShardChain).self) {
				res = true
				break
			}

		}
		if !res {
			for i := t.children.Front(); i != nil; i = i.Next() {
				if i.Value.(*ShardChain).wfIterator(check) {
					res = true
					break
				}

			}
		}
		return res
	}
}

// using routine "proc" to iterate all node, it breaks when "proc" return true
func (t *ShardChain) Iterator(deepFirst bool, proc func(node Node) bool) {
	t.wg.Lock()
	defer t.wg.Unlock()
	if deepFirst {
		t.dfIterator(proc)
	} else {
		t.wfIterator(proc)
	}
}
func (t *ShardChain)RemoveByHash(hash common.Hash, deleteSelf bool){
	var nodeFound Node = nil;
	t.wfIterator(func(node Node) bool {
		if node.Hash() == hash {
			nodeFound = node
			return true
		} else {
			return false
		}
	})

	if nodeFound != nil {
		t.Remove(nodeFound,deleteSelf)
	}
}
func (t *ShardChain) Remove(node Node, removeNode bool){
	t.wg.Lock()
	defer t.wg.Unlock()
	t.remove(node,removeNode)
}
func (t *ShardChain) remove(node Node, removeNode bool) {
	var target *ShardChain
	if node != nil {
		target = t.findNode(node, func(n1, n2 Node) bool {
			return n1.Hash() == n2.Hash()
		})
	} else {
		target = t
	}
	if target != nil {

		for i := target.children.Front(); i != nil; i = i.Next() {
			i.Value.(*ShardChain).Remove(nil, false)
		}
		target.children.Init()
		if node != nil && removeNode {
			ls := target.parent.children
			for it := ls.Front(); it != nil; it = it.Next() {
				if it.Value.(*ShardChain).self.Hash() == node.Hash() {
					ls.Remove(it)
					break
				}
			}
		}
	}

}
func (t *ShardChain) GetCount() int{
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.getCount()
}
func (t *ShardChain) getCount() int {
	count := 1;
	//if  {
		for it := t.children.Front(); it != nil; it = it.Next() {
			count += it.Value.(*ShardChain).getCount()
		}
//	}
	return count
}

type  ShardChainManager struct{
	shardId uint16
	trees map[common.Hash]*ShardChain
	rootHash common.Hash

}
func (t *ShardChainManager)RootHash() common.Hash {return t.rootHash}
func (t *ShardChainManager)Trees() map[common.Hash] *ShardChain { return t.trees }
func (t *ShardChainManager)TreeOf(hash common.Hash) (*ShardChain){ return t.trees[hash] }
func (t* ShardChainManager)SetRootHash(hash common.Hash) { t.rootHash = hash}
//add new BLock to tree, if more than 6 blocks has reached, a new block will popup to shard_pool
//if the node can not be add to  any existing tree, a new tree will be established
func (t *ShardChainManager)AddNewBlock(node Node)  {

	var found *ShardChain = nil
	for _, tree := range t.trees {
		if tree.AddNode(node)  {
			found = tree
			break;
		}
	}
	if found == nil{
		found = NewShardChain(node)
		t.trees[node.Hash()] = found
	}
	//do possible tree merge
	for _,value := range t.trees {
		if value.Node().ParentHash() == node.Hash() {
			found.MergeTree(value)
			break;
		}
	}
}
// remove shard block with given hash, do nothing if the block does not exist
func (t *ShardChainManager)RemoveBlock(node Node) {
	for _,value := range t.trees {
		value.Remove(node,true)
	}
}

func (t *ShardChainManager)RemoveBlockByHash(hash common.Hash) {
	for _,value := range t.trees {
		value.RemoveByHash(hash,true)
	}
}

func (t *ShardChainManager)GetPendingCount() int {
	count:=0
	for _,val := range t.trees {
		count += val.GetCount()
	}
	return count
}
//cut all node, on tree from node survived
func (t *ShardChainManager)ReduceTo(node Node) {
	invalidHashes := make([]common.Hash,1)
	var newTree *ShardChain = nil
	for hash,val := range  t.trees {
		if val.self.Number().Uint64() < node.Number().Uint64() {
			invalidHashes = append(invalidHashes,hash)
			newNode := val.ShrinkToBranch(node)
			if newNode != nil {
				newTree = newNode

			}
		}
	}

	for _,val := range  invalidHashes{
		delete (t.trees,val)
	}
	if newTree != nil {
		t.trees[newTree.self.Hash()] = newTree
	}

}

type ShardChainPool  struct {
	Shards map[uint16]*ShardChain
	vaoidShardFeed      event.Feed 			//new valid shardblock has emerged
	scope          event.SubscriptionScope
	mu           sync.RWMutex
}

func NewShardChainPool() *ShardChainPool{

	pool := &ShardChainPool{
		Shards:make(map[uint16]*ShardChain),
		shardFeed :=
	}
	return pool
}

func (scp *ShardChainPool) loop() {
	//waiting for shard blocks
}
func (scp *ShardChainPool) InsertChain(headers []*types.SHeader) error {
	scp.mu.Lock()
	defer scp.mu.Unlock()
	return scp.insertChain(headers)
}
func (scp *ShardChainPool) insertChain(headers []*types.SHeader,blocks) error {

}