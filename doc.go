// Package solanaswap extracts swaps from confirmed Solana transactions.
//
// It walks every instruction of a transaction, including inner instructions
// executed through cross-program invocation, so swaps are found regardless of
// which trading bot, aggregator or custom program wraps the DEX call. Each
// invocation of a recognised DEX program becomes a [Swap]:
//
//   - Programs that emit trade events (Pump.fun, Jupiter, ...) are decoded
//     from those events.
//   - Other DEX programs are decoded from the token transfers they execute,
//     attributing each transfer to the user side or the pool side.
//   - Transactions that touch no recognised DEX fall back to the trader's
//     balance changes.
//
// The swaps are then summarised into a [SwapInfo] describing what the trader
// paid and received, with trading-bot fees and tips reported separately.
//
// Use [Parse] with a getTransaction RPC result, [ParseJSON] with the raw
// JSON of one, or [ParseTransaction] with an already decoded transaction.
package solanaswap
