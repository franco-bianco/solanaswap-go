# PumpFun/Raydium Transaction Parser

A Solana transaction parser designed for PumpFun and Raydium transactions. It fetches transaction data from the Solana blockchain, identifies PumpFun and Raydium-related transactions, and extracts relevant event information such as trades, token creations, and transfers. The parser is written in Go and uses the Solana-go library for interacting with the Solana blockchain.

## Features

- Fetches parsed transaction data from Solana's RPC
- Identifies PumpFun and Raydium transactions
- Extracts and decodes Trade and Create events for PumpFun
- Parses Raydium transactions and extracts transfer information

## Recent Add

- Added Raydium transaction parsing functionality

## Usage

To run the Raydium parser:

```bash
go run example/raydium/main.go
```

<!-- I GENERATED THIS BY GPT -->