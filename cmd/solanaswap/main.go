// Command solanaswap prints the swap in a Solana transaction as JSON.
// Usage: solanaswap [-rpc URL] <signature> | solanaswap -file tx.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/franco-bianco/solanaswap-go/v2"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	endpoint := flag.String("rpc", envOr("SOLANA_RPC_URL", rpc.MainNetBeta_RPC), "Solana RPC endpoint")
	file := flag.String("file", "", "parse a getTransaction JSON result from this file instead of fetching")
	timeout := flag.Duration("timeout", 30*time.Second, "RPC request timeout")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-rpc URL] <signature> | -file tx.json\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	var (
		info *solanaswap.SwapInfo
		err  error
	)
	switch {
	case *file != "":
		data, readErr := os.ReadFile(*file)
		if readErr != nil {
			return readErr
		}
		info, err = solanaswap.ParseJSON(data)
	case flag.NArg() == 1:
		info, err = fetchAndParse(*endpoint, flag.Arg(0), *timeout)
	default:
		flag.Usage()
		return errors.New("expected a transaction signature or -file")
	}
	if err != nil {
		return err
	}

	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(info)
}

func fetchAndParse(endpoint, signature string, timeout time.Duration) (*solanaswap.SwapInfo, error) {
	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Version 1 is the newest format; requesting it also returns legacy and v0 transactions.
	maxVersion := uint64(1)
	tx, err := rpc.New(endpoint).GetTransaction(ctx, sig, &rpc.GetTransactionOpts{
		Encoding:                       solana.EncodingBase64,
		Commitment:                     rpc.CommitmentConfirmed,
		MaxSupportedTransactionVersion: &maxVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("get transaction: %w", err)
	}
	return solanaswap.Parse(tx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
