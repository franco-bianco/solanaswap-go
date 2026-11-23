package solanaswap

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"strings"

	"github.com/gagliardetto/solana-go"
)

// anchorEventIxTag prefixes the data of the self-invocation Anchor programs
// use to emit events with emit_cpi!. It is followed by the 8-byte event
// discriminator and the Borsh-encoded event.
var anchorEventIxTag = []byte{0xe4, 0x45, 0xa5, 0x2e, 0x51, 0xcb, 0x9a, 0x1d}

// discriminator is an 8-byte Anchor account, instruction or event tag.
type discriminator [8]byte

// cpiEvents returns the payloads (without tag and discriminator) of all
// events emitted by ix's program, via self-CPI, inside ix's subtree.
func cpiEvents(ix *instruction, disc discriminator) [][]byte {
	var out [][]byte
	ix.walk(func(child *instruction) bool {
		if child.programID == ix.programID && isAnchorEvent(child.data, disc) {
			out = append(out, child.data[16:])
		}
		return true
	})
	return out
}

func isAnchorEvent(data []byte, disc discriminator) bool {
	return len(data) >= 16 && bytes.Equal(data[:8], anchorEventIxTag) && bytes.Equal(data[8:16], disc[:])
}

// hasDiscriminator reports whether instruction data starts with disc.
func hasDiscriminator(data []byte, disc discriminator) bool {
	return len(data) >= 8 && bytes.Equal(data[:8], disc[:])
}

// borshReader reads little-endian Borsh values. Events grow new trailing
// fields over time, so reads past the end are not errors: they set ok to
// false and return zero values, letting decoders handle every layout version
// with one code path.
type borshReader struct {
	buf []byte
	ok  bool
}

func newBorshReader(b []byte) *borshReader { return &borshReader{buf: b, ok: true} }

func (r *borshReader) take(n int) []byte {
	if !r.ok || len(r.buf) < n {
		r.ok = false
		return nil
	}
	b := r.buf[:n]
	r.buf = r.buf[n:]
	return b
}

func (r *borshReader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *borshReader) bool() bool { return r.u8() != 0 }

func (r *borshReader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *borshReader) u64() uint64 {
	if b := r.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}

func (r *borshReader) i64() int64 { return int64(r.u64()) }

func (r *borshReader) skip(n int) { r.take(n) }

func (r *borshReader) pubkey() solana.PublicKey {
	if b := r.take(32); b != nil {
		return solana.PublicKeyFromBytes(b)
	}
	return solana.PublicKey{}
}

func (r *borshReader) string() string {
	n := r.u32()
	if n > uint32(len(r.buf)) {
		r.ok = false
		return ""
	}
	return string(r.take(int(n)))
}

// mapLogEvents attaches the payloads of "Program data:" log lines (Anchor
// emit! events) to the instruction that was executing when they were logged.
// Every "invoke" log line corresponds to the next instruction in execution
// order, so the log stream can be replayed against the instruction tree. If
// the logs do not line up (for example because the node truncated them), no
// events are attached rather than risking attributing them to the wrong
// instruction.
func (v *txView) mapLogEvents() {
	events := make(map[*instruction][][]byte)
	var stack []*instruction
	next := 0
	for _, line := range v.meta.LogMessages {
		switch {
		case strings.HasPrefix(line, "Program data: ") && len(stack) > 0:
			for chunk := range strings.FieldsSeq(strings.TrimPrefix(line, "Program data: ")) {
				if data, err := base64.StdEncoding.DecodeString(chunk); err == nil {
					top := stack[len(stack)-1]
					events[top] = append(events[top], data)
				}
			}
		case strings.HasPrefix(line, "Program ") && strings.Contains(line, " invoke ["):
			program := strings.Fields(line)[1]
			// Precompiles run without logging an invocation.
			for next < len(v.flat) && precompiles[v.flat[next].programID] && v.flat[next].programID.String() != program {
				next++
			}
			if next >= len(v.flat) || v.flat[next].programID.String() != program {
				return
			}
			stack = append(stack, v.flat[next])
			next++
		case strings.HasPrefix(line, "Program ") && (strings.HasSuffix(line, " success") || strings.Contains(line, " failed")):
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case strings.HasPrefix(line, "Log truncated"):
			return
		}
	}
	v.logEvents = events
}

// precompiles are native programs that execute without "invoke" log lines.
var precompiles = keySet(
	"Ed25519SigVerify111111111111111111111111111",
	"KeccakSecp256k11111111111111111111111111111",
	"Secp256r1SigVerify1111111111111111111111111",
)

// logEventsOf returns the payloads (without discriminator) of the emit!
// events ix's program logged while executing ix itself.
func (v *txView) logEventsOf(ix *instruction, disc discriminator) [][]byte {
	var out [][]byte
	for _, data := range v.logEvents[ix] {
		if hasDiscriminator(data, disc) {
			out = append(out, data[8:])
		}
	}
	return out
}
