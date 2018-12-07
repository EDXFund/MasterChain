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

package core

import (
	"fmt"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/consensus"
	"github.com/EDXFund/MasterChain/consensus/misc"
	"github.com/EDXFund/MasterChain/core/rawdb"
	"github.com/EDXFund/MasterChain/core/state"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/core/vm"
	"github.com/EDXFund/MasterChain/crypto"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/params"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
	txPool TxPoolIntf		   //pool for retrieve transactions
}

const  (
	TT_COMMON    = byte(1)
	TT_TOKEN_C
	TT_CONTRACT_TEMP
	TT_CONTRACT_INST
	TT_CONTRACT_CALL
)
type Instruction struct {
	TxType byte
	TxHash common.Hash
	Data   []byte

}
type InstructCommon struct {

	SrcAccount common.Address
	DstAccount common.Address
	TokenId	   uint64
	Amount	   uint64
}
type InstructTokenCreate struct {

	SrcAccount common.Address
	TokenId	   uint64
	VerifyCode	   []byte
}
type InstructContractCreate struct {

	SrcAccount common.Address
	TemplateId	   uint64
	ContractData	   []byte
}
type InstructContractInstance struct {

	SrcAccount common.Address
	TemplateId	   uint64
	ContractInst   uint64
}
type InstructContractCall struct {
	TxHash common.Hash
	SrcAccount common.Address
	TemplateId	   uint64
	ContractInst   uint64
	GasUsed        uint64
	DataResult	   []byte
}
// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine, txPool TxPoolIntf) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
		txPool: txPool,
	}
}
//状态处理有两种：
// 			当前是主链，处理某个分片信息   或者
//          当前是子链，同步其他节点生成的子链区块

func (p *StateProcessor) Process(block types.BlockIntf, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	if p.bc.shardId == types.ShardMaster{
		return p.MasterProcessMasterBlock(block,statedb,cfg)
	}else {
		return nil,nil,0,nil
		//return p.ShardProcessShardBlock(block,statedb,cfg)
	}
}
// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) MasterProcessShardBlock(block types.BlockIntf, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		gp       = new(GasPool).AddGas(block.GasLimit())
	)

	if block.ShardId() == types.ShardMaster {
		return nil,nil,0,ErrInvalidBlocks
	}
	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}

	if p.txPool == nil {
		log.Crit("Master processor must set txPool")
		return nil,nil,0,ErrNoTxPool
	}
	// Iterate over and process the individual transactions
	for i, instruct := range block.Results() {
		if instruct.TxType == TT_COMMON {
			tx := p.txPool.Get(instruct.TxHash)
			//get hash from pool
			statedb.Prepare(tx.Hash(), block.Hash(), i)
			receipt, _, err := ApplyTransaction(p.config, p.bc, nil, gp, nil,statedb, header, tx, usedGas, cfg)
			if err != nil {
				return nil, nil, 0, err
			}
			receipts = append(receipts, receipt)
			allLogs = append(allLogs, receipt.Logs...)
		}

	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb,block.ShardBlocks(),block.Results(), block.Transactions(),  receipts)

	return receipts, allLogs, *usedGas, nil
}
/*
func (p *StateProcessor) ShardProcessShardBlock(block types.BlockIntf, statedb *state.StateDB, cfg vm.Config) (types.ContractResults, uint64, error) {
	var (
		results types.ContractResults
		usedGas  = new(uint64)
		header   = block.Header()
		//allLogs  []*types.Log
		//gp       = new(GasPool).AddGas(block.GasLimit())
	)

	if block.ShardId() == types.ShardMaster {
		return nil,0,ErrInvalidBlocks
	}
	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}

	if p.txPool == nil {
		log.Crit("Master processor must set txPool")
		return nil,0,ErrNoTxPool
	}
	// Iterate over and process the individual transactions
	for i, instruct := range block.Results() {
		if instruct.TxType == TT_COMMON {
			tx := p.txPool.Get(instruct.TxHash)
			//get hash from pool
			statedb.Prepare(tx.Hash(), block.Hash(), i)
			result, _, err := ApplyToInstruction(p.config,  header, tx)
			if err != nil {
				return nil, 0, err
			}
			results = append(results, result)

		}

	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb,block.ShardBlocks(),block.Results(), block.Transactions(),  results)

	return results, *usedGas, nil
}*/

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) MasterProcessMasterBlock(block types.BlockIntf, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
	)
	if block.ShardId() != types.ShardMaster {
		return nil,nil,0,ErrInvalidBlocks
	}
	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}
	// Iterate over and process the individual transactions
	for _, blockInfo := range block.ShardBlocks() {
		//从数据库中取出所有的分片信息
		shardBlock := rawdb.ReadBlock(p.bc.db, blockInfo.Hash,blockInfo.BlockNumber)
		if shardBlock != nil {

			areceipts, aallLogs, ausedGas,aerr := p.MasterProcessShardBlock(shardBlock.ToSBlock(),statedb,cfg)
			if aerr == nil {
				receipts = append(receipts, areceipts...)
				allLogs = append(allLogs, aallLogs...)
				*usedGas += ausedGas
			}else {

			}
		}
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb,block.ShardBlocks(),block.Results(), block.Transactions(), receipts)

	return receipts, allLogs, *usedGas, nil
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, shardbase *common.Address,statedb *state.StateDB, header types.HeaderIntf, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, uint64, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number()))
	if err != nil {
		return nil, 0, err
	}
	// Create a new context to be used in the EVM environment
	context := NewEVMContext(msg, header, bc, author)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(context, statedb, config, cfg)
	// Apply the transaction to the current state (included in the env)
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp,shardbase)
	if err != nil {
		fmt.Println("Apply message error:",err, "\tmsg:",msg)
		return nil, 0, err
	}
	// Update the state with pending changes
	var root []byte
	if config.IsByzantium(header.Number()) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(header.Number())).Bytes()
	}
	*usedGas += gas

	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing whether the root touch-delete accounts.
	receipt := types.NewReceipt(root, failed, *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	return receipt, gas, err
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyToInstruction(config *params.ChainConfig, header types.HeaderIntf, tx *types.Transaction) (*types.ContractResult,uint64, error) {
	//check signiture
	_, err := tx.AsMessage(types.MakeSigner(config, header.Number()))
	if err != nil {
		return nil,0,  err
	}

	/*result := InstructCommon{msg.From(),*msg.To(),msg.TokenId(),msg.Value().Uint64()}
	data,err := rlp.EncodeToBytes(result)
	if err != nil {
		return nil,  err
	}*/

	//it only normal call now, contract data
	return &types.ContractResult{TT_COMMON,tx.Hash(),tx.GasPrice().Uint64(),nil,nil},0,nil

}


	//In initial edition, master node will apply transaction, this simplifies the block struct, but wastes cpu of master node
//In next edition, master node will execute the instructions created by shard node
/*func ApplyShardBlock(config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header types.HeaderIntf, usedGas *uint64, cfg vm.Config) (*types.Receipts, uint64, error) {
	if header.ShardId() == types.ShardMaster {
		rawdb.ReadBlock()
	}else {

	}
}*/