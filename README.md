# Solana Swap Transaction Parser

Parses a Solana transaction and extracts the swap info, supports multiple AMMs. Please note that parsing methods may not be convetional as there are many various ways to parse a Solana transaction. Feedback and contributions are welcome!

## Key Features

- Extracts swap information from swap transactions
- Parsing methods:
  - Pumpfun and Jupiter: parsing the event data
  - Raydium, Orca, and Meteora: parsing Transfer and TransferChecked methods of the token program

## Note

- Custom program swap transactions are not yet supported due to an outer instruction check in line 76 of `parser.go`. Feel free to experiment by removing this check. An example of such a transaction is `46Jp5EEUrmdCVcE3jeewqUmsMHhqiWWtj243UZNDFZ3mmma6h2DF4AkgPE9ToRYVLVrfKQCJphrvxbNk68Lub9vw`.
- Transaction timestamp is not included in `SwapInfo` response (should get this from block)
- Improvements could be made for `splTokenInfoMap` and `splDecimalsMap` use-case and logic

## Supported AMMs

- Raydium (V4, Route and CPMM)
- Orca
- Meteora (DLMM and Pools)
- Pumpfun
- Jupiter

## Example Output

```json
{
  "Signers": ["AkQWv1Qnvua6zJch9JrFe8a9YVE4QxCkvc3dgmHvc4Qn"],
  "Signatures": ["5kaAWK5X9DdMmsWm6skaUXLd6prFisuYJavd9B62A941nRGcrmwvncg3tRtUfn7TcMLsrrmjCChdEjK3sjxS6YG9"],
  "AMMs": ["Raydium", "Raydium"],
  "Timestamp": "0001-01-01T00:00:00Z",
  "TokenInMint": "5bpj3W9zC2Y5Zn2jDBcYVscGnCBUN5RD7152cfL9pump",
  "TokenInAmount": 38202111872,
  "TokenInDecimals": 6,
  "TokenOutMint": "So11111111111111111111111111111111111111112",
  "TokenOutAmount": 1125839750,
  "TokenOutDecimals": 9
}
```

## Latest Update

- Added Meteora and Orca support
- Added classification of transaction types in `ParseTransaction` based on outer instruction progID and `checks.go`
- Added multiple signers and signatures to `SwapInfo` response
