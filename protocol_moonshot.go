package solanaswap

import (
	"github.com/franco-bianco/solanaswap-go/v2/internal/borsh"
	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

// Moonshot logs TradeEvent(amount, collateral_amount, dex_fee, helio_fee, allocation, curve,
// cost_token, sender, trade_type, label); its discriminator equals Pump.fun's TradeEvent.
var moonshotTradeEvent = pumpFunTradeEvent

const moonshotMintAccount = 6 // index of the token mint in buy and sell

// decodeMoonshot decodes bonding-curve trades. Sells move SOL directly, so their output is the
// event's collateral minus the dex and Helio fees.
func (e *extractor) decodeMoonshot(ix *txview.Instruction) []Swap {
	mint := ix.Account(moonshotMintAccount)
	paidOut, paidIn := e.userFlows(ix)
	var swaps []Swap
	for _, data := range e.v.LogEvents(ix, moonshotTradeEvent) {
		r := borsh.NewReader(data)
		tokens, collateral, dexFee, helioFee := r.U64(), r.U64(), r.U64(), r.U64()
		r.U64()    // allocation
		r.Pubkey() // curve
		quote := r.Pubkey()
		sender := r.Pubkey()
		isSell := r.U8() == 1
		if !r.OK() || mint.IsZero() {
			continue
		}

		s := Swap{Trader: sender}
		if isSell {
			received := sumOf(paidIn, quote)
			if received == 0 {
				received = collateral - min(collateral, dexFee+helioFee)
			}
			s.InputMint, s.InputAmount = mint, tokens
			s.OutputMint, s.OutputAmount = quote, received
		} else {
			paid := sumOf(paidOut, quote)
			if paid == 0 {
				paid = collateral + dexFee + helioFee
			}
			s.InputMint, s.InputAmount = quote, paid
			s.OutputMint, s.OutputAmount = mint, tokens
		}
		swaps = append(swaps, s)
	}
	return swaps
}
