# PumpFun/Raydium Transaction Parser

A Solana transaction parser designed for PumpFun and Raydium transactions. It fetches transaction data from the Solana blockchain, identifies PumpFun and Raydium-related transactions, and extracts relevant event information such as trades, token creations, and transfers. The parser is written in Go and uses the Solana-go library for interacting with the Solana blockchain.

## Key features

- Fetches parsed transaction data from Solana's RPC
- Identifies PumpFun and Raydium transactions
- Extracts and decodes Trade and Create events for PumpFun
- Parses Raydium transactions and extracts transfer information
- Handles SPL Token transfers and identifies token mints

## Recent Add

- Added Raydium transaction parsing functionality

## Usage

To run the Raydium parser:

```bash
go run example/raydium/main.go
```

You will still have to obtain the swap information from the response, but it is pretty self-explanatory
