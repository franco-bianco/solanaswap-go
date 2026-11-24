// Package txview decodes a confirmed transaction into an instruction tree with
// resolved accounts, token transfers, balance changes and emitted events.
package txview

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// View is a decoded, index-resolved view of a confirmed transaction.
type View struct {
	Signers []solana.PublicKey // in account order
	Fee     uint64             // network fee in lamports

	Roots []*Instruction // top-level instructions
	Flat  []*Instruction // all instructions in execution order

	Decimals map[solana.PublicKey]uint8

	tokenAccounts map[solana.PublicKey]tokenAccount

	tx        *solana.Transaction
	meta      *rpc.TransactionMeta
	keys      solana.PublicKeySlice // static keys, then lookup-table writable and read-only keys
	keyIndex  map[solana.PublicKey]int
	isSigner  map[solana.PublicKey]bool
	logEvents map[*Instruction][][]byte // "Program data:" payloads per emitting instruction
}

// tokenAccount is what the transaction reveals about an SPL token account.
type tokenAccount struct {
	Mint  solana.PublicKey
	Owner solana.PublicKey
}

// New decodes tx; tx and meta must not be nil. For v0 transactions meta.LoadedAddresses
// must hold the lookup-table keys.
func New(tx *solana.Transaction, meta *rpc.TransactionMeta) (*View, error) {
	msg := tx.Message
	keys := make(solana.PublicKeySlice, 0, len(msg.AccountKeys)+
		len(meta.LoadedAddresses.Writable)+len(meta.LoadedAddresses.ReadOnly))
	keys = append(keys, msg.AccountKeys...)
	keys = append(keys, meta.LoadedAddresses.Writable...)
	keys = append(keys, meta.LoadedAddresses.ReadOnly...)

	numSigners := min(int(msg.Header.NumRequiredSignatures), len(msg.AccountKeys))
	v := &View{
		Signers:       msg.AccountKeys[:numSigners:numSigners],
		Fee:           meta.Fee,
		tx:            tx,
		meta:          meta,
		keys:          keys,
		tokenAccounts: make(map[solana.PublicKey]tokenAccount),
		Decimals:      map[solana.PublicKey]uint8{solana.WrappedSol: 9},
		keyIndex:      make(map[solana.PublicKey]int, len(keys)),
		isSigner:      make(map[solana.PublicKey]bool, numSigners),
	}
	for i, k := range keys {
		if _, dup := v.keyIndex[k]; !dup {
			v.keyIndex[k] = i
		}
	}
	for _, s := range v.Signers {
		v.isSigner[s] = true
	}

	if err := v.buildTree(); err != nil {
		return nil, err
	}
	v.indexTokenAccounts()
	for _, ix := range v.Flat {
		if t, ok := v.decodeTransfer(ix); ok && t.Amount > 0 {
			ix.Transfer = &t
		}
	}
	v.mapLogEvents()
	return v, nil
}

func (v *View) key(index uint16) (solana.PublicKey, error) {
	if int(index) >= len(v.keys) {
		return solana.PublicKey{}, fmt.Errorf(
			"account index %d out of range (%d keys; v0 transactions need meta.LoadedAddresses)", index, len(v.keys))
	}
	return v.keys[index], nil
}

// IsSigner reports whether k signed the transaction.
func (v *View) IsSigner(k solana.PublicKey) bool { return v.isSigner[k] }

// IsTokenAccount reports whether addr is a token account the transaction touches.
func (v *View) IsTokenAccount(addr solana.PublicKey) bool {
	_, ok := v.tokenAccounts[addr]
	return ok
}

// FeePayer returns the first account, which pays the fee, or the zero key if there is none.
func (v *View) FeePayer() solana.PublicKey {
	if len(v.tx.Message.AccountKeys) == 0 {
		return solana.PublicKey{}
	}
	return v.tx.Message.AccountKeys[0]
}

// OwnerOf returns the wallet that controls addr: the owner of a token account, else addr itself.
func (v *View) OwnerOf(addr solana.PublicKey) solana.PublicKey {
	if acct, ok := v.tokenAccounts[addr]; ok && !acct.Owner.IsZero() {
		return acct.Owner
	}
	return addr
}

// indexTokenAccounts records mint and owner of every token account the transaction touches,
// including temporary accounts that are absent from the token balances.
func (v *View) indexTokenAccounts() {
	for _, balances := range [][]rpc.TokenBalance{v.meta.PreTokenBalances, v.meta.PostTokenBalances} {
		for _, b := range balances {
			if int(b.AccountIndex) >= len(v.keys) {
				continue
			}
			acct := tokenAccount{Mint: b.Mint}
			if b.Owner != nil {
				acct.Owner = *b.Owner
			}
			v.setTokenAccount(v.keys[b.AccountIndex], acct)
			if b.UiTokenAmount != nil {
				v.Decimals[b.Mint] = b.UiTokenAmount.Decimals
			}
		}
	}

	for _, ix := range v.Flat {
		switch {
		case isTokenProgram(ix.ProgramID):
			v.indexTokenInstruction(ix)
		case ix.ProgramID.Equals(solana.SPLAssociatedTokenAccountProgramID) && len(ix.Accounts) >= 4:
			// Create / CreateIdempotent: [payer, ata, wallet, mint, ...]
			v.setTokenAccount(ix.Accounts[1], tokenAccount{Owner: ix.Accounts[2], Mint: ix.Accounts[3]})
		}
	}
}

func (v *View) indexTokenInstruction(ix *Instruction) {
	if len(ix.Data) == 0 {
		return
	}
	switch ix.Data[0] {
	case tokenIxInitializeAccount: // [account, mint, owner, rent]
		if len(ix.Accounts) >= 3 {
			v.setTokenAccount(ix.Accounts[0], tokenAccount{Mint: ix.Accounts[1], Owner: ix.Accounts[2]})
		}
	case tokenIxInitializeAccount2, tokenIxInitializeAccount3: // [account, mint, ...], owner in data
		if len(ix.Accounts) >= 2 && len(ix.Data) >= 33 {
			owner := solana.PublicKeyFromBytes(ix.Data[1:33])
			v.setTokenAccount(ix.Accounts[0], tokenAccount{Mint: ix.Accounts[1], Owner: owner})
		}
	case tokenIxTransferChecked: // [source, mint, destination, authority]
		if len(ix.Accounts) >= 3 && len(ix.Data) >= 10 {
			v.setTokenAccount(ix.Accounts[0], tokenAccount{Mint: ix.Accounts[1]})
			v.setTokenAccount(ix.Accounts[2], tokenAccount{Mint: ix.Accounts[1]})
			v.Decimals[ix.Accounts[1]] = ix.Data[9]
		}
	}
}

// setTokenAccount merges what is known about a token account without erasing known fields.
func (v *View) setTokenAccount(addr solana.PublicKey, info tokenAccount) {
	cur := v.tokenAccounts[addr]
	if cur.Mint.IsZero() {
		cur.Mint = info.Mint
	}
	if cur.Owner.IsZero() {
		cur.Owner = info.Owner
	}
	v.tokenAccounts[addr] = cur
}
