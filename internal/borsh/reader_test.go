package borsh

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func TestReaderTolerantTail(t *testing.T) {
	r := NewReader(binary.LittleEndian.AppendUint64(nil, 5))
	if r.U64() != 5 || !r.OK() {
		t.Fatal("first field")
	}
	if r.U64() != 0 || r.OK() {
		t.Fatal("reading past the end must return zero and clear OK")
	}
	if r.Str() != "" || r.Pubkey() != (solana.PublicKey{}) {
		t.Fatal("reads after failure must return zero values")
	}
}

// A crafted vector length must not panic (it used to on 32-bit platforms).
func TestReaderRejectsHugeLengths(t *testing.T) {
	r := NewReader([]byte{1, 2, 3})
	r.Skip(0xffffffff, 34)
	if r.OK() {
		t.Fatal("skip past the end must fail")
	}
	if r.take(-1) != nil {
		t.Fatal("negative take must fail")
	}
}
