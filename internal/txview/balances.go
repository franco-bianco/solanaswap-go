package txview

import (
	"math"
	"strconv"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// Holding identifies the balance of one mint held by one owner.
type Holding struct {
	Owner solana.PublicKey
	Mint  solana.PublicKey
}

// TokenChanges returns the post-pre raw token balance change of every holding.
func (v *View) TokenChanges() map[Holding]int64 {
	type sums struct{ pre, post uint64 }
	totals := make(map[Holding]*sums)
	add := func(balances []rpc.TokenBalance, post bool) {
		for _, b := range balances {
			if b.UiTokenAmount == nil || int(b.AccountIndex) >= len(v.keys) {
				continue
			}
			amount, err := strconv.ParseUint(b.UiTokenAmount.Amount, 10, 64)
			if err != nil {
				continue
			}
			h := Holding{Owner: v.OwnerOf(v.keys[b.AccountIndex]), Mint: b.Mint}
			if totals[h] == nil {
				totals[h] = &sums{}
			}
			if post {
				totals[h].post += amount
			} else {
				totals[h].pre += amount
			}
		}
	}
	add(v.meta.PreTokenBalances, false)
	add(v.meta.PostTokenBalances, true)

	changes := make(map[Holding]int64, len(totals))
	for h, s := range totals {
		changes[h] = signedDiff(s.post, s.pre)
	}
	return changes
}

// LamportChange returns the post-pre lamport balance change of addr.
func (v *View) LamportChange(addr solana.PublicKey) int64 {
	pre, post := v.lamports(addr)
	return signedDiff(post, pre)
}

func (v *View) lamports(addr solana.PublicKey) (pre, post uint64) {
	i, ok := v.keyIndex[addr]
	if !ok {
		return 0, 0
	}
	if i < len(v.meta.PreBalances) {
		pre = v.meta.PreBalances[i]
	}
	if i < len(v.meta.PostBalances) {
		post = v.meta.PostBalances[i]
	}
	return pre, post
}

func (v *View) preTokenAmount(addr solana.PublicKey) uint64 {
	for _, b := range v.meta.PreTokenBalances {
		if int(b.AccountIndex) < len(v.keys) && v.keys[b.AccountIndex] == addr && b.UiTokenAmount != nil {
			n, _ := strconv.ParseUint(b.UiTokenAmount.Amount, 10, 64)
			return n
		}
	}
	return 0
}

// signedDiff returns a-b clamped to the int64 range.
func signedDiff(a, b uint64) int64 {
	if a >= b {
		return int64(min(a-b, math.MaxInt64))
	}
	return -int64(min(b-a, math.MaxInt64))
}
