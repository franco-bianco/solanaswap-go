package solanaswap

import "github.com/gagliardetto/solana-go"

var pumpFunTradeEvent = discriminator{189, 219, 127, 211, 78, 230, 97, 238}

// pumpFunTrade holds the TradeEvent fields the parser uses. The event has
// grown over time; fields after virtualTokenReserves are absent in older
// transactions and decode as zero.
type pumpFunTrade struct {
	mint        solana.PublicKey
	solAmount   uint64
	tokenAmount uint64
	isBuy       bool
	user        solana.PublicKey

	// hasFees is false for events recorded before the fee fields were
	// added; the protocol fee was then a fixed 1% of solAmount.
	hasFees bool
	// fee is the total protocol fee, including the buyback share.
	fee        uint64
	creatorFee uint64
	// quoteMint and quoteAmount are set for coins quoted in a token other
	// than SOL.
	quoteMint   solana.PublicKey
	quoteAmount uint64
}

func parsePumpFunTrade(data []byte) (pumpFunTrade, bool) {
	r := newBorshReader(data)
	t := pumpFunTrade{
		mint:        r.pubkey(),
		solAmount:   r.u64(),
		tokenAmount: r.u64(),
		isBuy:       r.bool(),
		user:        r.pubkey(),
	}
	r.i64() // timestamp
	r.u64() // virtual_sol_reserves
	r.u64() // virtual_token_reserves
	if !r.ok {
		return t, false
	}

	r.u64()    // real_sol_reserves
	r.u64()    // real_token_reserves
	r.pubkey() // fee_recipient
	r.u64()    // fee_basis_points
	t.fee = r.u64()
	r.pubkey() // creator
	r.u64()    // creator_fee_basis_points
	t.creatorFee = r.u64()
	if !r.ok {
		return pumpFunTrade{mint: t.mint, solAmount: t.solAmount, tokenAmount: t.tokenAmount, isBuy: t.isBuy, user: t.user}, true
	}
	t.hasFees = true

	r.bool()   // track_volume
	r.u64()    // total_unclaimed_tokens
	r.u64()    // total_claimed_tokens
	r.u64()    // current_sol_volume
	r.i64()    // last_update_timestamp
	r.string() // ix_name
	r.bool()   // mayhem_mode
	r.u64()    // cashback_fee_basis_points
	r.u64()    // cashback
	r.u64()    // buyback_fee_basis_points
	r.u64()    // buyback_fee (part of fee)
	shareholders := r.u32()
	r.skip(int(shareholders) * (32 + 2)) // address, share_bps
	quoteMint := r.pubkey()
	quoteAmount := r.u64()
	if r.ok {
		t.quoteMint, t.quoteAmount = quoteMint, quoteAmount
	}
	return t, true
}

// legacyPumpFunFeeBps is the protocol fee that applied to every trade
// before TradeEvent reported fees.
const legacyPumpFunFeeBps = 100

// fees returns the fees the program charges on top of (buys) or deducts
// from (sells) the curve amount.
func (t pumpFunTrade) fees(curveAmount uint64) uint64 {
	if !t.hasFees {
		return curveAmount * legacyPumpFunFeeBps / 10_000
	}
	return t.fee + t.creatorFee
}

// decodePumpFun decodes Pump.fun bonding-curve trades from TradeEvent.
//
// Buys pay the quote through ordinary transfers, so the input is measured
// from the trader's outgoing transfers (curve price plus protocol and creator
// fees). Sells of SOL-quoted coins pay out by moving lamports directly, which
// leaves no transfer to measure, so the output is the event's curve amount
// minus the fees the program deducts.
func decodePumpFun(e *extractor, ix *instruction) []Swap {
	// Since August 2024 the event is emitted both through self-CPI and as a
	// log line; earlier transactions only have the log line.
	events := cpiEvents(ix, pumpFunTradeEvent)
	if len(events) == 0 {
		events = e.v.logEventsOf(ix, pumpFunTradeEvent)
	}
	var swaps []Swap
	for _, data := range events {
		t, ok := parsePumpFunTrade(data)
		if !ok {
			continue
		}
		quoteMint, quoteAmount := solana.WrappedSol, t.solAmount
		if !t.quoteMint.IsZero() && t.quoteMint != solana.WrappedSol {
			quoteMint, quoteAmount = t.quoteMint, t.quoteAmount
		}

		s := Swap{Trader: t.user}
		if t.isBuy {
			paid := e.userOutflow(ix, quoteMint)
			if paid == 0 {
				paid = quoteAmount + t.fees(quoteAmount)
			}
			s.InputMint, s.InputAmount = quoteMint, paid
			s.OutputMint, s.OutputAmount = t.mint, t.tokenAmount
		} else {
			received := e.userInflow(ix, quoteMint)
			if received == 0 {
				received = quoteAmount - min(quoteAmount, t.fees(quoteAmount))
			}
			s.InputMint, s.InputAmount = t.mint, t.tokenAmount
			s.OutputMint, s.OutputAmount = quoteMint, received
		}
		swaps = append(swaps, s)
	}
	return swaps
}

// userOutflow sums transfers of mint inside ix that a user-side authority
// sends to a non-user account.
func (e *extractor) userOutflow(ix *instruction, mint solana.PublicKey) uint64 {
	var sum uint64
	for _, t := range e.v.subtreeTransfers(ix) {
		if t.mint == mint && e.isUserSide(t.authority) && !e.isUserSide(t.destination) {
			sum += t.amount
		}
	}
	return sum
}

// userInflow sums transfers of mint inside ix that pay a user-side account
// from a non-user authority.
func (e *extractor) userInflow(ix *instruction, mint solana.PublicKey) uint64 {
	var sum uint64
	for _, t := range e.v.subtreeTransfers(ix) {
		if t.mint == mint && !e.isUserSide(t.authority) && e.isUserSide(t.destination) {
			sum += t.amount
		}
	}
	return sum
}
