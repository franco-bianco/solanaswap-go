package solanaswap

import (
	"encoding/binary"

	"github.com/gagliardetto/solana-go"
)

// SPL Token instruction tags (shared by Token and Token-2022).
const (
	tokenIxInitializeAccount  = 1
	tokenIxTransfer           = 3
	tokenIxCloseAccount       = 9
	tokenIxTransferChecked    = 12
	tokenIxInitializeAccount2 = 16
	tokenIxInitializeAccount3 = 18
	tokenIxTransferFeeExt     = 26 // Token-2022 transfer-fee extension
	transferFeeExtWithFee     = 1  // TransferCheckedWithFee sub-instruction
)

// System program instruction tags.
const (
	systemIxCreateAccount         = 0
	systemIxTransfer              = 2
	systemIxCreateAccountWithSeed = 3
	systemIxTransferWithSeed      = 11
)

func isTokenProgram(id solana.PublicKey) bool {
	return id.Equals(solana.TokenProgramID) || id.Equals(solana.Token2022ProgramID)
}

// transfer is a token or native SOL movement decoded from an instruction.
// Native SOL transfers use the wrapped SOL mint.
type transfer struct {
	ix          *instruction
	mint        solana.PublicKey // zero when the mint cannot be determined
	amount      uint64
	source      solana.PublicKey
	destination solana.PublicKey
	authority   solana.PublicKey
}

// decodeTransfer decodes SPL Token / Token-2022 transfers and System program
// lamport transfers. It reports false for any other instruction.
func (v *txView) decodeTransfer(ix *instruction) (transfer, bool) {
	switch {
	case isTokenProgram(ix.programID):
		return v.decodeTokenTransfer(ix)
	case ix.programID.Equals(solana.SystemProgramID):
		return decodeSystemTransfer(ix)
	}
	return transfer{}, false
}

func (v *txView) decodeTokenTransfer(ix *instruction) (transfer, bool) {
	d := ix.data
	if len(d) == 0 {
		return transfer{}, false
	}
	t := transfer{ix: ix}
	switch {
	case d[0] == tokenIxTransfer && len(d) >= 9 && len(ix.accounts) >= 3:
		t.amount = binary.LittleEndian.Uint64(d[1:9])
		t.source, t.destination, t.authority = ix.accounts[0], ix.accounts[1], ix.accounts[2]
		t.mint = v.tokenAccounts[t.source].mint
		if t.mint.IsZero() {
			t.mint = v.tokenAccounts[t.destination].mint
		}
	case d[0] == tokenIxTransferChecked && len(d) >= 9 && len(ix.accounts) >= 4,
		d[0] == tokenIxTransferFeeExt && len(d) >= 10 && d[1] == transferFeeExtWithFee && len(ix.accounts) >= 4:
		offset := 1
		if d[0] == tokenIxTransferFeeExt {
			offset = 2
		}
		t.amount = binary.LittleEndian.Uint64(d[offset : offset+8])
		t.source, t.mint, t.destination, t.authority = ix.accounts[0], ix.accounts[1], ix.accounts[2], ix.accounts[3]
	default:
		return transfer{}, false
	}
	return t, true
}

func decodeSystemTransfer(ix *instruction) (transfer, bool) {
	d := ix.data
	if len(d) < 12 {
		return transfer{}, false
	}
	t := transfer{ix: ix, mint: solana.WrappedSol}
	switch tag := binary.LittleEndian.Uint32(d[:4]); {
	case tag == systemIxTransfer && len(ix.accounts) >= 2:
		t.amount = binary.LittleEndian.Uint64(d[4:12])
		t.source, t.destination, t.authority = ix.accounts[0], ix.accounts[1], ix.accounts[0]
	case tag == systemIxTransferWithSeed && len(ix.accounts) >= 3:
		t.amount = binary.LittleEndian.Uint64(d[4:12])
		t.source, t.destination, t.authority = ix.accounts[0], ix.accounts[2], ix.accounts[1]
	default:
		return transfer{}, false
	}
	return t, true
}

// subtreeTransfers returns all transfers executed inside root (excluding
// root itself) in execution order.
func (v *txView) subtreeTransfers(root *instruction) []transfer {
	var out []transfer
	root.walk(func(ix *instruction) bool {
		if t, ok := v.decodeTransfer(ix); ok && t.amount > 0 {
			out = append(out, t)
		}
		return true
	})
	return out
}

// accountFunding returns the rent payer deposited into accounts created by
// the transaction that still exist afterwards, and the rent it recovered by
// closing accounts that existed before. Accounts created and closed within
// the transaction cancel out and are ignored. Neither is part of a trade.
func (v *txView) accountFunding(payer solana.PublicKey) (deposited, refunded uint64) {
	closed := make(map[solana.PublicKey]bool)
	for _, ix := range v.flat {
		switch {
		case ix.programID.Equals(solana.SystemProgramID) && len(ix.data) >= 12 && len(ix.accounts) >= 2:
			tag := binary.LittleEndian.Uint32(ix.data[:4])
			if ix.accounts[0] != payer || v.postLamports(ix.accounts[1]) == 0 {
				continue
			}
			switch tag {
			case systemIxCreateAccount: // tag, lamports, space, owner
				deposited += binary.LittleEndian.Uint64(ix.data[4:12])
			case systemIxCreateAccountWithSeed:
				if lamports, ok := createWithSeedLamports(ix.data); ok {
					deposited += lamports
				}
			}
		case isTokenProgram(ix.programID) && len(ix.data) > 0 && ix.data[0] == tokenIxCloseAccount && len(ix.accounts) >= 2:
			acct := ix.accounts[0]
			if ix.accounts[1] != payer || closed[acct] {
				continue
			}
			closed[acct] = true
			// The refund is the account's rent plus, for wrapped SOL, its
			// token balance; the token balance delta already covers the latter.
			rent := v.preLamports(acct)
			if v.tokenAccounts[acct].mint == solana.WrappedSol {
				rent -= min(rent, v.preTokenAmount(acct))
			}
			refunded += rent
		}
	}
	return deposited, refunded
}

func createWithSeedLamports(d []byte) (uint64, bool) {
	// tag(4) | base(32) | seed len(u64) | seed | lamports(u64) | space | owner
	if len(d) < 4+32+8 {
		return 0, false
	}
	seedLen := binary.LittleEndian.Uint64(d[36:44])
	off := 44 + seedLen
	if seedLen > 32 || uint64(len(d)) < off+8 {
		return 0, false
	}
	return binary.LittleEndian.Uint64(d[off : off+8]), true
}
