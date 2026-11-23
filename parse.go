package solanaswap

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

var (
	// ErrNoSwap is returned when a transaction contains no swap.
	ErrNoSwap = errors.New("solanaswap: transaction contains no swap")
	// ErrTransactionFailed is returned for transactions that failed on
	// chain; they move no tokens.
	ErrTransactionFailed = errors.New("solanaswap: transaction failed on chain")
	// ErrInvalidTransaction is returned when the transaction or its metadata
	// is missing or malformed.
	ErrInvalidTransaction = errors.New("solanaswap: invalid transaction")
)

// Parse extracts the swap from a transaction returned by the getTransaction
// RPC method. Fetch it with encoding "json" or "base64" and
// MaxSupportedTransactionVersion set to 1 (the newest format), otherwise the
// node rejects versioned transactions.
func Parse(tx *rpc.GetTransactionResult) (*SwapInfo, error) {
	if tx == nil || tx.Transaction == nil {
		return nil, fmt.Errorf("%w: missing transaction", ErrInvalidTransaction)
	}
	decoded, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("%w: decode transaction: %w", ErrInvalidTransaction, err)
	}
	if decoded == nil {
		return nil, fmt.Errorf("%w: unsupported transaction encoding (use json or base64)", ErrInvalidTransaction)
	}
	info, err := ParseTransaction(decoded, tx.Meta)
	if err != nil {
		return nil, err
	}
	info.Slot = tx.Slot
	info.Timestamp = blockTime(tx.BlockTime)
	return info, nil
}

// ParseJSON parses the JSON of a getTransaction result, either the bare
// "result" object or a complete JSON-RPC response containing it. It lets
// callers parse transactions obtained from any source (files, gRPC bridges,
// webhooks) without an RPC client.
func ParseJSON(data []byte) (*SwapInfo, error) {
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Result) > 0 {
		data = envelope.Result
	}
	var tx rpc.GetTransactionResult
	if err := json.Unmarshal(data, &tx); err != nil {
		return nil, fmt.Errorf("%w: decode JSON: %w", ErrInvalidTransaction, err)
	}
	return Parse(&tx)
}

// ParseTransaction extracts the swap from a decoded transaction and its
// status metadata, for callers that obtain transactions from sources other
// than getTransaction (for example Geyser/Yellowstone streams). For v0
// transactions meta.LoadedAddresses must hold the addresses resolved from
// lookup tables. Slot and Timestamp are left unset.
func ParseTransaction(tx *solana.Transaction, meta *rpc.TransactionMeta) (*SwapInfo, error) {
	if meta != nil && meta.Err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTransactionFailed, meta.Err)
	}
	v, err := newTxView(tx, meta)
	if err != nil {
		return nil, err
	}

	e := newExtractor(v)
	e.run()

	info := &SwapInfo{
		Signatures:     slices.Clone(tx.Signatures),
		Signers:        v.signerList(),
		TransactionFee: meta.Fee,
		Trader:         e.trader(),
		Fees:           e.collectFees(),
	}
	e.labelBots(info.Fees)
	info.Routers = e.routers

	swaps := e.swaps
	if len(swaps) == 0 {
		s, ok := e.fallbackSwap(info.Fees)
		if !ok {
			return nil, ErrNoSwap
		}
		swaps = []Swap{s}
		info.Trader = s.Trader
	}
	info.Swaps = swaps
	summarize(info, swaps)
	info.Trader = e.beneficiary(info)
	// Payouts to an order owner who did not sign are not fees.
	info.Fees = slices.DeleteFunc(info.Fees, func(f Fee) bool { return f.Recipient == info.Trader })
	return info, nil
}

// fallbackSwap infers a swap from balance changes when no swap instruction
// was recognised, trying each signer (fee payer first) so that swaps paid
// for by a relayer are attributed to the user.
func (e *extractor) fallbackSwap(fees []Fee) (Swap, bool) {
	for _, signer := range e.v.signerList() {
		if s, ok := e.balanceSwap(signer, fees); ok {
			return s, true
		}
	}
	return Swap{}, false
}

// blockTime converts an optional RPC block time.
func blockTime(t *solana.UnixTimeSeconds) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time().UTC()
}
