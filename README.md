# solanaswap-go

[![Go Reference](https://pkg.go.dev/badge/github.com/franco-bianco/solanaswap-go/v2.svg)](https://pkg.go.dev/github.com/franco-bianco/solanaswap-go/v2)

Extracts swaps from confirmed Solana transactions: which token the trader
paid, which token they received, through which venues, and what they paid in
trading-bot fees and tips.

> **Project status:** v2 is the final major version. The repository is
> maintained in read-only (archived) mode; pin the release you depend on.
> See [Versions](#versions).

## Install

```bash
go get github.com/franco-bianco/solanaswap-go/v2@v2.0.0
```

Requires Go 1.26 or newer (the minimum of `github.com/gagliardetto/solana-go`
v1.26).

## Usage

```go
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/franco-bianco/solanaswap-go/v2"
)

func main() {
	client := rpc.New(rpc.MainNetBeta_RPC)
	sig := solana.MustSignatureFromBase58("3HZzfDRBXQxSFEBqan25GMmMLAdthek72RX6P1QYTsePmUWMK4r2f5t4WRwh1JkAuge7BNRQ1xxFmVAnfTmbgnVc")

	maxVersion := uint64(1) // accept legacy, v0 and v1 transactions
	tx, err := client.GetTransaction(context.Background(), sig, &rpc.GetTransactionOpts{
		Encoding:                       solana.EncodingBase64,
		Commitment:                     rpc.CommitmentConfirmed,
		MaxSupportedTransactionVersion: &maxVersion,
	})
	if err != nil {
		log.Fatal(err)
	}

	info, err := solanaswap.Parse(tx)
	if err != nil {
		log.Fatal(err) // errors.Is(err, solanaswap.ErrNoSwap) for non-swap transactions
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(info)
}
```

Other entry points:

- `solanaswap.ParseJSON(data)` parses the JSON of a `getTransaction` result
  (or a full JSON-RPC response), e.g. from a file, webhook or queue.
- `solanaswap.ParseTransaction(tx, meta)` parses an already decoded
  `*solana.Transaction` with its `*rpc.TransactionMeta`, e.g. converted from a
  Geyser / Yellowstone gRPC stream.

### Output

```json
{
  "Signatures": ["3HZzfDRBXQxSFEBqan25GMmMLAdthek72RX6P1QYTsePmUWMK4r2f5t4WRwh1JkAuge7BNRQ1xxFmVAnfTmbgnVc"],
  "Signers": ["AZ47eJFFAYCjU51QWn6iazXCUWFkQoHnRSJR3tp2DGsP"],
  "Slot": 358490736,
  "Timestamp": "2025-08-07T15:14:07Z",
  "Trader": "AZ47eJFFAYCjU51QWn6iazXCUWFkQoHnRSJR3tp2DGsP",
  "AMMs": ["Meteora DLMM"],
  "Routers": ["Jupiter"],
  "TokenInMint": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
  "TokenInAmount": 138404396088,
  "TokenInDecimals": 6,
  "TokenOutMint": "Preb5VKsmKgMGhMKUhDpe7A2AhMDmrZtMMZmvFEhLbU",
  "TokenOutAmount": 201138329611,
  "TokenOutDecimals": 9,
  "Swaps": [
    {
      "Protocol": "Meteora DLMM",
      "ProgramID": "LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo",
      "Trader": "AZ47eJFFAYCjU51QWn6iazXCUWFkQoHnRSJR3tp2DGsP",
      "InputMint": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
      "InputAmount": 138404396088,
      "InputDecimals": 6,
      "OutputMint": "Preb5VKsmKgMGhMKUhDpe7A2AhMDmrZtMMZmvFEhLbU",
      "OutputAmount": 201138329611,
      "OutputDecimals": 9,
      "InstructionIndex": 3,
      "InnerInstructionIndex": -1
    }
  ],
  "TransactionFee": 16695,
  "Fees": null
}
```

Amounts are raw integer token amounts; divide by `10^decimals` for display.
Native SOL and wrapped SOL are both reported as
`So11111111111111111111111111111111111111112`.

What the amounts mean:

- `TokenInAmount` is what left the trader's side into the DEX instructions,
  including fees the DEX itself charges (LP, protocol, creator fees).
  `TokenOutAmount` is what the DEX instructions paid out.
- Multi-hop routes (A→B→C) report the first input and the final output; split
  routes (A→B twice) are summed. Every hop is listed in `Swaps`.
- Trading-bot and aggregator fees, and tips to Jito and other landing
  services, are **not** part of the swap amounts. They are listed in `Fees`
  (`"platform"` or `"tip"`). The network fee is `TransactionFee`.

### Command line

```bash
go run github.com/franco-bianco/solanaswap-go/v2/cmd/solanaswap@v2.0.0 -rpc "$SOLANA_RPC_URL" <signature>
go run github.com/franco-bianco/solanaswap-go/v2/cmd/solanaswap@v2.0.0 -file tx.json
```

The public `api.mainnet-beta.solana.com` endpoint is heavily rate limited and
blocks some clients; use your own RPC provider for anything beyond a quick test.

## How it works

1. The transaction's top-level and inner instructions are linked into a tree
   using the stack height the RPC node records, with address lookup tables
   resolved.
2. The tree is walked from the top, so a swap is found no matter which
   trading bot, aggregator or custom program wraps the DEX call.
3. Each invocation of a recognised DEX program becomes one `Swap`:
   - programs that emit trade events are decoded from the event (Pump.fun,
     Moonshot, Jupiter's per-hop events, ...), which is required where SOL
     moves by direct lamport changes rather than transfers;
   - other DEX programs are decoded from the SPL Token / Token-2022 / System
     transfers they execute. Transfers signed by the trader (or by a router
     holding the trader's funds) are the input; transfers paying out to the
     trader's side are the output.
4. Programs that are not in the registry but swap directly with a signer are
   detected from their transfers and reported by program ID.
5. If no swap instruction is found at all, the trader's balance changes are
   used, corrected for the network fee, tips, bot fees and account rent.

## Supported programs

`solanaswap.Programs()` returns the full registry. Unlisted venues are still
parsed through the generic paths above and reported by program ID.

<!-- supported-programs -->

## Versions

| Version  | Import path                                          | Notes |
|----------|------------------------------------------------------|-------|
| `v2.0.0` | `github.com/franco-bianco/solanaswap-go/v2`          | Final release. Instruction-tree engine, new API. |
| `v1.1.0` | `github.com/franco-bianco/solanaswap-go/solanaswap-go` | Final v1 release (last v1 commit). |
| `v1.0.0` | `github.com/franco-bianco/solanaswap-go/solanaswap-go` | |

### Migrating from v1

```go
// v1
parser, err := solanaswapgo.NewTransactionParser(tx)
data, err := parser.ParseTransaction()
info, err := parser.ProcessSwapData(data)

// v2
info, err := solanaswap.Parse(tx)
```

- `SwapInfo` keeps the v1 fields (`Signers`, `Signatures`, `AMMs`,
  `Timestamp`, `TokenIn*`, `TokenOut*`). `Timestamp` is now the block time
  instead of the parse time, and `AMMs` names the venue (e.g. `"Raydium CPMM"`,
  `"Meteora DLMM"`) rather than a family; aggregators and bots are in
  `Routers`.
- New fields: `Slot`, `Trader`, `Routers`, `Swaps`, `TransactionFee`, `Fees`.
- Bot fees are no longer added to the swap amount (they are in `Fees`).
- The library no longer logs; failures are returned as errors
  (`ErrNoSwap`, `ErrTransactionFailed`, `ErrInvalidTransaction`).

## Development

```bash
go test ./...
```

Tests run against real mainnet transactions stored in
`testdata/transactions`, with expected results in `testdata/cases.json`
derived independently from each transaction's balances and inner
instructions.
