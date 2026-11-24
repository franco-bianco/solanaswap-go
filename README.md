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
go get github.com/franco-bianco/solanaswap-go/v2@v2.0.1
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
`So11111111111111111111111111111111111111112`. The JSON uses Go field names
as in v1, and amounts are JSON numbers that can exceed 2^53, so decode them
with a 64-bit-safe parser outside Go.

What the amounts mean:

- `TokenInAmount` is what left the trader's side into the DEX instructions,
  including fees the DEX itself charges (LP, protocol, creator fees).
  `TokenOutAmount` is what the DEX instructions paid out.
- Multi-hop routes (A→B→C) report the first input and the final output; split
  routes (A→B twice) are summed. Every hop is listed in `Swaps`.
- Trading-bot and aggregator fees, and tips to Jito and other landing
  services, are **not** part of the swap amounts. They are listed in `Fees`
  (`"platform"` or `"tip"`). The network fee is `TransactionFee`.
- `Trader` is the wallet whose tokens were swapped. It is usually the fee
  payer, but for relayer-paid and gasless swaps it is the signing user, and
  for keeper-filled DCA and limit orders it is the order owner.
- `InstructionIndex` and `InnerInstructionIndex` locate the instruction a
  swap was decoded from (`-1` for the top level). Hops decoded from an
  aggregator's events point at the aggregator instruction, and swaps inferred
  from balance changes have both set to `-1`.

### Command line

```bash
go run github.com/franco-bianco/solanaswap-go/v2/cmd/solanaswap@v2.0.1 -rpc "$SOLANA_RPC_URL" <signature>
go run github.com/franco-bianco/solanaswap-go/v2/cmd/solanaswap@v2.0.1 -file tx.json
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
4. Programs that are not in the registry are treated as venues when an
   aggregator calls them, or when they swap directly with a signer and wrap
   no recognised program; they are reported by program ID.
5. If no swap instruction is found at all, the signers' balance changes are
   used, corrected for the network fee, tips, bot fees and account rent.

Limitations: a transaction with several unrelated swaps is summarised as one
route (see `Swaps` for the parts), amounts are gross of Token-2022 transfer
fees, and bots that charge fees through unregistered programs are not
labelled.

## Supported programs

`solanaswap.Programs()` returns the full registry. Unlisted venues are still
parsed through the generic paths above and reported by program ID.

### Launchpads and bonding curves

| Name | Program ID |
|---|---|
| Pump.fun | `6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P` |
| PumpSwap | `pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA` |
| Moonshot | `MoonCVVNZFSYkqNXP6bxHLPL6QQJiMagDL3qcqUQTrG` |
| Raydium LaunchLab | `LanMV9sAd7wArD4vJFi2qDdfnVhFxYSUg6eADduJ3uj` |
| Meteora DBC | `dbcij3LWUppWqq96dh6gJWwBifmcGfLSB5D4DuSMaqN` |
| Boop.fun | `boop8hVGQGqehUK2iVEMEnMrL5RbjywRzHKBmBE7ry4` |
| Heaven | `HEAVENoP2qxoeuF8Dj2oT1GHEnu49U5mJYkdeC8BAX2o` |
| Vertigo | `vrTGoBuy5rYSxAfV3jaRJWHH6nN9WK4NRExGxsk1bCJ` |

### AMMs and CLMMs

| Name | Program ID |
|---|---|
| Raydium AMM v4 | `675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8` |
| Raydium CPMM | `CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C` |
| Raydium CLMM | `CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK` |
| Orca Whirlpool | `whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc` |
| Meteora DLMM | `LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo` |
| Meteora DAMM v1 | `Eo7WjKq67rjJQSZxS6z3YkapzY3eMj6Xy8X5EQVn5UaB` |
| Meteora DAMM v2 | `cpamdpZCGKUy5JxQXB4dcpGPiikHawvSWAd6mEn1sGG` |
| Raydium Stable | `5quBtoiQqxF9Jv6KYKctB59NT3gtJD2Y65kdnB1Uev3h` |
| Orca Token Swap v2 | `9W959DqEETiGZocYWCQPaJ6sBmUzgfxXfqGeTEdp3aQP` |
| Meteora DLMM Pro | `dPro1D5wiqnGMH8vMDG23jh35btTHoWdyqXjqkqxe8n` |
| PancakeSwap | `HpNfyc2Saw7RKkQd8nEL4khUcuPhQ7WwY1B2qjx8jxFq` |
| Byreal | `REALQqNEomY6cQGZJUGwywTBD2UmDT32rZcNnfxQ5N2` |
| DefiTuna | `fUSioN9YKKSa3CUC2YUc4tPkHJ5Y6XW1yz8y6F7qWz9` |
| Stabble Stable Swap | `swapNyd8XiQwJ6ianp9snpu4brUqFxadzvHebnAXjJZ` |
| Stabble Weighted Swap | `swapFpHZwjELNnjvThjajtiVmkz3yPQEHjLtka2fwHW` |
| Saber | `SSwpkEEcbUqx4vtoEByFjSkhKdCT862DNVb52nZg1UZ` |
| Mercurial | `MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky` |
| Stableswap | `ghosty4ZU1Qk1HN7Ymz4pZ15QfspzJZgSYFkdKN6ZLK` |
| Perena | `NUMERUNsFCP3kuNmWZuXtm1AaQCPj9uw6Guv2Ekoi5P` |
| Sanctum Infinity | `5ocnV1qiCgaQR8Jb8xWnVbApfaygJ8tNoZfgPwsgx9kx` |
| FluxBeam | `FLUXubRmkEi2q6K3Y9kBPg9248ggaZVsoSFhtJHSrm1X` |
| GooseFX Gamma | `GAMMA7meSFWaBXF25oSUgmGRwaW6sCMFLmBNiMSdbHVT` |
| Saros | `SSwapUtytfBdBn1b9NUGG6foMVPtcWgpRU32HToDUZr` |
| Invariant | `HyaB3W9q6XdA5xwpU4XnSZV94htfmbmqJXZcEbRaJutt` |
| Crema | `CLMM9tUoggJu2wagPkkqs9eFG4BWhVBZWkP1qv3Sp7tR` |
| Lifinity v2 | `2wT8Yq49kHgDzXuPxZSaeLaH1qbmGXtEyPy64bL7aD3c` |
| Woofi | `WooFif76YGRNjk1pA8wCsN67aQsD9f9iLsz4NcJ1AVb` |

### Order books

| Name | Program ID |
|---|---|
| Phoenix | `PhoeNiXZ8ByJGLkxNfZRnkUfjvmuYqLR89jjFHGqdXY` |
| OpenBook v2 | `opnb2LAfJYbRMAHHvqjCwQxanZn7ReEHp1k81EohpZb` |
| Manifest | `MNFSTqtC93rEfYHB6hF82sKdZpUDFWkViLByLd1k1Ms` |
| Archer | `Archer8kgiavM61GyusMzaaS2ft5sALtNsD1HxkUPMhy` |
| Deriverse | `DRVSpZ2YUYYKgZP8XtLhAGtT1zYSCKzeHfb4DgRnrgqD` |
| 1DEX | `DEXYosS6oEGvk8uCDayvwEZz4qEyDJRf9nFgYCaqPMTm` |

### Proprietary market makers and RFQ

| Name | Program ID |
|---|---|
| HumidiFi | `9H6tua7jkLhdm3w8BvgpTn5LZNU7g4ZynDmCiNN3q6Rp` |
| SolFi v2 | `SV2EYYJyRz2YhfXwXnhNAevDEui5Q6yrfyo13WtupPF` |
| SolFi | `SoLFiHG9TfgtdUXUjWAxi3LtvYuFyDLVhBWxdMZxyCe` |
| Tessera V | `TessVdML9pBGgG9yGks7o4HewRaXVAMuoVj4x83GLQH` |
| GoonFi v2 | `goonuddtQRrWqqn5nFyczVKaie28f3kDkHWkHtURSLE` |
| ZeroFi | `ZERor4xhbUycZ6gb9ntrhqscUcZmAbQDjEAtCf4hbZY` |
| Obric v2 | `obriQD1zbpyLz95G5n7nJe6a4DPjpFwa5XYPoNm113y` |
| AlphaQ | `ALPHAQmeA7bjrVuccPsYPiCvsi428SNwte66Srvs4pHA` |
| BisonFi | `BiSoNHVpsVZW2F7rx2eQ59yQwKxzU5NvBcmKshCSUypi` |
| Aquifer | `AQU1FRd7papthgdrwPTTq5JacJh8YtwEXaBfKU3bTz45` |
| Scorch | `SCoRcH8c2dpjvcJD6FiPbCSQyQgu3PcUAWj2Xxx3mqn` |
| Hadron | `HADRoNbLovyqhCsocfYQYB7QdfCAAinN9HTePvBCVDQ8` |
| Kipseli | `3TK9D8aoBFYjYZtKCjciPrVrRStsnvo7KmpcJqDavpaU` |
| Obsidian | `HBVw6bZtcCaezhcBrmfyXBSBRWCdv72271xQ4GPvms2z` |
| Riptide | `riptK81hDxhe5pW5jSzSM9iRA8azgEgLJ4dXkPtBS7j` |
| Denali | `DNL1tgEj3nJovHw9jtyCCQD3arssCJzkmpDizknwzey4` |
| Jupiter RFQ | `61DFfeTKM7trxYcPQCM78bJ794ddZprZpAwAnLiwTpYH` |

### Aggregators and order programs

| Name | Program ID |
|---|---|
| Jupiter | `JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4` |
| DFlow | `DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH` |
| OKX DEX Router v2 | `proVF4pMXVaYqmy4NjniPh4pqKNfMmsihgd4wdkCX3u` |
| OKX DEX Router (closed 2026-07) | `6m2CDdhRgxpH4WjvdzxAYbGxwdGUz5MziiL5jek2kBma` |
| Titan | `T1TANpTeScyeqVzzgNViGDNrkQ6qHz9KrSBS4aNXvGT` |
| Raydium Router | `routeUGWgWzqBWFcrCfv8tritsqukccJPu3q5GPP3xS` |
| Jupiter Recurring | `DCA265Vj8a9CEuX1eb1LWRnDT7uK6q1xMipnNyatn23M` |
| Jupiter DCA (retired) | `DCAK36VfExkPdAkYUQg6ewgxyinvcEyPLyHjRbmveKFw` |
| Jupiter Trigger | `j1o2qRpjcyUwEvwtcfhEQefh773ZgjxcVRry7LDqg5X` |
| Jupiter Limit Order | `jupoNjAxXgZ4rjzxzPMP4oxduvQsQtZzyknqvzYNrNu` |

### Trading bots

| Name | Program ID |
|---|---|
| Axiom | `FLASHX8DrLbgeR8FcfNV1F5krxYcYMUdBkrP1EPBtxB9` |
| GMGN | `GMgnVFR8Jb39LoXsEVzb3DvBy3ywCmdmJquHUy1Lrkqb` |
| Photon | `BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW` |
| Trojan | `troyXT7Ty3s2rjJe4bqWaroUrS4Fjd8rbHHNHxcACF4` |
| Trojan (retired 2025) | `DM2dmBUi2he1bot4xF7PaSGLorUjWLm2DMFsWiTPuXzb` |
| BonkBot | `BBRouter1cVunVXvkcqeKkZQcBK7ruan37PPm3xzWaXD` |
| BonkBot | `CxvksNjwhdHDLr3qbCXNKVdeYACW8cs93vFqLqtgyFE5` |
| BonkBot (retired 2025) | `AnCKmodpxycRZoAFgVwGjWuxP9wJPamghfvUw1fwKdpK` |
| Banana Gun | `BANANAjs7FJiPQqJTGFzkZJndT9o7UmKiYYGaJz6frGu` |
| Maestro | `MaestroAAe9ge5HTc64VbBQZ6fP77pwvrhM8i1XWSAx` |
| Bloom | `b1oomGGqPKGD6errbyfbVMBuzSC8WtAAYo8MwNafWW1` |
| MinTech | `minTcHYRLVPubRK8nt6sqe2ZpWrGDLQoNLipDJCGocY` |
| Shuriken | `HEzM8TqVBvsgNKgqSbNkbL8J4Qq4DiRhz4NFXDKAKZiq` |
| Nova (retired 2026) | `NoVA1TmDUqksaj2hB1nayFkPysjJbFiU76dT4qPw2wm` |

### Bots recognised by their fee wallets

Several bots call the DEX directly from the user's wallet and only charge
their fee on chain, so they are labelled in `Routers` from the fee payment:
Photon, Trojan, BonkBot, Maestro, Bloom, Banana Gun, Axiom, GMGN, Phantom
(in-wallet swaps) and Fomo.

Tips to Jito, Helius Sender, Nozomi (Temporal), bloXroute, 0slot, NextBlock,
Astralane and Solana Vibe Station are reported as `"tip"` fees.


## Versions

| Version  | Import path                                          | Notes |
|----------|------------------------------------------------------|-------|
| `v2.0.1` | `github.com/franco-bianco/solanaswap-go/v2`          | Final release. Internal cleanup; same API and results as v2.0.0. |
| `v2.0.0` | `github.com/franco-bianco/solanaswap-go/v2`          | Instruction-tree engine, new API. |
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

Layout:

- the module root holds the public API (`parse.go`, `types.go`), the swap
  extraction engine (`extract.go`, `summary.go`), the program registry
  (`programs.go`, `wallets.go`) and protocol-specific decoders
  (`protocol_*.go`);
- `internal/txview` decodes a raw transaction: lookup-table accounts, the
  instruction tree, transfers, balance changes and emitted events;
- `internal/borsh` reads Borsh-encoded event data;
- `cmd/solanaswap` is the command-line tool.
