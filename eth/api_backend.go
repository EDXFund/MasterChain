// Copyright 2015 The go-ethereum Authors
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

package eth

import (
	"context"
	"math/big"
	"reflect"

	"github.com/EDXFund/MasterChain/accounts"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/common/math"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/bloombits"
	"github.com/EDXFund/MasterChain/core/state"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/core/vm"
	"github.com/EDXFund/MasterChain/eth/downloader"
	"github.com/EDXFund/MasterChain/eth/gasprice"
	"github.com/EDXFund/MasterChain/ethdb"
	"github.com/EDXFund/MasterChain/event"
	"github.com/EDXFund/MasterChain/params"
	"github.com/EDXFund/MasterChain/rpc"
)

// EthAPIBackend implements ethapi.Backend for full nodes
type EthAPIBackend struct {
	eth *Ethereum
	gpo *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *EthAPIBackend) ChainConfig() *params.ChainConfig {
	return b.eth.chainConfig
}

func (b *EthAPIBackend) CurrentBlock() types.BlockIntf {
	return b.eth.blockchain.CurrentBlock()
}

func (b *EthAPIBackend) SetHead(number uint64) {
	b.eth.protocolManager.downloader.Cancel()
	b.eth.blockchain.SetHead(number)
}

func (b *EthAPIBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (types.HeaderIntf, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.eth.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.eth.blockchain.CurrentBlock().Header(), nil
	}
	return b.eth.blockchain.GetHeaderByNumber(uint64(blockNr)), nil
}

func (b *EthAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (types.HeaderIntf, error) {
	return b.eth.blockchain.GetHeaderByHash(hash), nil
}

func (b *EthAPIBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (types.BlockIntf, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.eth.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.eth.blockchain.CurrentBlock(), nil
	}
	return b.eth.blockchain.GetBlockByNumber(uint64(blockNr)), nil
}

func (b *EthAPIBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, types.HeaderIntf, error) {
	// Pending state is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block, state := b.eth.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, blockNr)
	if header == nil || reflect.ValueOf(header).IsNil() || err != nil {
		return nil, nil, err
	}
	stateDb, err := b.eth.BlockChain().StateAt(header.Root())
	return stateDb, header, err
}

func (b *EthAPIBackend) GetBlock(ctx context.Context, hash common.Hash) (types.BlockIntf, error) {
	return b.eth.blockchain.GetBlockByHash(hash), nil
}

func (b *EthAPIBackend) GetShardBlock(ctx context.Context, hash common.Hash, shardId uint16) (types.BlockIntf, error) {
	return b.eth.shardPool.GetBlockByHash(hash, shardId), nil
}

func (b *EthAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return b.eth.blockchain.GetReceiptsByHash(hash), nil
}

func (b *EthAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	receipts := b.eth.blockchain.GetReceiptsByHash(hash)
	if receipts == nil {
		return nil, nil
	}
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (b *EthAPIBackend) GetTd(blockHash common.Hash) *big.Int {
	return b.eth.blockchain.GetTdByHash(blockHash)
}

func (b *EthAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header types.HeaderIntf, vmCfg vm.Config) (*vm.EVM, func() error, error) {
	state.SetBalance(msg.From(), math.MaxBig256)
	vmError := func() error { return nil }

	context := core.NewEVMContext(msg, header, b.eth.BlockChain(), nil)
	return vm.NewEVM(context, state, b.eth.chainConfig, vmCfg), vmError, nil
}

func (b *EthAPIBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeChainEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainShardsEvent(ch chan<- *core.ChainsShardEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeChainShardsEvent(ch)
}

func (b *EthAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.eth.BlockChain().SubscribeLogsEvent(ch)
}

func (b *EthAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.eth.txPool.AddLocals([]*types.Transaction{signedTx})[0]
}

func (b *EthAPIBackend) GetPoolTransactions() (types.Transactions, error) {
	pending, err := b.eth.txPool.Pending()
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *EthAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.eth.txPool.Get(hash)
}

func (b *EthAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.eth.txPool.State().GetNonce(addr), nil
}

func (b *EthAPIBackend) Stats() (pending int, queued int) {
	return b.eth.txPool.Stats()
}

func (b *EthAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.eth.TxPool().Content()
}

func (b *EthAPIBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.eth.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *EthAPIBackend) Downloader() *downloader.Downloader {
	return b.eth.Downloader()
}

func (b *EthAPIBackend) ProtocolVersion() int {
	return b.eth.EthVersion()
}

func (b *EthAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *EthAPIBackend) ChainDb() ethdb.Database {
	return b.eth.ChainDb()
}

func (b *EthAPIBackend) EventMux() *event.TypeMux {
	return b.eth.EventMux()
}

func (b *EthAPIBackend) AccountManager() *accounts.Manager {
	return b.eth.AccountManager()
}

func (b *EthAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.eth.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *EthAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.eth.bloomRequests)
	}
}
