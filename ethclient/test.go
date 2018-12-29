package ethclient

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/hdwallet"
	"math/big"
	"math/rand"
	"strconv"
	"time"
)

type TAccount struct {
	PvKey *ecdsa.PrivateKey
	Addr  common.Address
	txs   []*types.Transaction
}

func InitAccount(mnemonic string, len int) (senders []*TAccount, alloc map[common.Address]core.GenesisAccount, err error) {
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		return nil, nil, err
	}
	alloc = make(map[common.Address]core.GenesisAccount)
	for i := 0; i < len; i++ {

		path := hdwallet.MustParseDerivationPath("m/44'/60'/0'/0/" + strconv.Itoa(i))
		account, _ := wallet.Derive(path, true)

		pvKey, _ := wallet.PrivateKey(account)

		senders = append(senders, &TAccount{
			PvKey: pvKey,
			Addr:  account.Address,
		})

		alloc[account.Address] = core.GenesisAccount{Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))}

	}

	return senders, alloc, nil
}

func SendTx(client *Client, senders []*TAccount) error {
	gasPrice, _ := client.SuggestGasPrice(context.Background())
	chainID, _ := client.NetworkID(context.Background())

	var count = 0

	for _, sender := range senders {
		privateKey := sender.PvKey
		value := big.NewInt(1)    // in wei (1 eth)
		gasLimit := uint64(21000) // in units

		var data []byte

		for _, receiver := range senders {

			tx := types.NewTransaction(rand.Uint64(), receiver.Addr, value, gasLimit, gasPrice, data, 0)

			signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
			if err != nil {
				return err
			}

			err = client.SendTransaction(context.Background(), signedTx)
			if err != nil {
				return nil
			}
			count += 1

			//fmt.Printf("tx sent: %s", signedTx.Hash().Hex())
		}

	}
	fmt.Printf("tx sent end: %v ----  %v ", count, time.Now())
	return nil

}
