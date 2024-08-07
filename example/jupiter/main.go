package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sol-swap-parse/parse"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

func main() {
	rpcClient := rpc.New(rpc.MainNetBeta.RPC)
	txSig := solana.MustSignatureFromBase58("3zQKPvFSSfvZPBRACfTGcDEyzEEx2ZyuqrkLRjbPu8Sjh88euKjGyaBYt3EbRPHpSWh49hBMg6kuLynbx7XPcgTF")

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

	jp, err := parse.NewJupiterParser(tx)
	if err != nil {
		log.Fatalf("error creating jup parser: %s", err)
	}

	swapInfo, err := jp.ParseJupiterTransaction()
	if err != nil {
		log.Fatalf("error parsing jup tx: %s", err)
	}

	data, _ := json.MarshalIndent(swapInfo, "", "  ")
	fmt.Println(string(data))
}
