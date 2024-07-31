package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"pumpfun-parse/parse"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

func main() {
	rpcClient := rpc.New("https://api.mainnet-beta.solana.com")

	var maxTxVersion uint64 = 0

	tx, err := rpcClient.GetParsedTransaction(
		context.TODO(),
		solana.MustSignatureFromBase58("4kPxWuFqG6Jj5uutxv67K87DYuVrQukuBpP1UHbT7Hd16KUGA7fanQtZKgwTzE1HBK3WvzGHmRbhhadJTokLpchj"),
		&rpc.GetParsedTransactionOpts{
			MaxSupportedTransactionVersion: &maxTxVersion,
			Commitment:                     rpc.CommitmentConfirmed,
		},
	)
	if err != nil {
		log.Fatalf("failed to get tx: %s", err)
	}

	parsedTx, err := parse.ParseTx(*tx, rpcClient)
	if err != nil {
		log.Fatalf("failed to parse tx: %s", err)
	}

	marshalledTx, _ := json.MarshalIndent(parsedTx, "", "  ")
	fmt.Println(string(marshalledTx))
}
