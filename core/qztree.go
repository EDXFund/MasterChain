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
	"sync"
)

type Node interface {
	Hash() common.Hash
	Number() uint64
	ParentHash() common.Hash
	Difficulty() uint64
}

type QZTree struct {
	self     Node
	children *list.List
	//for quick search
	parent *QZTree
	wg     sync.RWMutex

}

func NewQZTree(root Node) *QZTree {
	return &QZTree{
		self:     root,
		children: list.New(),
	}
}

func (t *QZTree) Node() Node           { return t.self }
func (t *QZTree) Children() *list.List { return t.children }
func (t *QZTree) Parent() *QZTree      { return t.parent }
func (t *QZTree) FindNode(node Node, compare func(n1, n2 Node) bool) *QZTree {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.findNode(node,compare)
}
func (t *QZTree) findNode(node Node, compare func(n1, n2 Node) bool) *QZTree {
	if compare(t.self, node) {
		return t
	} else {
		var res *QZTree
		for i := t.children.Front(); i != nil; i = i.Next() {
			res = (i.Value).(*QZTree).findNode(node, compare)
			if res != nil {
				break
			}
		}
		return res
	}
}
func (t *QZTree) AddNode(node Node) bool {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.addNode(node)
}
//insert node , whose parent
func (t *QZTree) addNode(node Node) bool {

	parent := t.findNode(node, func(n1, n2 Node) bool {
		return n1.Hash() == n2.ParentHash()
	})
	if parent != nil {

		exist := false
		for i := parent.children.Front(); i != nil; i = i.Next() {
			if i.Value.(*QZTree).self.Hash() == node.Hash() {
				exist = true
				break
			}
		}
		if !exist {
			parent.children.PushBack(&QZTree{self: node, children: list.New(), parent: parent})
		}

		//make a qztree
		return true
	} else {
		return false
	}
}
func (t *QZTree) getMaxTdPath() (uint64, *QZTree) {
	td := t.self.Difficulty()
	maxTd := uint64(0)
	var maxNode *QZTree
	maxNode = nil
	for i := t.children.Front(); i != nil; i = i.Next() {
		curTd, node := i.Value.(*QZTree).getMaxTdPath()
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
func (t *QZTree) GetMaxTdPath(node Node) *QZTree {
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
func (t *QZTree) MergeTree(newT *QZTree) bool {
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.mergeTree(newT)
}
func (t *QZTree) mergeTree(newT *QZTree) bool {
	parent := t.findNode(newT.self, func(n1, n2 Node) bool {
		return n1.Hash() == n2.ParentHash()
	})
	if parent != nil {
		//check for duplication
		duplication := false
		for i := t.children.Front(); i != nil; i = i.Next() {
			if i.Value.(*QZTree).self.Hash() == newT.self.Hash() {
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

func (t *QZTree) dfIterator(check func(node Node) bool) bool {
	if check(t.self) {
		return true
	} else {
		for i := t.children.Front(); i != nil; i = i.Next() {
			res := i.Value.(*QZTree).dfIterator(check)
			if res {
				return true
			}
		}
		return false
	}
}
func (t *QZTree) wfIterator(check func(node Node) bool) bool {
	if check(t.self) {
		return true
	} else {
		res := false
		for i := t.children.Front(); i != nil; i = i.Next() {
			if check(i.Value.(*QZTree).self) {
				res = true
				break
			}

		}
		if !res {
			for i := t.children.Front(); i != nil; i = i.Next() {
				if i.Value.(*QZTree).wfIterator(check) {
					res = true
					break
				}

			}
		}
		return res
	}
}

// using routine "proc" to iterate all node, it breaks when "proc" return true
func (t *QZTree) Iterator(deepFirst bool, proc func(node Node) bool) {
	t.wg.Lock()
	defer t.wg.Unlock()
	if deepFirst {
		t.dfIterator(proc)
	} else {
		t.wfIterator(proc)
	}
}
func (t *QZTree) Remove(node Node, removeNode bool){
	t.wg.Lock()
	defer t.wg.Unlock()
	t.remove(node,removeNode)
}
func (t *QZTree) remove(node Node, removeNode bool) {
	var target *QZTree
	if node != nil {
		target = t.findNode(node, func(n1, n2 Node) bool {
			return n1.Hash() == n2.Hash()
		})
	} else {
		target = t
	}
	if target != nil {

		for i := target.children.Front(); i != nil; i = i.Next() {
			i.Value.(*QZTree).Remove(nil, false)
		}
		target.children.Init()
		if node != nil && removeNode {
			ls := target.parent.children
			for it := ls.Front(); it != nil; it = it.Next() {
				if it.Value.(*QZTree).self.Hash() == node.Hash() {
					ls.Remove(it)
					break
				}
			}
		}
	}

}
func (t *QZTree) GetCount() int{
	t.wg.Lock()
	defer t.wg.Unlock()
	return t.getCount()
}
func (t *QZTree) getCount() int {
	count := 1;
	//if  {
		for it := t.children.Front(); it != nil; it = it.Next() {
			count += it.Value.(*QZTree).getCount()
		}
//	}
	return count
}
type  QZTreeManager struct{
	shardId uint16
	trees map[common.Hash]*QZTree
	rootHash common.Hash

}
func (t *QZTreeManager)RootHash() common.Hash {return t.rootHash}
func (t *QZTreeManager)Trees() map[common.Hash] *QZTree { return t.trees }
func (t *QZTreeManager)TreeOf(hash common.Hash) (*QZTree){ return t.trees[hash] }
func (t* QZTreeManager)SetRootHash(hash common.Hash) { t.rootHash = hash}
//add new BLock to tree, if more than 6 blocks has reached, a new block will popup to shard_pool
//if the node can not be add to  any existing tree, a new tree will be established
func (t *QZTreeManager)AddNewBlock(node Node)  {

	var found *QZTree = nil
	for _, tree := range t.trees {
		if tree.AddNode(node)  {
			found = tree
			break;
		}
	}
	if found == nil{
		found = NewQZTree(node)
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
func (t *QZTreeManager)RemoveBlock(node Node) {
	for _,value := range t.trees {
		value.Remove(node,true)
	}
}

func (t *QZTreeManager)GetPendingCount() int {
	count:=0
	for _,val := range t.trees {
		count += val.GetCount()
	}
	return count
}