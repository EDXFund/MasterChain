package mdc

import (
	"errors"
	"github.com/EDXFund/MasterChain/common"
	"math/rand"
	"sync"
	"time"
)

type Mdb struct {

	self    common.Address
	//previously stored access
	prevAccessMasks uint16
    prevAccessNonce uint64
	provider   MdcIntf
}

type MdcIntf interface {

	SetProvider(string)

	Put(self common.Address,  hash common.Hash, value []byte)
	//Get data from storage, returns data, nonce,masks
	Get(self common.Address, hash common.Hash) ([]byte,error,uint64,uint16)
	//Write and send receipt of(nonce,masks ) storage
	SendReceiption(self common.Address, receipt []byte )
}

func (m *Mdb)Put(hash common.Hash,value []byte){
	m.provider.Put(m.self,hash,value)
}

func (m *Mdb)Get(hash common.Hash) ([]byte,error){
	data,err,nonce,masks := m.provider.Get(m.self,hash)
	if err == nil {
		m.prevAccessNonce = nonce
		m.prevAccessMasks = masks

		//m.provider.SendReceiption(crypto.Sign(crypto.Keccak256Hash()))

		return data,err
	}
	return data,err

}

type nonce struct {
	nonce uint64
	mask  uint16
	receipt   bool
}
type MDCDemo struct {
	addrInfo map[common.Address]*nonce
	mu       sync.RWMutex

	cache    map[common.Hash][]byte
}

func (m *MDCDemo)SetProvider(string) {

}

func (m *MDCDemo)Put(address common.Address,hash common.Hash,value []byte){
	m.cache[hash] = common.CopyBytes(value)
}

func (m *MDCDemo)setupMask(address common.Address){
	m.mu.Lock()
	defer m.mu.Unlock()
	value,ok := m.addrInfo[address]
	if !ok {
		value = &nonce{nonce:rand.New(rand.NewSource(time.Now().UnixNano())).Uint64(),receipt:false, mask:1}

		m.addrInfo[address] = value
	}else {
		value.mask = value.mask+1
	}
}
func (m *MDCDemo)Get(address common.Address,hash common.Hash)([]byte,error,uint64,uint16){

	value,ok := m.cache[hash]
	if ok {
		m.setupMask(address)
		return common.CopyBytes(value),nil,m.addrInfo[address].nonce,m.addrInfo[address].mask
	}else {
		return []byte{},errors.New("hash not exist"),0,0
	}
}