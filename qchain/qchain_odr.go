package qchain

import (
	"context"
	"errors"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/trie"
)

type QOdr struct {
	OdrBackend
	indexerConfig *IndexerConfig
	sdb, ldb      ethdb.Database
	disable       bool
}

func (odr *QOdr) Database() ethdb.Database {
	return odr.ldb
}

var ErrOdrDisabled = errors.New("ODR disabled")

func (odr *QOdr) Retrieve(ctx context.Context, req OdrRequest) error {
	if odr.disable {
		return ErrOdrDisabled
	}
	switch req := req.(type) {
	case *BlockRequest:
		number := rawdb.ReadHeaderNumber(odr.sdb, req.ShardId, req.Hash)
		if number != nil {
			req.Rlp = rawdb.ReadBodyRLP(odr.sdb, req.ShardId, req.Hash, *number)
		}
	case *ReceiptsRequest:
		number := rawdb.ReadHeaderNumber(odr.sdb, types.ShardMaster,req.Hash)
		if number != nil {
			req.Receipts = rawdb.ReadReceipts(odr.sdb, types.ShardMaster,req.Hash, *number)
		}
	case *TrieRequest:
		t, _ := trie.New(req.Id.Root, trie.NewDatabase(odr.sdb))
		nodes := NewNodeSet()
		t.Prove(req.Key, 0, nodes)
		req.Proof = nodes
	case *CodeRequest:
		req.Data, _ = odr.sdb.Get(req.Hash[:])
	}
	req.StoreResult(odr.ldb)
	return nil
}

func (odr *QOdr) IndexerConfig() *IndexerConfig {
	return odr.indexerConfig
}