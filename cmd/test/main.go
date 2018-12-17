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

	cfgs := []*gethConfig{
		{
			Eth:  eth.DefaultConfig,
			Node: defaultNodeConfig(),
		},
		{
			Eth:  eth.DefaultConfig,
			Node: defaultNodeConfig(),
		},
	}

	cfgs[1].Eth.ShardId = 0x0000

	var stacks [2]*node.Node
	add, _ := hexutil.Decode("0xEa3a1E0735507dBd305555A48411457D03AD4e88")
	addr := common.BytesToAddress(add)
	genesis := core.DeveloperGenesisBlock(0, addr)
	genesis.Config.Clique = nil

	for i, cfg := range cfgs {

		add := i + 1
		cfg.Eth.Ethash.PowMode = ethash.ModeFake
		cfg.Eth.SyncMode = downloader.FullSync
		cfg.Eth.NetworkId = genesis.Config.ChainID.Uint64()
		cfg.Eth.Genesis = genesis
		//cfg.Eth.Etherbase = addr
		cfg.Eth.Ethash.CacheDir = "ethash" + strconv.Itoa(add)
		cfg.Eth.Ethash.DatasetDir = ".ethash" + strconv.Itoa(add)
		cfg.Node.DataDir = ".etherrum" + strconv.Itoa(add)
		cfg.Node.P2P.ListenAddr = ":" + strconv.Itoa(30303+add)
		cfg.Node.HTTPHost = "0.0.0.0"
		cfg.Node.HTTPPort = 8545 + add*2
		cfg.Node.WSOrigins = []string{"*"}
		cfg.Node.WSHost = "0.0.0.0"
		cfg.Node.WSPort = 8546 + add*2
	}

	for i, cfg := range cfgs {

		if i == 1 {

			cfg.Eth.ShardId = 0
			cfg.Node.P2P.BootstrapNodes = make([]*enode.Node, 0, 1)
			server := stacks[0].Server()
			publicKey := server.PrivateKey.Public()
			publicKeyECDSA, _ := publicKey.(*ecdsa.PublicKey)
			//bootString := stacks[0].Server().NodeInfo().Enode
			bootString := "enode://" + hexutil.Encode(crypto.FromECDSAPub(publicKeyECDSA))[4:] + "@192.168.31.9" + cfgs[0].Node.P2P.ListenAddr
			node, err := enode.ParseV4(bootString)
			if err == nil {
				cfg.Node.P2P.BootstrapNodes = append(cfg.Node.P2P.BootstrapNodes, node)
			}

		}

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

		if id == 1 {

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

	sendTx(client)

	stacks[0].Wait()

}

type gethConfig struct {
	Eth  eth.Config
	Node node.Config
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

func sendTx(client *ethclient.Client) {
	privateKey, err := crypto.HexToECDSA("ecd5bb67db7b936a52a7d310ddeaf2defdb390703eb04d8310ed0a689abd0a18")
	if err != nil {
		log.Debug("")
	}
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

	value := big.NewInt(1)    // in wei (1 eth)
	gasLimit := uint64(21000) // in units
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Debug("")
	}

	toAddress := common.HexToAddress("0x4592d8f8d7b001e72cb26a73e4fa1806a51ac79d")
	var data []byte
	tx := types.NewTransaction(0, toAddress, value, gasLimit, gasPrice, data, 0)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Error("")
	}

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		log.Error("")
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Error("")
	}

	fmt.Printf("tx sent: %s", signedTx.Hash().Hex())
}
