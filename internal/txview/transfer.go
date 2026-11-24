package txview

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

// Transfer is a token or native SOL movement. Native SOL uses the wrapped SOL mint.
type Transfer struct {
	Mint        solana.PublicKey // zero when the mint cannot be determined
	Amount      uint64
	Source      solana.PublicKey
	Destination solana.PublicKey
	Authority   solana.PublicKey
}

// decodeTransfer decodes Token, Token-2022 and System program transfers.
func (v *View) decodeTransfer(ix *Instruction) (Transfer, bool) {
	switch {
	case isTokenProgram(ix.ProgramID):
		return v.decodeTokenTransfer(ix)
	case ix.ProgramID.Equals(solana.SystemProgramID):
		return decodeSystemTransfer(ix)
	}
	return Transfer{}, false
}

func (v *View) decodeTokenTransfer(ix *Instruction) (Transfer, bool) {
	d := ix.Data
	if len(d) == 0 {
		return Transfer{}, false
	}
	var t Transfer
	switch {
	case d[0] == tokenIxTransfer && len(d) >= 9 && len(ix.Accounts) >= 3:
		t.Amount = binary.LittleEndian.Uint64(d[1:9])
		t.Source, t.Destination, t.Authority = ix.Accounts[0], ix.Accounts[1], ix.Accounts[2]
		t.Mint = v.TokenAccounts[t.Source].Mint
		if t.Mint.IsZero() {
			t.Mint = v.TokenAccounts[t.Destination].Mint
		}
	case d[0] == tokenIxTransferChecked && len(d) >= 9 && len(ix.Accounts) >= 4,
		d[0] == tokenIxTransferFeeExt && len(d) >= 10 && d[1] == transferFeeExtWithFee && len(ix.Accounts) >= 4:
		offset := 1
		if d[0] == tokenIxTransferFeeExt {
			offset = 2
		}
		t.Amount = binary.LittleEndian.Uint64(d[offset : offset+8])
		t.Source, t.Mint, t.Destination, t.Authority = ix.Accounts[0], ix.Accounts[1], ix.Accounts[2], ix.Accounts[3]
	default:
		return Transfer{}, false
	}
	return t, true
}

func decodeSystemTransfer(ix *Instruction) (Transfer, bool) {
	d := ix.Data
	if len(d) < 12 {
		return Transfer{}, false
	}
	t := Transfer{Mint: solana.WrappedSol}
	switch tag := binary.LittleEndian.Uint32(d[:4]); {
	case tag == systemIxTransfer && len(ix.Accounts) >= 2:
		t.Amount = binary.LittleEndian.Uint64(d[4:12])
		t.Source, t.Destination, t.Authority = ix.Accounts[0], ix.Accounts[1], ix.Accounts[0]
	case tag == systemIxTransferWithSeed && len(ix.Accounts) >= 3:
		t.Amount = binary.LittleEndian.Uint64(d[4:12])
		t.Source, t.Destination, t.Authority = ix.Accounts[0], ix.Accounts[2], ix.Accounts[1]
	default:
		return Transfer{}, false
	}
	return t, true
}

// AccountFunding returns the rent payer deposited into new accounts that persist, and the rent
// it recovered by closing pre-existing accounts. Neither is part of a trade.
func (v *View) AccountFunding(payer solana.PublicKey) (deposited, refunded uint64) {
	closed := make(map[solana.PublicKey]bool)
	for _, ix := range v.Flat {
		switch {
		case ix.ProgramID.Equals(solana.SystemProgramID) && len(ix.Data) >= 12 && len(ix.Accounts) >= 2:
			if _, post := v.lamports(ix.Accounts[1]); ix.Accounts[0] != payer || post == 0 {
				continue
			}
			switch binary.LittleEndian.Uint32(ix.Data[:4]) {
			case systemIxCreateAccount: // tag, lamports, space, owner
				deposited += binary.LittleEndian.Uint64(ix.Data[4:12])
			case systemIxCreateAccountWithSeed:
				if lamports, ok := createWithSeedLamports(ix.Data); ok {
					deposited += lamports
				}
			}
		case isTokenProgram(ix.ProgramID) && len(ix.Data) > 0 && ix.Data[0] == tokenIxCloseAccount &&
			len(ix.Accounts) >= 2:
			acct := ix.Accounts[0]
			if ix.Accounts[1] != payer || closed[acct] {
				continue
			}
			closed[acct] = true
			// A wrapped SOL refund also returns the token balance, which balance changes already cover.
			rent, _ := v.lamports(acct)
			if v.TokenAccounts[acct].Mint == solana.WrappedSol {
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
