package types

import (
	"encoding/binary"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/common/hexutil"
	"github.com/EDXFund/MasterChain/crypto/sha3"
	"github.com/EDXFund/MasterChain/rlp"
	"io"
	"math/big"
	"time"
)

var (
	EmptyRootHash  = DeriveSha(Transactions{})
	EmptyUncleHash = CalcUncleHash(nil)
)

// A BlockNonce is a 64-bit hash which proves (combined with the
// mix-hash) that a sufficient amount of computation has been carried
// out on a block.
type BlockNonce [8]byte

// EncodeNonce converts the given integer to a block nonce.
func EncodeNonce(i uint64) BlockNonce {
	var n BlockNonce
	binary.BigEndian.PutUint64(n[:], i)
	return n
}

// Uint64 returns the integer value of a block nonce.
func (n BlockNonce) Uint64() uint64 {
	return binary.BigEndian.Uint64(n[:])
}

// MarshalText encodes n as a hex string with 0x prefix.
func (n BlockNonce) MarshalText() ([]byte, error) {
	return hexutil.Bytes(n[:]).MarshalText()

}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *BlockNonce) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("BlockNonce", input, n[:])
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

type ShardStatus uint16

var (
	ShardMaster uint16 = 0xFFFF
)
var (
	ShardEnableLen = 32
)

type contractReception struct {
	key   uint64 `json:"key"  	gencodec:"required"`
	value []byte `json:"value" 	gencodec:"required"`
}

// Minimized data to transfer
// contract data should be retrieved by other command
type ContractResult struct {
	TxType    byte        `json:"TxType" 		gencodec:"required"`
	TxHash    common.Hash `json:"txHash" 		gencodec:"required"`
	GasUsed   uint64      `json:"gasPrice"		gencodec:"required"`
	PostState []byte      `json:"root"		gencodec:"required"`
	Data      []byte      `json:"data"`
}
type ContractResults []*ContractResult

// Len returns the length of s.
func (s ContractResults) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s ContractResults) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s ContractResults) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

type LastShardInfo struct {
	ShardId     uint16
	BlockNumber uint64
	Hash        common.Hash
	Td          uint64
	time        time.Time
}

// TxDifference returns a new set which is the difference between a and b.
func ShardBlockDifference(a, b ShardBlockInfos) ShardBlockInfos {
	keep := make(ShardBlockInfos, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, shard := range b {
		remove[shard.Hash] = struct{}{}
	}

	for _, shard := range a {
		if _, ok := remove[shard.Hash]; !ok {
			keep = append(keep, shard)
		}
	}

	return keep
}

// TODO: copies
type BlockIntf interface {
	DecodeRLP(s *rlp.Stream) error
	EncodeRLP(w io.Writer) error
	ShardBlock(hash common.Hash) *ShardBlockInfo
	Uncles() []HeaderIntf
	Header() HeaderIntf
	Number() *big.Int
	GasLimit() uint64
	GasUsed() uint64
	Difficulty() *big.Int
	Time() *big.Int

	NumberU64() uint64
	MixDigest() common.Hash
	Nonce() BlockNonce
	Bloom() Bloom
	BloomRejected() Bloom
	Coinbase() common.Address
	Root() common.Hash
	ParentHash() common.Hash
	TxHash() common.Hash
	ReceiptHash() common.Hash
	UncleHash() common.Hash
	Extra() []byte

	ShardId() uint16
	ShardExp() uint16
	ShardEnabled() [32]byte
	// Body returns the non-header content of the block.
	Body() *SuperBody
	Size() common.StorageSize
	WithSeal(header HeaderIntf) BlockIntf
	WithBody(shardBlocksInfos []*ShardBlockInfo, receipts []*Receipt, transactions []*Transaction, results []*ContractResult) BlockIntf
	//extract as block
	ToBlock() *Block
	//extract as shard block
	ToSBlock() *SBlock
	Hash() common.Hash
	ClearHashCache()

	//Uncles()       []*Header

	//
	ReceivedAt() time.Time
	SetReceivedAt(tm time.Time)
	SetReceivedFrom(interface{})

	Transactions() []*Transaction

	ShardBlocks() []*ShardBlockInfo
	Receipts() []*Receipt
	Results() []*ContractResult
}

type HeaderIntf interface {
	Hash() common.Hash
	Size() common.StorageSize
	ShardId() uint16
	Number() *big.Int
	NumberU64() uint64
	ToHeader() *Header
	ToSHeader() *SHeader
	ParentHash() common.Hash
	UncleHash() common.Hash
	ReceiptHash() common.Hash
	ResultHash() common.Hash
	TxHash() common.Hash
	ShardTxsHash() common.Hash
	Extra() []byte
	Time() *big.Int
	Coinbase() common.Address
	Root() common.Hash
	Bloom() Bloom
	Difficulty() *big.Int
	GasLimit() uint64
	GasUsed() uint64
	MixDigest() common.Hash
	Nonce() BlockNonce
	//func (b *Header) ExtraPtr() *[]byte            { return &b.extra }
	GasUsedPtr() *uint64
	CoinbasePtr() *common.Address
	SetShardId(uint16)
	SetNumber(*big.Int)
	SetParentHash(common.Hash)
	SetUncleHash(common.Hash)
	SetReceiptHash(common.Hash)
	SetTxHash(common.Hash)
	SetExtra([]byte)
	SetTime(*big.Int)
	SetCoinbase(common.Address)
	SetRoot(common.Hash)
	SetBloom(Bloom)
	SetDifficulty(*big.Int)
	SetGasLimit(uint64)
	SetGasUsed(uint64)
	SetMixDigest(common.Hash)
	SetNonce(BlockNonce)

	setHashDirty(bool)
}

type SuperBody struct {
	ShardBlocks []*ShardBlockInfo

	Uncles []*Header

	Transactions []*Transaction
	Receipts     []*Receipt

	//receipts
	Results []*ContractResult
}

func (sb *SuperBody) ToBody() *Body {
	return &Body{Transactions: sb.ShardBlocks, Uncles: sb.Uncles}
}

func (sb *SuperBody) ToSBody() *SBody {
	return &SBody{Receipts: sb.Results}
}

type HeadEncode struct {
	ShardId uint16
	Header  []byte
}

type BodyEncode struct {
	ShardId uint16
	Body    []byte
}

// TxDifference returns a new set which is the difference between a and b.
func ShardInfoDifference(a, b ShardBlockInfos) ShardBlockInfos {
	keep := make(ShardBlockInfos, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash]; !ok {
			keep = append(keep, tx)
		}
	}

	return keep
}
