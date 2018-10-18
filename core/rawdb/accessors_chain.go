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
	//"encoding/hex"
	"math/big"
	//"errors"

	"strconv"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/rlp"
)

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadCanonicalHash(db DatabaseReader, shardId uint16, number uint64) common.Hash {
	data, _ := db.Get(headerHashKey(shardId, number))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number of geive shardId.
func WriteCanonicalHash(db DatabaseWriter, hash common.Hash, shardId uint16, number uint64) {
	if err := db.Put(headerHashKey(shardId, number), hash.Bytes()); err != nil {
		log.Crit("Failed to store number to hash mapping", "err", err)
	}
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHash(db DatabaseDeleter, shardId uint16, number uint64) {
	if err := db.Delete(headerHashKey(shardId, number)); err != nil {
		log.Crit("Failed to delete number to hash mapping", "err", err)
	}
}

// ReadHeaderNumber returns the shardId and header number assigned to a hash.
func ReadHeaderNumber(db DatabaseReader, hash common.Hash) (uint16, *uint64) {
	data, _ := db.Get(headerNumberKey(hash))
	if len(data) != 10 {
		return 0xFFFF, nil
	}
	shardId := binary.BigEndian.Uint16(data[0:2])
	number := binary.BigEndian.Uint64(data[2:])
	return shardId, &number
}

// ReadHeadHeaderHash retrieves the shard's hash of the current canonical head header  with given shardId.
func ReadHeadHeaderHash(db DatabaseReader, shardId uint16) common.Hash {
	data, _ := db.Get(append(headHeaderKey, strconv.FormatInt((int64)(shardId), 16)...))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadHeaderHash stores shard's the hash of the current canonical head header.
func WriteHeadHeaderHash(db DatabaseWriter, hash common.Hash, shardId uint16) {
	if err := db.Put(append(headHeaderKey, strconv.FormatInt((int64)(shardId), 16)...), hash.Bytes()); err != nil {
		log.Crit("Failed to store last header's hash", "err", err)
	}
}

// With given shardId, ReadHeadBlockHash retrieves hash of the current canonical head block.
func ReadHeadBlockHash(db DatabaseReader, shardId uint16) common.Hash {
	data, _ := db.Get(append(headBlockKey, strconv.FormatInt((int64)(shardId), 16)...))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHash(db DatabaseWriter, hash common.Hash, shardId uint16) {
	if err := db.Put(append(headBlockKey, strconv.FormatInt((int64)(shardId), 16)...), hash.Bytes()); err != nil {
		log.Crit("Failed to store last block's hash", "err", err)
	}
}

// ReadHeadFastBlockHash retrieves the shard's hash of the current fast-sync head block.
func ReadHeadFastBlockHash(db DatabaseReader, shardId uint16) common.Hash {
	data, _ := db.Get(append(headFastBlockKey, strconv.FormatInt((int64)(shardId), 16)...))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadFastBlockHash stores the hash of the current fast-sync head block.
func WriteHeadFastBlockHash(db DatabaseWriter, hash common.Hash, shardId uint16) {
	if err := db.Put(append(headFastBlockKey, strconv.FormatInt((int64)(shardId), 16)...), hash.Bytes()); err != nil {
		log.Crit("Failed to store last fast block's hash", "err", err)
	}
}

// ReadFastTrieProgress retrieves the number of tries nodes fast synced to allow
// reporting correct numbers across restarts.
func ReadFastTrieProgress(db DatabaseReader, shardId uint16) uint64 {
	data, _ := db.Get(append(fastTrieProgressKey, strconv.FormatInt((int64)(shardId), 16)...))
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
func ReadHeaderRLP(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) rlp.RawValue {
	data, _ := db.Get(headerKey(shardId, number, hash))
	return data
}

// HasHeader verifies the existence of a block header corresponding to the hash.
<<<<<<< HEAD
func HasHeader(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) bool {
	if has, err := db.Has(headerKey(shardId, number, hash)); !has || err != nil {
=======
func HasHeader(db DatabaseReader, shardId uint16,hash common.Hash, number uint64) bool {
	if has, err := db.Has(headerKey(shardId,number, hash)); !has || err != nil {
>>>>>>> 8bcab3d0bd32ec56f062e40ed3a814fa3e6d40e8
		return false
	}
	return true
}
func WriteLastHeaders() {

}
func ReadLastHeaders() {

}

// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeader(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) *types.Header {
	data := ReadHeaderRLP(db, hash, shardId, number)
	if len(data) == 0 {
		return nil
	}
	header := new(types.Header)

	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "hash", hash, "err", err)
		return nil
	}
	return header
}

// WriteHeader stores a block header into the database and also stores the hash-
// to-number mapping.
func WriteHeader(db DatabaseWriter, header *types.Header) {
	// Write the hash -> number mapping
	var (
		shardId = header.ShardId
		hash    = header.Hash()
		number  = header.Number.Uint64()
		encoded = encodeBlockNumberWithShardId(shardId, number)
	)
	key := headerNumberKey(hash)
	if err := db.Put(key, encoded); err != nil {
		log.Crit("Failed to store hash to number mapping", "err", err)
	}
	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	key = headerKey(shardId, number, hash)
	if err := db.Put(key, data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
	//[shardId,number,hash] -> encoded(header)
	//hash --> [shardId,blockNumber]
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeader(db DatabaseDeleter, hash common.Hash, shardId uint16, number uint64) {
	if err := db.Delete(headerKey(shardId, number, hash)); err != nil {
		log.Crit("Failed to delete header", "err", err)
	}
	if err := db.Delete(headerNumberKey(hash)); err != nil {
		log.Crit("Failed to delete hash to number mapping", "err", err)
	}
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLP(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) rlp.RawValue {
	data, _ := db.Get(blockBodyKey(shardId, number, hash))
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLP(db DatabaseWriter, hash common.Hash, shardId uint16, number uint64, rlp rlp.RawValue) {
	if err := db.Put(blockBodyKey(shardId, number, hash), rlp); err != nil {
		log.Crit("Failed to store block body", "err", err)
	}
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBody(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) bool {
	if has, err := db.Has(blockBodyKey(shardId, number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBody(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) *types.Body {
	data := ReadBodyRLP(db, hash, shardId, number)
	if len(data) == 0 {
		return nil
	}
	body := new(types.Body)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}
	return body
}

// WriteBody storea a block body into the database.
//shard doesn't exist in body,so parameter of shardId should be specified.
func WriteBody(db DatabaseWriter, hash common.Hash, shardId uint16, number uint64, body *types.Body) {
	data, err := rlp.EncodeToBytes(body)
	if err != nil {
		log.Crit("Failed to RLP encode body", "err", err)
	}
	WriteBodyRLP(db, hash, shardId, number, data)
}

// DeleteBody removes all block body data associated with a hash.
func DeleteBody(db DatabaseDeleter, hash common.Hash, shardId uint16, number uint64) {
	if err := db.Delete(blockBodyKey(shardId, number, hash)); err != nil {
		log.Crit("Failed to delete block body", "err", err)
	}
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTd(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) *big.Int {
	data, _ := db.Get(headerTDKey(shardId, number, hash))
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
func WriteTd(db DatabaseWriter, hash common.Hash, shardId uint16, number uint64, td *big.Int) {
	data, err := rlp.EncodeToBytes(td)
	if err != nil {
		log.Crit("Failed to RLP encode block total difficulty", "err", err)
	}
	if err := db.Put(headerTDKey(shardId, number, hash), data); err != nil {
		log.Crit("Failed to store block total difficulty", "err", err)
	}
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTd(db DatabaseDeleter, hash common.Hash, shardId uint16, number uint64) {
	if err := db.Delete(headerTDKey(shardId, number, hash)); err != nil {
		log.Crit("Failed to delete block total difficulty", "err", err)
	}
}

// ReadReceipts retrieves all the transaction receipts belonging to a block.
func ReadReceipts(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) types.Receipts {
	// Retrieve the flattened receipt slice
	data, _ := db.Get(blockReceiptsKey(shardId, number, hash))
	if len(data) == 0 {
		return nil
	}
	// Convert the revceipts from their storage form to their internal representation
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
func WriteReceipts(db DatabaseWriter, hash common.Hash, shardId uint16, number uint64, receipts types.Receipts) {
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
func DeleteReceipts(db DatabaseDeleter, hash common.Hash, shardId uint16, number uint64) {
	if err := db.Delete(blockReceiptsKey(shardId, number, hash)); err != nil {
		log.Crit("Failed to delete block receipts", "err", err)
	}
}

// ReadBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body. If either the header or body could not
// be retrieved nil is returned.
//
// Note, due to concurrent download of header and block body the header and thus
// canonical hash can be stored in the database but the body data not (yet).
func ReadBlock(db DatabaseReader, hash common.Hash, shardId uint16, number uint64) *types.Block {
	header := ReadHeader(db, hash, shardId, number)
	if header == nil {
		return nil
	}
	body := ReadBody(db, hash, shardId, number)
	if body == nil {
		return nil
	}
	return types.NewBlockWithHeader(header).WithBody(body.Transactions, body.Receipts, body.BlockInfos, body.RejectInfos)
}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlock(db DatabaseWriter, block *types.Block) {
	WriteBody(db, block.Hash(), block.Header().ShardId, block.NumberU64(), block.Body())
	WriteHeader(db, block.Header())
}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlock(db DatabaseDeleter, hash common.Hash, shardId uint16, number uint64) {
	DeleteReceipts(db, hash, shardId, number)
	DeleteHeader(db, hash, shardId, number)
	DeleteBody(db, hash, shardId, number)
	DeleteTd(db, hash, shardId, number)
}

// FindCommonAncestor returns the last common ancestor of two block headers
func FindCommonAncestor(db DatabaseReader, a, b *types.Header) *types.Header {
	for bn := b.Number.Uint64(); a.Number.Uint64() > bn; {
		a = ReadHeader(db, a.ParentHash, a.ShardId, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
	}
	for an := a.Number.Uint64(); an < b.Number.Uint64(); {
		b = ReadHeader(db, b.ParentHash, b.ShardId, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	for a.Hash() != b.Hash() {
		a = ReadHeader(db, a.ParentHash, a.ShardId, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
		b = ReadHeader(db, b.ParentHash, b.ShardId, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	return a
}
