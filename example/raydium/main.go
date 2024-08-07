package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"pumpfun-parse/raydium_parse"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

func main() {
	rpcClient := rpc.New(rpc.MainNetBeta.RPC)
	txSig := solana.MustSignatureFromBase58("3eX1BY3v8shJXVv7f8Y632SM6ErbfXJ4M8usSsDSeU85LysVSrPY2ABg9RU4hRw71NxPaUbiGMgLD1U8teRa2irx")

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

	rp, err := raydium_parse.NewRaydiumTransactionParser(tx)
	if err != nil {
		log.Fatalf("error creating raydium parser: %s", err)
	}

	raydiumTransactionData, err := rp.ParseRaydiumTransaction()
	if err != nil {
		log.Fatalf("error parsing raydium tx: %s", err)
	}

	swapInfo := raydium_parse.ConvertToSwapInfo(raydiumTransactionData)

	data, _ := json.MarshalIndent(swapInfo, "", "  ")
	fmt.Println(string(data))
}
