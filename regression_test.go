package solanaswap

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// txBuilder assembles synthetic transactions for shapes that are hard to
// find on chain. Every account starts with 1 SOL.
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

var raydiumCPMM = solana.MustPublicKeyFromBase58("CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C")

// An unregistered custodial router that takes a SOL fee and forwards the
// output from its own account must not hide the registered DEX it calls.
func TestUnknownWrapperDoesNotHideDEX(t *testing.T) {
	user, feeWallet, router, pda := testKey(1), testKey(2), testKey(3), testKey(4)
	userWSOL, vaultA, vaultB, pdaB, userB, poolAuth, mintB := testKey(5), testKey(6), testKey(7), testKey(8), testKey(9), testKey(10), testKey(11)

	b := newTxBuilder(user)
	o := b.top(router, []byte{9}, user, pda)
	b.cpi(o, 2, solana.SystemProgramID, binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, 10_000_000), user, feeWallet)
	b.cpi(o, 2, raydiumCPMM, []byte{1}, user, poolAuth)
	b.tokenTransfer(o, 3, 1_000_000_000, userWSOL, vaultA, user)
	b.tokenTransfer(o, 3, 500, vaultB, pdaB, poolAuth)
	b.tokenTransfer(o, 2, 500, pdaB, userB, pda)
	b.balance(userWSOL, solana.WrappedSol, user, 1_000_000_000, 0)
	b.balance(vaultA, solana.WrappedSol, poolAuth, 1, 1_000_000_001)
	b.balance(vaultB, mintB, poolAuth, 1000, 500)
	b.balance(pdaB, mintB, pda, 0, 0)
	b.balance(userB, mintB, user, 0, 500)

	info, err := b.parse(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.AMMs) != 1 || info.AMMs[0] != "Raydium CPMM" {
		t.Errorf("AMMs = %q, want [Raydium CPMM]", info.AMMs)
	}
	if info.TokenInAmount != 1_000_000_000 || info.TokenOutAmount != 500 {
		t.Errorf("amounts = %d -> %d, want 1000000000 -> 500", info.TokenInAmount, info.TokenOutAmount)
	}
	if len(info.Fees) != 1 || info.Fees[0].Amount != 10_000_000 || info.Fees[0].Recipient != feeWallet {
		t.Errorf("Fees = %+v, want the 0.01 SOL router fee", info.Fees)
	}
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

// Moonshot sells pay SOL by lamport changes and are decoded from a log
// event. Program-written log lines must not derail the log replay, and
// without logs the balance fallback must still find the swap.
func TestMoonshotSellLogs(t *testing.T) {
	user, userATA, curveATA, curve, mint := testKey(1), testKey(2), testKey(3), testKey(4), testKey(5)
	ms, tk := moonshotProgramID.String(), solana.TokenProgramID.String()
	tests := []struct {
		name string
		logs []string
		out  uint64
	}{
		{"event", []string{
			"Program " + ms + " invoke [1]", "Program log: Instruction: Sell",
			"Program " + tk + " invoke [2]", "Program " + tk + " success",
			"Program data: " + moonshotSellEvent(1000, 1_000_000_000, user), "Program " + ms + " success",
		}, 1_000_000_000},
		{"program log ending in success", []string{
			"Program " + ms + " invoke [1]", "Program log: Instruction: Sell",
			"Program " + tk + " invoke [2]", "Program " + tk + " success",
			"Program log: curve update success", "Program log: retry failed: no",
			"Program data: " + moonshotSellEvent(1000, 1_000_000_000, user), "Program " + ms + " success",
		}, 1_000_000_000},
		{"no logs", nil, 1_000_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTxBuilder(user)
			o := b.top(moonshotProgramID, []byte{1, 2, 3, 4, 5, 6, 7, 8}, user, testKey(20), curveATA, curve, userATA, testKey(21), mint)
			b.tokenTransfer(o, 2, 1000, userATA, curveATA, user)
			b.balance(userATA, mint, user, 1000, 0)
			b.balance(curveATA, mint, curve, 5000, 6000)
			b.post[b.key(user)] += 1_000_000_000 - 5000
			b.post[b.key(curve)] -= 1_000_000_000
			b.logLines = tt.logs

			info, err := b.parse(t)
			if err != nil {
				t.Fatal(err)
			}
			if info.TokenInMint != mint || info.TokenInAmount != 1000 || info.TokenOutMint != solana.WrappedSol || info.TokenOutAmount != tt.out {
				t.Errorf("got %d %s -> %d %s", info.TokenInAmount, info.TokenInMint, info.TokenOutAmount, info.TokenOutMint)
			}
			if len(info.Fees) != 0 {
				t.Errorf("Fees = %+v, want none", info.Fees)
			}
		})
	}
}

// A deposit into another protocol after a swap is not a fee.
func TestDepositIsNotAFee(t *testing.T) {
	user, userA, userB, vaultA, vaultB, poolAuth := testKey(1), testKey(2), testKey(3), testKey(4), testKey(5), testKey(6)
	lending, reserve, lendingAuth, mintA, mintB := testKey(7), testKey(8), testKey(9), testKey(10), testKey(11)

	b := newTxBuilder(user)
	o := b.top(raydiumCPMM, []byte{1}, user)
	b.tokenTransfer(o, 2, 100, userA, vaultA, user)
	b.tokenTransfer(o, 2, 50, vaultB, userB, poolAuth)
	d := b.top(lending, []byte{1}, user)
	b.tokenTransfer(d, 2, 50, userB, reserve, user)
	b.balance(userA, mintA, user, 100, 0)
	b.balance(userB, mintB, user, 0, 0)
	b.balance(vaultA, mintA, poolAuth, 0, 100)
	b.balance(vaultB, mintB, poolAuth, 50, 0)
	b.balance(reserve, mintB, lendingAuth, 0, 50)

	info, err := b.parse(t)
	if err != nil {
		t.Fatal(err)
	}
	if info.TokenInAmount != 100 || info.TokenOutAmount != 50 || len(info.Fees) != 0 {
		t.Errorf("got %d -> %d, fees %+v; want 100 -> 50 and no fees", info.TokenInAmount, info.TokenOutAmount, info.Fees)
	}
}

func sysTransfer(lamports uint64) []byte {
	return binary.LittleEndian.AppendUint64([]byte{2, 0, 0, 0}, lamports)
}

// Transactions recorded before stack heights existed: a DEX invoked by a
// bot program must still own the transfers that follow it, but not the
// bot's own fee payment afterwards.
func TestMissingStackHeights(t *testing.T) {
	user, bot, userA, userB, vaultA, vaultB, poolAuth, mintA, mintB := testKey(1), testKey(2), testKey(3), testKey(4), testKey(5), testKey(6), testKey(7), testKey(8), testKey(9)
	trojanFees := solana.MustPublicKeyFromBase58("9yMwSPk9mrXSN7yDHUuZurAh1sjbJsfpUqjZ7SvVtdco")

	b := newTxBuilder(user)
	o := b.top(bot, []byte{1}, user)
	b.cpi(o, 0, raydiumCPMM, []byte{1}, user, poolAuth, userA, userB, vaultA, vaultB)
	b.tokenTransfer(o, 0, 100, userA, vaultA, user)
	b.tokenTransfer(o, 0, 70, vaultB, userB, poolAuth)
	b.cpi(o, 0, solana.SystemProgramID, sysTransfer(1_000_000), user, trojanFees)
	b.balance(userA, mintA, user, 100, 0)
	b.balance(userB, mintB, user, 0, 70)
	b.balance(vaultA, mintA, poolAuth, 0, 100)
	b.balance(vaultB, mintB, poolAuth, 70, 0)

	info, err := b.parse(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.AMMs) != 1 || info.AMMs[0] != "Raydium CPMM" || info.TokenInAmount != 100 || info.TokenOutAmount != 70 {
		t.Errorf("got %q %d -> %d, want Raydium CPMM 100 -> 70", info.AMMs, info.TokenInAmount, info.TokenOutAmount)
	}
	if len(info.Fees) != 1 || info.Fees[0].Amount != 1_000_000 || !slices.Contains(info.Routers, "Trojan") {
		t.Errorf("Fees = %+v, Routers = %q; want the Trojan fee", info.Fees, info.Routers)
	}
}

// An unregistered router swaps A->B on a registered DEX and B->C on an
// unregistered one, then pays C to the user: both hops must be found.
func TestUnregisteredRouterWithMixedHops(t *testing.T) {
	user, router, pda := testKey(1), testKey(2), testKey(3)
	userA, pdaA, pdaB, pdaC, userC := testKey(4), testKey(5), testKey(6), testKey(7), testKey(8)
	vA, vB1, vB2, vC, auth1, auth2, unknownVenue := testKey(9), testKey(10), testKey(11), testKey(12), testKey(13), testKey(14), testKey(15)
	mA, mB, mC := testKey(16), testKey(17), testKey(18)

	b := newTxBuilder(user)
	o := b.top(router, []byte{9}, user, pda)
	b.tokenTransfer(o, 2, 1000, userA, pdaA, user)
	b.cpi(o, 2, raydiumCPMM, []byte{1}, pda)
	b.tokenTransfer(o, 3, 1000, pdaA, vA, pda)
	b.tokenTransfer(o, 3, 500, vB1, pdaB, auth1)
	b.cpi(o, 2, unknownVenue, []byte{1}, pda)
	b.tokenTransfer(o, 3, 500, pdaB, vB2, pda)
	b.tokenTransfer(o, 3, 200, vC, pdaC, auth2)
	b.tokenTransfer(o, 2, 200, pdaC, userC, pda)
	b.balance(userA, mA, user, 1000, 0)
	b.balance(pdaA, mA, pda, 0, 0)
	b.balance(pdaB, mB, pda, 0, 0)
	b.balance(pdaC, mC, pda, 0, 0)
	b.balance(userC, mC, user, 0, 200)
	b.balance(vA, mA, auth1, 0, 1000)
	b.balance(vB1, mB, auth1, 500, 0)
	b.balance(vB2, mB, auth2, 0, 500)
	b.balance(vC, mC, auth2, 200, 0)

	info, err := b.parse(t)
	if err != nil {
		t.Fatal(err)
	}
	if info.TokenInMint != mA || info.TokenInAmount != 1000 || info.TokenOutMint != mC || info.TokenOutAmount != 200 || len(info.Swaps) != 2 {
		t.Errorf("got %d %s -> %d %s in %d swaps, want 1000 A -> 200 C in 2 swaps",
			info.TokenInAmount, info.TokenInMint, info.TokenOutAmount, info.TokenOutMint, len(info.Swaps))
	}
}

// Fees charged by a separate unregistered program are still reported when
// they go to a known bot fee wallet or tip account.
func TestSeparateFeeProgram(t *testing.T) {
	user, feeProgram, userA, userB, vaultA, vaultB, poolAuth, mintA, mintB := testKey(1), testKey(2), testKey(3), testKey(4), testKey(5), testKey(6), testKey(7), testKey(8), testKey(9)
	trojanFees := solana.MustPublicKeyFromBase58("9yMwSPk9mrXSN7yDHUuZurAh1sjbJsfpUqjZ7SvVtdco")
	jito := solana.MustPublicKeyFromBase58("96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5")

	b := newTxBuilder(user)
	o := b.top(raydiumCPMM, []byte{1}, user)
	b.tokenTransfer(o, 2, 100, userA, vaultA, user)
	b.tokenTransfer(o, 2, 70, vaultB, userB, poolAuth)
	f := b.top(feeProgram, []byte{1}, user, trojanFees, jito)
	b.cpi(f, 2, solana.SystemProgramID, sysTransfer(1_000_000), user, trojanFees)
	b.cpi(f, 2, solana.SystemProgramID, sysTransfer(2_000_000), user, jito)
	b.balance(userA, mintA, user, 100, 0)
	b.balance(userB, mintB, user, 0, 70)
	b.balance(vaultA, mintA, poolAuth, 0, 100)
	b.balance(vaultB, mintB, poolAuth, 70, 0)

	info, err := b.parse(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Fees) != 2 || info.Fees[0].Kind != FeeKindPlatform || info.Fees[1].Kind != FeeKindTip || !slices.Contains(info.Routers, "Trojan") {
		t.Errorf("Fees = %+v, Routers = %q; want a Trojan fee and a Jito tip", info.Fees, info.Routers)
	}
}

// A relayer's own tip must not be credited to the user's SOL input.
func TestFallbackIgnoresRelayerTip(t *testing.T) {
	relayer, user, venue, userX, vaultX, mintX := testKey(1), testKey(2), testKey(3), testKey(4), testKey(5), testKey(6)
	jito := solana.MustPublicKeyFromBase58("96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5")

	b := newTxBuilder(relayer, user)
	b.top(venue, []byte{1}, user)
	b.top(solana.SystemProgramID, sysTransfer(50_000_000), relayer, jito)
	b.balance(userX, mintX, user, 0, 70)
	b.balance(vaultX, mintX, venue, 70, 0)
	b.post[b.key(relayer)] -= 5000 + 50_000_000
	b.post[b.key(jito)] += 50_000_000
	b.post[b.key(user)] -= 200_000_000
	b.post[b.key(venue)] += 200_000_000

	info, err := b.parse(t)
	if err != nil {
		t.Fatal(err)
	}
	if info.Trader != user || info.TokenInMint != solana.WrappedSol || info.TokenInAmount != 200_000_000 || info.TokenOutAmount != 70 {
		t.Errorf("got trader %s %d %s -> %d, want the user 200000000 SOL -> 70", info.Trader, info.TokenInAmount, info.TokenInMint, info.TokenOutAmount)
	}
}

// A relayer pays the fee for a swap on an unrecognised venue; the fallback
// must attribute the swap to the user, not the relayer.
func TestFallbackFindsRelayedTrader(t *testing.T) {
	relayer, user, venue, userA, userB, vaultA, vaultB, mintA, mintB := testKey(1), testKey(2), testKey(3), testKey(4), testKey(5), testKey(6), testKey(7), testKey(8), testKey(9)

	b := newTxBuilder(relayer, user)
	b.top(venue, []byte{1}, user) // no inner instructions recorded
	b.balance(userA, mintA, user, 100, 0)
	b.balance(userB, mintB, user, 0, 70)
	b.balance(vaultA, mintA, venue, 0, 100)
	b.balance(vaultB, mintB, venue, 70, 0)
	b.post[b.key(relayer)] -= 5000

	info, err := b.parse(t)
	if err != nil {
		t.Fatal(err)
	}
	if info.Trader != user || info.TokenInAmount != 100 || info.TokenOutAmount != 70 {
		t.Errorf("got trader %s %d -> %d, want the user 100 -> 70", info.Trader, info.TokenInAmount, info.TokenOutAmount)
	}
}

func TestFailedTransactionKeepsReason(t *testing.T) {
	b := newTxBuilder(testKey(1))
	tx := &solana.Transaction{Message: solana.Message{AccountKeys: b.keys, Header: solana.MessageHeader{NumRequiredSignatures: 1}}}
	_, err := ParseTransaction(tx, &rpc.TransactionMeta{Err: map[string]any{"InstructionError": []any{0, "Custom"}}})
	if !errors.Is(err, ErrTransactionFailed) || err.Error() == ErrTransactionFailed.Error() {
		t.Fatalf("err = %v, want ErrTransactionFailed with the on-chain reason", err)
	}
}

func TestParseLogFrame(t *testing.T) {
	id := moonshotProgramID.String()
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

// A crafted vector length must not panic (it used to on 32-bit platforms).
func TestBorshReaderRejectsHugeLengths(t *testing.T) {
	r := newBorshReader([]byte{1, 2, 3})
	r.skip(0xffffffff, 34)
	if r.ok {
		t.Fatal("skip past the end must fail")
	}
	if r.take(-1) != nil {
		t.Fatal("negative take must fail")
	}
}

func TestProgramsDeterministic(t *testing.T) {
	first := Programs()
	for range 20 {
		again := Programs()
		for i := range first {
			if first[i] != again[i] {
				t.Fatalf("Programs() order changed at %d: %v vs %v", i, first[i], again[i])
			}
		}
	}
}
