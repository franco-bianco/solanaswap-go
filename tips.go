package solanaswap

import "github.com/gagliardetto/solana-go"

// tipAccounts are the published tip accounts of block engines and
// transaction-landing services. Payments to them are reported as FeeKindTip.
var tipAccounts = keySet(
	// Jito
	"96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5",
	"HFqU5x63VTqvQss8hp11i4wVV8bD44PvwucfZ2bU7gRe",
	"Cw8CFyM9FkoMi7K7Crf6HNQqf4uEMzpKw6QNghXLvLkY",
	"ADaUMid9yfUytqMBgopwjb2DTLSokTSzL1zt6iGPaS49",
	"DfXygSm4jCyNCybVYYK6DwvWqjKee8pbDmJGcLWNDXjh",
	"ADuUkR4vqLUMWXxW9gh6D6L8pMSawimctcNZ5pGwDcEt",
	"DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL",
	"3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKZ6jT",
)

func keySet(addrs ...string) map[solana.PublicKey]bool {
	m := make(map[solana.PublicKey]bool, len(addrs))
	for _, a := range addrs {
		m[solana.MustPublicKeyFromBase58(a)] = true
	}
	return m
}
