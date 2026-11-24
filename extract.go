package solanaswap

import (
	"cmp"
	"slices"

	"github.com/gagliardetto/solana-go"

	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

// extractor finds the swaps in a transaction by walking its whole invocation tree,
// decoding each DEX invocation from its events or, failing that, its token transfers.
type extractor struct {
	v *txview.View

	// swapRoot maps every instruction inside a DEX invocation to that invocation.
	swapRoot map[*txview.Instruction]*txview.Instruction
	// userAuthorities are the signers plus authorities moving tokens outside DEX invocations
	// (routers, bots); pool authorities only ever sign inside their own DEX.
	userAuthorities map[solana.PublicKey]bool
	// swapDebited holds accounts debited inside DEX invocations: route staging accounts, not fee recipients.
	swapDebited map[solana.PublicKey]bool

	swaps   []Swap
	routers []string
}

func newExtractor(v *txview.View) *extractor {
	e := &extractor{
		v:               v,
		swapRoot:        make(map[*txview.Instruction]*txview.Instruction),
		userAuthorities: make(map[solana.PublicKey]bool),
		swapDebited:     make(map[solana.PublicKey]bool),
	}
	for _, s := range v.Signers {
		e.userAuthorities[s] = true
	}
	for _, root := range v.Roots {
		e.markSwapRoots(root, false)
	}
	for _, ix := range v.Flat {
		switch t := ix.Transfer; {
		case t == nil:
		case e.swapRoot[ix] == nil:
			e.userAuthorities[t.Authority] = true
		default:
			e.swapDebited[t.Source] = true
		}
	}
	return e
}

// markSwapRoots finds the DEX invocations below ix. An unregistered program counts as a DEX when
// a router calls it, or when it swaps with a signer and calls no registered program itself.
func (e *extractor) markSwapRoots(ix *txview.Instruction, calledByRouter bool) {
	spec, known := registry[ix.ProgramID]
	isDEX := known && spec.Kind == KindDEX
	isRouter := known && spec.Kind == KindAggregator
	if !known && !txview.IsInfrastructure(ix.ProgramID) {
		swapsWithSigner := e.looksLikeSwap(ix)
		switch {
		case calledByRouter, swapsWithSigner && !hasRegisteredDescendant(ix):
			isDEX = true
		case swapsWithSigner:
			isRouter = true // an unregistered bot or aggregator: its calls are the hops
		}
	}
	if isDEX {
		e.swapRoot[ix] = ix
		ix.Walk(func(n *txview.Instruction) bool {
			e.swapRoot[n] = ix
			return true
		})
		return
	}
	for _, child := range ix.Children {
		// A program invoking itself is emitting an event, not a hop.
		e.markSwapRoots(child, isRouter && child.ProgramID != ix.ProgramID)
	}
}

func hasRegisteredDescendant(ix *txview.Instruction) bool {
	found := false
	ix.Walk(func(child *txview.Instruction) bool {
		if _, ok := registry[child.ProgramID]; ok {
			found = true
		}
		return !found
	})
	return found
}

// looksLikeSwap reports whether an unrecognised program's own transfers make a signer pay one
// mint and receive another, so venues missing from the registry are still found.
func (e *extractor) looksLikeSwap(ix *txview.Instruction) bool {
	paid := make(map[solana.PublicKey]bool)
	var received []solana.PublicKey
	for _, child := range ix.Children {
		t := child.Transfer
		if t == nil || t.Mint.IsZero() {
			continue
		}
		fromSigner := e.v.IsSigner(t.Authority)
		toSigner := e.v.IsSigner(e.v.OwnerOf(t.Destination))
		switch {
		case fromSigner && !toSigner:
			paid[t.Mint] = true
		case !fromSigner && toSigner:
			received = append(received, t.Mint)
		}
	}
	for _, mint := range received {
		for p := range paid {
			if p != mint {
				return true
			}
		}
	}
	return false
}

// visit decodes the swaps below ix in execution order.
func (e *extractor) visit(ix *txview.Instruction) {
	spec, known := registry[ix.ProgramID]
	if known && spec.Kind != KindDEX {
		e.addRouter(spec.Name)
	}

	if known && spec.decode != nil {
		if swaps := spec.decode(e, ix); len(swaps) > 0 {
			e.emit(ix, swaps...)
			return
		}
	}
	if e.swapRoot[ix] == ix {
		if s, ok := e.transferSwap(ix, protocolName(ix.ProgramID)); ok {
			e.emit(ix, s)
		}
		return
	}
	for _, child := range ix.Children {
		e.visit(child)
	}
}

func (e *extractor) emit(root *txview.Instruction, swaps ...Swap) {
	for _, s := range swaps {
		if s.InputAmount == 0 && s.OutputAmount == 0 {
			continue
		}
		s.InstructionIndex, s.InnerInstructionIndex = root.OuterIndex, root.InnerIndex
		if s.ProgramID.IsZero() {
			s.ProgramID = root.ProgramID
		}
		if s.Protocol == "" {
			s.Protocol = protocolName(s.ProgramID)
		}
		s.InputDecimals = e.v.Decimals[s.InputMint]
		s.OutputDecimals = e.v.Decimals[s.OutputMint]
		e.swaps = append(e.swaps, s)
	}
}

func (e *extractor) addRouter(name string) {
	if !slices.Contains(e.routers, name) {
		e.routers = append(e.routers, name)
	}
}

// trader returns the wallet whose funds were swapped: the first swap's signer, or the signer
// that funded the router supplying the input, or the fee payer.
func (e *extractor) trader() solana.PublicKey {
	if len(e.swaps) == 0 {
		return e.v.FeePayer()
	}
	first := e.swaps[0].Trader
	if first.IsZero() || e.v.IsSigner(first) {
		return cmp.Or(first, e.v.FeePayer())
	}
	for _, ix := range e.v.Flat {
		if t := ix.Transfer; t != nil && e.v.IsSigner(t.Authority) && e.v.OwnerOf(t.Destination) == first {
			return t.Authority
		}
	}
	return first // keeper-executed order: the program account owns the funds
}

// beneficiary refines the trader once the route is known: the order owner for keeper fills,
// else the signer whose output token balance grew most (input balance shrank most for SOL outputs).
func (e *extractor) beneficiary(info *SwapInfo) solana.PublicKey {
	if owner, ok := e.orderOwner(); ok {
		return owner
	}
	mint, sign := info.TokenOutMint, int64(1)
	if mint == solana.WrappedSol {
		mint, sign = info.TokenInMint, -1
	}
	if mint == solana.WrappedSol || info.TokenInMint == info.TokenOutMint {
		return info.Trader
	}
	best, bestChange := info.Trader, int64(0)
	for h, change := range e.v.TokenChanges() {
		if d := sign * change; h.Mint == mint && e.v.IsSigner(h.Owner) && d > bestChange {
			best, bestChange = h.Owner, d
		}
	}
	return best
}

// isUserSide reports whether account belongs to the user side of a swap.
func (e *extractor) isUserSide(account solana.PublicKey) bool {
	return e.userAuthorities[account] || e.userAuthorities[e.v.OwnerOf(account)]
}

// userFlows splits the transfers below ix into those leaving the user side and those paying it.
func (e *extractor) userFlows(ix *txview.Instruction) (out, in []txview.Transfer) {
	for _, t := range ix.Transfers() {
		fromUser, toUser := e.isUserSide(t.Authority), e.isUserSide(t.Destination)
		switch {
		case fromUser && !toUser:
			out = append(out, t)
		case !fromUser && toUser:
			in = append(in, t)
		}
	}
	return out, in
}

// sumOf adds up the transfers of mint.
func sumOf(transfers []txview.Transfer, mint solana.PublicKey) uint64 {
	var sum uint64
	for _, t := range transfers {
		if t.Mint == mint {
			sum += t.Amount
		}
	}
	return sum
}

// transferSwap derives a swap from the transfers inside a DEX invocation. DEX fees paid from
// the user side count towards the input, because the DEX charges them as part of the swap.
func (e *extractor) transferSwap(ix *txview.Instruction, protocol string) (Swap, bool) {
	inputs, outputs := e.userFlows(ix)
	inMint, inAmount := e.dominantMint(inputs, solana.PublicKey{})
	outMint, outAmount := e.dominantMint(outputs, inMint)
	if inMint.IsZero() || outMint.IsZero() || inAmount == 0 || outAmount == 0 {
		return Swap{}, false
	}
	first := slices.IndexFunc(inputs, func(t txview.Transfer) bool { return t.Mint == inMint })
	return Swap{
		Protocol:     protocol,
		ProgramID:    ix.ProgramID,
		Trader:       inputs[first].Authority,
		InputMint:    inMint,
		InputAmount:  inAmount,
		OutputMint:   outMint,
		OutputAmount: outAmount,
	}, true
}

// dominantMint picks the mint one side of a swap trades and sums it, skipping exclude.
// Token-account transfers beat plain SOL transfers, then more transfers win, then the first seen.
func (e *extractor) dominantMint(transfers []txview.Transfer, exclude solana.PublicKey) (solana.PublicKey, uint64) {
	type total struct {
		amount uint64
		count  int
		token  bool // moved into or out of a token account
	}
	totals := make(map[solana.PublicKey]*total)
	var order []solana.PublicKey
	for _, t := range transfers {
		if t.Mint.IsZero() || t.Mint == exclude {
			continue
		}
		if totals[t.Mint] == nil {
			totals[t.Mint] = &total{}
			order = append(order, t.Mint)
		}
		tot := totals[t.Mint]
		tot.amount += t.Amount
		tot.count++
		tot.token = tot.token || e.v.IsTokenAccount(t.Source) || e.v.IsTokenAccount(t.Destination)
	}
	var best solana.PublicKey
	for _, m := range order {
		if best.IsZero() {
			best = m
			continue
		}
		a, b := totals[m], totals[best]
		if (a.token && !b.token) || (a.token == b.token && a.count > b.count) {
			best = m
		}
	}
	if best.IsZero() {
		return best, 0
	}
	return best, totals[best].amount
}
