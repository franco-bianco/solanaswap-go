# solanaswap-go: Solana Swap Transaction Parser

Parses a Solana transaction and extracts the swap info, supports multiple AMMs. Please note that parsing methods may not be convetional as there are many various ways to parse a Solana transaction. Feedback and contributions are welcome!

## Key Features

- Extracts swap information from swap transactions
- Parsing methods:
  - Pumpfun and Jupiter: parsing the event data
  - Raydium, Orca, Meteora, and PumpSwap: parsing Transfer and TransferChecked methods of the token program
  - Moonshot: parsing the instruction data of the Trade instruction

## Installation

To install the solanaswap-go package, use the following command:

```bash
go get github.com/franco-bianco/solanaswap-go
```

## Usage

### 1. Import the Package

Begin by importing the `solanaswap-go` package into your Go project:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gagliardetto/solana-go/rpc"
	solana "github.com/gagliardetto/solana-go"
	solanaswapgo "github.com/franco-bianco/solanaswap-go/solanaswap-go"
)
```

### 2. Fetch and Parse a Solana Transaction

Use the package to fetch and parse a Solana transaction. Here's an example of how to use the `solanaswap-go` library to parse swap information from a transaction signature:

```go
func main() {
	// Set up RPC client for Solana mainnet
	rpcClient := rpc.New(rpc.MainNetBeta.RPC)

	// Replace with your actual transaction signature
	txSig := solana.MustSignatureFromBase58("2XYu86VrUXiwNNj8WvngcXGytrCsSrpay69Rt3XBz9YZvCQcZJLjvDfh9UWETFtFW47vi4xG2CkiarRJwSe6VekE")

	// Specify the maximum transaction version supported
	var maxTxVersion uint64 = 0

	// Fetch the transaction data using the RPC client
	tx, err := rpcClient.GetTransaction(
		context.TODO(),
		txSig,
		&rpc.GetTransactionOpts{
			Commitment:                     rpc.CommitmentConfirmed,
			MaxSupportedTransactionVersion: &maxTxVersion,
		},
	)
	if err != nil {
		log.Fatalf("Error fetching transaction: %s", err)
	}

	// Initialize the transaction parser
	parser, err := solanaswapgo.NewTransactionParser(tx)
	if err != nil {
		log.Fatalf("Error initializing transaction parser: %s", err)
	}

	// Parse the transaction to extract basic data
	transactionData, err := parser.ParseTransaction()
	if err != nil {
		log.Fatalf("Error parsing transaction: %s", err)
	}

	// Print the parsed transaction data
	marshalledData, _ := json.MarshalIndent(transactionData, "", "  ")
	fmt.Println(string(marshalledData))

	// Process and extract swap-specific data from the parsed transaction
	swapData, err := parser.ProcessSwapData(transactionData)
	if err != nil {
		log.Fatalf("Error processing swap data: %s", err)
	}

	// Print the parsed swap data
	marshalledSwapData, _ := json.MarshalIndent(swapData, "", "  ")
	fmt.Println(string(marshalledSwapData))
}
```

### 3. Output

The above code fetches a Solana transaction, parses its contents, and extracts swap-specific data. The `ProcessSwapData` function processes swap data and outputs it in JSON format.

#### Example Output

```json
{
  "Signers": [
    "4k8WHszi2uBzTiypTKUYH1hzYkUBCARPPn6ZjPNMhDoc"
  ],
  "Signatures": [
    "2XYu86VrUXiwNNj8WvngcXGytrCsSrpay69Rt3XBz9YZvCQcZJLjvDfh9UWETFtFW47vi4xG2CkiarRJwSe6VekE"
  ],
  "AMMs": [
    "Moonshot"
  ],
  "Timestamp": "0001-01-01T00:00:00Z",
  "TokenInMint": "CQn88snXCipTxn6DBbwgSA7d9v1sXPmyxzCNNiVNXzFy",
  "TokenInAmount": 59948049312246101,
  "TokenInDecimals": 9,
  "TokenOutMint": "So11111111111111111111111111111111111111112",
  "TokenOutAmount": 1711486459,
  "TokenOutDecimals":
```

### Recent Updates

- Added support for PumpSwap AMM transactions
- Improved transaction type handling for different swap types
- Fixed type conversion issues for various data formats

## Note

- Custom program swap transactions are not yet supported due to the outer instruction check
- Transaction timestamp is not included in `SwapInfo` response (should get this from block)
- Improvements could be made for `splTokenInfoMap` and `splDecimalsMap` use-case and logic

## Supported AMMs

- Raydium (V4, Route, CPMM, ConcentratedLiquidity)
- Orca
- Meteora (DLMM and Pools)
- PumpSwap (PumpFun AMM Program)
- MoonShot
- Pumpfun
- Jupiter
- OKX Dex Router

## Supported Sniper Trading Bots

- BananaGun Bot
- Bloom Bot
- MinTech
- Maestro
- Nova Bot
