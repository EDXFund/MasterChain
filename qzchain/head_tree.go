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

package qzchain

import (
	"container/list"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/types"
	"math/big"
	"sync"
)

type Header interface {
	Hash() common.Hash
	Number() *big.Int
	ParentHash() common.Hash
	Difficulty() *big.Int
}

type HeaderTree struct {
	self     Header
	children *list.List
	//for quick search
	parent *HeaderTree
	wg     sync.RWMutex

}

func NewHeaderTree(root Header) *HeaderTree {
	return &HeaderTree{
		self:     root,
		children: list.New(),
	}
}

func (t *HeaderTree) Header() Header           { return t.self }
func (t *HeaderTree) Children() *list.List { return t.children }
func (t *HeaderTree) Parent() *HeaderTree      { return t.parent }
func (t *HeaderTree) FindHeader(node Header, compare func(n1, n2 Header) bool) *HeaderTree {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.findHeader(node,compare)
}
func (t *HeaderTree) findHeader(node Header, compare func(n1, n2 Header) bool) *HeaderTree {
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
func (t *HeaderTree) AddHeader(node Header) bool {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.addHeader(node)
}
//insert node , whose parent
func (t *HeaderTree) addHeader(node Header) bool {

	parent := t.findHeader(node, func(n1, n2 Header) bool {
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
			parent.children.PushBack(&HeaderTree{self: node, children: list.New(), parent: parent})
		}

		//make a HeaderTree
		return true
	} else {
		return false
	}
}
//cut all branch's except root by node ,return a HeaderTree with node as root
func (t *HeaderTree) ShrinkToBranch(node Header) *HeaderTree{
	t.wg.Lock()
	defer t.wg.Unlock()
	parent := t.findHeader(node, func(n1, n2 Header) bool {
		if n1.Hash() == n2.ParentHash() {
			return true
		}else {
			return false
		}
	})
	if parent != nil{
		var result *HeaderTree = nil
		for it:=parent.Children().Front(); it != nil; it = it.Next() {
			if it.Value.(*HeaderTree).self.Hash() == node.Hash() {
				result = it.Value.(*HeaderTree)
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
//found max td return (td, the longest tree node)
func (t *HeaderTree) getMaxTdPath() (uint64, *HeaderTree) {
	td := t.self.Difficulty().Uint64()
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
func (t *HeaderTree) GetMaxTdPath(node Header) *HeaderTree {
	t.wg.Lock()
	defer t.wg.Unlock()
	if node != nil {
		nodeTree := t.findHeader(node, func(n1, n2 Header) bool {
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
	parent := t.findHeader(newT.self, func(n1, n2 Header) bool {
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

func (t *HeaderTree) dfIterator(check func(node Header) bool) bool {
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
func (t *HeaderTree) wfIterator(check func(node Header) bool) bool {
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
func (t *HeaderTree) Iterator(deepFirst bool, proc func(node Header) bool) {
	t.wg.Lock()
	defer t.wg.Unlock()
	if deepFirst {
		t.dfIterator(proc)
	} else {
		t.wfIterator(proc)
	}
}
func (t *HeaderTree)RemoveByHash(hash common.Hash, deleteSelf bool){
	var nodeFound Header = nil;
	t.wfIterator(func(node Header) bool {
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
func (t *HeaderTree) Remove(node Header, removeHeader bool){
	t.wg.Lock()
	defer t.wg.Unlock()
	t.remove(node,removeHeader)
}
func (t *HeaderTree) remove(node Header, removeHeader bool) {
	var target *HeaderTree
	if node != nil {
		target = t.findHeader(node, func(n1, n2 Header) bool {
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
func (t *HeaderTree) GetCount() int{
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.getCount()
}
func (t *HeaderTree) getCount() int {
	count := 1;
	//if  {
		for it := t.children.Front(); it != nil; it = it.Next() {
			count += it.Value.(*HeaderTree).getCount()
		}
//	}
	return count
}

type  HeaderTreeManager struct{
	shardId uint16
	trees map[common.Hash]*HeaderTree
	rootHash common.Hash
	blockCh  chan types.HeaderIntf

}

func (t *HeaderTreeManager)Trees() map[common.Hash] *HeaderTree { return t.trees }
func (t *HeaderTreeManager)TreeOf(hash common.Hash) (*HeaderTree){ return t.trees[hash] }
func (t* HeaderTreeManager)SetRootHash(hash common.Hash) { t.rootHash = hash}
//add new BLock to tree, if more than 6 blocks has reached, a new block will popup to shard_pool
//if the node can not be add to  any existing tree, a new tree will be established
func (t *HeaderTreeManager)AddNewHead(node Header)  {

	var found *HeaderTree = nil
	for _, tree := range t.trees {
		if tree.AddHeader(node)  {
			found = tree
			break;
		}
	}
	if found == nil{
		found = NewHeaderTree(node)
		t.trees[node.Hash()] = found
	}
	//do possible tree merge
	for _,value := range t.trees {
		if value.Header().ParentHash() == node.Hash() {
			found.MergeTree(value)
			break;
		}
	}
}
// remove shard block with given hash, do nothing if the block does not exist
func (t *HeaderTreeManager)RemoveHead(node Header) {
	for _,value := range t.trees {
		value.Remove(node,true)
	}
}

func (t *HeaderTreeManager)RemoveHeadByHash(hash common.Hash) {
	for _,value := range t.trees {
		value.RemoveByHash(hash,true)
	}
}

func (t *HeaderTreeManager)GetPendingCount() int {
	count:=0
	for _,val := range t.trees {
		count += val.GetCount()
	}
	return count
}
//cut all node, on tree from node survived
func (t *HeaderTreeManager)ReduceTo(node Header) {
	invalidHashes := make([]common.Hash,1)
	var newTree *HeaderTree = nil
	for hash,val := range  t.trees {
		if val.self.Number().Uint64() < node.Number().Uint64() {
			invalidHashes = append(invalidHashes,hash)
			newHeader := val.ShrinkToBranch(node)
			if newHeader != nil {
				newTree = newHeader

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

