package solanaswap

import (
	"github.com/gagliardetto/solana-go"

	"github.com/franco-bianco/solanaswap-go/v2/internal/borsh"
	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

var pumpFunTradeEvent = txview.Discriminator{189, 219, 127, 211, 78, 230, 97, 238}

// legacyPumpFunFeeBps is the fixed protocol fee charged before TradeEvent reported fees.
const legacyPumpFunFeeBps = 100

// pumpFunTrade holds the TradeEvent fields the parser uses. Fields after the reserves were added
// over time and decode as zero in older transactions.
type pumpFunTrade struct {
	mint        solana.PublicKey
	solAmount   uint64
	tokenAmount uint64
	isBuy       bool
	user        solana.PublicKey

	hasFees    bool   // false before fee fields existed (legacy fixed fee)
	fee        uint64 // total protocol fee, including the buyback share
	creatorFee uint64

	quoteMint   solana.PublicKey // set for coins quoted in a token other than SOL
	quoteAmount uint64
}

func parsePumpFunTrade(data []byte) (pumpFunTrade, bool) {
	r := borsh.NewReader(data)
	t := pumpFunTrade{
		mint:        r.Pubkey(),
		solAmount:   r.U64(),
		tokenAmount: r.U64(),
		isBuy:       r.Bool(),
		user:        r.Pubkey(),
	}
	r.I64() // timestamp
	r.U64() // virtual_sol_reserves
	r.U64() // virtual_token_reserves
	if !r.OK() {
		return t, false
	}

	r.U64()    // real_sol_reserves
	r.U64()    // real_token_reserves
	r.Pubkey() // fee_recipient
	r.U64()    // fee_basis_points
	t.fee = r.U64()
	r.Pubkey() // creator
	r.U64()    // creator_fee_basis_points
	t.creatorFee = r.U64()
	if !r.OK() {
		return pumpFunTrade{
			mint: t.mint, solAmount: t.solAmount, tokenAmount: t.tokenAmount, isBuy: t.isBuy, user: t.user,
		}, true
	}
	t.hasFees = true

	r.Bool()              // track_volume
	r.U64()               // total_unclaimed_tokens
	r.U64()               // total_claimed_tokens
	r.U64()               // current_sol_volume
	r.I64()               // last_update_timestamp
	r.Str()               // ix_name
	r.Bool()              // mayhem_mode
	r.U64()               // cashback_fee_basis_points
	r.U64()               // cashback
	r.U64()               // buyback_fee_basis_points
	r.U64()               // buyback_fee (part of fee)
	r.Skip(r.U32(), 32+2) // shareholders: address, share_bps
	quoteMint := r.Pubkey()
	quoteAmount := r.U64()
	if r.OK() {
		t.quoteMint, t.quoteAmount = quoteMint, quoteAmount
	}
	return t, true
}

// fees returns the fees charged on top of a buy or deducted from a sell's curve amount.
func (t pumpFunTrade) fees(curveAmount uint64) uint64 {
	if !t.hasFees {
		return curveAmount * legacyPumpFunFeeBps / 10_000
	}
	return t.fee + t.creatorFee
}

// decodePumpFun decodes bonding-curve trades from TradeEvent. Amounts come from the user's
// transfers; SOL sells move lamports directly, so they use the event amount minus fees.
func (e *extractor) decodePumpFun(ix *txview.Instruction) []Swap {
	// Emitted through self-CPI since August 2024; older transactions only log it.
	events := ix.CPIEvents(pumpFunTradeEvent)
	if len(events) == 0 {
		events = e.v.LogEvents(ix, pumpFunTradeEvent)
	}
	paidOut, paidIn := e.userFlows(ix)
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
			paid := sumOf(paidOut, quoteMint)
			if paid == 0 {
				paid = quoteAmount + t.fees(quoteAmount)
			}
			s.InputMint, s.InputAmount = quoteMint, paid
			s.OutputMint, s.OutputAmount = t.mint, t.tokenAmount
		} else {
			received := sumOf(paidIn, quoteMint)
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
