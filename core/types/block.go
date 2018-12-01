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
	//"encoding/binary"

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
type ShardState struct {
	ShardId  	 uint16
	BlockNumber  uint64
	RewardRemains uint32
}
//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go1

// Header represents a block header in the Ethereum blockchain.
type Header struct {
	parentHash     common.Hash    `json:"parentHash"       gencodec:"required"`
	uncleHash      common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	coinbase       common.Address `json:"miner"            gencodec:"required"`
	shardMaskEp    uint16          `json:"shardHash"		gencodec:"required"` //how many shard can be restarted
	shardEnabled   [32]byte         `json:"shardHash"		gencodec:"required"` //shard enabed/disabled state
	root           common.Hash    `json:"stateRoot"        gencodec:"required"`
	shardTxsHash   common.Hash    `json:"transactionsRoot" gencodec:"required"`
	receiptHash    common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	bloom          Bloom          `json:"logsBloom"        gencodec:"required"`
	bloomReject    Bloom          `json:"rjLogsBloom"        gencodec:"required"` //fast check rejected transactions
	difficulty     *big.Int       `json:"difficulty"       gencodec:"required"`
	number         *big.Int       `json:"number"           gencodec:"required"`
	gasLimit       uint64         `json:"gasLimit"         gencodec:"required"`
	gasUsed        uint64         `json:"gasUsed"          gencodec:"required"`
	time           *big.Int       `json:"timestamp"        gencodec:"required"`
	extra          []byte         `json:"extraData"        gencodec:"required"`
	shardState  	[]ShardState    	  `json:"rewardRemains"        gencodec:"required"`
	mixDigest      common.Hash    `json:"mixHash"          gencodec:"required"`
	nonce          BlockNonce     `json:"nonce"            gencodec:"required"`
	dirty          bool
}

type HeaderStruct struct {
	ParentHash     common.Hash    `json:"parentHash"       gencodec:"required"`
	UncleHash      common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	Coinbase       common.Address `json:"miner"            gencodec:"required"`
	ShardMaskEp    uint16          `json:"shardHash"		gencodec:"required"` //how many shard can be restarted
	ShardEnabled   [32]byte         `json:"shardHash"		gencodec:"required"` //shard enabed/disabled state
	Root           common.Hash    `json:"stateRoot"        gencodec:"required"`
	ShardTxsHash   common.Hash    `json:"transactionsRoot" gencodec:"required"` //hash of all included shardinfo
	ReceiptHash    common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom          Bloom          `json:"logsBloom"        gencodec:"required"`
	BloomReject    Bloom          `json:"rjLogsBloom"        gencodec:"required"` //fast check rejected transactions
	Difficulty     *big.Int       `json:"difficulty"       gencodec:"required"`
	Number         *big.Int       `json:"number"           gencodec:"required"`
	GasLimit       uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed        uint64         `json:"gasUsed"          gencodec:"required"`
	Time           *big.Int       `json:"timestamp"        gencodec:"required"`
	Extra          []byte         `json:"extraData"        gencodec:"required"`
	MixDigest      common.Hash    `json:"mixHash"          gencodec:"required"`
	ShardState  	[]ShardState   `json:"rewardRemains"        gencodec:"required"`
	Nonce          BlockNonce     `json:"nonce"            gencodec:"required"`
}

func (h *Header) FillBy(h2 *HeaderStruct) {
	h.parentHash = h2.ParentHash
	h.uncleHash = h2.UncleHash
	h.coinbase = h2.Coinbase
	h.root = h2.Root
	h.shardState = h2.ShardState
	h.shardMaskEp = h2.ShardMaskEp
	h.shardEnabled = h2.ShardEnabled

	h.shardTxsHash = h2.ShardTxsHash
	h.receiptHash = h2.ReceiptHash
	h.bloom = h2.Bloom
	h.bloomReject = h2.BloomReject
	h.difficulty = h2.Difficulty
	h.number = h2.Number

	h.gasLimit = h2.GasLimit
	h.gasUsed = h2.GasUsed
	h.time = h2.Time
	h.extra = h2.Extra
	h.mixDigest = h2.MixDigest
	h.nonce = h2.Nonce

	h.dirty = true
}
func (h *Header) ToHeaderStruct() *HeaderStruct {
	return &HeaderStruct{

		ParentHash:h.parentHash,
		UncleHash:h.uncleHash,
		Coinbase:h.coinbase,
		Root:h.root,

		ShardState:h.shardState,
		ShardTxsHash:h.shardTxsHash,
		ShardMaskEp:h.shardMaskEp,
		ShardEnabled:h.shardEnabled,
		ReceiptHash:h.receiptHash,
		Bloom:h.bloom,
		BloomReject:h.bloomReject,
		Difficulty:h.difficulty,
		Number:h.number,
		GasUsed:h.gasUsed,
		GasLimit:h.gasLimit,
		Time:h.time,
		Extra:h.extra,
		MixDigest:h.mixDigest,
		Nonce:h.nonce,

	}
}
func (h *Header) ShardId() uint16 {
	return ShardMaster
}
func (h *Header) setHashDirty(dirty bool) {
	h.dirty = dirty
}
func (h *Header) ToHeader() *Header {
	return h
}
func (h *Header) ToSHeader() *SHeader {
	return nil
}

// field type overrides for gencodec
type headerMarshaling struct {
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
func (h *Header) Hash() common.Hash {
	return rlpHash(h.ToHeaderStruct())
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *Header) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.extra)+(h.difficulty.BitLen()+h.number.BitLen()+h.time.BitLen())/8)
}

func (b *Header) Number() *big.Int { return new(big.Int).Set(b.number) }
func (b *Header) GasLimit() uint64 { return b.gasLimit }
func (b *Header) GasUsed() uint64  { return b.gasUsed }
func (b *Header) Difficulty() *big.Int {
	if b.difficulty == nil {
		return nil
	} else {
		return new(big.Int).Set(b.difficulty)
	}
}
func (b *Header) Time() *big.Int {
	if b.time == nil {
		return common.Big0
	} else {
		return new(big.Int).Set(b.time)
	}
}
func (b *Header) GasUsedPtr() *uint64          { return &b.gasUsed }
func (b *Header) CoinbasePtr() *common.Address { return &b.coinbase }
func (b *Header) NumberU64() uint64            { return b.number.Uint64() }
func (b *Header) MixDigest() common.Hash       { return b.mixDigest }
func (b *Header) Nonce() BlockNonce            { return b.nonce }
func (b Header) Bloom() Bloom                  { return b.bloom }
func (b Header) BloomRejected() Bloom          { return b.bloomReject }
func (b *Header) Coinbase() common.Address     { return b.coinbase }
func (b *Header) Root() common.Hash            { return b.root }
func (b *Header) ParentHash() common.Hash      { return b.parentHash }
func (b *Header) TxHash() common.Hash          { return EmptyRootHash }
func (b *Header) ShardTxsHash() common.Hash    { return b.shardTxsHash }
func (b *Header) ReceiptHash() common.Hash     { return b.receiptHash }
func (b *Header) ResultHash() common.Hash      { return EmptyRootHash }
func (b *Header) UncleHash() common.Hash       { return b.uncleHash }
func (b *Header) Extra() []byte                { return common.CopyBytes(b.extra) }
func (b *Header) ExtraPtr() *[]byte            { return &b.extra }
func (b *Header) ShardState() []ShardState {return b.shardState}


func (b *Header) ShardExp() uint16       { return b.shardMaskEp }
func (b *Header) ShardEnabled() [32]byte { return b.shardEnabled }

func (b *Header) SetShardId(v uint16)   { ; b.setHashDirty(true) }
func (b *Header) SetNumber(v *big.Int)  { b.number = new(big.Int).Set(v); b.setHashDirty(true) }
func (b *Header) SetNumberU64(v uint64) { b.number = new(big.Int).SetUint64(v); b.setHashDirty(true) }

func (b *Header) SetParentHash(v common.Hash)  { b.parentHash = v; b.setHashDirty(true) }
func (b *Header) SetUncleHash(v common.Hash)   { b.uncleHash = v; b.setHashDirty(true) }
func (b *Header) SetReceiptHash(v common.Hash) { b.receiptHash = v; b.setHashDirty(true) }
func (b *Header) SetTxHash(v common.Hash)      { ; b.setHashDirty(true) }
func (b *Header) SetShardTxHash(v common.Hash) { b.shardTxsHash = v; b.setHashDirty(true) }
func (b *Header) SetExtra(v []byte)            { b.extra = common.CopyBytes(v); b.setHashDirty(true) }
func (b *Header) SetTime(v *big.Int)           { b.time = v }
func (b *Header) SetCoinbase(v common.Address) {
	b.coinbase = v
	b.setHashDirty(true)
}
func (b *Header) SetRoot(v common.Hash) { b.root = v; b.setHashDirty(true) }
func (b *Header) SetBloom(v Bloom)      { b.bloom = v; b.setHashDirty(true) }
func (b *Header) SetDifficulty(v *big.Int) {
	b.difficulty = new(big.Int).SetUint64(v.Uint64())
	b.setHashDirty(true)
}
func (b *Header) SetGasLimit(v uint64) { b.gasLimit = v; b.setHashDirty(true) }
func (b *Header) SetGasUsed(v uint64) {

	b.gasUsed = v; b.setHashDirty(true)
	}
func (b *Header) SetMixDigest(v common.Hash){b.mixDigest = v; b.setHashDirty(true)}
func (b *Header) SetNonce(v BlockNonce) {b.nonce = v; b.setHashDirty(true)}
func (b *Header) SetShardState(v []ShardState ) { b.shardState =v ; b.setHashDirty(true)}



type ShardBlockInfo struct {
	shardId     uint16
	blockNumber uint64
	blockHash   common.Hash

	parentHash  common.Hash  //for easy check parents hash
	coinbase    common.Address
	td  uint64

}
type ShardBlockInfoStruct struct {
	ShardId     uint16
	BlockNumber uint64
	BlockHash   common.Hash
	ParentHash  common.Hash

	Coinbase    common.Address
	Td  uint64

}

func (t *ShardBlockInfo) FillBy(t1 *ShardBlockInfoStruct) {
	t.shardId = t1.ShardId
	t.blockNumber = t1.BlockNumber
	t.blockHash = t1.BlockHash
	t.parentHash = t1.ParentHash
	t.coinbase  = t1.Coinbase
	t.td = t1.Td
}
func (t *ShardBlockInfo) ShardId() uint16         { return t.shardId }
func (t *ShardBlockInfo) BlockNumber() uint64     { return t.blockNumber }
func (t *ShardBlockInfo) Number() *big.Int        { return new(big.Int).SetUint64(t.blockNumber) }
func (t *ShardBlockInfo) NumberU64() uint64       { return t.blockNumber }
func (t *ShardBlockInfo) Hash() common.Hash       { return t.blockHash }
func (t *ShardBlockInfo) ParentHash() common.Hash { return t.parentHash }
func (t *ShardBlockInfo) Difficulty() *big.Int    { return new(big.Int).SetUint64(t.td) }
func (t *ShardBlockInfo) DifficultyU64() uint64   { return t.td }
func (t *ShardBlockInfo) Coinbase() common.Address   { return t.coinbase }
// Transactions is a Transaction slice type for basic sorting.
type ShardBlockInfos []*ShardBlockInfo

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
// In 1st Implemention, Uncles has not been removed
type Body struct {
	Transactions []*ShardBlockInfo

	Uncles []*Header
}

// Block represents an entire block in the Ethereum blockchain.
type Block struct {
	header      *Header
	uncles      []*Header
	shardBlocks ShardBlockInfos

	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	receivedAt   time.Time
	ReceivedFrom interface{}
}

//
func (b *Block) ReceivedAt() time.Time {
	return b.receivedAt
}

//
func (b *Block) SetReceivedAt(tm time.Time) {

	b.receivedAt = tm
}

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
func (b *Block) DeprecatedTd() *big.Int {
	return b.td
}

// [deprecated by eth/63]
// StorageBlock defines the RLP encoding of a Block stored in the
// state database. The StorageBlock encoding contains fields that
// would otherwise need to be recomputed.
type StorageBlock Block

// "external" block encoding. used for eth protocol, etc.
type extblock struct {
	Header *Header
	Blks   []*ShardBlockInfo
	Uncles []*Header
}

// [deprecated by eth/63]
// "storage" block encoding. used for database.
type storageblock struct {
	Header *Header
	Blks   []*ShardBlockInfo
	Uncles []*Header
	TD     *big.Int
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewBlock(header HeaderIntf, blks []*ShardBlockInfo, uncles []HeaderIntf, receipts []*Receipt) BlockIntf {
	b := &Block{header: CopyHeader(header.ToHeader()), td: new(big.Int)}

	// TODO: panic if len(txs) != len(receipts)
	if len(blks) == 0 {
		b.header.shardTxsHash = EmptyRootHash
	} else {
		b.header.shardTxsHash = DeriveSha(ShardBlockInfos(blks))
		b.shardBlocks = make(ShardBlockInfos, len(blks))
		copy(b.shardBlocks, blks)
	}

	if len(receipts) == 0 {
		b.header.receiptHash = EmptyRootHash
	} else {
		b.header.receiptHash = DeriveSha(Receipts(receipts))
		b.header.bloom = CreateBloom(receipts)
	}

	if len(uncles) == 0 {
		b.header.uncleHash = EmptyUncleHash
	} else {
		b.header.uncleHash = CalcUncleHash(uncles)
		b.uncles = make([]*Header, len(uncles))
		for i := range uncles {
			b.uncles[i] = CopyHeader(uncles[i].ToHeader())
		}
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header HeaderIntf) BlockIntf {
	if header.ShardId() == ShardMaster {
		return &Block{header: CopyHeaderIntf(header).ToHeader()}
	} else {
		return &SBlock{header: CopyHeaderIntf(header).ToSHeader()}
	}

}

// poor implemention
func CopyHeaderIntf(h HeaderIntf) HeaderIntf {
	if h.ShardId() == ShardMaster {
		return CopyHeader(h.ToHeader())
	} else {
		return CopySHeader(h.ToSHeader())
	}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {
	cpy := *h
	if cpy.time = new(big.Int); h.time != nil {
		cpy.time.Set(h.time)
	}
	if cpy.difficulty = new(big.Int); h.difficulty != nil {
		cpy.difficulty.Set(h.difficulty)
	}
	if cpy.number = new(big.Int); h.number != nil {
		cpy.number.Set(h.number)
	}
	if len(h.extra) > 0 {
		cpy.extra = make([]byte, len(h.extra))
		copy(cpy.extra, h.extra)
	}
	return &cpy
}

// DecodeRLP decodes the Ethereum
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.uncles, b.shardBlocks = eb.Header, eb.Uncles, eb.Blks
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Blks:   b.shardBlocks,
		Uncles: b.uncles,
	})
}

// [deprecated by eth/63]
func (b *StorageBlock) DecodeRLP(s *rlp.Stream) error {
	var sb storageblock
	if err := s.Decode(&sb); err != nil {
		return err
	}
	b.header, b.uncles, b.shardBlocks, b.td = sb.Header, sb.Uncles, sb.Blks, sb.TD
	return nil
}

func (b *Block) Uncles() []HeaderIntf {
	result := make([]HeaderIntf, len(b.uncles))
	for k, v := range b.uncles {
		result[k] = v.ToHeader()
	}
	return result
}

func (b *Block) ShardBlock(hash common.Hash) *ShardBlockInfo {
	for _, transaction := range b.shardBlocks {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}
func (b *Block) ClearHashCache() {
	b.hash.Store(common.Hash{})
}
func (b *Block) ShardId() uint16                 { return b.header.ShardId() }
func (b *Block) Header() HeaderIntf              { return b.header }
func (b *Block) Number() *big.Int                { return new(big.Int).Set(b.header.number) }
func (b *Block) GasLimit() uint64                { return b.header.gasLimit }
func (b *Block) GasUsed() uint64                 { return b.header.gasUsed }
func (b *Block) Difficulty() *big.Int            { return new(big.Int).Set(b.header.difficulty) }
func (b *Block) Time() *big.Int                  { return new(big.Int).Set(b.header.time) }
func (b *Block) GasUsedPtr() *uint64             { return &b.header.gasUsed }
func (b *Block) CoinbasePtr() *common.Address    { return &b.header.coinbase }
func (b *Block) NumberU64() uint64               { return b.header.number.Uint64() }
func (b *Block) MixDigest() common.Hash          { return b.header.mixDigest }
func (b *Block) Nonce() BlockNonce               { return b.header.nonce }
func (b Block) Bloom() Bloom                     { return b.header.bloom }
func (b Block) BloomRejected() Bloom             { return b.header.bloomReject }
func (b *Block) Coinbase() common.Address        { return b.header.coinbase }
func (b *Block) Root() common.Hash               { return b.header.root }
func (b *Block) ParentHash() common.Hash         { return b.header.parentHash }
func (b *Block) TxHash() common.Hash             { return b.header.shardTxsHash }
func (b *Block) ReceiptHash() common.Hash        { return b.header.receiptHash }
func (b *Block) UncleHash() common.Hash          { return b.header.uncleHash }
func (b *Block) Extra() []byte                   { return common.CopyBytes(b.header.extra) }
func (b *Block) SetReceivedFrom(val interface{}) { b.ReceivedFrom = val }

func (b *Block) ShardExp() uint16       { return b.header.shardMaskEp }
func (b *Block) ShardEnabled() [32]byte { return b.header.shardEnabled }

// Body returns the non-header content of the block.
func (b *Block) Body() *SuperBody { return &SuperBody{b.shardBlocks, b.uncles, nil, nil, nil} }
func (b *Block) Transactions() []*Transaction {
	return nil
}
func (b *Block) ShardBlocks() []*ShardBlockInfo {
	return b.shardBlocks
}

func (b *Block) Results() []*ContractResult {
	return nil
}
func (b *Block) ToBlock() *Block {
	return b
}
func (b *Block) ToSBlock() *SBlock {
	return nil
}

//Uncles()       []*Header

func (b *Block) Receipts() []*Receipt {
	return nil
}

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *Block) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

func CalcUncleHash(uncles []HeaderIntf) common.Hash {
	return rlpHash(uncles)
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
func (b *Block) WithSeal(header HeaderIntf) BlockIntf {
	cpy := header.ToHeader()

	return &Block{
		header:      cpy,
		shardBlocks: b.shardBlocks,
		uncles:      b.uncles,
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
func (b *Block) WithBody(shardBlocksInfos []*ShardBlockInfo, receipts []*Receipt, transactions []*Transaction, results []*ContractResult) BlockIntf {
	block := &Block{
		header: CopyHeader(b.header),
	}
	if len(shardBlocksInfos) > 0 {
		block.shardBlocks = make([]*ShardBlockInfo, len(shardBlocksInfos))

		copy(block.shardBlocks, shardBlocksInfos)
		b.header.SetShardTxHash(DeriveSha(ShardBlockInfos(shardBlocksInfos)))

	} else {

		b.header.SetShardTxHash(EmptyRootHash)
	}

	if len(receipts) > 0 {
		block.header.SetReceiptHash(DeriveSha(Receipts(receipts)))
	} else {
		block.header.SetReceiptHash(EmptyRootHash)
	}
	/*	for i := range uncles {
		block.uncles[i] = CopyHeader(uncles[i])
	}*/
	return block
}

// WithBody returns a new block with the given transaction and uncle contents.
func (b *Block) WithBodyOfTransactions(transactions []*Transaction, receitps []*ContractResult) *Block {

	return nil
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Block) Hash() common.Hash {

	if hash := b.hash.Load(); (!b.header.dirty && hash != nil && hash != common.Hash{}) {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	b.header.dirty = false
	return v
}

type Blocks []*Block
type BlockIntfs []BlockIntf

type BlockBy func(b1, b2 BlockIntf) bool

func (self BlockBy) Sort(blocks BlockIntfs) {
	bs := blockSorter{
		blocks: blocks,
		by:     self,
	}
	sort.Sort(bs)
}

type blockSorter struct {
	blocks BlockIntfs
	by     func(b1, b2 BlockIntf) bool
}

func (self blockSorter) Len() int { return len(self.blocks) }
func (self blockSorter) Swap(i, j int) {
	self.blocks[i], self.blocks[j] = self.blocks[j], self.blocks[i]
}
func (self blockSorter) Less(i, j int) bool { return self.by(self.blocks[i], self.blocks[j]) }

func Number(b1, b2 BlockIntf) bool { return b1.Header().Number().Cmp(b2.Header().Number()) < 0 }
