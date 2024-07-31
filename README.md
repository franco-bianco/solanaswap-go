# PumpFun Transaction Parser

A Solana transaction parser specifically designed for PumpFun transactions. It fetches transaction data from the Solana blockchain, identifies PumpFun-related transactions, and extracts relevant event information such as trades and token creations. The parser is written in Go and uses the Solana-go library for interacting with the Solana blockchain.

## Key features

- Fetches parsed transaction data from Solana's RPC
- Identifies PumpFun transactions
- Extracts and decodes Trade and Create events
