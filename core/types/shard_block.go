package types

import (
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
	shardId    uint16      `json:"shardId"			gencodec:"required"`
	parentHash common.Hash `json:"parentHash"       gencodec:"required"`


	coinbase common.Address `json:"miner"            gencodec:"required"`
	root     common.Hash    `json:"stateRoot,omitempty"        gencodec:"nil"`
	txHash   common.Hash    `json:"transactionsRoot,omitempty" gencodec:"nil"`
	receiptHash common.Hash `json:"receiptsRoot,omitempty"     gencodec:"nil"`

	bloom       Bloom       `json:"logsBloom"        gencodec:"required"`
	difficulty  *big.Int    `json:"difficulty"       gencodec:"required"`
	number      *big.Int    `json:"number"           gencodec:"required"`
	gasLimit    uint64      `json:"gasLimit"         gencodec:"required"`
	gasUsed     uint64      `json:"gasUsed"          gencodec:"required"`
	time        *big.Int    `json:"timestamp"        gencodec:"required"`
	extra       []byte      `json:"extraData"        gencodec:"required"`
	mixDigest   common.Hash `json:"mixHash"          gencodec:"required"`
	nonce       BlockNonce  `json:"nonce"            gencodec:"required"`
}
type SHeaderStruct struct {
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
func (h *SHeader) FillBy(h2 *SHeaderStruct)  {
	h.shardId    = h2.ShardId
	h.parentHash = h2.ParentHash


	h.coinbase = h2.Coinbase
	h.root  = h2.Root
	h.txHash = h2.TxHash
	h.receiptHash = h2.ReceiptHash

	h.bloom  = h2.Bloom
	h.difficulty = h2.Difficulty
	h.number = h2.Number
	h.gasLimit = h2.GasLimit
	h.gasUsed = h2.GasUsed
	h.time = h2.Time
	h.extra = h2.Extra
	h.mixDigest = h2.MixDigest
	h.nonce = h2.Nonce
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *SHeader) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.extra)+(h.difficulty.BitLen()+h.number.BitLen()+h.time.BitLen())/8)
}

func (h *SHeader) ToHeader() *Header{
	return nil
}
func (h *SHeader) ToSHeader() *SHeader {
	return h
}

func (b *SHeader) ShardId() uint16      { return b.shardId }
func (b *SHeader) Number() *big.Int     { return new(big.Int).Set(b.number) }
func (b *SHeader) GasLimit() uint64     { return b.gasLimit }
func (b *SHeader) GasUsed() uint64      { return b.gasUsed }
func (b *SHeader) Difficulty() *big.Int { return new(big.Int).Set(b.difficulty) }
func (b *SHeader) Time() *big.Int       { return new(big.Int).Set(b.time) }
func (b *SHeader) UncleHash() common.Hash { return common.Hash{}}
func (b *SHeader) NumberU64() uint64        { return b.number.Uint64() }
func (b *SHeader) MixDigest() common.Hash   { return b.mixDigest }
func (b *SHeader) Nonce() BlockNonce            { return b.nonce }
func (b *SHeader) Bloom() Bloom             { return b.bloom }
func (b *SHeader) Coinbase() common.Address { return b.coinbase }
func (b *SHeader) Root() common.Hash        { return b.root }
func (b *SHeader) ParentHash() common.Hash  { return b.parentHash }
func (b *SHeader) TxHash() common.Hash      { return b.txHash }
func (b *SHeader) ReceiptHash() common.Hash { return b.receiptHash }
func (b *SHeader) Extra() []byte { return b.extra }

func (b *SHeader) SetShardId(shardId uint16)  { b.shardId = shardId }

func (b *SHeader) SetNumber(v *big.Int){b.number = new(big.Int).Set(v)}
func (b *SHeader) SetNumberU64(v uint64){b.number = new(big.Int).SetUint64(v)}

func (b *SHeader) SetParentHash(v common.Hash){b.parentHash = v}
func (b *SHeader) SetUncleHash(v common.Hash){}
func (b *SHeader) SetReceiptHash(v common.Hash){b.receiptHash =v }
func (b *SHeader) SetTxHash(v common.Hash){b.txHash = v}
func (b *SHeader) SetExtra(v []byte){b.extra = common.CopyBytes(v)}
func (b *SHeader) SetTime(v *big.Int){b.time = v}
func (b *SHeader) SetCoinbase(v common.Address) {b.coinbase = v}
func (b *SHeader) SetRoot(v common.Hash){b.root = v}
func (b *SHeader) SetBloom(v Bloom){b.bloom = v}
func (b *SHeader) SetDifficulty( v *big.Int){b.difficulty = new(big.Int).SetUint64(v.Uint64())}
func (b *SHeader) SetGasLimit(v  uint64) { b.gasLimit = v}
func (b *SHeader) SetGasUsed(v uint64) { b.gasUsed = v}
func (b *SHeader) SetMixDigest(v common.Hash){b.mixDigest = v}
func (b *SHeader) SetNonce(v BlockNonce) {b.nonce = v}

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
	receivedAt   time.Time
	ReceivedFrom interface{}
}
type SBlocks []*SBlock

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
func (b *SBlock) DeprecatedTd() *big.Int {
	return b.td
}
func (b *SBlock) ReceivedAt() time.Time {
	return b.receivedAt
}
func (b *SBlock) SetReceivedAt(t time.Time) {
	b.receivedAt = t
}
func (b *SBlock) ToSBlock() *SBlock { return b }
// dummy implementions
func (b *SBlock)  ShardBlock(hash common.Hash) *ShardBlockInfo {return nil}
func (b *SBlock)  ShardBlocks() []*ShardBlockInfo {return nil}
func (b *SBlock) ShardExp() uint16      { return 0 }
func (b *SBlock) ShardEnabled() [32]byte { return [32]byte{0} }
func (b *SBlock) ToBlock() *Block { return nil }
func (b *SBlock) UncleHash() common.Hash   { return common.Hash{} }
func (b *SBlock) Uncles() []HeaderIntf   { return nil }
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
func NewSBlock(header HeaderIntf, txs []*Transaction, receipts []*ContractResult) BlockIntf {
	b := &SBlock{header: CopySHeader(header.ToSHeader()), td: new(big.Int)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.txHash = EmptyRootHash
	} else {
		b.header.txHash = DeriveSha(Transactions(txs))
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.receiptHash = EmptyRootHash
	} else {
		b.header.receiptHash = DeriveSha(ContractResults(receipts))
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


func (b *SBlock) Transactions() []*Transaction { return b.transactions }

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
func (b *SBlock) ShardId() uint16      { return b.header.shardId }
func (b *SBlock) Number() *big.Int     { return new(big.Int).Set(b.header.number) }
func (b *SBlock) GasLimit() uint64     { return b.header.gasLimit }
func (b *SBlock) GasUsed() uint64      { return b.header.gasUsed }
func (b *SBlock) Difficulty() *big.Int { return new(big.Int).Set(b.header.difficulty) }
func (b *SBlock) Time() *big.Int       { return new(big.Int).Set(b.header.time) }

func (b *SBlock) NumberU64() uint64        { return b.header.number.Uint64() }
func (b *SBlock) MixDigest() common.Hash   { return b.header.mixDigest }
func (b *SBlock) Nonce() BlockNonce            { return b.header.nonce }
func (b *SBlock) Bloom() Bloom             { return b.header.bloom }
func (b *SBlock) BloomRejected() Bloom             { return Bloom{0} }
func (b *SBlock) Coinbase() common.Address { return b.header.coinbase }
func (b *SBlock) Root() common.Hash        { return b.header.root }
func (b *SBlock) ParentHash() common.Hash  { return b.header.parentHash }
func (b *SBlock) TxHash() common.Hash      { return b.header.txHash }
func (b *SBlock) ReceiptHash() common.Hash { return b.header.receiptHash }
func (b *SBlock) Receipts() []*Receipt { return nil }
//func (b *Block) UncleHash() common.Hash   { return b.header.UncleHash }
func (b *SBlock) Extra() []byte                 { return common.CopyBytes(b.header.extra) }

func (b *SBlock) Header() HeaderIntf               { return b.header }

// Body returns the non-header content of the block.
func (b *SBlock) Body() *SuperBody { return &SuperBody{nil,nil,b.transactions, b.receipts} }

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
func (b *SBlock) WithSeal(header HeaderIntf) BlockIntf {
	cpy := header.ToSHeader()

	return &SBlock{
		header:       cpy,
		transactions: b.transactions,
		receipts:     b.receipts,
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
func (b *SBlock) WithBodyOfTransactions(transactions []*Transaction, contractReceipts ContractResults) *SBlock {
	block := &SBlock{
		header: CopySHeader(b.header),
	}
	block.transactions = make([]*Transaction, len(transactions))
	block.receipts = make(ContractResults, len(contractReceipts))

	copy(block.transactions, transactions)
	copy(block.receipts, contractReceipts)

	return block
}
// WithBody returns a new block with the given transaction and uncle contents.
func (b *SBlock) WithBody(shardBlocksInfos []*ShardBlockInfo, uncles []HeaderIntf,transactions []*Transaction,receipts []*ContractResult) BlockIntf {

	return b.WithBodyOfTransactions(transactions,receipts)
}
func (b *SBlock)WithBodyOfShardBlocks(shardBlocksInfos []*ShardBlockInfo, uncles []*Header) *Block{
	return nil
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

func SNumber(b1, b2 *SBlock) bool { return b1.header.number.Cmp(b2.header.number) < 0 }
