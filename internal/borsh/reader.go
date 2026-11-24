// Package borsh reads the little-endian Borsh encoding used by Solana program events.
package borsh

import (
	"encoding/binary"

	"github.com/gagliardetto/solana-go"
)

// Reader decodes Borsh values in order. Reading past the end returns zero values and
// clears OK, so one decoder handles events that gained trailing fields over time.
type Reader struct {
	buf []byte
	ok  bool
}

func NewReader(b []byte) *Reader { return &Reader{buf: b, ok: true} }

// OK reports whether every read so far was within the data.
func (r *Reader) OK() bool { return r.ok }

func (r *Reader) take(n int) []byte {
	if !r.ok || n < 0 || len(r.buf) < n {
		r.ok = false
		return nil
	}
	b := r.buf[:n]
	r.buf = r.buf[n:]
	return b
}

func (r *Reader) U8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *Reader) Bool() bool { return r.U8() != 0 }

func (r *Reader) U32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *Reader) U64() uint64 {
	if b := r.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}

func (r *Reader) I64() int64 { return int64(r.U64()) }

func (r *Reader) Pubkey() solana.PublicKey {
	if b := r.take(32); b != nil {
		return solana.PublicKeyFromBytes(b)
	}
	return solana.PublicKey{}
}

// Str reads a length-prefixed UTF-8 string.
func (r *Reader) Str() string {
	n := r.U32()
	if n > uint32(len(r.buf)) {
		r.ok = false
		return ""
	}
	return string(r.take(int(n)))
}

// Skip discards count elements of size bytes each.
func (r *Reader) Skip(count uint32, size int) {
	if uint64(count)*uint64(size) > uint64(len(r.buf)) {
		r.ok = false
		return
	}
	r.take(int(count) * size)
}
