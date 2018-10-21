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


"github.com/EDXFund/MasterChain/rlp"
)




// Header represents a block header in the Ethereum blockchain.
type SHeader struct {
	ShardId    uint16      `json:"shardId"			gencodec:"required"`
	ParentHash common.Hash `json:"parentHash"       gencodec:"required"`


	Coinbase common.Address `json:"miner"            gencodec:"required"`
	Root     common.Hash    `json:"stateRoot,omitempty"        gencodec:"nil"`
	TxHash   common.Hash    `json:"transactionsRoot,omitempty" gencodec:"nil"`
	ReceiptHash common.Hash `json:"receiptsRoot,omitempty"     gencodec:"nil"`

	Bloom       Bloom       `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int    `json:"difficulty"       gencodec:"required"`
	Number      *big.Int    `json:"number"           gencodec:"required"`
	GasLimit    uint64      `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64      `json:"gasUsed"          gencodec:"required"`
	Time        *big.Int    `json:"timestamp"        gencodec:"required"`
	Extra       []byte      `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash `json:"mixHash"          gencodec:"required"`
	Nonce       BlockNonce  `json:"nonce"            gencodec:"required"`
}

type SHeaderMarshal struct {
	ShardId    uint16      `json:"shardId"			gencodec:"required"`
	ParentHash common.Hash `json:"parentHash"       gencodec:"required"`
	Coinbase     common.Address `json:"miner"            gencodec:"required"`
	Root         common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash       common.Hash    `json:"transactionsRoot" gencodec:"required"`
	// hash of shard block included in this master block
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *hexutil.Big   `json:"difficulty"       gencodec:"required"`
	Number      *hexutil.Big   `json:"number"           gencodec:"required"`
	GasLimit    hexutil.Uint64 `json:"gasLimit"         gencodec:"required"`
	GasUsed     hexutil.Uint64 `json:"gasUsed"          gencodec:"required"`
	Time        *hexutil.Big   `json:"timestamp"        gencodec:"required"`
	Extra       hexutil.Bytes  `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    `json:"mixHash"          gencodec:"required"`
	Nonce       BlockNonce     `json:"nonce"            gencodec:"required"`
	Hash        common.Hash    `json:"hash"`
}
type SHeaderUnmarshal struct {
	ShardId    uint16       `json:"shardId"			gencodec:"required"`
	ParentHash *common.Hash `json:"parentHash"       gencodec:"required"`
	Coinbase     *common.Address `json:"miner"            gencodec:"required"`
	Root         *common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash       *common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash *common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       *Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int        `json:"difficulty"       gencodec:"required"`
	Number      *big.Int        `json:"number"           gencodec:"required"`
	GasLimit    *hexutil.Uint64 `json:"gasLimit"         gencodec:"required"`
	GasUsed     *hexutil.Uint64 `json:"gasUsed"          gencodec:"required"`
	Time        *big.Int        `json:"timestamp"        gencodec:"required"`
	Extra       *hexutil.Bytes  `json:"extraData"        gencodec:"required"`
	MixDigest   *common.Hash    `json:"mixHash"          gencodec:"required"`
	Nonce       *BlockNonce     `json:"nonce"            gencodec:"required"`
}

// field type overrides for gencodec
type scheaderMarshaling struct {
	ShardId    hexutil.Uint64
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
func (h *SHeader) Hash() common.Hash {
	return rlpHash(h)
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *SHeader) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+(h.Difficulty.BitLen()+h.Number.BitLen()+h.Time.BitLen())/8)
}




// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
type SBody struct {
	Transactions []*Transaction

	//receipts
	Receipts ContractResults


}

// Block represents an entire block in the Ethereum blockchain.
type SBlock struct {
	header *SHeader

	transactions Transactions
	receipts     ContractResults


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
type SBlocks []*SBlock

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
func (b *SBlock) DeprecatedTd() *big.Int {
	return b.td
}

// [deprecated by eth/63]
// StorageBlock defines the RLP encoding of a Block stored in the
// state database. The StorageBlock encoding contains fields that
// would otherwise need to be recomputed.
type StorageSBlock SBlock

// "external" block encoding. used for eth protocol, etc.
type sextblock struct {
	Header   *SHeader
	Txs      []*Transaction
	Receipts []*ContractResult
}

// [deprecated by eth/63]
// "storage" block encoding. used for database.
type sstorageblock struct {
	Header   *SHeader
	Txs      []*Transaction
	Receipts []*ContractResult
	TD       *big.Int
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewSBlock(header *SHeader, txs []*Transaction, receipts []*ContractResult) *SBlock {
	b := &SBlock{header: CopySHeader(header), td: new(big.Int)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs))
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptHash = EmptyRootHash
	} else {
		b.header.ReceiptHash = DeriveSha(ContractResults(receipts))
		//b.header.Bloom = CreateBloom(receipts)
	}


	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewSBlockWithHeader(header *SHeader) *SBlock {
	return &SBlock{header: CopySHeader(header)}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopySHeader(h *SHeader) *SHeader {
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
func (b *SBlock) DecodeRLP(s *rlp.Stream) error {
	var eb sextblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.transactions, b.receipts = eb.Header, eb.Txs, eb.Receipts
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *SBlock) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, sextblock{
		Header:   b.header,
		Txs:      b.transactions,
		Receipts: b.receipts,
	})
}

// [deprecated by eth/63]
func (b *StorageSBlock) DecodeRLP(s *rlp.Stream) error {
	var sb sstorageblock
	if err := s.Decode(&sb); err != nil {
		return err
	}
	b.header, b.transactions, b.receipts, b.td = sb.Header, sb.Txs, sb.Receipts, sb.TD
	return nil
}

// TODO: copies


func (b *SBlock) Transactions() Transactions { return b.transactions }

func (b *SBlock) Receiptions() ContractResults { return b.receipts }

func (b *SBlock) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range b.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}
func (b *SBlock) ContractReceipts() ContractResults { return b.receipts }

func (b *SBlock) ContrcatReceipt(hash common.Hash) *ContractResult {
	for _, receipt := range b.receipts {
		if receipt.TxHash == hash {
			return receipt
		}
	}
	return nil
}
func (b *SBlock) ShardId() uint16      { return b.header.ShardId }
func (b *SBlock) Number() *big.Int     { return new(big.Int).Set(b.header.Number) }
func (b *SBlock) GasLimit() uint64     { return b.header.GasLimit }
func (b *SBlock) GasUsed() uint64      { return b.header.GasUsed }
func (b *SBlock) Difficulty() *big.Int { return new(big.Int).Set(b.header.Difficulty) }
func (b *SBlock) Time() *big.Int       { return new(big.Int).Set(b.header.Time) }

func (b *SBlock) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *SBlock) MixDigest() common.Hash   { return b.header.MixDigest }
func (b *SBlock) Nonce() uint64            { return binary.BigEndian.Uint64(b.header.Nonce[:]) }
func (b *SBlock) Bloom() Bloom             { return b.header.Bloom }
func (b *SBlock) Coinbase() common.Address { return b.header.Coinbase }
func (b *SBlock) Root() common.Hash        { return b.header.Root }
func (b *SBlock) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *SBlock) TxHash() common.Hash      { return b.header.TxHash }
func (b *SBlock) ReceiptHash() common.Hash { return b.header.ReceiptHash }

//func (b *Block) UncleHash() common.Hash   { return b.header.UncleHash }
func (b *SBlock) Extra() []byte                 { return common.CopyBytes(b.header.Extra) }

func (b *SBlock) Header() *SHeader               { return CopySHeader(b.header) }

// Body returns the non-header content of the block.
func (b *SBlock) Body() *SBody { return &SBody{b.transactions, b.receipts} }

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *SBlock) Size() common.StorageSize {
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
func (b *SBlock) WithSeal(header *SHeader) *SBlock {
	cpy := *header

	return &SBlock{
		header:       &cpy,
		transactions: b.transactions,
		receipts:     b.receipts,
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
func (b *SBlock) WithBody(transactions []*Transaction, contractReceipts ContractResults) *SBlock {
	block := &SBlock{
		header: CopySHeader(b.header),
	}
	block.transactions = make([]*Transaction, len(transactions))
	block.receipts = make(ContractResults, len(contractReceipts))

	copy(block.transactions, transactions)
	copy(block.receipts, contractReceipts)

	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *SBlock) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

type SBlockBy func(b1, b2 *SBlock) bool

func (self SBlockBy) Sort(blocks SBlocks) {
	bs := scblockSorter{
		blocks: blocks,
		by:     self,
	}
	sort.Sort(bs)
}

type scblockSorter struct {
	blocks SBlocks
	by     func(b1, b2 *SBlock) bool
}

func (self scblockSorter) Len() int { return len(self.blocks) }
func (self scblockSorter) Swap(i, j int) {
	self.blocks[i], self.blocks[j] = self.blocks[j], self.blocks[i]
}
func (self scblockSorter) Less(i, j int) bool { return self.by(self.blocks[i], self.blocks[j]) }

func SNumber(b1, b2 *SBlock) bool { return b1.header.Number.Cmp(b2.header.Number) < 0 }
