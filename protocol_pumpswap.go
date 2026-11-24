package solanaswap

import (
	"github.com/gagliardetto/solana-go"

	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

// PumpSwap multi_hop_swap takes 16 fixed accounts, then per hop: base mint, quote mint,
// pool, base vault, quote vault.
var pumpSwapMultiHop = txview.Discriminator{0x2b, 0x64, 0x49, 0x13, 0xe9, 0xf6, 0x6f, 0x94}

const (
	pumpSwapMultiHopFixedAccounts = 16
	pumpSwapMultiHopHopAccounts   = 5
)

// decodePumpSwap splits a multi-hop swap into one Swap per hop, measured at the hop's vaults.
// Other instructions, and hops through SOL bonding curves, use the transfer-based decoder.
func (e *extractor) decodePumpSwap(ix *txview.Instruction) []Swap {
	if !pumpSwapMultiHop.Matches(ix.Data) {
		return nil
	}
	rest := len(ix.Accounts) - pumpSwapMultiHopFixedAccounts
	if rest < 2*pumpSwapMultiHopHopAccounts || rest%pumpSwapMultiHopHopAccounts != 0 {
		return nil
	}
	whole, ok := e.transferSwap(ix, "")
	if !ok {
		return nil
	}

	transfers := ix.Transfers()
	vaults := make(map[solana.PublicKey]bool)
	for off := pumpSwapMultiHopFixedAccounts; off < len(ix.Accounts); off += pumpSwapMultiHopHopAccounts {
		vaults[ix.Accounts[off+3]], vaults[ix.Accounts[off+4]] = true, true
	}

	held := whole.InputMint
	var swaps []Swap
	for off := pumpSwapMultiHopFixedAccounts; off < len(ix.Accounts); off += pumpSwapMultiHopHopAccounts {
		hop := ix.Accounts[off : off+pumpSwapMultiHopHopAccounts]
		baseMint, quoteMint, baseVault, quoteVault := hop[0], hop[1], hop[3], hop[4]

		// Holding the quote mint makes the hop a buy, holding the base a sell.
		inMint, inVault, outMint, outVault := quoteMint, quoteVault, baseMint, baseVault
		if held == baseMint {
			inMint, inVault, outMint, outVault = baseMint, baseVault, quoteMint, quoteVault
		} else if held != quoteMint {
			return nil
		}

		// Output counts only payouts to the next hop or the user, not protocol or creator fees.
		var in, out uint64
		for _, t := range transfers {
			switch {
			case t.Mint == inMint && t.Destination == inVault:
				in += t.Amount
			case t.Mint == outMint && t.Source == outVault && (vaults[t.Destination] || e.isUserSide(t.Destination)):
				out += t.Amount
			}
		}
		if in == 0 || out == 0 {
			return nil
		}
		swaps = append(swaps, Swap{
			Trader:    whole.Trader,
			InputMint: inMint, InputAmount: in,
			OutputMint: outMint, OutputAmount: out,
		})
		held = outMint
	}
	return swaps
}
