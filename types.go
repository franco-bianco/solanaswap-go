package solanaswap

import (
	"time"

	"github.com/gagliardetto/solana-go"
)

// SwapInfo summarises a transaction's swaps for the trader as one route: TokenIn is the
// first input and TokenOut the final output. Unrelated swaps are told apart via Swaps.
type SwapInfo struct {
	Signatures []solana.Signature
	Signers    []solana.PublicKey
	Slot       uint64
	// Timestamp is the block time, or the zero time when the node did not report one.
	Timestamp time.Time

	// Trader is the wallet whose tokens were swapped: usually the fee payer, but the user
	// for relayer-paid swaps and the order owner for keeper-filled orders.
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
	// Fees lists payments made outside the swaps (bot fees, tips); TokenInAmount excludes them.
	Fees []Fee
}

// Swap is one hop of a route, executed by one DEX program. Amounts are as sent into and out
// of the DEX, so DEX fees are included and Token-2022 transfer fees are not deducted.
type Swap struct {
	// Protocol is the human-readable venue name, e.g. "Raydium CPMM".
	Protocol  string
	ProgramID solana.PublicKey
	// Trader is the authority that supplied this hop's input, possibly a router account.
	Trader solana.PublicKey

	InputMint     solana.PublicKey
	InputAmount   uint64
	InputDecimals uint8

	OutputMint     solana.PublicKey
	OutputAmount   uint64
	OutputDecimals uint8

	// InstructionIndex is the top-level instruction holding the swap and InnerInstructionIndex
	// its inner index (-1 at top level). Both are -1 for swaps inferred from balance changes.
	InstructionIndex      int
	InnerInstructionIndex int
}

// FeeKind classifies a payment made alongside a swap.
type FeeKind string

const (
	// FeeKindTip is a tip to a block engine or landing service (Jito, Nozomi, bloXroute, ...).
	FeeKindTip FeeKind = "tip"
	// FeeKindPlatform is any other payment, typically a bot, wallet or aggregator commission.
	FeeKindPlatform FeeKind = "platform"
)

// Fee is a payment from the trader's side that is not part of a swap.
type Fee struct {
	Kind      FeeKind
	Mint      solana.PublicKey
	Amount    uint64
	Recipient solana.PublicKey

	payer solana.PublicKey // authority that signed the payment
}
