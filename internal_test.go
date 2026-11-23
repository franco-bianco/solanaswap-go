package solanaswap

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

func key(b byte) solana.PublicKey {
	var k solana.PublicKey
	k[0], k[31] = b, 1
	return k
}

func TestBuildInstructionTree(t *testing.T) {
	keys := []solana.PublicKey{key(1), key(2), key(3), key(4)}
	tx := &solana.Transaction{Message: solana.Message{
		AccountKeys:  keys,
		Header:       solana.MessageHeader{NumRequiredSignatures: 1},
		Instructions: []solana.CompiledInstruction{{ProgramIDIndex: 1}, {ProgramIDIndex: 2}},
	}}

	t.Run("stack heights", func(t *testing.T) {
		meta := &rpc.TransactionMeta{InnerInstructions: []rpc.InnerInstruction{{
			Index: 0,
			Instructions: []rpc.CompiledInstruction{
				{ProgramIDIndex: 2, StackHeight: 2},
				{ProgramIDIndex: 3, StackHeight: 3},
				{ProgramIDIndex: 3, StackHeight: 3},
				{ProgramIDIndex: 2, StackHeight: 2},
			},
		}}}
		v, err := newTxView(tx, meta)
		if err != nil {
			t.Fatal(err)
		}
		root := v.roots[0]
		if len(root.children) != 2 || len(root.children[0].children) != 2 || len(root.children[1].children) != 0 {
			t.Fatalf("unexpected tree shape: %d children, first has %d", len(root.children), len(root.children[0].children))
		}
		if got := len(v.flat); got != 6 {
			t.Fatalf("flat has %d instructions, want 6", got)
		}
		if v.flat[2].parent != root.children[0] || v.flat[2].innerIndex != 1 {
			t.Fatalf("grandchild linked wrongly")
		}
	})

	t.Run("missing stack heights", func(t *testing.T) {
		meta := &rpc.TransactionMeta{InnerInstructions: []rpc.InnerInstruction{{
			Index:        1,
			Instructions: []rpc.CompiledInstruction{{ProgramIDIndex: 3}, {ProgramIDIndex: 3}},
		}}}
		v, err := newTxView(tx, meta)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(v.roots[1].children); got != 2 {
			t.Fatalf("legacy inner instructions: got %d children, want 2", got)
		}
	})

	t.Run("account index out of range", func(t *testing.T) {
		meta := &rpc.TransactionMeta{InnerInstructions: []rpc.InnerInstruction{{
			Instructions: []rpc.CompiledInstruction{{ProgramIDIndex: 9}},
		}}}
		if _, err := newTxView(tx, meta); err == nil {
			t.Fatal("expected an error for an out-of-range account index")
		}
	})
}

func TestDecodeTransfer(t *testing.T) {
	src, mint, dst, auth := key(1), key(2), key(3), key(4)
	v := &txView{tokenAccounts: map[solana.PublicKey]tokenAccount{dst: {mint: mint}}}
	amount := func(tag byte, n uint64, extra ...byte) []byte {
		d := append([]byte{tag}, binary.LittleEndian.AppendUint64(nil, n)...)
		return append(d, extra...)
	}

	tests := []struct {
		name string
		ix   *instruction
		want transfer
	}{
		{
			name: "transfer resolves mint from destination",
			ix:   &instruction{programID: solana.TokenProgramID, accounts: []solana.PublicKey{src, dst, auth}, data: amount(3, 500)},
			want: transfer{mint: mint, amount: 500, source: src, destination: dst, authority: auth},
		},
		{
			name: "token-2022 transfer checked",
			ix:   &instruction{programID: solana.Token2022ProgramID, accounts: []solana.PublicKey{src, mint, dst, auth}, data: amount(12, 7, 6)},
			want: transfer{mint: mint, amount: 7, source: src, destination: dst, authority: auth},
		},
		{
			name: "transfer checked with fee",
			ix:   &instruction{programID: solana.Token2022ProgramID, accounts: []solana.PublicKey{src, mint, dst, auth}, data: append([]byte{26}, amount(1, 9, 6, 0, 0, 0, 0, 0, 0, 0, 0)...)},
			want: transfer{mint: mint, amount: 9, source: src, destination: dst, authority: auth},
		},
		{
			name: "system transfer is native SOL",
			ix:   &instruction{programID: solana.SystemProgramID, accounts: []solana.PublicKey{src, dst}, data: binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, 42)},
			want: transfer{mint: solana.WrappedSol, amount: 42, source: src, destination: dst, authority: src},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := v.decodeTransfer(tt.ix)
			if !ok {
				t.Fatal("not decoded")
			}
			got.ix = nil
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}

	if _, ok := v.decodeTransfer(&instruction{programID: solana.TokenProgramID, accounts: []solana.PublicKey{src}, data: amount(3, 1)}); ok {
		t.Fatal("decoded a transfer with too few accounts")
	}
}

func TestSummarize(t *testing.T) {
	a, b, c := key(1), key(2), key(3)
	hop := func(in solana.PublicKey, inAmt uint64, out solana.PublicKey, outAmt uint64) Swap {
		return Swap{Protocol: "X", InputMint: in, InputAmount: inAmt, OutputMint: out, OutputAmount: outAmt}
	}
	tests := []struct {
		name              string
		swaps             []Swap
		in, out           solana.PublicKey
		inAmount, outAmnt uint64
	}{
		{"single", []Swap{hop(a, 10, b, 20)}, a, b, 10, 20},
		{"multi-hop", []Swap{hop(a, 10, b, 20), hop(b, 20, c, 30)}, a, c, 10, 30},
		{"split", []Swap{hop(a, 10, b, 20), hop(a, 5, b, 9)}, a, b, 15, 29},
		{"split then merge", []Swap{hop(a, 10, b, 20), hop(b, 12, c, 5), hop(b, 8, c, 4)}, a, c, 10, 9},
		{"arbitrage cycle", []Swap{hop(a, 10, b, 20), hop(b, 20, a, 11)}, a, a, 10, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info SwapInfo
			summarize(&info, tt.swaps)
			if info.TokenInMint != tt.in || info.TokenOutMint != tt.out ||
				info.TokenInAmount != tt.inAmount || info.TokenOutAmount != tt.outAmnt {
				t.Fatalf("got %s %d -> %s %d", info.TokenInMint, info.TokenInAmount, info.TokenOutMint, info.TokenOutAmount)
			}
		})
	}
}

func TestBorshReaderTolerantTail(t *testing.T) {
	data := binary.LittleEndian.AppendUint64(nil, 5)
	r := newBorshReader(data)
	if r.u64() != 5 || !r.ok {
		t.Fatal("first field")
	}
	if r.u64() != 0 || r.ok {
		t.Fatal("reading past the end must return zero and clear ok")
	}
	if r.string() != "" || r.pubkey() != (solana.PublicKey{}) {
		t.Fatal("reads after failure must return zero values")
	}
}
