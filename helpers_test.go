package solanaswap

import (
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// txBuilder assembles synthetic transactions for shapes that are hard to find on chain.
// Every account starts with 1 SOL.
type txBuilder struct {
	keys     []solana.PublicKey
	index    map[solana.PublicKey]uint16
	signers  uint8
	outer    []solana.CompiledInstruction
	inner    map[int][]rpc.CompiledInstruction
	pre      []uint64
	post     []uint64
	preTok   []rpc.TokenBalance
	postTok  []rpc.TokenBalance
	logLines []string
}

func newTxBuilder(signers ...solana.PublicKey) *txBuilder {
	b := &txBuilder{index: map[solana.PublicKey]uint16{}, inner: map[int][]rpc.CompiledInstruction{}}
	for _, s := range signers {
		b.key(s)
	}
	b.signers = uint8(len(signers))
	return b
}

func (b *txBuilder) key(k solana.PublicKey) uint16 {
	if i, ok := b.index[k]; ok {
		return i
	}
	b.index[k] = uint16(len(b.keys))
	b.keys = append(b.keys, k)
	b.pre = append(b.pre, 1_000_000_000)
	b.post = append(b.post, 1_000_000_000)
	return b.index[k]
}

func (b *txBuilder) keyIndexes(ks []solana.PublicKey) []uint16 {
	out := make([]uint16, len(ks))
	for i, k := range ks {
		out[i] = b.key(k)
	}
	return out
}

func (b *txBuilder) top(program solana.PublicKey, data []byte, accounts ...solana.PublicKey) int {
	b.outer = append(b.outer, solana.CompiledInstruction{ProgramIDIndex: b.key(program), Accounts: b.keyIndexes(accounts), Data: data})
	return len(b.outer) - 1
}

func (b *txBuilder) cpi(outer int, height uint16, program solana.PublicKey, data []byte, accounts ...solana.PublicKey) {
	b.inner[outer] = append(b.inner[outer], rpc.CompiledInstruction{
		ProgramIDIndex: b.key(program), Accounts: b.keyIndexes(accounts), Data: data, StackHeight: height,
	})
}

// tokenTransfer adds an SPL Token Transfer CPI.
func (b *txBuilder) tokenTransfer(outer int, height uint16, amount uint64, from, to, authority solana.PublicKey) {
	b.cpi(outer, height, solana.TokenProgramID, binary.LittleEndian.AppendUint64([]byte{3}, amount), from, to, authority)
}

// balance records a token account's pre and post balance.
func (b *txBuilder) balance(account, mint, owner solana.PublicKey, pre, post uint64) {
	o := owner
	for i, amount := range []uint64{pre, post} {
		tb := rpc.TokenBalance{AccountIndex: b.key(account), Mint: mint, Owner: &o,
			UiTokenAmount: &rpc.UiTokenAmount{Amount: strconv.FormatUint(amount, 10), Decimals: 6}}
		if i == 0 {
			b.preTok = append(b.preTok, tb)
		} else {
			b.postTok = append(b.postTok, tb)
		}
	}
}

func (b *txBuilder) parse(t *testing.T) (*SwapInfo, error) {
	t.Helper()
	tx := &solana.Transaction{Message: solana.Message{
		AccountKeys:  b.keys,
		Header:       solana.MessageHeader{NumRequiredSignatures: b.signers},
		Instructions: b.outer,
	}}
	meta := &rpc.TransactionMeta{Fee: 5000, PreBalances: b.pre, PostBalances: b.post,
		PreTokenBalances: b.preTok, PostTokenBalances: b.postTok, LogMessages: b.logLines}
	for i := range b.outer {
		if ins, ok := b.inner[i]; ok {
			meta.InnerInstructions = append(meta.InnerInstructions, rpc.InnerInstruction{Index: uint16(i), Instructions: ins})
		}
	}
	return ParseTransaction(tx, meta)
}

func testKey(n int) solana.PublicKey {
	var k solana.PublicKey
	binary.LittleEndian.PutUint64(k[:], uint64(n)+1000)
	k[31] = 7
	return k
}

func sysTransfer(lamports uint64) []byte {
	return binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, lamports)
}

func moonshotSellEvent(tokens, collateral uint64, sender solana.PublicKey) string {
	d := append([]byte{}, moonshotTradeEvent[:]...)
	for _, v := range []uint64{tokens, collateral, 0, 0, 0} {
		d = binary.LittleEndian.AppendUint64(d, v)
	}
	d = append(d, make([]byte, 32)...) // curve
	d = append(d, solana.WrappedSol[:]...)
	d = append(d, sender[:]...)
	d = append(d, 1)          // sell
	d = append(d, 0, 0, 0, 0) // empty label
	return base64.StdEncoding.EncodeToString(d)
}
