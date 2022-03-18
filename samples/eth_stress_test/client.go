package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/tak1827/blockchain-tps-test/tps"
)

var (
	_ tps.Client = (*EthClient)(nil)
)

type EthClient struct {
	client *ethclient.Client
}

func NewClient(url string) (c EthClient, err error) {
	c.client, err = ethclient.Dial(url)
	if err != nil {
		return
	}
	return
}

func (c EthClient) LatestBlockHeight(ctx context.Context) (uint64, error) {

	res, err := c.client.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (c EthClient) CountTx(ctx context.Context, height uint64) (int, error) {

	block, err := c.client.BlockByNumber(ctx, big.NewInt(int64(height)))
	if err != nil {
		return 0, err
	}

	return len(block.Transactions()), nil
}

func (c EthClient) CountPendingTx(ctx context.Context) (int, error) {

	count, err := c.client.PendingTransactionCount(ctx)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (c EthClient) Nonce(ctx context.Context, address string) (uint64, error) {

	nonce, err := c.client.PendingNonceAt(ctx, common.HexToAddress(address))
	if err != nil {
		log.Fatalln("error in getNonce while calling pending nonce at", err)

		return 0, err
	}
	return nonce, nil
}

func (c EthClient) signTransaciton(ctx context.Context, privHex string, nonce uint64, to string, value int64) (*types.Transaction, error) {

	// value
	amount := big.NewInt(value)

	gasLimit := uint64(21000)

	// gas price
	gasPrice := c.getGasPriceSuggestion(ctx)

	// address
	toAddress := common.HexToAddress(to)

	chainID := c.getChainID(ctx)

	// making transaction
	tx := types.NewTransaction(nonce, toAddress, amount, gasLimit, &gasPrice, []byte{})

	// signing transaction
	signTX, err := types.SignTx(tx, types.NewEIP155Signer(&chainID), privateKeyHextoECDSA(privHex))
	if err != nil {
		log.Fatal("error in signTransaciton while signTX: ", err)

		return nil, err
	}
	fmt.Println("transaction successfully signed")

	return signTX, nil
}

func (c EthClient) SignerFnForPk(ctx context.Context, privHex string) bind.SignerFn {

	privKey := privateKeyHextoECDSA(privHex)
	keyAddr := crypto.PubkeyToAddress(privKey.PublicKey)
	chainID := c.getChainID(ctx)

	return func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {

		signer := types.NewEIP155Signer(&chainID)
		if address != keyAddr {
			return nil, errors.New("not authorized to sign this account")
		}
		signature, err := crypto.Sign(signer.Hash(tx).Bytes(), privKey)
		if err != nil {
			return nil, err
		}
		return tx.WithSignature(signer, signature)
	}
}

func generatePublicKey(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := priv.Public()
	pubKeyECDSA, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		fmt.Println("error in generatePrivatekey, can not assert ecdsa.publickey type : ")
		return nil
	}

	pubKeyBytes := crypto.FromECDSAPub(pubKeyECDSA)
	fmt.Println("public key is:", hexutil.Encode(pubKeyBytes))

	return pubKeyECDSA

}

func generateAddress(pub *ecdsa.PublicKey) common.Address {
	address := crypto.PubkeyToAddress(*pub)
	fmt.Println("address is:", hexutil.Encode(address[:]))
	return address
}

func privateKeyHextoECDSA(privHex string) *ecdsa.PrivateKey {
	priv, err := crypto.HexToECDSA(privHex)
	if err != nil {
		fmt.Println("error in generateAddressFromPrivHex while decoding hex to privatekey: %w", err)
		return nil

	}
	return priv

}

func (c *EthClient) getChainID(ctx context.Context) big.Int {

	chainid, err := c.client.ChainID(ctx)
	if err != nil {
		log.Fatalln("error in getChainID while getting chain id:", err)
		return big.Int{}
	}
	return *chainid

}

func (c *EthClient) getGasPriceSuggestion(ctx context.Context) big.Int {

	gasPrice, err := c.client.SuggestGasPrice(ctx)
	if err != nil {
		log.Fatalln("error in SuggestGasPrice while calling pending nonce at", err)

		return big.Int{}
	}
	return *gasPrice

}

// sends transaction to the network
func (c *EthClient) SendTx(ctx context.Context, privHex string, nonce uint64, to string, value int64) (common.Hash, error) {

	signedtx, err := c.signTransaciton(ctx, privHex, nonce, to, value)

	if err != nil {
		return common.BytesToHash([]byte("")), err
	}
	if err := c.client.SendTransaction(ctx, signedtx); err != nil {
		log.Fatalln("error in SendTx while getting chain id:", err)

	}
	fmt.Println("transaction sent. txid: ", signedtx.Hash().Hex(), "nonce: ", nonce)

	return signedtx.Hash(), nil
}

func (c EthClient) Erc721TransferFrom(ctx context.Context, privHex string, nonce uint64, to string, value int64, erc721address string, tokenId int64) (common.Hash, error) {

	fromAddress := generateAddress(generatePublicKey(privateKeyHextoECDSA(privHex)))
	nft, err := NewErc721(common.HexToAddress(erc721address), c.client)

	if err != nil {
		return common.BytesToHash([]byte("")), err
	}

	gasLimit := uint64(210000)

	// gas price
	gasPrice := c.getGasPriceSuggestion(ctx)

	// address
	toAddress := common.HexToAddress(to)

	opts := bind.TransactOpts{
		From:     fromAddress,
		Nonce:    big.NewInt(int64(nonce)),
		Signer:   c.SignerFnForPk(ctx, privHex),
		GasPrice: &gasPrice,
		GasLimit: gasLimit,
		Context:  ctx,
		NoSend:   false,
	}

	res, err := nft.TransferFrom(&opts, fromAddress, toAddress, big.NewInt(int64(tokenId)))

	if err != nil {
		return common.BytesToHash([]byte("")), err
	}

	return res.Hash(), nil
}

func (c EthClient) Erc721TokenOwner(ctx context.Context, erc721address string, tokenId int64) (string, error) {

	nft, err := NewErc721(common.HexToAddress(erc721address), c.client)

	if err != nil {
		fmt.Println("Erc721TokenOwner NewErc721", err)
		return "", err
	}

	opt := bind.CallOpts{
		Pending: true,
		From:    common.HexToAddress(erc721address),
		Context: ctx,
	}
	address, err := nft.OwnerOf(&opt, big.NewInt(tokenId))

	if err != nil {
		fmt.Println("Erc721TokenOwner OwnerOf", err)
		return "", err
	}
	return address.Hex(), nil
}
