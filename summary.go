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
// holding the trader's funds in transit) to accounts outside the user side
// that are not part of any DEX invocation: trading-bot and aggregator fees,
// and tips. To keep unrelated activity out (deposits into other protocols),
// a payment must be a top-level instruction, made by a registered bot or
// aggregator program, or made inside a program call that also performs a
// swap.
func (e *extractor) collectFees() []Fee {
	var fees []Fee
	for _, ix := range e.v.flat {
		t := ix.transfer
		if t == nil || ix.swapRoot != nil || !e.isUserSide(t.authority) {
			continue
		}
		if e.isUserSide(t.destination) || e.swapDebited[t.destination] || !e.besideSwap(ix) {
			continue
		}
		recipient := e.v.ownerOf(t.destination)
		kind := FeeKindPlatform
		if tipAccounts[recipient] {
			kind = FeeKindTip
		}
		fees = append(fees, Fee{Kind: kind, Mint: t.mint, Amount: t.amount, Recipient: recipient})
	}
	return fees
}

// besideSwap reports whether a transfer instruction is top-level, made by a
// registered bot or aggregator, or made by a program whose invocation also
// contains a swap.
func (e *extractor) besideSwap(ix *instruction) bool {
	caller := ix.parent
	for caller != nil && infrastructurePrograms[caller.programID] {
		caller = caller.parent
	}
	if caller == nil {
		return true
	}
	if _, ok := registry[caller.programID]; ok {
		return true
	}
	found := false
	caller.walk(func(n *instruction) bool {
		found = found || n.swapRoot != nil
		return !found
	})
	return found
}

// labelBots adds the trading bots whose fee wallets received fees.
func (e *extractor) labelBots(fees []Fee) {
	for _, f := range fees {
		if bot, ok := botFeeWallets[f.Recipient]; ok {
			e.addRouter(bot)
		}
	}
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
