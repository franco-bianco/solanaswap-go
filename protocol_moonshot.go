package solanaswap

// Moonshot (Moonit) logs a TradeEvent with emit!:
// amount: u64, collateral_amount: u64, dex_fee: u64, helio_fee: u64,
// allocation: u64, curve: pubkey, cost_token: pubkey, sender: pubkey,
// trade_type: u8 (0 buy, 1 sell), label: string.
// It shares its discriminator with Pump.fun's TradeEvent (both are
// sha256("event:TradeEvent")).
var moonshotTradeEvent = pumpFunTradeEvent

// moonshotMintAccount is the index of the token mint in buy and sell.
const moonshotMintAccount = 6

// decodeMoonshot decodes Moonshot bonding-curve trades. Sells pay SOL out by
// moving lamports directly, so the output is the event's collateral minus
// the dex and Helio fees the program deducts.
func decodeMoonshot(e *extractor, ix *instruction) []Swap {
	mint := ix.account(moonshotMintAccount)
	var swaps []Swap
	for _, data := range e.v.logEventsOf(ix, moonshotTradeEvent) {
		r := newBorshReader(data)
		tokens, collateral, dexFee, helioFee := r.u64(), r.u64(), r.u64(), r.u64()
		r.u64()    // allocation
		r.pubkey() // curve
		quote := r.pubkey()
		sender := r.pubkey()
		isSell := r.u8() == 1
		if !r.ok || mint.IsZero() {
			continue
		}

		s := Swap{Trader: sender}
		if isSell {
			received := e.userInflow(ix, quote)
			if received == 0 {
				received = collateral - min(collateral, dexFee+helioFee)
			}
			s.InputMint, s.InputAmount = mint, tokens
			s.OutputMint, s.OutputAmount = quote, received
		} else {
			paid := e.userOutflow(ix, quote)
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
