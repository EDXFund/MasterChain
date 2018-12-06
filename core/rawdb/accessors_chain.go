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
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"reflect"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/rlp"
)

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadCanonicalHash(db DatabaseReader, shardId  uint16, number uint64) common.Hash {
	data, _ := db.Get(headerHashKey(shardId,number))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteCanonicalHash(db DatabaseWriter, shardId uint16,hash common.Hash, number uint64) {
	if err := db.Put(headerHashKey(shardId,number), hash.Bytes()); err != nil {
		log.Crit("Failed to store number to hash mapping", "err", err)
	}
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHash(db DatabaseDeleter, shardId uint16,number uint64) {
	if err := db.Delete(headerHashKey(shardId,number)); err != nil {
		log.Crit("Failed to delete number to hash mapping", "err", err)
	}
}

// ReadHeaderNumber returns the header number assigned to a hash.
func ReadHeaderNumber(db DatabaseReader, hash common.Hash) *uint64 {
	data, _ := db.Get(headerNumberKey(types.ShardMaster, hash))
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// ReadHeadHeaderHash retrieves the hash of the current canonical head header.
func ReadHeadHeaderHash(db DatabaseReader,shardId uint16) common.Hash {
	data, _ := db.Get(headHeaderKey(shardId))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadHeaderHash stores the hash of the current canonical head header.
func WriteHeadHeaderHash(db DatabaseWriter,shardId uint16, hash common.Hash) {
	if err := db.Put(headHeaderKey(shardId), hash.Bytes()); err != nil {
		log.Crit("Failed to store last header's hash", "err", err)
	}
}

// ReadHeadBlockHash retrieves the hash of the current canonical head block.
func ReadHeadBlockHash(db DatabaseReader,shardId uint16) common.Hash {
	data, _ := db.Get(headBlockKey(shardId))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHash(db DatabaseWriter, shardId uint16, hash common.Hash) {
	if err := db.Put(headBlockKey(shardId), hash.Bytes()); err != nil {
		log.Crit("Failed to store last block's hash", "err", err)
	}
}

// ReadHeadFastBlockHash retrieves the hash of the current fast-sync head block.
func ReadHeadFastBlockHash(db DatabaseReader,shardId uint16) common.Hash {
	data, _ := db.Get(headFastBlockKey(shardId))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadFastBlockHash stores the hash of the current fast-sync head block.
func WriteHeadFastBlockHash(db DatabaseWriter,shardId uint16, hash common.Hash) {
	if err := db.Put(headFastBlockKey(shardId), hash.Bytes()); err != nil {
		log.Crit("Failed to store last fast block's hash", "err", err)
	}
}

// ReadFastTrieProgress retrieves the number of tries nodes fast synced to allow
// reporting correct numbers across restarts.
func ReadFastTrieProgress(db DatabaseReader) uint64 {
	data, _ := db.Get(fastTrieProgressKey)
	if len(data) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(data).Uint64()
}

// WriteFastTrieProgress stores the fast sync trie process counter to support
// retrieving it across restarts.
func WriteFastTrieProgress(db DatabaseWriter, count uint64) {
	if err := db.Put(fastTrieProgressKey, new(big.Int).SetUint64(count).Bytes()); err != nil {
		log.Crit("Failed to store fast sync trie progress", "err", err)
	}
}

// ReadHeaderRLP retrieves a block header in its raw RLP database encoding.
func ReadHeaderRLP(db DatabaseReader, hash common.Hash, number uint64) rlp.RawValue {
	key:=headerKey(number, types.ShardMaster,hash)
	data, _ := db.Get(key)
	return data
}

// HasHeader verifies the existence of a block header corresponding to the hash.
func HasHeader(db DatabaseReader,  hash common.Hash, number uint64) bool {
	if has, err := db.Has(headerKey(number, types.ShardMaster, hash)); !has || err != nil {
		return false
	}
	return true
}


// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeader(db DatabaseReader, hash common.Hash, number uint64) types.HeaderIntf {
	data := ReadHeaderRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	header := new(types.HeadEncode)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "hash", hash, "err", err)
		return nil
	}
	if header.ShardId == types.ShardMaster {
		mheader := new (types.Header)
		headerstruct := new (types.HeaderStruct)
		if err := rlp.Decode(bytes.NewReader(header.Header), headerstruct); err != nil {
			log.Error("Invalid block header RLP", "hash", hash, "err", err)
			return nil
		}
		mheader.FillBy(headerstruct)
		return mheader
	} else {
		sheaderstruct := new (types.SHeaderStruct)
		sheader := new (types.SHeader)
		if err := rlp.Decode(bytes.NewReader(header.Header), sheaderstruct); err != nil {
			log.Error("Invalid block header RLP", "hash", hash, "err", err)
			return nil
		}
		sheader.FillBy(sheaderstruct)
		return sheader
	}

}

// WriteHeader stores a block header into the database and also stores the hash-
// to-number mapping.
func WriteHeader(db DatabaseWriter, header types.HeaderIntf) {
	// Write the hash -> number mapping
	var (
		hash    = header.Hash()
		number  = header.NumberU64()
		encoded = encodeBlockNumber(number)
	)
	key := headerNumberKey(types.ShardMaster,hash)
	if err := db.Put(key, encoded); err != nil {
		log.Crit("Failed to store hash to number mapping", "err", err)
	}
	// Write the encoded header
	shardId := header.ShardId();
	 toEncode := types.HeadEncode{ShardId:shardId}
	 if shardId == types.ShardMaster {

		 bytes,_ := rlp.EncodeToBytes(header.ToHeader().ToHeaderStruct())
		 toEncode.Header = bytes
	 }else {
		 bytes,_ := rlp.EncodeToBytes(*header.ToSHeader().ToStruct())
		 toEncode.Header = bytes
	 }

	data, err := rlp.EncodeToBytes(toEncode)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	key = headerKey(number, types.ShardMaster,hash)

	if err := db.Put(key, data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeader(db DatabaseDeleter, shardId uint16, hash common.Hash, number uint64) {
	if err := db.Delete(headerKey(number, shardId,hash)); err != nil {
		log.Crit("Failed to delete header", "err", err)
	}
	if err := db.Delete(headerNumberKey(shardId,hash)); err != nil {
		log.Crit("Failed to delete hash to number mapping", "err", err)
	}
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLP(db DatabaseReader,hash common.Hash, number uint64) rlp.RawValue {
	data, _ := db.Get(blockBodyKey(types.ShardMaster,number, hash))
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLP(db DatabaseWriter,hash common.Hash, number uint64, rlp rlp.RawValue) {
	if err := db.Put(blockBodyKey(types.ShardMaster,number,  hash), rlp); err != nil {
		log.Crit("Failed to store block body", "err", err)
	}
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBody(db DatabaseReader,hash common.Hash, number uint64) bool {
	if has, err := db.Has(blockBodyKey(types.ShardMaster,number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBody(db DatabaseReader, hash common.Hash, number uint64) *types.SuperBody {
	data := ReadBodyRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	body := new(types.BodyEncode)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}
	if body.ShardId == types.ShardMaster {
		bodyres := new (types.SuperBody)
		if err := rlp.Decode(bytes.NewReader(body.Body), bodyres); err != nil {
			log.Error("Invalid block body RLP", "hash", hash, "err", err)
			return nil
		}
		return bodyres;//&types.SuperBody{bodyres.Transactions,bodyres.Uncles,nil,nil}
	}else {
		bodyres := new (types.SuperBody)
		if err := rlp.Decode(bytes.NewReader(body.Body), bodyres); err != nil {
			log.Error("Invalid block body RLP", "hash", hash, "err", err)
			return nil
		}
		return bodyres;//&types.SuperBody{nil,nil,bodyres.Transactions,bodyres.Receipts}
	}

}

// WriteBody storea a block body into the database.
//add shardid into db, so we can judge which body should be used on reading
func WriteBody(db DatabaseWriter, hash common.Hash,shardId uint16, number uint64, body *types.SuperBody) {
	log.Trace("Write body:","hash:",hash,"data:",body)
	data, err := rlp.EncodeToBytes(body)

	if err != nil {
		log.Crit("Failed to RLP encode body", "err", err)
	} else {
		body2 := &types.BodyEncode{shardId,data}
		data2,err2 := rlp.EncodeToBytes(body2)
		if err2 != nil {
			log.Crit("Failed to RLP encode body", "err", err2)
		}else {
			WriteBodyRLP(db,hash, number, data2)
		}

	}

}

// DeleteBody removes all block body data associated with a hash.
func DeleteBody(db DatabaseDeleter,shardId uint16, hash common.Hash, number uint64) {
	if err := db.Delete(blockBodyKey(types.ShardMaster, number, hash)); err != nil {
		log.Crit("Failed to delete block body", "err", err)
	}
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTd(db DatabaseReader, shardId uint16,hash common.Hash, number uint64) *big.Int {
	data, _ := db.Get(headerTDKey(number, shardId,hash))
	if len(data) == 0 {
		return nil
	}
	td := new(big.Int)
	if err := rlp.Decode(bytes.NewReader(data), td); err != nil {
		log.Error("Invalid block total difficulty RLP", "hash", hash, "err", err)
		return nil
	}
	return td
}

// WriteTd stores the total difficulty of a block into the database.
func WriteTd(db DatabaseWriter,shardId uint16, hash common.Hash, number uint64, td *big.Int) {
	data, err := rlp.EncodeToBytes(td)
	if err != nil {
		log.Crit("Failed to RLP encode block total difficulty", "err", err)
	}
	if err := db.Put(headerTDKey(number,shardId, hash), data); err != nil {
		log.Crit("Failed to store block total difficulty", "err", err)
	}
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTd(db DatabaseDeleter, shardId uint16,hash common.Hash, number uint64) {
	if err := db.Delete(headerTDKey(number, shardId,hash)); err != nil {
		log.Crit("Failed to delete block total difficulty", "err", err)
	}
}

// ReadReceipts retrieves all the transaction receipts belonging to a block.
func ReadReceipts(db DatabaseReader, hash common.Hash, number uint64) types.Receipts {
	// Retrieve the flattened receipt slice
	data, _ := db.Get(blockReceiptsKey(types.ShardMaster, number, hash))
	if len(data) == 0 {
		return nil
	}
	// Convert the receipts from their storage form to their internal representation
	storageReceipts := []*types.ReceiptForStorage{}
	if err := rlp.DecodeBytes(data, &storageReceipts); err != nil {
		log.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}
	receipts := make(types.Receipts, len(storageReceipts))
	for i, receipt := range storageReceipts {
		receipts[i] = (*types.Receipt)(receipt)
	}
	return receipts
}

// WriteReceipts stores all the transaction receipts belonging to a block.
func WriteReceipts(db DatabaseWriter, shardId uint16,hash common.Hash, number uint64, receipts types.Receipts) {
	// Convert the receipts into their storage form and serialize them
	storageReceipts := make([]*types.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*types.ReceiptForStorage)(receipt)
	}
	bytes, err := rlp.EncodeToBytes(storageReceipts)
	if err != nil {
		log.Crit("Failed to encode block receipts", "err", err)
	}
	// Store the flattened receipt slice
	if err := db.Put(blockReceiptsKey(shardId, number, hash), bytes); err != nil {
		log.Crit("Failed to store block receipts", "err", err)
	}
}

// DeleteReceipts removes all receipt data associated with a block hash.
func DeleteReceipts(db DatabaseDeleter, shardId uint16,hash common.Hash, number uint64) {
	if err := db.Delete(blockReceiptsKey(shardId ,number, hash)); err != nil {
		log.Crit("Failed to delete block receipts", "err", err)
	}
}

// ReadBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body. If either the header or body could not
// be retrieved nil is returned.
//
// Note, due to concurrent download of header and block body the header and thus
// canonical hash can be stored in the database but the body data not (yet).
func ReadBlock(db DatabaseReader,hash common.Hash, number uint64) types.BlockIntf {
	if number == 0 {
		_number := ReadHeaderNumber(db,  hash)
		if _number != nil {
			number = *_number
		}
	}
	if number == 0 {
		return nil
	}
	header := ReadHeader(db, hash, number)

	if header == nil || reflect.ValueOf(header).IsNil() {
		return nil
	}
	if header.Hash() != hash {
		fmt.Println("error hash:")
	}
	body := ReadBody(db, hash, number)
	if body == nil {
		return nil
	}
	if header.ShardId() == types.ShardMaster  {
		return types.NewBlockWithHeader(header).WithBody(body.ShardBlocks,body.Receipts,body.Transactions, body.Results)
	}else{
		return types.NewSBlockWithHeader(header).WithBody(body.ShardBlocks,body.Receipts,body.Transactions, body.Results)
	}

}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlock(db DatabaseWriter, block types.BlockIntf) {

	WriteBody(db, block.Hash(),block.ShardId(), block.NumberU64(), block.Body())

	WriteHeader(db, block.Header())

}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlock(db DatabaseDeleter, shardId uint16,hash common.Hash, number uint64) {
	DeleteReceipts(db, shardId,hash, number)
	DeleteHeader(db, shardId,hash, number)
	DeleteBody(db,shardId, hash, number)
	DeleteTd(db, shardId,hash, number)
}

// FindCommonAncestor returns the last common ancestor of two block headers
func FindCommonAncestor(db DatabaseReader, a, b types.HeaderIntf) types.HeaderIntf {
	if a.ShardId() != b.ShardId() {
		return nil
	}
	for bn := b.NumberU64(); a.NumberU64() > bn; {
		a = ReadHeader(db, a.ParentHash(), a.NumberU64()-1)
		if a == nil  || reflect.ValueOf(a).IsNil() {
			return nil
		}
	}
	for an := a.NumberU64(); an < b.NumberU64(); {
		b = ReadHeader(db, b.ParentHash(), b.NumberU64()-1)
		if b == nil  || reflect.ValueOf(b).IsNil() {
			return nil
		}
	}
	for a.Hash() != b.Hash() {
		a = ReadHeader(db,a.ParentHash(), a.NumberU64()-1)
		if a == nil  || reflect.ValueOf(a).IsNil() {
			return nil
		}
		b = ReadHeader(db,b.ParentHash(), b.NumberU64()-1)
		if b == nil  || reflect.ValueOf(b).IsNil(){
			return nil
		}
	}
	return a
}

type ShardStoreInfo struct {
	confirmedHash common.Hash
	confirmedNumber uint64
	maxTdHash    common.Hash
	maxTdNumber     uint64
}
// WriteLatestShardInfo stores a block header into the database and also stores the hash-
// to-number mapping.
func WriteLatestShardInfo(db DatabaseWriter, shardConfirmed types.HeaderIntf, shardMax types.HeaderIntf) {
	// Write the hash -> number mapping
	info := &ShardStoreInfo{shardConfirmed.Hash(),shardConfirmed.NumberU64(),shardMax.Hash(),shardMax.NumberU64()}
	result,err := rlp.EncodeToBytes(info)
	if err != nil {
		log.Crit("write shard Info error ","error:",err)
	}else {
		db.Put(latestShardsKey(shardMax.ShardId()),result)
	}

}
// Get latest informations of specified shard
// returns: result.confirmedHash,result.confirmedNumber,result.maxTdHash,result.maxTdNumber
func ReadLatestShardInfo(db DatabaseReader,shardId uint16) (common.Hash,uint64,common.Hash,uint64){
	data,err := db.Get(latestShardsKey(shardId))
	if err != nil {
		log.Error("Read shard Info error ","error:",err)
		return common.Hash{},0,common.Hash{},0
	}else {
		result := ShardStoreInfo{}
		err = rlp.DecodeBytes(data,result)
		if err != nil {
			log.Crit("write shard Info error ","error:",err)
			return common.Hash{},0,common.Hash{},0
		}else {
			return result.confirmedHash,result.confirmedNumber,result.maxTdHash,result.maxTdNumber
		}
	}
}