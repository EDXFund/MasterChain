package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/common/hexutil"
	"github.com/EDXFund/MasterChain/crypto/sha3"
	"github.com/EDXFund/MasterChain/rlp"
	"io"
	"math/big"
	"time"
	"unsafe"
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
	TxHash            common.Hash `json:"txHash" 		gencodec:"required"`
	PostState         []byte      `json:"root"`
	Status            uint64      `json:"status"`
	CumulativeGasUsed uint64      `json:"cumulativeGasUsed" gencodec:"required"`
	GasUsed           uint64      `json:"gasUsed" gencodec:"required"`
}
type ContractResults []*ContractResult;
// Len returns the length of s.
func (s ContractResults) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s ContractResults) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s ContractResults) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

// NewReceipt creates a barebone transaction receipt, copying the init fields.
func NewContractResult(root []byte, failed bool, cumulativeGasUsed uint64) *ContractResult {
	r := &ContractResult{PostState: common.CopyBytes(root), CumulativeGasUsed: cumulativeGasUsed}
	if failed {
		r.Status = ReceiptStatusFailed
	} else {
		r.Status = ReceiptStatusSuccessful
	}
	return r
}

// EncodeRLP implements rlp.Encoder, and flattens the consensus fields of a receipt
// into an RLP stream. If no post state is present, byzantium fork is assumed.
func (r *ContractResult) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &ContractResultRLP{&r.TxHash, r.statusEncoding(), r.CumulativeGasUsed})
}

// DecodeRLP implements rlp.Decoder, and loads the consensus fields of a receipt
// from an RLP stream.
func (r *ContractResult) DecodeRLP(s *rlp.Stream) error {
	var dec ContractResultRLP
	if err := s.Decode(&dec); err != nil {
		return err
	}
	if err := r.setStatus(dec.PostStatePostStateOrStatus); err != nil {
		return err
	}
	r.TxHash, r.CumulativeGasUsed = *dec.TxHash, dec.CumulativeGasUsed
	return nil
}

func (r *ContractResult) setStatus(postStateOrStatus []byte) error {
	switch {
	case bytes.Equal(postStateOrStatus, receiptStatusSuccessfulRLP):
		r.Status = ReceiptStatusSuccessful
	case bytes.Equal(postStateOrStatus, receiptStatusFailedRLP):
		r.Status = ReceiptStatusFailed
	case len(postStateOrStatus) == len(common.Hash{}):
		r.PostState = postStateOrStatus
	default:
		return fmt.Errorf("invalid receipt status %x", postStateOrStatus)
	}
	return nil
}

func (r *ContractResult) statusEncoding() []byte {
	if len(r.PostState) == 0 {
		if r.Status == ReceiptStatusFailed {
			return receiptStatusFailedRLP
		}
		return receiptStatusSuccessfulRLP
	}
	return r.PostState
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (r *ContractResult) Size() common.StorageSize {
	size := common.StorageSize(unsafe.Sizeof(*r)) + common.StorageSize(len(r.PostState))

	return size
}

// receiptRLP is the consensus encoding of a contract .
type ContractResultRLP struct {
	TxHash                     *common.Hash `json:"txHash" 		gencodec:"required"`
	PostStatePostStateOrStatus []byte       `json:"root"`

	CumulativeGasUsed uint64 `json:"cumulativeGasUsed" gencodec:"required"`
}

type ContractResultStorageRLP struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	TxHash            common.Hash
}
type ContractResultStorage ContractResult

// EncodeRLP implements rlp.Encoder, and flattens all content fields of a receipt
// into an RLP stream.
func (r *ContractResultStorage) EncodeRLP(w io.Writer) error {
	enc := &ContractResultStorageRLP{
		PostStateOrStatus: (*ContractResult)(r).statusEncoding(),
		CumulativeGasUsed: r.CumulativeGasUsed,

		TxHash: r.TxHash,
	}

	return rlp.Encode(w, enc)
}

// DecodeRLP implements rlp.Decoder, and loads both consensus and implementation
// fields of a receipt from an RLP stream.
func (r *ContractResultStorage) DecodeRLP(s *rlp.Stream) error {
	var dec receiptStorageRLP
	if err := s.Decode(&dec); err != nil {
		return err
	}
	if err := (*ContractResult)(r).setStatus(dec.PostStateOrStatus); err != nil {
		return err
	}
	// Assign the consensus fields
	r.CumulativeGasUsed = dec.CumulativeGasUsed

	// Assign the implementation fields
	r.TxHash = dec.TxHash
	return nil
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
	for _, tx := range b {
		remove[tx.Hash()] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash()]; !ok {
			keep = append(keep, tx)
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

	ShardId()  uint16
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

	//Uncles()       []*Header


	//
	ReceivedAt() time.Time
	SetReceivedAt(tm time.Time)
	SetReceivedFrom(interface{})

	Transactions() []*Transaction

	ShardBlocks() []*ShardBlockInfo
	Receipts()    []*Receipt
	Results()    []*ContractResult
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
	ResultHash()  common.Hash
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
	SetShardId( uint16)
	SetNumber( *big.Int)
	SetParentHash( common.Hash)
	SetUncleHash( common.Hash)
	SetReceiptHash( common.Hash)
	SetTxHash( common.Hash)
	SetExtra( []byte)
	SetTime( *big.Int)
	SetCoinbase( common.Address)
	SetRoot( common.Hash)
	SetBloom( Bloom)
	SetDifficulty( *big.Int)
	SetGasLimit( uint64)
	SetGasUsed( uint64)
	SetMixDigest( common.Hash)
	SetNonce( BlockNonce)
}

type SuperBody struct {
	ShardBlocks []*ShardBlockInfo

	Uncles []*Header

	Transactions []*Transaction
	Receipts   []*Receipt

	//receipts
	Results []*ContractResult
}

func (sb *SuperBody) ToBody() *Body {
	return &Body{Transactions: sb.ShardBlocks, Uncles: sb.Uncles}
}

func (sb *SuperBody) ToSBody() *SBody {
	return &SBody{Transactions: sb.Transactions, Receipts: sb.Results}
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
		remove[tx.Hash()] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash()]; !ok {
			keep = append(keep, tx)
		}
	}

	return keep
}