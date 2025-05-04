package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	solanaswapgo "github.com/franco-bianco/solanaswap-go/solanaswap-go"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

/*
Example Transactions:
- Orca: 2kAW5GAhPZjM3NoSrhJVHdEpwjmq9neWtckWnjopCfsmCGB27e3v2ZyMM79FdsL4VWGEtYSFi1sF1Zhs7bqdoaVT
- Pumpfun: 4Cod1cNGv6RboJ7rSB79yeVCR4Lfd25rFgLY3eiPJfTJjTGyYP1r2i1upAYZHQsWDqUbGd1bhTRm1bpSQcpWMnEz
- Pumpfun AMM (Pumpswap): 23QJ6qbKcwzA76TX2uSaEb3EtBorKYty9phGYUueMyGoazopvyyZfPfGmGgGzmdt5CPW9nEuB72nnBfaGnydUa6D
- Banana Gun: oXUd22GQ1d45a6XNzfdpHAX6NfFEfFa9o2Awn2oimY89Rms3PmXL1uBJx3CnTYjULJw6uim174b3PLBFkaAxKzK
- Jupiter: DBctXdTTtvn7Rr4ikeJFCBz4AtHmJRyjHGQFpE59LuY3Shb7UcRJThAXC7TGRXXskXuu9LEm9RqtU6mWxe5cjPF
- Jupiter DCA: 4mxr44yo5Qi7Rabwbknkh8MNUEWAMKmzFQEmqUVdx5JpHEEuh59TrqiMCjZ7mgZMozRK1zW8me34w8Myi8Qi1tWP
- Meteora DLMM: 125MRda3h1pwGZpPRwSRdesTPiETaKvy4gdiizyc3SWAik4cECqKGw2gggwyA1sb2uekQVkupA2X9S4vKjbstxx3
- Rayd V4: 5kaAWK5X9DdMmsWm6skaUXLd6prFisuYJavd9B62A941nRGcrmwvncg3tRtUfn7TcMLsrrmjCChdEjK3sjxS6YG9
- Rayd Routing: 51nj5GtAmDC23QkeyfCNfTJ6Pdgwx7eq4BARfq1sMmeEaPeLsx9stFA3Dzt9MeLV5xFujBgvghLGcayC3ZevaQYi
- Rayd CPMM: afUCiFQ6amxuxx2AAwsghLt7Q9GYqHfZiF4u3AHhAzs8p1ThzmrtSUFMbcdJy8UnQNTa35Fb1YqxR6F9JMZynYp
- Rayd Concentrated Liquidity SwapV2: 2durZHGFkK4vjpWFGc5GWh5miDs8ke8nWkuee8AUYJA8F9qqT2Um76Q5jGsbK3w2MMgqwZKbnENTLWZoi3d6o2Ds
- Rayd Concentrated Liquidity Swap: 4MSVpVBwxnYTQSF3bSrAB99a3pVr6P6bgoCRDsrBbDMA77WeQqoBDDDXqEh8WpnUy5U4GeotdCG9xyExjNTjYE1u
- Maestro: mWaH4FELcPj4zeY4Cgk5gxUirQDM7yE54VgMEVaqiUDQjStyzwNrxLx4FMEaKEHQoYsgCRhc1YdmBvhGDRVgRrq
- Meteora Pools Program: 4uuw76SPksFw6PvxLFkG9jRyReV1F4EyPYNc3DdSECip8tM22ewqGWJUaRZ1SJEZpuLJz1qPTEPb2es8Zuegng9Z
- Meteora DLMM: 5PC8qXvzyeqjiTuYkNKyKRShutvVUt7hXySvg6Ux98oa9xuGT6DpTaYoEJKaq5b3tL4XFtJMxZW8SreujL2YkyPg
- Moonshot: AhiFQX1Z3VYbkKQH64ryPDRwxUv8oEPzQVjSvT7zY58UYDm4Yvkkt2Ee9VtSXtF6fJz8fXmb5j3xYVDF17Gr9CG (Buy)
- Moonshot: 2XYu86VrUXiwNNj8WvngcXGytrCsSrpay69Rt3XBz9YZvCQcZJLjvDfh9UWETFtFW47vi4xG2CkiarRJwSe6VekE (Sell)
- Multiple AMMs: 46Jp5EEUrmdCVcE3jeewqUmsMHhqiWWtj243UZNDFZ3mmma6h2DF4AkgPE9ToRYVLVrfKQCJphrvxbNk68Lub9vw //! not supported yet
- OKX: 5xaT2SXQUyvyLGsnyyoKMwsDoHrx1enCKofkdRMdNaL5MW26gjQBM3AWebwjTJ49uqEqnFu5d9nXJek6gUSGCqbL
*/

type Trade struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	TxID            string `json:"tx_id" gorm:"unique;size:88"`
	Type            string `json:"type" gorm:"size:4"`
	DexProvider     string `json:"dex_provider" gorm:"size:20"`
	Timestamp       int64  `json:"timestamp"`
	WalletAddress   string `json:"wallet_address" gorm:"size:44"`
	TokenInAddress  string `json:"token_in_address" gorm:"size:44"`
	TokenOutAddress string `json:"token_out_address" gorm:"size:44"`
	TokenInAmount   uint64 `json:"token_in_amount" gorm:"size:50"`
	TokenOutAmount  uint64 `json:"token_out_amount" gorm:"size:50"`
}

func main() {
	rpcClient := rpc.New(rpc.MainNetBeta.RPC)

	txSig := solana.MustSignatureFromBase58("jhz8dHkk2Y5nZpNiMSKBwLah5Bkuxv5Fi6vxsooCEqHBCfpsS7RzWbMvB7KG6PvZ2L7MTBnzNcZrg42QEHiVcR2") // buy pumpswap
	// txSig := solana.MustSignatureFromBase58("3JKnwtnmb7hJswPd3WjqmoBrDwRce5dvSN2jCjtX1J4KPdaPZdBgjvfrbTcPQutYuFqTDKacyNTUe6L1hThYdYjM") // buy pumpswap

	var maxTxVersion uint64 = 0
	tx, err := rpcClient.GetTransaction(
		context.TODO(),
		txSig,
		&rpc.GetTransactionOpts{
			Commitment:                     rpc.CommitmentConfirmed,
			MaxSupportedTransactionVersion: &maxTxVersion,
		},
	)
	if err != nil {
		log.Fatalf("error getting tx: %s", err)
	}

	parser, err := solanaswapgo.NewTransactionParser(tx)
	if err != nil {
		log.Fatalf("error creating parser: %s", err)
	}

	transactionData, err := parser.ParseTransaction()
	if err != nil {
		log.Fatalf("error parsing transaction: %s", err)
	}

	marshalledData, _ := json.MarshalIndent(transactionData, "", "  ")
	fmt.Println(string(marshalledData))

	swapInfo, err := parser.ProcessSwapData(transactionData)
	if err != nil {
		log.Fatalf("error processing swap data: %s", err)
	}

	marshalledSwapData, _ := json.MarshalIndent(swapInfo, "", "  ")
	fmt.Println(string(marshalledSwapData))

	WSOL := "So11111111111111111111111111111111111111112"
	USDC := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	USDT := "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB"

	tradeType := "buy"
	if swapInfo.TokenOutMint.String() == WSOL || swapInfo.TokenOutMint.String() == USDC || swapInfo.TokenOutMint.String() == USDT {
		tradeType = "sell"
	}

	transactionInfo := Trade{
		Type:            tradeType,
		DexProvider:     strings.ToLower(swapInfo.AMMs[0]),
		Timestamp:       time.Now().Unix(),
		WalletAddress:   swapInfo.Signers[0].String(),
		TokenInAddress:  swapInfo.TokenInMint.String(),
		TokenOutAddress: swapInfo.TokenOutMint.String(),
		TokenInAmount:   swapInfo.TokenInAmount,
		TokenOutAmount:  swapInfo.TokenOutAmount,
		TxID:            txSig.String(),
	}

	transactionInfoJson, _ := json.MarshalIndent(transactionInfo, "", "\t")
	fmt.Println(string(transactionInfoJson))
}
