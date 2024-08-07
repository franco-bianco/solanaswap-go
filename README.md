# SOL Transaction Parser

## Features

- Extracts and decodes Trade and Create events for PumpFun transactions
- Parses Raydium swap transactions and returns the SwapInfo
- Parses Jupiter swap transactions and returns the SwapInfo

## Recent Add

- Added Raydium transaction parsing
- Added Pumpfun transaction parsing
- Standardized SwapInfo return

## Usage

To run the Jupiter parser:

```bash
go run example/jupiter/main.go
```

To run the Raydium parser:

```bash
go run example/raydium/main.go
```

Jupiter/Raydium swap transaction parser returns the following response:

```json
{
  "Signature": "3zQKPvFSSfvZPBRACfTGcDEyzEEx2ZyuqrkLRjbPu8Sjh88euKjGyaBYt3EbRPHpSWh49hBMg6kuLynbx7XPcgTF",
  "Signers": [
    "GddtzNX1hbAdg2t76iCcn546oTirRwLn7SgR82UVVgQx"
  ],
  "TokenInMint": "So11111111111111111111111111111111111111112",
  "TokenInAmount": 0.577845325,
  "TokenOutMint": "4HT1b2ysGXdyD5vxemDKq25G2sj3xeh2SvE6XMhNpump",
  "TokenOutAmount": 47337.335701
}
```

To run the Pumpfun Events parser:

```bash
go run example/pumpfun/main.go
```

Pumpfun events transaction parser returns the following response:

```json
{
  "signature": "4kPxWuFqG6Jj5uutxv67K87DYuVrQukuBpP1UHbT7Hd16KUGA7fanQtZKgwTzE1HBK3WvzGHmRbhhadJTokLpchj",
  "Events": [
    {
      "Mint": "MJSwwzhTxfBKgVShhfDwyz7JEiSARUPRKFECLeNpump",
      "SolAmount": 20000001,
      "TokenAmount": 505816083029,
      "IsBuy": true,
      "User": "E1tT7KB5LKFuuzrcNEPmbCvd4aPeXtDtiTPgru1f5nqr",
      "Timestamp": 1722455139,
      "VirtualSolReserves": 35686249671,
      "VirtualTokenReserves": 902028135197117
    },
    {
      "Mint": "MJSwwzhTxfBKgVShhfDwyz7JEiSARUPRKFECLeNpump",
      "SolAmount": 20000000,
      "TokenAmount": 505816083029,
      "IsBuy": false,
      "User": "E1tT7KB5LKFuuzrcNEPmbCvd4aPeXtDtiTPgru1f5nqr",
      "Timestamp": 1722455139,
      "VirtualSolReserves": 35666249671,
      "VirtualTokenReserves": 902533951280146
    }
  ]
}
```
