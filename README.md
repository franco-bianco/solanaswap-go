# SOL Transaction Parser

## Features

- Extracts and decodes Trade and Create events for PumpFun transactions
- Parses Raydium swap transactions and returns the SwapInfo
- Parses Jupiter swap transactions and returns the SwapInfo

## Recent Add

- Added Raydium transaction parsing
- Added Pumpfun transaction parsing
- Added standardized SwapInfo return

## Usage

To run the Jupiter parser:

```bash
go run example/jupiter/main.go
```

To run the Pumpfun Events parser:

```bash
go run example/pumpfun/main.go
```

To run the Raydium parser:

```bash
go run example/raydium/main.go
```
