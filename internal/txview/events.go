package txview

import (
	"bytes"
	"encoding/base64"
	"strings"

	"github.com/gagliardetto/solana-go"
)

// Discriminator is an 8-byte Anchor instruction or event tag.
type Discriminator [8]byte

// Matches reports whether data starts with d.
func (d Discriminator) Matches(data []byte) bool {
	return len(data) >= 8 && bytes.Equal(data[:8], d[:])
}

// anchorEventTag starts the data of the self-invocation that Anchor's emit_cpi! uses for events.
var anchorEventTag = Discriminator{0xe4, 0x45, 0xa5, 0x2e, 0x51, 0xcb, 0x9a, 0x1d}

// CPIEvents returns the payloads of the disc events ix's program emitted through self-CPI below ix.
func (ix *Instruction) CPIEvents(disc Discriminator) [][]byte {
	var out [][]byte
	ix.Walk(func(child *Instruction) bool {
		if child.ProgramID == ix.ProgramID && anchorEventTag.Matches(child.Data) && disc.Matches(child.Data[8:]) {
			out = append(out, child.Data[16:])
		}
		return true
	})
	return out
}

// LogEvents returns the payloads of the disc events ix's program logged with emit! while running ix.
func (v *View) LogEvents(ix *Instruction, disc Discriminator) [][]byte {
	var out [][]byte
	for _, data := range v.logEvents[ix] {
		if disc.Matches(data) {
			out = append(out, data[8:])
		}
	}
	return out
}

// mapLogEvents replays the invocation log lines against the instruction tree to attach each
// "Program data:" payload to its instruction. It attaches nothing if the logs do not line up.
func (v *View) mapLogEvents() {
	events := make(map[*Instruction][][]byte)
	var stack []*Instruction
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
			for next < len(v.Flat) && precompiles[v.Flat[next].ProgramID] && v.Flat[next].ProgramID != program {
				next++
			}
			if next >= len(v.Flat) || v.Flat[next].ProgramID != program {
				return
			}
			stack = append(stack, v.Flat[next])
			next++
		case len(stack) > 0: // success or failed
			stack = stack[:len(stack)-1]
		}
	}
	v.logEvents = events
}

// parseLogFrame parses the runtime's "Program <id> invoke [n]", "success" and "failed:" lines.
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
var precompiles = map[solana.PublicKey]bool{
	solana.MustPublicKeyFromBase58("Ed25519SigVerify111111111111111111111111111"): true,
	solana.MustPublicKeyFromBase58("KeccakSecp256k11111111111111111111111111111"): true,
	solana.MustPublicKeyFromBase58("Secp256r1SigVerify1111111111111111111111111"): true,
}
