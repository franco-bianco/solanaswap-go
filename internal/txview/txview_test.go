package txview

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

func TestBuildTree(t *testing.T) {
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
		v, err := New(tx, meta)
		if err != nil {
			t.Fatal(err)
		}
		root := v.Roots[0]
		if len(root.Children) != 2 || len(root.Children[0].Children) != 2 || len(root.Children[1].Children) != 0 {
			t.Fatalf("unexpected tree shape: %d children, first has %d",
				len(root.Children), len(root.Children[0].Children))
		}
		if got := len(v.Flat); got != 6 {
			t.Fatalf("Flat has %d instructions, want 6", got)
		}
		if v.Flat[2].Parent != root.Children[0] || v.Flat[2].InnerIndex != 1 {
			t.Fatalf("grandchild linked wrongly")
		}
	})

	t.Run("missing stack heights", func(t *testing.T) {
		meta := &rpc.TransactionMeta{InnerInstructions: []rpc.InnerInstruction{{
			Index:        1,
			Instructions: []rpc.CompiledInstruction{{ProgramIDIndex: 3}, {ProgramIDIndex: 3}},
		}}}
		v, err := New(tx, meta)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(v.Roots[1].Children); got != 2 {
			t.Fatalf("legacy inner instructions: got %d children, want 2", got)
		}
	})

	t.Run("account index out of range", func(t *testing.T) {
		meta := &rpc.TransactionMeta{InnerInstructions: []rpc.InnerInstruction{{
			Instructions: []rpc.CompiledInstruction{{ProgramIDIndex: 9}},
		}}}
		if _, err := New(tx, meta); err == nil {
			t.Fatal("expected an error for an out-of-range account index")
		}
	})
}

func TestDecodeTransfer(t *testing.T) {
	src, mint, dst, auth := key(1), key(2), key(3), key(4)
	v := &View{TokenAccounts: map[solana.PublicKey]TokenAccount{dst: {Mint: mint}}}
	amount := func(tag byte, n uint64, extra ...byte) []byte {
		d := append([]byte{tag}, binary.LittleEndian.AppendUint64(nil, n)...)
		return append(d, extra...)
	}
	accounts := func(ks ...solana.PublicKey) []solana.PublicKey { return ks }

	tests := []struct {
		name string
		ix   *Instruction
		want Transfer
	}{
		{
			name: "transfer resolves mint from destination",
			ix: &Instruction{ProgramID: solana.TokenProgramID, Accounts: accounts(src, dst, auth),
				Data: amount(3, 500)},
			want: Transfer{Mint: mint, Amount: 500, Source: src, Destination: dst, Authority: auth},
		},
		{
			name: "token-2022 transfer checked",
			ix: &Instruction{ProgramID: solana.Token2022ProgramID, Accounts: accounts(src, mint, dst, auth),
				Data: amount(12, 7, 6)},
			want: Transfer{Mint: mint, Amount: 7, Source: src, Destination: dst, Authority: auth},
		},
		{
			name: "transfer checked with fee",
			ix: &Instruction{ProgramID: solana.Token2022ProgramID, Accounts: accounts(src, mint, dst, auth),
				Data: append([]byte{26}, amount(1, 9, 6, 0, 0, 0, 0, 0, 0, 0, 0)...)},
			want: Transfer{Mint: mint, Amount: 9, Source: src, Destination: dst, Authority: auth},
		},
		{
			name: "system transfer is native SOL",
			ix: &Instruction{ProgramID: solana.SystemProgramID, Accounts: accounts(src, dst),
				Data: binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, 42)},
			want: Transfer{Mint: solana.WrappedSol, Amount: 42, Source: src, Destination: dst, Authority: src},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := v.decodeTransfer(tt.ix)
			if !ok {
				t.Fatal("not decoded")
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}

	short := &Instruction{ProgramID: solana.TokenProgramID, Accounts: accounts(src), Data: amount(3, 1)}
	if _, ok := v.decodeTransfer(short); ok {
		t.Fatal("decoded a transfer with too few accounts")
	}
}

func TestParseLogFrame(t *testing.T) {
	id := key(9).String()
	tests := []struct {
		line string
		kind string
		ok   bool
	}{
		{"Program " + id + " invoke [2]", "invoke", true},
		{"Program " + id + " success", "success", true},
		{"Program " + id + " failed: custom program error: 0x1", "failed", true},
		{"Program " + id + " consumed 100 of 200 compute units", "", false},
		{"Program log: swap success", "", false},
		{"Program log: invoke [2]", "", false},
		{"Program return: " + id + " AQ==", "", false},
	}
	for _, tt := range tests {
		_, kind, ok := parseLogFrame(tt.line)
		if ok != tt.ok || kind != tt.kind {
			t.Errorf("parseLogFrame(%q) = %q, %v; want %q, %v", tt.line, kind, ok, tt.kind, tt.ok)
		}
	}
}
