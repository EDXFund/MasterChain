package core

import (
	"fmt"
	"testing"
)

type TestNode struct {
	hash        uint64
	blockNumber uint64
	parentHash  uint64
	td          uint64
}

func (ts *TestNode) Hash() uint64        { return ts.hash }
func (ts *TestNode) BlockNumber() uint64 { return ts.blockNumber }
func (ts *TestNode) ParentHash() uint64  { return ts.parentHash }
func (ts *TestNode) Td() uint64          { return ts.td }

const (
	NODE1 = iota
	NODE11
	NODE12
	NODE111
	NODE112
	NODE121
	NODE122
	NODE123
	NODE1111
	NODE1112
	NODE11121
	NODE1121
	NODE1122
	NODE1123
	NODE11221
	NODE11222
)

var nodes = []TestNode{
	TestNode{1234, 1, 0, 10},
	/*node11*/ TestNode{12345, 2, 1234, 20},
	/*node12*/ TestNode{12346, 2, 1234, 21},
	/*&node111*/ TestNode{123455, 3, 12345, 30},
	/*&node112*/ TestNode{123456, 3, 12345, 31},
	/*node121 =*/ TestNode{22345, 3, 12346, 32},
	/*node122 = */ TestNode{22346, 3, 12346, 33},
	/*node123 = */ TestNode{22347, 3, 12346, 33},
	/*node1111 = */ TestNode{11111234, 4, 123455, 45},
	/*node1112 = */ TestNode{11111235, 4, 123455, 47},
	/*node11121 = */ TestNode{111112356, 5, 11111235, 55},
	/*&node1121*/ TestNode{1234561, 4, 123456, 41},
	/*&node1122*/ TestNode{1234562, 4, 123456, 44},
	/*&node1123*/ TestNode{1234563, 4, 123456, 43},
	/*&node11221*/ TestNode{12345621, 5, 1234562, 53},
	/*&node11222*/ TestNode{12345622, 5, 1234562, 51},
}

func Test_Base_1(t *testing.T) {
	ts := NewQZTree(&nodes[NODE1])
	if ts.Node().Hash() == nodes[NODE1].Hash() {
		t.Log("create ok")
	}

	res := ts.AddNode(&nodes[NODE111])
	if !res {
		t.Log("Add invalid node")
	} else {
		t.Error("Add invalid node failed")
	}
	for i := NODE11; i <= NODE11121; i++ {
		ts.AddNode(&nodes[i])
	}

	nodefailed := false
	for i := 0; i <= NODE11121; i++ {
		if ts.FindNode(&nodes[i], func(n1, n2 Node) bool {
			return n1.Hash() == n2.Hash()
		}) == nil {
			nodefailed = true
			break
		}
	}

	if nodefailed {
		t.Error(" add node failed")
	} else {
		t.Log(" all nodes add ok")
	}
}
func Test_noduplication(t *testing.T) {
	ts := NewQZTree(&nodes[NODE1])
	for i := NODE1; i <= NODE11; i++ {
		ts.AddNode(&nodes[i])
	}
	for i := NODE11; i <= NODE11121; i++ {
		ts.AddNode(&nodes[i])
	}

	counts := map[uint64]uint64{}
	ts.Iterator(true, func(node Node) bool {
		fmt.Println(node.(*TestNode))
		hash := node.(*TestNode).Hash()
		val, ok := counts[hash]
		if !ok {
			counts[hash] = 1
		} else {
			counts[hash] = val + 1
		}

		return false
	})

	//check for all hash counts
	duplicated := false
	for _, val := range counts {
		if val != 1 {
			duplicated = true
		}
	}
	if duplicated {
		t.Error(" found duplicated!")
	} else {
		t.Log(" duplicated auto removed")
	}
}
func Test_MaxTd_1(t *testing.T) {

	ts := NewQZTree(&nodes[NODE1])

	for i := 0; i <= NODE11121; i++ {
		ts.AddNode(&nodes[i])
	}

	node := ts.GetMaxTdPath(nil)
	if node != nil && node.self.Hash() == 111112356 {
		t.Log(" Find Max TD PATH OK")
	} else {
		t.Error(" Max TD Path failed!")
	}
}

func Test_MaxTd_2(t *testing.T) {

	ts := NewQZTree(&nodes[NODE1])

	for i := 0; i <= NODE11222; i++ {

		ts.AddNode(&nodes[i])
	}

	node := ts.GetMaxTdPath(&nodes[NODE112])
	if node != nil && node.self.Hash() == nodes[NODE11221].Hash() {
		t.Log(" Find Special Max TD PATH OK")
	} else {
		t.Error(" Max Special TD Path failed!")
	}
}

func Test_SeperateNode_1(t *testing.T) {

	ts := NewQZTree(&nodes[NODE1])

	for i := 0; i <= NODE11222; i++ {
		if i != NODE112 {
			ts.AddNode(&nodes[i])
		}

	}

	if ts.FindNode(&nodes[NODE1121], func(n1, n2 Node) bool {
		return n1.Hash() == n2.Hash()
	}) == nil {
		t.Log(" seperated node test ok")
	} else {
		t.Error(" seperated node test  failed!")
	}
}

func Test_MergeNode_1(t *testing.T) {

	ts := NewQZTree(&nodes[NODE1])

	for i := 0; i <= NODE11222; i++ {
		if i != NODE112 {
			ts.AddNode(&nodes[i])
		}

	}
	ts2 := NewQZTree(&nodes[NODE112])
	for i := 0; i <= NODE11222; i++ {

		ts2.AddNode(&nodes[i])

	}

	ts.Iterator(true, func(node Node) bool {
		fmt.Println(node.(*TestNode))
		return false
	})

	fmt.Println("---------")
	ts2.Iterator(false, func(node Node) bool {
		fmt.Println(node.(*TestNode))
		return false
	})

	ts.MergeTree(ts2)

	ts.Iterator(true, func(node Node) bool {
		fmt.Println(node.(*TestNode))
		return false
	})

	fmt.Println("---------")
	if ts.FindNode(&nodes[NODE1121], func(n1, n2 Node) bool {
		return n1.Hash() == n2.Hash()
	}) != nil {
		t.Log(" seperated node test ok")
	} else {
		t.Error(" seperated node test  failed!")
	}
}

func Test_ClearAll_1(t *testing.T) {

	ts := NewQZTree(&nodes[NODE1])

	for i := 0; i <= NODE11222; i++ {
		ts.AddNode(&nodes[i])
	}

	ts.Remove(nil, false)

	if ts.children.Len() == 0 {
		t.Log(" Clear All nodes ok")
	} else {
		t.Error(" Clear all Node failed!")
	}
}

func Test_ClearByNode_1(t *testing.T) {

	ts := NewQZTree(&nodes[NODE1])

	for i := 0; i <= NODE11222; i++ {
		ts.AddNode(&nodes[i])
	}

	ts.Remove(&nodes[NODE11222], false)
	remain := ts.FindNode(&nodes[NODE11222], func(n1, n2 Node) bool {
		return n1.Hash() == n2.Hash()
	})
	if remain != nil && (remain.children.Len() == 0) {
		t.Log(" Clear Special nodes ok")
	} else {
		t.Error(" Clear Special Node failed!")
	}
}

func Test_ClearByNode_2(t *testing.T) {

	ts := NewQZTree(&nodes[NODE1])

	for i := 0; i <= NODE11222; i++ {
		ts.AddNode(&nodes[i])
	}

	ts.Remove(&nodes[NODE11222], true)
	remain := ts.FindNode(&nodes[NODE11222], func(n1, n2 Node) bool {
		return n1.Hash() == n2.Hash()
	})
	if remain == nil {
		t.Log(" Clear Special nodes ok")
	} else {
		t.Error(" Clear Special Node failed!")
	}
}
