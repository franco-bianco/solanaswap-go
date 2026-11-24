package solanaswap

import (
	"slices"

	"github.com/gagliardetto/solana-go"

	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

// summarize fills the route fields of info: first input to last output, netted across hops.
// For arbitrage (same mint in and out) the gross amounts are used.
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

// collectFees returns user-side payments outside DEX invocations (bot fees, tips) and labels the
// bots that received them. Unknown recipients count only when paid beside a swap.
func (e *extractor) collectFees() []Fee {
	var fees []Fee
	for _, ix := range e.v.Flat {
		t := ix.Transfer
		if t == nil || e.swapRoot[ix] != nil || !e.isUserSide(t.Authority) {
			continue
		}
		if e.isUserSide(t.Destination) || e.swapDebited[t.Destination] {
			continue
		}
		recipient := e.v.OwnerOf(t.Destination)
		bot, botWallet := botFeeWallets[recipient]
		if !tipAccounts[recipient] && !botWallet && !e.besideSwap(ix) {
			continue
		}
		kind := FeeKindPlatform
		if tipAccounts[recipient] {
			kind = FeeKindTip
		}
		if botWallet {
			e.addRouter(bot)
		}
		fees = append(fees, Fee{Kind: kind, Mint: t.Mint, Amount: t.Amount, Recipient: recipient, payer: t.Authority})
	}
	return fees
}

// besideSwap reports whether a payment is top-level, made by a registered program, or made
// by a program whose invocation also contains a swap.
func (e *extractor) besideSwap(ix *txview.Instruction) bool {
	caller := ix.Parent
	for caller != nil && txview.IsInfrastructure(caller.ProgramID) {
		caller = caller.Parent
	}
	if caller == nil {
		return true
	}
	if _, ok := registry[caller.ProgramID]; ok {
		return true
	}
	found := false
	caller.Walk(func(n *txview.Instruction) bool {
		found = found || e.swapRoot[n] != nil
		return !found
	})
	return found
}

// fallbackSwap infers a swap from balance changes when no swap instruction was recognised,
// trying each signer so relayer-paid swaps are attributed to the user.
func (e *extractor) fallbackSwap(fees []Fee) (Swap, bool) {
	for _, signer := range e.v.Signers {
		if s, ok := e.balanceSwap(signer, fees); ok {
			return s, true
		}
	}
	return Swap{}, false
}

// balanceSwap infers a swap from trader's balance changes: exactly one asset must fall and one rise.
// SOL is corrected for the network fee, rent, and fees the trader (or a router) paid.
func (e *extractor) balanceSwap(trader solana.PublicKey, fees []Fee) (Swap, bool) {
	delta := make(map[solana.PublicKey]int64)
	for h, change := range e.v.TokenChanges() {
		if h.Owner == trader {
			delta[h.Mint] += change
		}
	}

	sol := e.v.LamportChange(trader)
	if trader == e.v.FeePayer() {
		sol += int64(e.v.Meta.Fee)
	}
	deposited, refunded := e.v.AccountFunding(trader)
	sol += int64(deposited) - int64(refunded)
	delta[solana.WrappedSol] += sol
	for _, f := range fees {
		if f.payer == trader || !e.v.IsSigner(f.payer) {
			delta[f.Mint] += int64(f.Amount)
		}
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
	const dust = 100_000 // lamport noise below 0.0001 SOL is fee rounding, not a trade
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
		InputDecimals:         e.v.Decimals[in[0]],
		OutputMint:            out[0],
		OutputAmount:          uint64(delta[out[0]]),
		OutputDecimals:        e.v.Decimals[out[0]],
		InstructionIndex:      -1,
		InnerInstructionIndex: -1,
	}, true
}

// outermostProgram returns the first top-level program that is not infrastructure,
// the best label for an unrecognised swap.
func (e *extractor) outermostProgram() solana.PublicKey {
	for _, root := range e.v.Roots {
		if !txview.IsInfrastructure(root.ProgramID) {
			return root.ProgramID
		}
	}
	return solana.PublicKey{}
}
