package solanaswap

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// instruction is one node of a transaction's invocation tree: either a
// top-level instruction or an inner instruction executed through CPI.
type instruction struct {
	programID solana.PublicKey
	accounts  []solana.PublicKey
	data      []byte

	outerIndex int // index of the top-level instruction this node belongs to
	innerIndex int // index within the inner instruction list, -1 for top-level
	depth      int // invocation stack height, 1 for top-level instructions

	parent   *instruction
	children []*instruction

	// transfer is the token or SOL movement this instruction performs, if any.
	transfer *transfer
	// swapRoot is the DEX invocation this instruction belongs to, if any.
	swapRoot *instruction
}

// account returns the i-th account of the instruction, or the zero key when
// the instruction has fewer accounts.
func (ix *instruction) account(i int) solana.PublicKey {
	if i < 0 || i >= len(ix.accounts) {
		return solana.PublicKey{}
	}
	return ix.accounts[i]
}

// walk calls fn for every node of the subtree rooted at ix (excluding ix
// itself) in execution order. Returning false from fn skips that node's children.
func (ix *instruction) walk(fn func(*instruction) bool) {
	for _, child := range ix.children {
		if fn(child) {
			child.walk(fn)
		}
	}
}

// tokenAccount describes an SPL token account seen in the transaction.
type tokenAccount struct {
	mint  solana.PublicKey
	owner solana.PublicKey
}

// txView is a decoded, index-resolved view of a confirmed transaction.
type txView struct {
	tx   *solana.Transaction
	meta *rpc.TransactionMeta

	keys       solana.PublicKeySlice
	keyIndex   map[solana.PublicKey]int
	signers    map[solana.PublicKey]bool
	numSigners int

	roots []*instruction // top-level instructions
	flat  []*instruction // all instructions in execution order

	tokenAccounts map[solana.PublicKey]tokenAccount
	decimals      map[solana.PublicKey]uint8
	// logEvents holds "Program data:" payloads per emitting instruction.
	logEvents map[*instruction][][]byte
}

func newTxView(tx *solana.Transaction, meta *rpc.TransactionMeta) (*txView, error) {
	if tx == nil {
		return nil, fmt.Errorf("%w: transaction is nil", ErrInvalidTransaction)
	}
	if meta == nil {
		return nil, fmt.Errorf("%w: transaction meta is nil", ErrInvalidTransaction)
	}

	// Account order for v0 transactions: static keys, then loaded writable,
	// then loaded read-only addresses.
	keys := make(solana.PublicKeySlice, 0, len(tx.Message.AccountKeys)+
		len(meta.LoadedAddresses.Writable)+len(meta.LoadedAddresses.ReadOnly))
	keys = append(keys, tx.Message.AccountKeys...)
	keys = append(keys, meta.LoadedAddresses.Writable...)
	keys = append(keys, meta.LoadedAddresses.ReadOnly...)

	v := &txView{
		tx:            tx,
		meta:          meta,
		keys:          keys,
		keyIndex:      make(map[solana.PublicKey]int, len(keys)),
		signers:       make(map[solana.PublicKey]bool),
		numSigners:    min(int(tx.Message.Header.NumRequiredSignatures), len(tx.Message.AccountKeys)),
		tokenAccounts: make(map[solana.PublicKey]tokenAccount),
		decimals:      map[solana.PublicKey]uint8{solana.WrappedSol: 9},
	}
	for i, k := range keys {
		if _, dup := v.keyIndex[k]; !dup {
			v.keyIndex[k] = i
		}
	}
	for _, s := range tx.Message.AccountKeys[:v.numSigners] {
		v.signers[s] = true
	}

	if err := v.buildInstructionTree(); err != nil {
		return nil, err
	}
	v.indexTokenAccounts()
	for _, ix := range v.flat {
		if t, ok := v.decodeTransfer(ix); ok && t.amount > 0 {
			ix.transfer = &t
		}
	}
	v.mapLogEvents()
	return v, nil
}

func (v *txView) key(index uint16) (solana.PublicKey, error) {
	if int(index) >= len(v.keys) {
		return solana.PublicKey{}, fmt.Errorf("%w: account index %d out of range (%d keys; v0 transactions need meta.LoadedAddresses)",
			ErrInvalidTransaction, index, len(v.keys))
	}
	return v.keys[index], nil
}

func (v *txView) newInstruction(programIndex uint16, accounts []uint16, data []byte) (*instruction, error) {
	programID, err := v.key(programIndex)
	if err != nil {
		return nil, err
	}
	ix := &instruction{programID: programID, data: data, accounts: make([]solana.PublicKey, len(accounts))}
	for i, a := range accounts {
		if ix.accounts[i], err = v.key(a); err != nil {
			return nil, err
		}
	}
	return ix, nil
}

// buildInstructionTree links top-level and inner instructions into a tree
// using the stack height recorded by the RPC node.
func (v *txView) buildInstructionTree() error {
	inner := make(map[int][]rpc.CompiledInstruction, len(v.meta.InnerInstructions))
	for _, set := range v.meta.InnerInstructions {
		inner[int(set.Index)] = append(inner[int(set.Index)], set.Instructions...)
	}

	for i, outer := range v.tx.Message.Instructions {
		root, err := v.newInstruction(outer.ProgramIDIndex, outer.Accounts, outer.Data)
		if err != nil {
			return err
		}
		root.outerIndex, root.innerIndex, root.depth = i, -1, 1
		v.roots = append(v.roots, root)
		v.flat = append(v.flat, root)

		stack := []*instruction{root}
		var lastProgram *instruction // latest non-infrastructure inner instruction
		for j, in := range inner[i] {
			ix, err := v.newInstruction(in.ProgramIDIndex, in.Accounts, in.Data)
			if err != nil {
				return err
			}
			ix.outerIndex, ix.innerIndex = i, j

			if in.StackHeight == 0 {
				// Nodes did not record stack heights before 2023. Assume
				// token and system instructions belong to the most recent
				// program invocation and other programs to the top level.
				ix.parent = root
				switch {
				case !infrastructurePrograms[ix.programID]:
					lastProgram = ix
				case lastProgram != nil:
					ix.parent = lastProgram
				}
				ix.depth = ix.parent.depth + 1
			} else {
				ix.depth = int(in.StackHeight)
				for len(stack) > 1 && stack[len(stack)-1].depth >= ix.depth {
					stack = stack[:len(stack)-1]
				}
				ix.parent = stack[len(stack)-1]
				stack = append(stack, ix)
			}
			ix.parent.children = append(ix.parent.children, ix)
			v.flat = append(v.flat, ix)
		}
	}
	return nil
}

// indexTokenAccounts records mint and owner for every token account the
// transaction touches, including temporary accounts that are created and
// closed within the transaction and therefore absent from token balances.
func (v *txView) indexTokenAccounts() {
	for _, balances := range [][]rpc.TokenBalance{v.meta.PreTokenBalances, v.meta.PostTokenBalances} {
		for _, b := range balances {
			if int(b.AccountIndex) >= len(v.keys) {
				continue
			}
			acct := tokenAccount{mint: b.Mint}
			if b.Owner != nil {
				acct.owner = *b.Owner
			}
			v.setTokenAccount(v.keys[b.AccountIndex], acct)
			if b.UiTokenAmount != nil {
				v.decimals[b.Mint] = b.UiTokenAmount.Decimals
			}
		}
	}

	for _, ix := range v.flat {
		switch {
		case isTokenProgram(ix.programID):
			v.indexTokenInstruction(ix)
		case ix.programID.Equals(solana.SPLAssociatedTokenAccountProgramID) && len(ix.accounts) >= 4:
			// Create / CreateIdempotent: [payer, ata, wallet, mint, ...]
			v.setTokenAccount(ix.accounts[1], tokenAccount{owner: ix.accounts[2], mint: ix.accounts[3]})
		}
	}
}

func (v *txView) indexTokenInstruction(ix *instruction) {
	if len(ix.data) == 0 {
		return
	}
	switch ix.data[0] {
	case tokenIxInitializeAccount: // [account, mint, owner, rent]
		if len(ix.accounts) >= 3 {
			v.setTokenAccount(ix.accounts[0], tokenAccount{mint: ix.accounts[1], owner: ix.accounts[2]})
		}
	case tokenIxInitializeAccount2, tokenIxInitializeAccount3: // [account, mint, ...], owner in data
		if len(ix.accounts) >= 2 && len(ix.data) >= 33 {
			v.setTokenAccount(ix.accounts[0], tokenAccount{mint: ix.accounts[1], owner: solana.PublicKeyFromBytes(ix.data[1:33])})
		}
	case tokenIxTransferChecked: // [source, mint, destination, authority]
		if len(ix.accounts) >= 3 && len(ix.data) >= 10 {
			v.setTokenAccount(ix.accounts[0], tokenAccount{mint: ix.accounts[1]})
			v.setTokenAccount(ix.accounts[2], tokenAccount{mint: ix.accounts[1]})
			v.decimals[ix.accounts[1]] = ix.data[9]
		}
	}
}

// setTokenAccount merges what is known about a token account, never
// overwriting a known field with an unknown one.
func (v *txView) setTokenAccount(addr solana.PublicKey, info tokenAccount) {
	cur := v.tokenAccounts[addr]
	if cur.mint.IsZero() {
		cur.mint = info.mint
	}
	if cur.owner.IsZero() {
		cur.owner = info.owner
	}
	v.tokenAccounts[addr] = cur
}

// ownerOf returns the wallet that controls addr: the owner for token
// accounts, the address itself otherwise.
func (v *txView) ownerOf(addr solana.PublicKey) solana.PublicKey {
	if acct, ok := v.tokenAccounts[addr]; ok && !acct.owner.IsZero() {
		return acct.owner
	}
	return addr
}

// feePayer returns the first signer, which pays the transaction fee.
func (v *txView) feePayer() solana.PublicKey {
	if len(v.tx.Message.AccountKeys) == 0 {
		return solana.PublicKey{}
	}
	return v.tx.Message.AccountKeys[0]
}

// signerList returns the transaction's signers in account order.
func (v *txView) signerList() []solana.PublicKey {
	return append([]solana.PublicKey(nil), v.tx.Message.AccountKeys[:v.numSigners]...)
}

// tokenDeltas returns post-pre raw token balance changes keyed by
// (owner, mint) for the token accounts accepted by keep.
func (v *txView) tokenDeltas(keep func(owner, mint solana.PublicKey) bool) map[[2]solana.PublicKey]int64 {
	type sums struct{ pre, post uint64 }
	totals := make(map[[2]solana.PublicKey]*sums)
	add := func(balances []rpc.TokenBalance, post bool) {
		for _, b := range balances {
			if b.UiTokenAmount == nil || int(b.AccountIndex) >= len(v.keys) {
				continue
			}
			owner := v.ownerOf(v.keys[b.AccountIndex])
			if !keep(owner, b.Mint) {
				continue
			}
			amount, err := strconv.ParseUint(b.UiTokenAmount.Amount, 10, 64)
			if err != nil {
				continue
			}
			k := [2]solana.PublicKey{owner, b.Mint}
			if totals[k] == nil {
				totals[k] = &sums{}
			}
			if post {
				totals[k].post += amount
			} else {
				totals[k].pre += amount
			}
		}
	}
	add(v.meta.PreTokenBalances, false)
	add(v.meta.PostTokenBalances, true)

	delta := make(map[[2]solana.PublicKey]int64, len(totals))
	for k, s := range totals {
		delta[k] = signedDiff(s.post, s.pre)
	}
	return delta
}

// signedDiff returns a-b clamped to the int64 range.
func signedDiff(a, b uint64) int64 {
	if a >= b {
		return int64(min(a-b, math.MaxInt64))
	}
	return -int64(min(b-a, math.MaxInt64))
}

// tokenBalanceDelta returns post-pre raw token balance changes per mint for
// all token accounts owned by owner.
func (v *txView) tokenBalanceDelta(owner solana.PublicKey) map[solana.PublicKey]int64 {
	out := make(map[solana.PublicKey]int64)
	for k, d := range v.tokenDeltas(func(o, _ solana.PublicKey) bool { return o == owner }) {
		out[k[1]] += d
	}
	return out
}

// ownerDeltas returns post-pre raw balance changes of mint per owner.
func (v *txView) ownerDeltas(mint solana.PublicKey) map[solana.PublicKey]int64 {
	out := make(map[solana.PublicKey]int64)
	for k, d := range v.tokenDeltas(func(_, m solana.PublicKey) bool { return m == mint }) {
		out[k[0]] += d
	}
	return out
}

// preLamports and postLamports return the lamports held by addr before and
// after the transaction.
func (v *txView) preLamports(addr solana.PublicKey) uint64 {
	return lamportsAt(v.meta.PreBalances, v.keyIndex, addr)
}

func (v *txView) postLamports(addr solana.PublicKey) uint64 {
	return lamportsAt(v.meta.PostBalances, v.keyIndex, addr)
}

func lamportsAt(balances []uint64, index map[solana.PublicKey]int, addr solana.PublicKey) uint64 {
	if i, ok := index[addr]; ok && i < len(balances) {
		return balances[i]
	}
	return 0
}

// lamportDelta returns post-pre lamports of the account at addr.
func (v *txView) lamportDelta(addr solana.PublicKey) int64 {
	return signedDiff(v.postLamports(addr), v.preLamports(addr))
}

// preTokenAmount returns the raw token balance of addr before the transaction.
func (v *txView) preTokenAmount(addr solana.PublicKey) uint64 {
	for _, b := range v.meta.PreTokenBalances {
		if int(b.AccountIndex) < len(v.keys) && v.keys[b.AccountIndex] == addr && b.UiTokenAmount != nil {
			n, _ := strconv.ParseUint(b.UiTokenAmount.Amount, 10, 64)
			return n
		}
	}
	return 0
}
