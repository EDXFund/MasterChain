// Copyright 2014 The go-ethereum Authors
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

// Package types contains data types related to Ethereum consensus.
package types

import (
	"encoding/binary"
	"io"
	"math/big"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/common/hexutil"
	//"github.com/EDXFund/MasterChain/crypto/sha3"
	"github.com/EDXFund/MasterChain/rlp"
)


//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

// Header represents a block header in the Ethereum blockchain.
type MCHeader struct {
	//always 0xFFFF
	ShardId		uint16		   `json:"shardId"			gencodec:"required"`
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	//hash of most lasted blocks of shards
	ShardHash   common.Hash    `json:"shardHash"       	gencodec:"required"`
	ShardMask   uint16			`json:"shardMask"		gencodec:"required"`
	ShardEnabled []byte			`json:"shardEnabled"		gencodec:"required"`
	Coinbase    common.Address `json:"miner"            gencodec:"required"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	RjHash 		common.Hash    `json:"rejectRoot"     	gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        *big.Int       `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    `json:"mixHash"          gencodec:"required"`
	Nonce       BlockNonce     `json:"nonce"            gencodec:"required"`
}

// field type overrides for gencodec
type mcheaderMarshaling struct {
	Difficulty *hexutil.Big
	Number     *hexutil.Big
	GasLimit   hexutil.Uint64
	GasUsed    hexutil.Uint64
	Time       *hexutil.Big
	Extra      hexutil.Bytes
	Hash       common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *MCHeader) Hash() common.Hash {
	return rlpHash(h)
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *MCHeader) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+(h.Difficulty.BitLen()+h.Number.BitLen()+h.Time.BitLen())/8)
}



type RejectInfo struct {
	TxHash   *common.Hash
	Reason	 uint16
}
type RejectInfos []*RejectInfo

// Len returns the length of s.
func (s RejectInfos) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s RejectInfos) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s RejectInfos) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}


type ShardBlockInfo struct {
	ShardId  uint16
	Number   *big.Int
	Hash     common.Hash
}

type ShardBlockInfos  []*ShardBlockInfo;

// Len returns the length of s.
func (s ShardBlockInfos) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s ShardBlockInfos) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s ShardBlockInfos) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}
// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
type MCBody struct {
	BlockInfos []*ShardBlockInfo
	RejectInfos  []*RejectInfo

}

// Block represents an entire block in the Ethereum blockchain.
type MCBlock struct {
	header       *MCHeader
	
	blockInfos 	ShardBlockInfos
	rejectInfos  RejectInfos
	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
func (b *MCBlock) DeprecatedTd() *big.Int {
	return b.td
}

// [deprecated by eth/63]
// StorageBlock defines the RLP encoding of a Block stored in the
// state database. The StorageBlock encoding contains fields that
// would otherwise need to be recomputed.
type MCStorageBlock MCBlock

// "external" block encoding. used for eth protocol, etc.
type mcextblock struct {
	Header *MCHeader
	Blks    []*ShardBlockInfo
	Rjs    []*RejectInfo
}

// [deprecated by eth/63]
// "storage" block encoding. used for database.
type storagemcblock struct {
	Header *MCHeader
	Blks    []*ShardBlockInfo
	Rjs    []*RejectInfo
	TD     *big.Int
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewMCBlock(header *MCHeader, blks []*ShardBlockInfo,  rejects []*RejectInfo) *MCBlock {
	b := &MCBlock{header: CopyMCHeader(header), td: new(big.Int)}

	// TODO: panic if len(txs) != len(receipts)
	if len(blks) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(ShardBlockInfos(blks))
		b.blockInfos = make(ShardBlockInfos, len(blks))
		copy(b.blockInfos, blks)
	}
	if len(rejects) == 0 {
		b.header.RjHash = EmptyRootHash
	}else {
		b.header.TxHash = DeriveSha(RejectInfos(rejects))
		b.rejectInfos = make(RejectInfos, len(rejects))
		copy(b.rejectInfos, rejects)
	}


	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewMCBlockWithHeader(header *MCHeader) *MCBlock {
	return &MCBlock{header: CopyMCHeader(header)}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyMCHeader(h *MCHeader) *MCHeader {
	cpy := *h
	if cpy.Time = new(big.Int); h.Time != nil {
		cpy.Time.Set(h.Time)
	}
	if cpy.Difficulty = new(big.Int); h.Difficulty != nil {
		cpy.Difficulty.Set(h.Difficulty)
	}
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}

// DecodeRLP decodes the Ethereum
func (b *MCBlock) DecodeRLP(s *rlp.Stream) error {
	var eb mcextblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.blockInfos,b.rejectInfos = eb.Header, eb.Blks,eb.Rjs
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *MCBlock) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, mcextblock{
		Header: b.header,
		Blks:    b.blockInfos,
		Rjs:	b.rejectInfos,
	})
}

// [deprecated by eth/63]
func (b *MCStorageBlock) DecodeRLP(s *rlp.Stream) error {
	var sb storagemcblock
	if err := s.Decode(&sb); err != nil {
		return err
	}
	b.header, b.blockInfos, b.rejectInfos,b.td = sb.Header,  sb.Blks, sb.Rjs,sb.TD
	return nil
}

// TODO: copies


func (b *MCBlock) ShardBlockInfos() ShardBlockInfos { return b.blockInfos }

func (b *MCBlock) ShardBlockInfo(hash common.Hash) *ShardBlockInfo {
	for _, blockInfo := range b.blockInfos {
		if blockInfo.Hash == hash {
			return blockInfo
		}
	}
	return nil
}
func (b *MCBlock) ShardBlockInfoByNumber(number uint64) *ShardBlockInfo {
	for _, blockInfo := range b.blockInfos {
		if blockInfo.Number.Uint64() == number {
			return blockInfo
		}
	}
	return nil
}
func (b *MCBlock) RejectInfos() RejectInfos { return b.rejectInfos }

func (b *MCBlock) RejectInfo(hash common.Hash) *RejectInfo {
	for _, rj_info := range b.rejectInfos {
		if *rj_info.TxHash == hash {
			return rj_info
		}
	}
	return nil
}

func (b *MCBlock) Number() *big.Int     { return new(big.Int).Set(b.header.Number) }
func (b *MCBlock) GasLimit() uint64     { return b.header.GasLimit }
func (b *MCBlock) GasUsed() uint64      { return b.header.GasUsed }
func (b *MCBlock) Difficulty() *big.Int { return new(big.Int).Set(b.header.Difficulty) }
func (b *MCBlock) Time() *big.Int       { return new(big.Int).Set(b.header.Time) }

func (b *MCBlock) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *MCBlock) MixDigest() common.Hash   { return b.header.MixDigest }
func (b *MCBlock) Nonce() uint64            { return binary.BigEndian.Uint64(b.header.Nonce[:]) }
func (b *MCBlock) Bloom() Bloom             { return b.header.Bloom }
func (b *MCBlock) Coinbase() common.Address { return b.header.Coinbase }
func (b *MCBlock) Root() common.Hash        { return b.header.Root }
func (b *MCBlock) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *MCBlock) TxHash() common.Hash      { return b.header.TxHash }
func (b *MCBlock) ReceiptHash() common.Hash { return b.header.ReceiptHash }
//func (b *MCBlock) UncleHash() common.Hash   { return b.header.UncleHash }
func (b *MCBlock) Extra() []byte            { return common.CopyBytes(b.header.Extra) }

func (b *MCBlock) Header() *MCHeader { return CopyMCHeader(b.header) }

// Body returns the non-header content of the block.
func (b *MCBlock) MCBody() *MCBody { return &MCBody{b.blockInfos, b.rejectInfos} }

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *MCBlock) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}






// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
func (b *MCBlock) WithSeal(header *MCHeader) *MCBlock {
	cpy := *header

	return &MCBlock{
		header:       	&cpy,
		blockInfos: 	b.blockInfos,
		rejectInfos:	b.rejectInfos,
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
func (b *MCBlock) WithBody(blockInfos []*ShardBlockInfo, rejectinfos []*RejectInfo) *MCBlock {
	block := &MCBlock{
		header:       CopyMCHeader(b.header),
		blockInfos: make([]*ShardBlockInfo, len(blockInfos)),
		rejectInfos:       make([]*RejectInfo, len(rejectinfos)),
	}
	copy(block.blockInfos, blockInfos)
	copy(block.rejectInfos, rejectinfos)

	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *MCBlock) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

type MCBlocks []*MCBlock

type MCBlockBy func(b1, b2 *MCBlock) bool

func (self MCBlockBy) Sort(blocks MCBlocks) {
	bs := mcblockSorter{
		blocks: blocks,
		by:     self,
	}
	sort.Sort(bs)
}

type mcblockSorter struct {
	blocks MCBlocks
	by     func(b1, b2 *MCBlock) bool
}

func (self mcblockSorter) Len() int { return len(self.blocks) }
func (self mcblockSorter) Swap(i, j int) {
	self.blocks[i], self.blocks[j] = self.blocks[j], self.blocks[i]
}
func (self mcblockSorter) Less(i, j int) bool { return self.by(self.blocks[i], self.blocks[j]) }

func MCNumber(b1, b2 *MCBlock) bool { return b1.header.Number.Cmp(b2.header.Number) < 0 }
