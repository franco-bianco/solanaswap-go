package solanaswap

import (
	"slices"

	"github.com/gagliardetto/solana-go"
)

// summarize fills the route-level fields of info from its swaps.
//
// TokenIn is the first swap's input and TokenOut the last swap's output.
// Amounts are netted across hops so split routes (A→B twice) add up and
// intermediate tokens of multi-hop routes (A→B→C) cancel out. When TokenIn
// and TokenOut are the same mint (arbitrage), the gross amounts are used.
func summarize(info *SwapInfo, swaps []Swap) {
	first, last := swaps[0], swaps[len(swaps)-1]
	info.TokenInMint, info.TokenInDecimals = first.InputMint, first.InputDecimals
	info.TokenOutMint, info.TokenOutDecimals = last.OutputMint, last.OutputDecimals

	var spentIn, receivedIn, spentOut, receivedOut uint64
	for _, s := range swaps {
		if s.InputMint == info.TokenInMint {
			spentIn += s.InputAmount
		}
		if s.OutputMint == info.TokenInMint {
			receivedIn += s.OutputAmount
		}
		if s.InputMint == info.TokenOutMint {
			spentOut += s.InputAmount
		}
		if s.OutputMint == info.TokenOutMint {
			receivedOut += s.OutputAmount
		}
		if !slices.Contains(info.AMMs, s.Protocol) {
			info.AMMs = append(info.AMMs, s.Protocol)
		}
	}

	info.TokenInAmount, info.TokenOutAmount = spentIn, receivedOut
	if info.TokenInMint != info.TokenOutMint {
		info.TokenInAmount = netOr(spentIn, receivedIn, first.InputAmount)
		info.TokenOutAmount = netOr(receivedOut, spentOut, last.OutputAmount)
	}
	if info.Trader.IsZero() {
		info.Trader = first.Trader
	}
}

// netOr returns a-b, or fallback when that would not be positive.
func netOr(a, b, fallback uint64) uint64 {
	if a > b {
		return a - b
	}
	return fallback
}

// collectFees returns payments from the user side (the trader, or a router
// holding the trader's funds in transit) made outside of any swap
// instruction to accounts outside the user side: trading-bot and aggregator
// fees, and tips.
func (e *extractor) collectFees() []Fee {
	var fees []Fee
	for _, ix := range e.v.flat {
		t, ok := e.v.decodeTransfer(ix)
		if !ok || t.amount == 0 || e.isConsumed(ix) || !e.isUserSide(t.authority) {
			continue
		}
		recipient := e.v.ownerOf(t.destination)
		if e.isUserSide(t.destination) || e.feedsSwap(t.destination) {
			continue
		}
		kind := FeeKindPlatform
		if tipAccounts[recipient] {
			kind = FeeKindTip
		}
		if bot, ok := botFeeWallets[recipient]; ok {
			e.addRouter(bot)
		}
		fees = append(fees, Fee{Kind: kind, Mint: t.mint, Amount: t.amount, Recipient: recipient})
	}
	return fees
}

func (e *extractor) isConsumed(ix *instruction) bool {
	return slices.ContainsFunc(e.consumed, ix.within)
}

// feedsSwap reports whether account is debited inside a decoded swap, i.e.
// it is a staging account used by the swap rather than a fee recipient.
func (e *extractor) feedsSwap(account solana.PublicKey) bool {
	for _, root := range e.consumed {
		for _, t := range e.v.subtreeTransfers(root) {
			if t.source == account {
				return true
			}
		}
	}
	return false
}

// balanceSwap infers a swap from the trader's balance changes. It is the
// fallback for venues this package does not recognise: exactly one asset
// must decrease and one increase. Native SOL changes are corrected for the
// network fee, tips and bot fees, and account rent deposits and refunds.
func (e *extractor) balanceSwap(trader solana.PublicKey, fees []Fee) (Swap, bool) {
	delta := e.v.tokenBalanceDelta(trader)

	sol := e.v.lamportDelta(trader)
	if trader == e.v.feePayer() {
		sol += int64(e.v.meta.Fee)
	}
	deposited, refunded := e.v.accountFunding(trader)
	sol += int64(deposited) - int64(refunded)
	delta[solana.WrappedSol] += sol
	for _, f := range fees {
		delta[f.Mint] += int64(f.Amount)
	}

	var in, out []solana.PublicKey
	for mint, d := range delta {
		switch {
		case d < 0:
			in = append(in, mint)
		case d > 0:
			out = append(out, mint)
		}
	}
	if len(in) != 1 || len(out) != 1 {
		return Swap{}, false
	}
	// Lamport noise below 0.0001 SOL is fee rounding, not a trade.
	const dust = 100_000
	if (in[0] == solana.WrappedSol && -delta[in[0]] < dust) || (out[0] == solana.WrappedSol && delta[out[0]] < dust) {
		return Swap{}, false
	}

	program := e.outermostProgram()
	return Swap{
		Protocol:              protocolName(program),
		ProgramID:             program,
		Trader:                trader,
		InputMint:             in[0],
		InputAmount:           uint64(-delta[in[0]]),
		InputDecimals:         e.decimals(in[0]),
		OutputMint:            out[0],
		OutputAmount:          uint64(delta[out[0]]),
		OutputDecimals:        e.decimals(out[0]),
		InstructionIndex:      -1,
		InnerInstructionIndex: -1,
	}, true
}

// infrastructurePrograms never execute swaps themselves.
var infrastructurePrograms = map[solana.PublicKey]bool{
	solana.SystemProgramID:                    true,
	solana.ComputeBudget:                      true,
	solana.TokenProgramID:                     true,
	solana.Token2022ProgramID:                 true,
	solana.SPLAssociatedTokenAccountProgramID: true,
	solana.MemoProgramID:                      true,
}

// outermostProgram returns the first top-level program that is not
// infrastructure, which is the best label available for an unrecognised swap.
func (e *extractor) outermostProgram() solana.PublicKey {
	for _, root := range e.v.roots {
		if !infrastructurePrograms[root.programID] {
			return root.programID
		}
	}
	return solana.PublicKey{}
}
