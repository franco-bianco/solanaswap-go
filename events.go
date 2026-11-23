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
	if !r.ok || n < 0 || len(r.buf) < n {
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

// skip discards count elements of size bytes each.
func (r *borshReader) skip(count uint32, size int) {
	if uint64(count)*uint64(size) > uint64(len(r.buf)) {
		r.ok = false
		return
	}
	r.take(int(count) * size)
}

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
		if payload, ok := strings.CutPrefix(line, "Program data: "); ok {
			if len(stack) > 0 {
				for chunk := range strings.FieldsSeq(payload) {
					if data, err := base64.StdEncoding.DecodeString(chunk); err == nil {
						top := stack[len(stack)-1]
						events[top] = append(events[top], data)
					}
				}
			}
			continue
		}
		if strings.HasPrefix(line, "Log truncated") {
			return
		}
		program, kind, ok := parseLogFrame(line)
		switch {
		case !ok:
		case kind == "invoke":
			// Precompiles run without logging an invocation.
			for next < len(v.flat) && precompiles[v.flat[next].programID] && v.flat[next].programID != program {
				next++
			}
			if next >= len(v.flat) || v.flat[next].programID != program {
				return
			}
			stack = append(stack, v.flat[next])
			next++
		case len(stack) > 0: // success or failed
			stack = stack[:len(stack)-1]
		}
	}
	v.logEvents = events
}

// parseLogFrame parses the invocation lines the runtime logs:
// "Program <id> invoke [<depth>]", "Program <id> success" and
// "Program <id> failed: <reason>". Program-written lines ("Program log: ...")
// and other runtime lines (consumed, return) are not frames.
func parseLogFrame(line string) (program solana.PublicKey, kind string, ok bool) {
	f := strings.Fields(line)
	if len(f) < 3 || f[0] != "Program" {
		return program, "", false
	}
	switch f[2] {
	case "invoke", "success", "failed:":
	default:
		return program, "", false
	}
	program, err := solana.PublicKeyFromBase58(f[1])
	if err != nil {
		return program, "", false
	}
	return program, strings.TrimSuffix(f[2], ":"), true
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
