package txview

import (
	"slices"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// Instruction is a node of the invocation tree: a top-level instruction or an inner (CPI) one.
type Instruction struct {
	ProgramID solana.PublicKey
	Accounts  []solana.PublicKey
	Data      []byte

	OuterIndex int // index of the top-level instruction this node belongs to
	InnerIndex int // index among that instruction's inner instructions, -1 at top level
	depth      int // invocation stack height, 1 at top level

	Parent   *Instruction
	Children []*Instruction

	Transfer *Transfer // token or SOL movement this instruction performs, if any
}

// Account returns the i-th account, or the zero key when there are fewer accounts.
func (ix *Instruction) Account(i int) solana.PublicKey {
	if i < 0 || i >= len(ix.Accounts) {
		return solana.PublicKey{}
	}
	return ix.Accounts[i]
}

// Walk calls fn for every descendant of ix in execution order.
// Returning false skips that node's children.
func (ix *Instruction) Walk(fn func(*Instruction) bool) {
	for _, child := range ix.Children {
		if fn(child) {
			child.Walk(fn)
		}
	}
}

// Transfers returns the transfers executed below ix in execution order.
func (ix *Instruction) Transfers() []Transfer {
	var out []Transfer
	ix.Walk(func(n *Instruction) bool {
		if n.Transfer != nil {
			out = append(out, *n.Transfer)
		}
		return true
	})
	return out
}

// infrastructure lists native programs that never execute swaps themselves.
var infrastructure = map[solana.PublicKey]bool{
	solana.SystemProgramID:                    true,
	solana.ComputeBudget:                      true,
	solana.TokenProgramID:                     true,
	solana.Token2022ProgramID:                 true,
	solana.SPLAssociatedTokenAccountProgramID: true,
	solana.MemoProgramID:                      true,
}

// IsInfrastructure reports whether id is a native program that never swaps (system, token, memo, ...).
func IsInfrastructure(id solana.PublicKey) bool { return infrastructure[id] }

// buildTree links top-level and inner instructions using the stack heights the node recorded.
func (v *View) buildTree() error {
	inner := make(map[int][]rpc.CompiledInstruction, len(v.meta.InnerInstructions))
	for _, set := range v.meta.InnerInstructions {
		inner[int(set.Index)] = append(inner[int(set.Index)], set.Instructions...)
	}

	for i, outer := range v.tx.Message.Instructions {
		root, err := v.newInstruction(outer.ProgramIDIndex, outer.Accounts, outer.Data)
		if err != nil {
			return err
		}
		root.OuterIndex, root.InnerIndex, root.depth = i, -1, 1
		v.Roots = append(v.Roots, root)
		v.Flat = append(v.Flat, root)

		stack := []*Instruction{root}
		var lastProgram *Instruction // latest non-infrastructure inner instruction
		for j, in := range inner[i] {
			ix, err := v.newInstruction(in.ProgramIDIndex, in.Accounts, in.Data)
			if err != nil {
				return err
			}
			ix.OuterIndex, ix.InnerIndex = i, j

			if in.StackHeight == 0 {
				// No stack heights before 2023: attach token and system instructions to the
				// latest program call when they share one of its non-signer accounts.
				ix.Parent = root
				switch {
				case !infrastructure[ix.ProgramID]:
					lastProgram = ix
				case lastProgram != nil && v.sharesAccount(ix, lastProgram):
					ix.Parent = lastProgram
				}
				ix.depth = ix.Parent.depth + 1
			} else {
				ix.depth = int(in.StackHeight)
				for len(stack) > 1 && stack[len(stack)-1].depth >= ix.depth {
					stack = stack[:len(stack)-1]
				}
				ix.Parent = stack[len(stack)-1]
				stack = append(stack, ix)
			}
			ix.Parent.Children = append(ix.Parent.Children, ix)
			v.Flat = append(v.Flat, ix)
		}
	}
	return nil
}

func (v *View) newInstruction(programIndex uint16, accounts []uint16, data []byte) (*Instruction, error) {
	programID, err := v.key(programIndex)
	if err != nil {
		return nil, err
	}
	ix := &Instruction{ProgramID: programID, Data: data, Accounts: make([]solana.PublicKey, len(accounts))}
	for i, a := range accounts {
		if ix.Accounts[i], err = v.key(a); err != nil {
			return nil, err
		}
	}
	return ix, nil
}

// sharesAccount reports whether a and b reference a common account that is not a signer.
func (v *View) sharesAccount(a, b *Instruction) bool {
	for _, x := range a.Accounts {
		if !v.isSigner[x] && slices.Contains(b.Accounts, x) {
			return true
		}
	}
	return false
}
