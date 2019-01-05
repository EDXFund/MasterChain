package main

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/EDXFund/MasterChain/cmd/utils"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/common/hexutil"
	"github.com/EDXFund/MasterChain/consensus/ethash"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/crypto"
	"github.com/EDXFund/MasterChain/dashboard"
	"github.com/EDXFund/MasterChain/eth"
	"github.com/EDXFund/MasterChain/eth/downloader"
	"github.com/EDXFund/MasterChain/ethclient"
	"github.com/EDXFund/MasterChain/log"
	"github.com/EDXFund/MasterChain/node"
	"github.com/EDXFund/MasterChain/p2p/enode"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"io"
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

const mnemonic string = "whip matter defense behave advance boat belt purse oil hamster stable clump"

func main() {
	time.Sleep(time.Second * 2)
	log.PrintOrigins(true)
	glogger.Verbosity(log.Lvl(4))
	//glogger.Vmodule(ctx.GlobalString(vmoduleFlag.Name))
	//glogger.BacktraceAt(ctx.GlobalString(backtraceAtFlag.Name))
	log.Root().SetHandler(glogger)

	shardNumber := 4

	_, alloc, _ := ethclient.InitAccount(mnemonic, 200)

	genesis := core.DeveloperGenesisBlock(0, common.Address{})
	genesis.Config.Clique = nil
	genesis.Alloc = alloc
	genesis.ShardExp = 2
	genesis.ShardEnabled = [32]byte{0x03}
	cfgs := make([]*gethConfig, 0)

	for i := 0; i < shardNumber+1; i++ {

		cfg := &gethConfig{
			Eth:       eth.DefaultConfig,
			Node:      defaultNodeConfig(),
			Dashboard: dashboard.DefaultConfig,
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
		cfg.Node.DataDir = ".edxchain" + strconv.Itoa(int(shardId))
		cfg.Node.P2P.NoDiscovery = true
		cfg.Node.P2P.ListenAddr = ":" + strconv.Itoa(30303+add)
		cfg.Node.IPCPath = cfg.Node.IPCPath + strconv.Itoa(int(shardId))
		cfg.Dashboard.Host = "0.0.0.0"
		cfg.Dashboard.Port = 8081 + add

		cfgs = append(cfgs, cfg)

		os.RemoveAll(cfg.Node.ResolvePath(""))
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

		stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
			return dashboard.New(&cfg.Dashboard, strconv.Itoa(int(cfg.Eth.ShardId)), ctx.ResolvePath("logs")), nil
		})

		if err != nil {
			fmt.Errorf("Register error :%d", i)
		}

		err = stack.Start()

		if err != nil {
			fmt.Errorf("start error :%d", i)
		}

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

	//rpcClient, err := stacks[0].Attach()
	//if err != nil {
	//	log.Error("rpcClient error")
	//}

	//client := ethclient.NewClient(rpcClient)

	//go ethclient.SendTx(client, senders)
	timer1 := time.NewTicker(50 * time.Minute)
	for {
		select {
		case <-timer1.C:
			return
		}
	}

}

type gethConfig struct {
	Eth       eth.Config
	Node      node.Config
	Dashboard dashboard.Config
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
