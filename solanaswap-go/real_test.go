package solanaswapgo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/test-go/testify/assert"
)

func TestParser(t *testing.T) {

	cluster := rpc.MainNetBeta

	rpcClient := rpc.New(cluster.RPC)

	var maxTxVersion uint64 = 0

	txids := make(map[string]string)
	txids["Orca"] = "2kAW5GAhPZjM3NoSrhJVHdEpwjmq9neWtckWnjopCfsmCGB27e3v2ZyMM79FdsL4VWGEtYSFi1sF1Zhs7bqdoaVT"
	txids["PumpFun"] = "4Cod1cNGv6RboJ7rSB79yeVCR4Lfd25rFgLY3eiPJfTJjTGyYP1r2i1upAYZHQsWDqUbGd1bhTRm1bpSQcpWMnEz"
	txids["Raydium"] = "oXUd22GQ1d45a6XNzfdpHAX6NfFEfFa9o2Awn2oimY89Rms3PmXL1uBJx3CnTYjULJw6uim174b3PLBFkaAxKzK"

	for amm, txid := range txids {
		fmt.Println("start process: ", amm)
		txSig := solana.MustSignatureFromBase58(txid)

		tx, err := rpcClient.GetTransaction(
			context.Background(),
			txSig,
			&rpc.GetTransactionOpts{
				Commitment:                     rpc.CommitmentConfirmed,
				MaxSupportedTransactionVersion: &maxTxVersion,
			},
		)
		assert.Equal(t, err, nil)

		parser, err := NewTransactionParser(tx)
		assert.Equal(t, err, nil)
		transactionData, err := parser.ParseTransaction()
		assert.Equal(t, err, nil)
		swapData, err := parser.ProcessSwapData(transactionData)
		assert.Equal(t, err, nil)
		assert.Equal(t, swapData.AMMs[0], amm)

		time.Sleep(3 * time.Second)
	}

}
