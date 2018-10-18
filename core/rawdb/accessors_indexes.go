// Copyright 2018 The go-ethereum Authors
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

package rawdb

import (
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/rlp"
)

// ReadTxLookupEntry retrieves the positional metadata associated with a transaction
// hash to allow retrieving the transaction or receipt by hash.
<<<<<<< HEAD
func ReadTxLookupEntry(db DatabaseReader, hash common.Hash) (common.Hash, uint16, uint64, uint64) {
	data, _ := db.Get(txLookupKey(hash))
	if len(data) == 0 {
		return common.Hash{}, 0, 0, 0
=======
func ReadTxLookupEntry(db DatabaseReader, hash common.Hash) (uint16, common.Hash, uint64, uint64) {
	data, _ := db.Get(txLookupKey(hash))
	if len(data) == 0 {
		return 0, common.Hash{}, 0, 0
>>>>>>> 8bcab3d0bd32ec56f062e40ed3a814fa3e6d40e8
	}
	var entry TxLookupEntry
	if err := rlp.DecodeBytes(data, &entry); err != nil {
		log.Error("Invalid transaction lookup entry RLP", "hash", hash, "err", err)
<<<<<<< HEAD
		return common.Hash{}, 0, 0, 0
	}
	return entry.BlockHash, entry.ShardId, entry.BlockIndex, entry.Index
=======
		return 0, common.Hash{}, 0, 0
	}
	return entry.ShardId, entry.BlockHash, entry.BlockIndex, entry.Index
>>>>>>> 8bcab3d0bd32ec56f062e40ed3a814fa3e6d40e8
}

// WriteTxLookupEntries stores a positional metadata for every transaction from
// a block, enabling hash based transaction and receipt lookups.
func WriteTxLookupEntries(db DatabaseWriter, block *types.Block) {
	//// MUST TODO, check this function effect
	for i, tx := range block.Transactions() {
		entry := TxLookupEntry{
<<<<<<< HEAD
			ShardId:    block.ShardId(),
=======
			ShardId:    block.shardId,
>>>>>>> 8bcab3d0bd32ec56f062e40ed3a814fa3e6d40e8
			BlockHash:  block.Hash(),
			BlockIndex: block.NumberU64(),
			Index:      uint64(i),
		}
		data, err := rlp.EncodeToBytes(entry)
		if err != nil {
			log.Crit("Failed to encode transaction lookup entry", "err", err)
		}
		if err := db.Put(txLookupKey(tx.Hash()), data); err != nil {
			log.Crit("Failed to store transaction lookup entry", "err", err)
		}
	}
}

// DeleteTxLookupEntry removes all transaction data associated with a hash.
func DeleteTxLookupEntry(db DatabaseDeleter, hash common.Hash) {
	db.Delete(txLookupKey(hash))
}

// ReadTransaction retrieves a specific transaction from the database, along with
// its added positional metadata.
<<<<<<< HEAD
func ReadTransaction(db DatabaseReader, hash common.Hash) (*types.Transaction, common.Hash, uint16, uint64, uint64) {
	blockHash, shardId, blockNumber, txIndex := ReadTxLookupEntry(db, hash)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0, 0
	}
	body := ReadBody(db, blockHash, shardId, blockNumber)
	if body == nil || len(body.Transactions) <= int(txIndex) {
		log.Error("Transaction referenced missing", "number", blockNumber, "hash", blockHash, "index", txIndex)
		return nil, common.Hash{}, 0, 0, 0
	}
	return body.Transactions[txIndex], blockHash, shardId, blockNumber, txIndex
=======
func ReadTransaction(db DatabaseReader, hash common.Hash) (*types.Transaction, uint16, common.Hash, uint64, uint64) {
	shardId, blockHash, blockNumber, txIndex := ReadTxLookupEntry(db, hash)
	if blockHash == (common.Hash{}) {
		return nil, shardId, common.Hash{}, 0, 0
	}
	body := ReadBody(db, shardId, blockHash, blockNumber)
	if body == nil || len(body.Transactions) <= int(txIndex) {
		log.Error("Transaction referenced missing", "number", blockNumber, "hash", blockHash, "index", txIndex)
		return nil, shardId, common.Hash{}, 0, 0
	}
	return body.Transactions[txIndex], shardId, blockHash, blockNumber, txIndex
>>>>>>> 8bcab3d0bd32ec56f062e40ed3a814fa3e6d40e8
}

// ReadReceipt retrieves a specific transaction receipt from the database, along with
// its added positional metadata.
<<<<<<< HEAD
func ReadReceipt(db DatabaseReader, hash common.Hash) (*types.Receipt, common.Hash, uint16, uint64, uint64) {
	blockHash, shardId, blockNumber, receiptIndex := ReadTxLookupEntry(db, hash)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0, 0
	}
	receipts := ReadReceipts(db, blockHash, uint16(shardId), blockNumber)
	if len(receipts) <= int(receiptIndex) {
		log.Error("Receipt refereced missing", "number", blockNumber, "hash", blockHash, "index", receiptIndex)
		return nil, common.Hash{}, 0, 0, 0
	}
	return receipts[receiptIndex], blockHash, shardId, blockNumber, receiptIndex
=======
func ReadReceipt(db DatabaseReader, hash common.Hash) (*types.Receipt, uint16, common.Hash, uint64, uint64) {
	shardId, blockHash, blockNumber, receiptIndex := ReadTxLookupEntry(db, hash)
	if blockHash == (common.Hash{}) {
		return nil, 0, common.Hash{}, 0, 0
	}
	receipts := ReadReceipts(db, shardId, blockHash, blockNumber)
	if len(receipts) <= int(receiptIndex) {
		log.Error("Receipt refereced missing", "number", blockNumber, "hash", blockHash, "index", receiptIndex)
		return nil, 0, common.Hash{}, 0, 0
	}
	return receipts[receiptIndex], shardId, blockHash, blockNumber, receiptIndex
>>>>>>> 8bcab3d0bd32ec56f062e40ed3a814fa3e6d40e8
}

// ReadBloomBits retrieves the compressed bloom bit vector belonging to the given
// section and bit index from the.
func ReadBloomBits(db DatabaseReader, bit uint, section uint64, head common.Hash) ([]byte, error) {
	return db.Get(bloomBitsKey(bit, section, head))
}

// WriteBloomBits stores the compressed bloom bits vector belonging to the given
// section and bit index.
func WriteBloomBits(db DatabaseWriter, bit uint, section uint64, head common.Hash, bits []byte) {
	if err := db.Put(bloomBitsKey(bit, section, head), bits); err != nil {
		log.Crit("Failed to store bloom bits", "err", err)
	}
}
