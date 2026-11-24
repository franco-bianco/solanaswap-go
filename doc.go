// Package solanaswap extracts swaps from confirmed Solana transactions, whichever bot,
// aggregator or program wraps the DEX call. Use [Parse], [ParseJSON] or [ParseTransaction].
//
// Each DEX invocation becomes a [Swap], decoded from its events or token transfers, and the
// swaps are summarised in a [SwapInfo]. See the README for amount semantics and coverage.
package solanaswap
