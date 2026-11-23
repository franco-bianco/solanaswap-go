package solanaswap

import "github.com/gagliardetto/solana-go"

// Jupiter v6 events. Older routes emit one SwapEvent per hop
// (amm, input_mint, input_amount, output_mint, output_amount); the v2 route
// instructions emit a single SwapsEvent holding a vector of
// (input_mint, input_amount, output_mint, output_amount, amm).
var (
	jupiterSwapEvent  = discriminator{0x40, 0xc6, 0xcd, 0xe8, 0x26, 0x08, 0x71, 0xe2}
	jupiterSwapsEvent = discriminator{0x98, 0x2f, 0x4e, 0xeb, 0xc0, 0x60, 0x6e, 0x6a}
)

// decodeJupiter decodes the per-hop events of a Jupiter route. They report
// the amounts Jupiter measured around each DEX invocation, which also covers
// venues this package has no dedicated support for.
func decodeJupiter(e *extractor, ix *instruction) []Swap {
	trader := e.jupiterTrader(ix)
	var swaps []Swap
	for _, data := range cpiEvents(ix, jupiterSwapEvent) {
		r := newBorshReader(data)
		s := Swap{Trader: trader}
		s.ProgramID = r.pubkey()
		s.InputMint, s.InputAmount = r.pubkey(), r.u64()
		s.OutputMint, s.OutputAmount = r.pubkey(), r.u64()
		if r.ok {
			swaps = append(swaps, s)
		}
	}
	for _, data := range cpiEvents(ix, jupiterSwapsEvent) {
		r := newBorshReader(data)
		for range r.u32() {
			s := Swap{Trader: trader}
			s.InputMint, s.InputAmount = r.pubkey(), r.u64()
			s.OutputMint, s.OutputAmount = r.pubkey(), r.u64()
			s.ProgramID = r.pubkey()
			if !r.ok {
				break
			}
			swaps = append(swaps, s)
		}
	}
	return swaps
}

// jupiterTrader returns the first transaction signer that funds the route,
// falling back to the fee payer.
func (e *extractor) jupiterTrader(ix *instruction) solana.PublicKey {
	for _, t := range e.v.subtreeTransfers(ix) {
		if e.v.signers[t.authority] {
			return t.authority
		}
	}
	return e.v.feePayer()
}
