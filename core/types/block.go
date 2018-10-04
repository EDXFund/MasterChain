package types

import (
	"io"
	"math/big"
	"encoding/binary"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/common/hexutil"

	"github.com/EDXFund/MasterChain/crypto/sha3"
	"github.com/EDXFund/MasterChain/rlp"
)

var (
	EmptyRootHash  = DeriveSha(Transactions{})
//	EmptyUncleHash = CalcUncleHash(nil)
)

var (
	ShardEnableLen = 32
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

type HeaderIntf interface{
	ShardId() uint16
	Time() *big.Int 
	ParentHash() common.Hash
	Coinbase()   common.Address
	Difficulty() *big.Int
	Number()     *big.Int
	Extra()      []byte
	Nonce()      BlockNonce
	MixDigest()    common.Hash
}


type BlockIntf interface{
	EncodeRLP( io.Writer)
	Transactions() Transactions 
	Transaction(hash common.Hash) *Transaction
	 Number() *big.Int  
 GasLimit() uint64     
 GasUsed() uint64      
 Difficulty() *big.Int 
 Time() *big.Int      

 NumberU64() uint64       
 MixDigest() common.Hash   
 Nonce() uint64            
 Bloom() Bloom             
 Coinbase() common.Address 
 Root() common.Hash       
 ParentHash() common.Hash  
 TxHash() common.Hash     
 ReceiptHash() common.Hash 

 Extra() []byte           

 Header() *HeaderIntf 



// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
 Size() common.StorageSize
}