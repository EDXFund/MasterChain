package core

import (
	"container/list"
)

type Node interface {
	Hash() uint64
	BlockNumber() uint64
	ParentHash() uint64
	Td() uint64
}

type QZTree struct {
	self     Node
	children *list.List
	//for quick search
	parent *QZTree
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
	if compare(t.self, node) {
		return t
	} else {
		var res *QZTree
		for i := t.children.Front(); i != nil; i = i.Next() {
			res = (i.Value).(*QZTree).FindNode(node, compare)
			if res != nil {
				break
			}
		}
		return res
	}
}

//insert node , whose parent
func (t *QZTree) AddNode(node Node) bool {
	parent := t.FindNode(node, func(n1, n2 Node) bool {
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
	td := t.self.Td()
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

	if node != nil {
		nodeTree := t.FindNode(node, func(n1, n2 Node) bool {
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
	parent := t.FindNode(newT.self, func(n1, n2 Node) bool {
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
	if deepFirst {
		t.dfIterator(proc)
	} else {
		t.wfIterator(proc)
	}
}

func (t *QZTree) Remove(node Node, removeNode bool) {
	var target *QZTree
	if node != nil {
		target = t.FindNode(node, func(n1, n2 Node) bool {
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
