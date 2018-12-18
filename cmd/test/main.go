package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/EDXFund/MasterChain/cmd/utils"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/common/hexutil"
	"github.com/EDXFund/MasterChain/consensus/ethash"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/crypto"
	"github.com/EDXFund/MasterChain/eth"
	"github.com/EDXFund/MasterChain/eth/downloader"
	"github.com/EDXFund/MasterChain/ethclient"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/node"
	"github.com/EDXFund/MasterChain/p2p/enode"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"io"
	"math/big"
	"os"
	"strconv"
	"time"
)

var (
	ostream log.Handler
	glogger *log.GlogHandler
)

func init() {
	usecolor := (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	output := io.Writer(os.Stdout)
	if usecolor {
		output = colorable.NewColorableStdout()
	}
	ostream = log.StreamHandler(output, log.TerminalFormat(usecolor))
	glogger = log.NewGlogHandler(ostream)
}

func main() {

	log.PrintOrigins(true)
	glogger.Verbosity(log.Lvl(4))
	//glogger.Vmodule(ctx.GlobalString(vmoduleFlag.Name))
	//glogger.BacktraceAt(ctx.GlobalString(backtraceAtFlag.Name))
	log.Root().SetHandler(glogger)

	shardNumber := 4

	senders, receivers, alloc := initAccount(100)

	genesis := core.DeveloperGenesisBlock(0, common.Address{})
	genesis.Config.Clique = nil
	genesis.Alloc = alloc
	cfgs := make([]*gethConfig, 0)

	for i := 0; i < shardNumber+1; i++ {

		cfg := &gethConfig{
			Eth:  eth.DefaultConfig,
			Node: defaultNodeConfig(),
		}
		add := i + 1
		var shardId uint16
		if i == 0 {
			shardId = uint16(65535)
			cfg.Node.HTTPHost = "0.0.0.0"
			cfg.Node.HTTPPort = 8545 + add*2
			cfg.Node.WSOrigins = []string{"*"}
			cfg.Node.WSHost = "0.0.0.0"
			cfg.Node.WSPort = 8546 + add*2
		} else {
			shardId = uint16(i - 1)
		}
		cfg.Eth.ShardId = shardId
		cfg.Eth.Ethash.PowMode = ethash.ModeFake

		cfg.Eth.SyncMode = downloader.FullSync
		cfg.Eth.NetworkId = genesis.Config.ChainID.Uint64()
		cfg.Eth.Genesis = genesis
		//cfg.Eth.Etherbase = addr
		cfg.Eth.Ethash.CacheDir = "ethash" + strconv.Itoa(int(shardId))
		cfg.Eth.Ethash.DatasetDir = ".ethash" + strconv.Itoa(int(shardId))
		cfg.Node.DataDir = ".etherrum" + strconv.Itoa(int(shardId))
		cfg.Node.P2P.NoDiscovery = true
		cfg.Node.P2P.ListenAddr = ":" + strconv.Itoa(30303+add)

		cfgs = append(cfgs, cfg)
	}

	stacks := make([]*node.Node, shardNumber+1)

	for i, cfg := range cfgs {

		stack, err := node.New(&cfg.Node)

		if err != nil {
			fmt.Errorf("new Node error :%d", i)
		}

		err = stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
			fullNode, err := eth.New(ctx, &cfg.Eth)
			//if fullNode != nil && cfg.Eth.LightServ > 0 {
			//	ls, _ := les.NewLesServer(fullNode, &cfg.Eth)
			//	fullNode.AddLesServer(ls)
			//}
			return fullNode, err
		})

		if err != nil {
			fmt.Errorf("Register error :%d", i)
		}

		err = stack.Start()
		if i > 0 {

			//cfg.Node.P2P.BootstrapNodes = make([]*enode.Node, 0, 1)
			server := stacks[0].Server()
			publicKey := server.PrivateKey.Public()
			publicKeyECDSA, _ := publicKey.(*ecdsa.PublicKey)
			//bootString := stacks[0].Server().NodeInfo().Enode
			bootString := "enode://" + hexutil.Encode(crypto.FromECDSAPub(publicKeyECDSA))[4:] + "@127.0.0.1" + cfgs[0].Node.P2P.ListenAddr
			node, err := enode.ParseV4(bootString)
			if err == nil {
				stack.Server().AddPeer(node)
				//cfg.Node.P2P.BootstrapNodes = append(cfg.Node.P2P.BootstrapNodes, node)
			}

		}

		// Set the gas price to the limits from the CLI and start mining
		/*	gasprice := utils.GlobalBig(ctx, utils.MinerLegacyGasPriceFlag.Name)
			if ctx.IsSet(utils.MinerGasPriceFlag.Name) {
				gasprice = utils.GlobalBig(ctx, utils.MinerGasPriceFlag.Name)
			}
			ethereum.TxPool().SetGasPrice(gasprice)

			threads := ctx.GlobalInt(utils.MinerLegacyThreadsFlag.Name)
			if ctx.GlobalIsSet(utils.MinerThreadsFlag.Name) {
				threads = ctx.GlobalInt(utils.MinerThreadsFlag.Name)
			}*/

		if err != nil {
			fmt.Errorf("start node error :%d", i)
		}

		stacks[i] = stack

	}

	time.Sleep(time.Second * 2)

	for id, node := range stacks {

		if id == 0 {

		}
		var ethereum *eth.Ethereum
		if err := node.Service(&ethereum); err != nil {
			log.Crit("Ethereum service not running: %v", err)
		}
		if err := ethereum.StartMining(1); err != nil {
			utils.Fatalf("Failed to start mining: %v", err)
		}
	}

	rpcClient, err := stacks[0].Attach()
	if err != nil {
		log.Error("rpcClient error")
	}

	client := ethclient.NewClient(rpcClient)

	go sendTx(client, senders, receivers)

	stacks[0].Wait()

}

type gethConfig struct {
	Eth  eth.Config
	Node node.Config
}

type TAccount struct {
	pvKey *ecdsa.PrivateKey
	addr  common.Address
	nonce uint64
	txs   []*types.Transaction
}

func initAccount(len int) (senders []*TAccount, receivers []*TAccount, alloc map[common.Address]core.GenesisAccount) {
	alloc = make(map[common.Address]core.GenesisAccount)
	for i := 0; i < len; i++ {

		senderKey, _ := crypto.GenerateKey()
		senderAddress := crypto.PubkeyToAddress(senderKey.PublicKey)

		senders = append(senders, &TAccount{
			pvKey: senderKey,
			nonce: uint64(0),
			addr:  senderAddress,
		})

		alloc[senderAddress] = core.GenesisAccount{Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))}

	}

	for i := 0; i < len; i++ {
		receiversKey, _ := crypto.GenerateKey()
		senderAddress := crypto.PubkeyToAddress(receiversKey.PublicKey)

		receivers = append(receivers, &TAccount{
			pvKey: receiversKey,
			nonce: uint64(0),
			addr:  senderAddress,
		})
	}

	return senders, receivers, alloc
}

func defaultNodeConfig() node.Config {
	cfg := node.DefaultConfig
	cfg.Name = "edx"
	cfg.Version = "0.0.1"
	cfg.HTTPModules = append(cfg.HTTPModules, "eth")
	cfg.WSModules = append(cfg.WSModules, "eth")
	cfg.IPCPath = "edx.ipc"
	return cfg
}

func sendTx(client *ethclient.Client, senders []*TAccount, receivers []*TAccount) {
	gasPrice, _ := client.SuggestGasPrice(context.Background())
	chainID, _ := client.NetworkID(context.Background())

	var count = 0

	for _, sender := range senders {
		privateKey := sender.pvKey
		value := big.NewInt(1)    // in wei (1 eth)
		gasLimit := uint64(21000) // in units

		var data []byte

		for _, receiver := range receivers {

			tx := types.NewTransaction(sender.nonce, receiver.addr, value, gasLimit, gasPrice, data, 0)

			signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
			if err != nil {
				log.Error("")
			}
			sender.nonce += 1
			err = client.SendTransaction(context.Background(), signedTx)
			if err != nil {

				log.Error("")
			}
			count += 1

			//fmt.Printf("tx sent: %s", signedTx.Hash().Hex())
		}

	}
	fmt.Printf("tx sent end: %v ----  %v ", count, time.Now())

	//
	//publicKey := privateKey.Public()
	//publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	//if !ok {
	//	log.Debug("error casting public key to ECDSA")
	//}
	//
	//fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	//nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	//if err != nil {
	//	log.Debug("")
	//}

}
