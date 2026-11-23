package solanaswap

import "github.com/gagliardetto/solana-go"

// Per-hop swap events of Jupiter v6 (also emitted, byte-identical, by
// DFlow). Route instructions of the v1 family emit one SwapEvent per hop:
// (amm, input_mint, input_amount, output_mint, output_amount). The *_v2
// route instructions emit a single SwapsEvent with a vector of
// (input_mint, input_amount, output_mint, output_amount, amm).
var (
	swapEvent  = discriminator{0x40, 0xc6, 0xcd, 0xe8, 0x26, 0x08, 0x71, 0xe2}
	swapsEvent = discriminator{0x98, 0x2f, 0x4e, 0xeb, 0xc0, 0x60, 0x6e, 0x6a}
)

// decodeSwapEvents decodes the per-hop events of a Jupiter or DFlow route.
// They report the amounts the aggregator measured around each DEX
// invocation, which also covers venues this package does not recognise.
func decodeSwapEvents(e *extractor, ix *instruction) []Swap {
	trader := e.routeTrader(ix)
	var swaps []Swap
	add := func(s Swap) {
		// v2 routes can report hops that moved nothing.
		if s.InputAmount > 0 && s.OutputAmount > 0 {
			s.Trader = trader
			swaps = append(swaps, s)
		}
	}
	for _, data := range cpiEvents(ix, swapEvent) {
		r := newBorshReader(data)
		var s Swap
		s.ProgramID = r.pubkey()
		s.InputMint, s.InputAmount = r.pubkey(), r.u64()
		s.OutputMint, s.OutputAmount = r.pubkey(), r.u64()
		if r.ok {
			add(s)
		}
	}
	for _, data := range cpiEvents(ix, swapsEvent) {
		r := newBorshReader(data)
		for range r.u32() {
			var s Swap
			s.InputMint, s.InputAmount = r.pubkey(), r.u64()
			s.OutputMint, s.OutputAmount = r.pubkey(), r.u64()
			s.ProgramID = r.pubkey()
			if !r.ok {
				break
			}
			add(s)
		}
	}
	return swaps
}

// routeTrader returns the first transaction signer that funds the route,
// falling back to the fee payer.
func (e *extractor) routeTrader(ix *instruction) solana.PublicKey {
	for _, t := range e.v.subtreeTransfers(ix) {
		if e.v.signers[t.authority] {
			return t.authority
		}
	}
	return e.v.feePayer()
}

// Jupiter's order engine settles RFQ quotes (JupiterZ, used by Ultra):
// fill(input_amount: u64, output_amount: u64, expire_at: i64) with
// accounts [taker, maker, ...]. Both parties sign, so the transfer-based
// decoder cannot tell them apart; the instruction names the taker.
var jupiterRFQFill = discriminator{0xa8, 0x60, 0xb7, 0xa3, 0x5c, 0x0a, 0x28, 0xa0}

const jupiterRFQTaker = 0

func decodeJupiterRFQ(e *extractor, ix *instruction) []Swap {
	if !hasDiscriminator(ix.data, jupiterRFQFill) {
		return nil
	}
	r := newBorshReader(ix.data[8:])
	inAmount, outAmount := r.u64(), r.u64()
	taker := ix.account(jupiterRFQTaker)
	if !r.ok || taker.IsZero() {
		return nil
	}
	var inMint, outMint solana.PublicKey
	for _, t := range e.v.subtreeTransfers(ix) {
		switch {
		case inMint.IsZero() && t.authority == taker:
			inMint = t.mint
		case outMint.IsZero() && t.authority != taker:
			outMint = t.mint
		}
	}
	if inMint.IsZero() || outMint.IsZero() {
		return nil
	}
	return []Swap{{
		Trader:    taker,
		InputMint: inMint, InputAmount: inAmount,
		OutputMint: outMint, OutputAmount: outAmount,
	}}
}
