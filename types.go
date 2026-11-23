package solanaswap

import (
	"time"

	"github.com/gagliardetto/solana-go"
)

// SwapInfo summarises the swap activity of a transaction from the trader's
// point of view. For multi-hop or split routes, TokenIn is the route's first
// input and TokenOut its final output; the individual hops are in Swaps.
type SwapInfo struct {
	Signatures []solana.Signature
	Signers    []solana.PublicKey
	Slot       uint64
	// Timestamp is the block time. It is the zero time when the RPC node did
	// not report one.
	Timestamp time.Time

	// Trader is the wallet (or program-derived account) whose tokens were
	// swapped. It is usually the fee payer, but differs for keeper-executed
	// orders and custodial routers.
	Trader solana.PublicKey

	// AMMs lists the venues that executed the swaps, in execution order.
	AMMs []string
	// Routers lists the aggregators and trading bots the swaps went through.
	Routers []string

	TokenInMint     solana.PublicKey
	TokenInAmount   uint64
	TokenInDecimals uint8

	TokenOutMint     solana.PublicKey
	TokenOutAmount   uint64
	TokenOutDecimals uint8

	// Swaps holds every individual swap (route hop) in execution order.
	Swaps []Swap

	// TransactionFee is the network fee in lamports (base + priority fee).
	TransactionFee uint64
	// Fees lists payments the trader made outside the swap instructions,
	// such as trading-bot fees and validator tips. They are not included in
	// TokenInAmount.
	Fees []Fee
}

// Swap is a single trade executed by one DEX program, i.e. one hop of a route.
//
// InputAmount is what left the trader's side into the DEX instruction and
// OutputAmount is what the DEX instruction paid out, so fees charged by the
// DEX itself (LP, protocol and creator fees) are reflected in the amounts.
type Swap struct {
	// Protocol is the human-readable venue name, e.g. "Raydium CPMM".
	Protocol  string
	ProgramID solana.PublicKey
	// Trader is the authority that supplied the input tokens for this hop.
	// For routed hops this can be a router-owned account.
	Trader solana.PublicKey

	InputMint     solana.PublicKey
	InputAmount   uint64
	InputDecimals uint8

	OutputMint     solana.PublicKey
	OutputAmount   uint64
	OutputDecimals uint8

	// InstructionIndex is the index of the top-level instruction that
	// contains the swap; InnerInstructionIndex is the index of the swap
	// instruction within that instruction's inner instructions, or -1 when
	// the swap is the top-level instruction itself.
	InstructionIndex      int
	InnerInstructionIndex int
}

// FeeKind classifies a payment made alongside a swap.
type FeeKind string

const (
	// FeeKindTip is a tip to a block-engine or transaction-landing service
	// (Jito, Nozomi, bloXroute, ...).
	FeeKindTip FeeKind = "tip"
	// FeeKindPlatform is any other payment, typically a trading-bot,
	// front-end or aggregator commission.
	FeeKindPlatform FeeKind = "platform"
)

// Fee is a transfer from the trader that is not part of a swap.
type Fee struct {
	Kind      FeeKind
	Mint      solana.PublicKey
	Amount    uint64
	Recipient solana.PublicKey
}
