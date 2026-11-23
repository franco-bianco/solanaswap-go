package solanaswap

import (
	"cmp"
	"slices"

	"github.com/gagliardetto/solana-go"
)

// extractor finds the swaps executed by a transaction.
//
// It walks the whole invocation tree, so swaps are found no matter which
// trading bot, router or custom program wraps the DEX call. Each invocation
// of a known DEX program is decoded into a Swap, preferably from the event
// the program emits and otherwise from the token transfers it performs.
type extractor struct {
	v *txView

	// swapRoots are the outermost invocations of known DEX programs.
	swapRoots []*instruction
	// userAuthorities are accounts that act on the user side of a swap: the
	// transaction signers and every authority that moves tokens outside of a
	// DEX invocation (routers and bots holding funds in transit). Pool
	// authorities only ever sign transfers inside their own DEX program.
	userAuthorities map[solana.PublicKey]bool

	// unknownDEX marks swap roots of programs missing from the registry.
	unknownDEX map[*instruction]bool

	swaps   []Swap
	routers []string
	// consumed marks instructions whose transfers belong to a decoded swap.
	consumed []*instruction
}

func newExtractor(v *txView) *extractor {
	e := &extractor{
		v:               v,
		userAuthorities: make(map[solana.PublicKey]bool),
		unknownDEX:      make(map[*instruction]bool),
	}
	for s := range v.signers {
		e.userAuthorities[s] = true
	}
	for _, root := range v.roots {
		e.collectSwapRoots(root)
	}
	for _, ix := range v.flat {
		t, ok := v.decodeTransfer(ix)
		if ok && !e.insideSwapRoot(ix) {
			e.userAuthorities[t.authority] = true
		}
	}
	return e
}

func (e *extractor) collectSwapRoots(ix *instruction) {
	spec, known := registry[ix.programID]
	switch {
	case known && spec.Kind == KindDEX:
		e.swapRoots = append(e.swapRoots, ix)
		return
	case !known && !infrastructurePrograms[ix.programID] && e.looksLikeSwap(ix):
		e.swapRoots = append(e.swapRoots, ix)
		e.unknownDEX[ix] = true
		return
	}
	for _, child := range ix.children {
		e.collectSwapRoots(child)
	}
}

// looksLikeSwap reports whether an unrecognised program swaps directly with a
// transaction signer: among the transfers it executes itself, the signer
// pays one mint and receives another from an account the program controls.
// This lets the parser support venues that are not in the registry.
func (e *extractor) looksLikeSwap(ix *instruction) bool {
	paid := make(map[solana.PublicKey]bool)
	var received []solana.PublicKey
	for _, child := range ix.children {
		t, ok := e.v.decodeTransfer(child)
		if !ok || t.amount == 0 || t.mint.IsZero() {
			continue
		}
		fromSigner := e.v.signers[t.authority]
		toSigner := e.v.signers[e.v.ownerOf(t.destination)]
		switch {
		case fromSigner && !toSigner:
			paid[t.mint] = true
		case !fromSigner && toSigner:
			received = append(received, t.mint)
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

func (e *extractor) insideSwapRoot(ix *instruction) bool {
	for _, root := range e.swapRoots {
		if ix.within(root) {
			return true
		}
	}
	return false
}

// run decodes all swaps in execution order.
func (e *extractor) run() {
	for _, root := range e.v.roots {
		e.visit(root)
	}
}

func (e *extractor) visit(ix *instruction) {
	spec, known := registry[ix.programID]
	if known && spec.Kind != KindDEX {
		e.addRouter(spec.Name)
	}

	if known && spec.decode != nil {
		if swaps := spec.decode(e, ix); len(swaps) > 0 {
			e.emit(ix, swaps...)
			return
		}
	}
	if (known && spec.Kind == KindDEX) || e.unknownDEX[ix] {
		if s, ok := e.transferSwap(ix, protocolName(ix.programID)); ok {
			e.emit(ix, s)
		}
		return
	}
	for _, child := range ix.children {
		e.visit(child)
	}
}

func (e *extractor) emit(root *instruction, swaps ...Swap) {
	for _, s := range swaps {
		if s.InputAmount == 0 && s.OutputAmount == 0 {
			continue
		}
		s.InstructionIndex, s.InnerInstructionIndex = root.outerIndex, root.innerIndex
		if s.ProgramID.IsZero() {
			s.ProgramID = root.programID
		}
		if s.Protocol == "" {
			s.Protocol = protocolName(s.ProgramID)
		}
		s.InputDecimals = e.decimals(s.InputMint)
		s.OutputDecimals = e.decimals(s.OutputMint)
		e.swaps = append(e.swaps, s)
	}
	e.consumed = append(e.consumed, root)
}

func (e *extractor) addRouter(name string) {
	if !slices.Contains(e.routers, name) {
		e.routers = append(e.routers, name)
	}
}

func (e *extractor) decimals(mint solana.PublicKey) uint8 {
	return e.v.decimals[mint]
}

// trader returns the wallet whose funds were swapped. It is the first swap's
// input authority when that is a signer. When a router account supplied the
// input, it is the signer that funded the router; otherwise the fee payer.
func (e *extractor) trader() solana.PublicKey {
	if len(e.swaps) == 0 {
		return e.v.feePayer()
	}
	first := e.swaps[0].Trader
	if first.IsZero() || e.v.signers[first] {
		return cmp.Or(first, e.v.feePayer())
	}
	for _, ix := range e.v.flat {
		if t, ok := e.v.decodeTransfer(ix); ok && e.v.signers[t.authority] && e.v.ownerOf(t.destination) == first {
			return t.authority
		}
	}
	// Keeper-executed orders: the program account owns the funds.
	return first
}

// isUserSide reports whether account belongs to the user side of a swap.
func (e *extractor) isUserSide(account solana.PublicKey) bool {
	return e.userAuthorities[account] || e.userAuthorities[e.v.ownerOf(account)]
}

// transferSwap derives a swap from the token transfers executed inside a DEX
// invocation. Transfers signed by a user-side authority are the input; the
// transfers that pay out to a user-side account are the output.
//
// Transfers to fee recipients inside the DEX instruction (protocol, creator
// or referral fees) count towards the input, because the DEX charges them as
// part of the swap.
func (e *extractor) transferSwap(ix *instruction, protocol string) (Swap, bool) {
	transfers := e.v.subtreeTransfers(ix)
	if len(transfers) < 2 {
		return Swap{}, false
	}

	var inputs, outputs []transfer
	for _, t := range transfers {
		fromUser := e.isUserSide(t.authority)
		toUser := e.isUserSide(t.destination)
		switch {
		case fromUser && !toUser:
			inputs = append(inputs, t)
		case !fromUser && toUser:
			outputs = append(outputs, t)
		}
	}
	inMint, inAmount := dominantMint(inputs, solana.PublicKey{})
	outMint, outAmount := dominantMint(outputs, inMint)
	if inMint.IsZero() || outMint.IsZero() || inAmount == 0 || outAmount == 0 {
		return Swap{}, false
	}
	return Swap{
		Protocol:     protocol,
		ProgramID:    ix.programID,
		Trader:       e.v.ownerOf(firstWithMint(inputs, inMint).authority),
		InputMint:    inMint,
		InputAmount:  inAmount,
		OutputMint:   outMint,
		OutputAmount: outAmount,
	}, true
}

// dominantMint sums transfers per mint and returns the mint with the largest
// number of transfers (ties broken by first appearance), skipping exclude.
// Swaps move one mint per side; extra mints are rare dust or rebates.
func dominantMint(transfers []transfer, exclude solana.PublicKey) (solana.PublicKey, uint64) {
	type total struct {
		amount uint64
		count  int
	}
	totals := make(map[solana.PublicKey]*total)
	var order []solana.PublicKey
	for _, t := range transfers {
		if t.mint.IsZero() || t.mint == exclude {
			continue
		}
		if totals[t.mint] == nil {
			totals[t.mint] = &total{}
			order = append(order, t.mint)
		}
		totals[t.mint].amount += t.amount
		totals[t.mint].count++
	}
	var best solana.PublicKey
	for _, m := range order {
		if best.IsZero() || totals[m].count > totals[best].count {
			best = m
		}
	}
	if best.IsZero() {
		return best, 0
	}
	return best, totals[best].amount
}

func firstWithMint(transfers []transfer, mint solana.PublicKey) transfer {
	for _, t := range transfers {
		if t.mint == mint {
			return t
		}
	}
	return transfer{}
}
