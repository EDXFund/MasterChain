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
	"fmt"
	"reflect"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/rlp"
)


// WriteShardBlockEntries stores a positional metadata for every shardBlock from
// a master block, enabling hash based transaction and receipt lookups.
func WriteShardBlockEntries(db DatabaseWriter, block types.BlockIntf) {
	if block != nil && !reflect.ValueOf(block).IsNil() {
		for i, shardBlock := range block.ShardBlocks() {
			entry := TxLookupEntry{
				ShardId:    block.ShardId(),
				BlockHash:  block.Hash(),
				BlockIndex: block.NumberU64(),
				Index:      uint64(i),
			}
			data, err := rlp.EncodeToBytes(entry)
			if err != nil {
				log.Crit("Failed to encode transaction lookup entry", "err", err)
			}
			if err := db.Put(txLookupKey(shardBlock.Hash), data); err != nil {
				log.Crit("Failed to store transaction lookup entry", "err", err)
			}
		}
	}

}

// DeleteTxLookupEntry removes all transaction data associated with a hash.
func DeleteShardBlockEntry(db DatabaseDeleter, hash common.Hash) {
	db.Delete(txLookupKey(hash))
}

// ReadTxLookupEntry retrieves the positional metadata associated with a transaction
// hash to allow retrieving the transaction or receipt by hash.
func ReadTxLookupEntry(db DatabaseReader, hash common.Hash) (uint16, common.Hash, uint64, uint64) {
	fmt.Println("read tx:",txLookupKey(hash))
	data, _ := db.Get(txLookupKey(hash))
	if len(data) == 0 {
		return 0, common.Hash{}, 0, 0
	}
	var entry TxLookupEntry
	if err := rlp.DecodeBytes(data, &entry); err != nil {
		log.Error("Invalid transaction lookup entry RLP", "hash", hash, "err", err)
		return 0, common.Hash{}, 0, 0
	}
	return entry.ShardId, entry.BlockHash, entry.BlockIndex, entry.Index
}

// WriteTxLookupEntries stores a positional metadata for every transaction from
// a block, enabling hash based transaction and receipt lookups.
func WriteTxLookupEntries(db DatabaseWriter, block types.BlockIntf) {

	if block != nil && !reflect.ValueOf(block).IsNil() {
		for i, tx := range block.Transactions() {
			entry := TxLookupEntry{
				ShardId:    block.ShardId(),
				BlockHash:  block.Header().Hash(),
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

}

// DeleteTxLookupEntry removes all transaction data associated with a hash.
func DeleteTxLookupEntry(db DatabaseDeleter, hash common.Hash) {
	db.Delete(txLookupKey(hash))
}
func GetTxOfAccountNonce(db DatabaseReader, account common.Address,nonce uint64) (common.Hash,bool){
	data, _ := db.Get(txAccountNonceKey(account,nonce))
	if len(data) == 0 {
		return  common.Hash{},false
	}else {
		var txHash  common.Hash
		rlp.DecodeBytes(data,txHash)
		return txHash,true
	}
}
func WriteTxOfAccountNonce(db DatabaseWriter, account common.Address,nonce uint64,hash common.Hash) error{
	err := db.Put(txAccountNonceKey(account,nonce),hash.Bytes())
	if err != nil {
		log.Crit("error in put txHash",err)
	}
	return err
}
func WriteTransactoin(db DatabaseWriter, hash common.Hash, tx *types.Transaction) error{
	data,err := rlp.EncodeToBytes(tx)
	if err != nil {
		log.Crit("error in encode transaction",err)
		return err
	}
	err = db.Put(txKey(hash),data)
	if err != nil {
		log.Crit("error in put transaction",err)
	}
	return err
}

// ReadTransaction retrieves a specific transaction from the database, along with
// its added positional metadata.
func ReadTransaction(db DatabaseReader, hash common.Hash) (*types.Transaction, common.Hash, uint64, uint64) {
	_,blockHash, blockNumber, txIndex:= ReadTxLookupEntry(db,hash)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}
	body := ReadBody(db, blockHash, blockNumber)
	if body == nil || len(body.Transactions) <= int(txIndex) {
		log.Error("Transaction referenced missing", "number", blockNumber, "hash", blockHash, "index", txIndex)
		return nil, common.Hash{}, 0, 0
	}
	return body.Transactions[txIndex], blockHash, blockNumber, txIndex
}

// ReadReceipt retrieves a specific transaction receipt from the database, along with
// its added positional metadata.
func ReadReceipt(db DatabaseReader, hash common.Hash) (*types.Receipt, common.Hash, uint64, uint64) {
	_ ,blockHash, blockNumber, receiptIndex := ReadTxLookupEntry(db, hash)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}
	receipts := ReadReceipts(db, blockHash, blockNumber)
	if len(receipts) <= int(receiptIndex) {
		log.Error("Receipt refereced missing", "number", blockNumber, "hash", blockHash, "index", receiptIndex)
		return nil, common.Hash{}, 0, 0
	}
	return receipts[receiptIndex], blockHash, blockNumber, receiptIndex
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
// ReadBloomBits retrieves the compressed bloom bit vector belonging to the given
// section and bit index from the.
func ReadRejectedBloomBits(db DatabaseReader, bit uint, section uint64, head common.Hash) ([]byte, error) {
	return db.Get(bloomBitsKey(bit, section, head))
}

// WriteBloomBits stores the compressed bloom bits vector belonging to the given
// section and bit index.
func WriteRejectedBloomBits(db DatabaseWriter, bit uint, section uint64, head common.Hash, bits []byte) {
	if err := db.Put(bloomBitsKey(bit, section, head), bits); err != nil {
		log.Crit("Failed to store bloom bits", "err", err)
	}
}
/*
// WriteTxLookupEntries stores a positional metadata for every transaction from
// a block, enabling hash based transaction and receipt lookups.
func ReadLatestEntry(db DatabaseReader, hash common.Hash) (map[uint16]*types.LastShardInfo,error){
	val,error := db.Get(latestShardKey(hash))
	result := make(map[uint16]*types.LastShardInfo)
	temp := make([]types.LastShardInfo,1)
	if error == nil {
		rlp.DecodeBytes(val,temp)
		for i:= len(temp)-1; i>0;i-- {
			result[temp[i-1].ShardId] = & temp[i-1]
		}
	}
	return result,error
}
func WriteLatestEntry(db DatabaseWriter, hash common.Hash,latestShardInfo map[uint16]*types.LastShardInfo) {
	//convert to an array, and then encode it
	data := make([]*types.LastShardInfo,len(latestShardInfo))
	index := 0
	for _,val := range latestShardInfo{
		data[index] = val
	}
	value,er1 := rlp.EncodeToBytes(data)
	if er1  != nil {
		log.Crit("Failed to encode ", "err", er1)
		return
	}
	if err :=  db.Put(latestShardKey(hash),value); err != nil {
		log.Crit("Failed to store  block latest shard entry", "err", err)
	}

}

func DeleteShardLatestEntry(db DatabaseDeleter,hash common.Hash) {
	if err := db.Delete(latestShardKey(hash)) ; err != nil {
		log.Crit("Failed to delete block latest shard entry", "err", err)
	}
}*/