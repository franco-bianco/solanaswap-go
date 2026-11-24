package solanaswap

import (
	"github.com/gagliardetto/solana-go"

	"github.com/franco-bianco/solanaswap-go/v2/internal/borsh"
	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

// Per-hop events of Jupiter v6, also emitted byte-identically by DFlow.
var (
	// SwapEvent(amm, in_mint, in_amt, out_mint, out_amt), one per hop.
	swapEvent = txview.Discriminator{0x40, 0xc6, 0xcd, 0xe8, 0x26, 0x08, 0x71, 0xe2}
	// SwapsEvent(vec of in_mint, in_amt, out_mint, out_amt, amm), one per route.
	swapsEvent = txview.Discriminator{0x98, 0x2f, 0x4e, 0xeb, 0xc0, 0x60, 0x6e, 0x6a}
)

// decodeSwapEvents decodes the per-hop events of a Jupiter or DFlow route, which also cover
// venues this package does not recognise.
func (e *extractor) decodeSwapEvents(ix *txview.Instruction) []Swap {
	trader := e.routeTrader(ix)
	var swaps []Swap
	add := func(s Swap) {
		// v2 routes can report hops that moved nothing.
		if s.InputAmount > 0 && s.OutputAmount > 0 {
			s.Trader = trader
			swaps = append(swaps, s)
		}
	}
	for _, data := range ix.CPIEvents(swapEvent) {
		r := borsh.NewReader(data)
		var s Swap
		s.ProgramID = r.Pubkey()
		s.InputMint, s.InputAmount = r.Pubkey(), r.U64()
		s.OutputMint, s.OutputAmount = r.Pubkey(), r.U64()
		if r.OK() {
			add(s)
		}
	}
	for _, data := range ix.CPIEvents(swapsEvent) {
		r := borsh.NewReader(data)
		for range r.U32() {
			var s Swap
			s.InputMint, s.InputAmount = r.Pubkey(), r.U64()
			s.OutputMint, s.OutputAmount = r.Pubkey(), r.U64()
			s.ProgramID = r.Pubkey()
			if !r.OK() {
				break
			}
			add(s)
		}
	}
	return swaps
}

// routeTrader returns the first signer that funds the route, else the fee payer.
func (e *extractor) routeTrader(ix *txview.Instruction) solana.PublicKey {
	for _, t := range ix.Transfers() {
		if e.v.IsSigner(t.Authority) {
			return t.Authority
		}
	}
	return e.v.FeePayer()
}

// Jupiter's order engine settles RFQ quotes with fill(input_amount, output_amount, expire_at)
// and accounts [taker, maker, ...]. Both sign, so only the instruction names the taker.
var jupiterRFQFill = txview.Discriminator{0xa8, 0x60, 0xb7, 0xa3, 0x5c, 0x0a, 0x28, 0xa0}

const jupiterRFQTaker = 0

func (e *extractor) decodeJupiterRFQ(ix *txview.Instruction) []Swap {
	if !jupiterRFQFill.Matches(ix.Data) {
		return nil
	}
	r := borsh.NewReader(ix.Data[8:])
	inAmount, outAmount := r.U64(), r.U64()
	taker := ix.Account(jupiterRFQTaker)
	if !r.OK() || taker.IsZero() {
		return nil
	}
	var inMint, outMint solana.PublicKey
	for _, t := range ix.Transfers() {
		switch {
		case inMint.IsZero() && t.Authority == taker:
			inMint = t.Mint
		case outMint.IsZero() && t.Authority != taker:
			outMint = t.Mint
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
