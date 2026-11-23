package solanaswap

import "github.com/gagliardetto/solana-go"

// PumpSwap multi_hop_swap trades through two or more pump pools or bonding
// curves in one instruction. After 16 fixed accounts it takes five accounts
// per hop: base mint, quote mint, pool (or bonding curve), base vault and
// quote vault. Intermediate coins move vault to vault and never reach the
// user.
var pumpSwapMultiHop = discriminator{0x2b, 0x64, 0x49, 0x13, 0xe9, 0xf6, 0x6f, 0x94}

const (
	pumpSwapMultiHopFixedAccounts = 16
	pumpSwapMultiHopHopAccounts   = 5
)

// decodePumpSwap splits multi-hop instructions into one Swap per hop. Each
// hop's input is what was transferred into its vault of the input mint and
// its output what its vault of the output mint paid to the next hop or the
// user (excluding protocol and creator fees). Other instructions,
// and hops through SOL bonding curves (which move native SOL without
// transfers), are left to the transfer-based decoder.
func decodePumpSwap(e *extractor, ix *instruction) []Swap {
	if !hasDiscriminator(ix.data, pumpSwapMultiHop) {
		return nil
	}
	rest := len(ix.accounts) - pumpSwapMultiHopFixedAccounts
	if rest < 2*pumpSwapMultiHopHopAccounts || rest%pumpSwapMultiHopHopAccounts != 0 {
		return nil
	}
	whole, ok := e.transferSwap(ix, "")
	if !ok {
		return nil
	}

	transfers := e.v.subtreeTransfers(ix)
	vaults := make(map[solana.PublicKey]bool)
	for off := pumpSwapMultiHopFixedAccounts; off < len(ix.accounts); off += pumpSwapMultiHopHopAccounts {
		vaults[ix.accounts[off+3]], vaults[ix.accounts[off+4]] = true, true
	}

	held := whole.InputMint
	var swaps []Swap
	for off := pumpSwapMultiHopFixedAccounts; off < len(ix.accounts); off += pumpSwapMultiHopHopAccounts {
		hop := ix.accounts[off : off+pumpSwapMultiHopHopAccounts]
		baseMint, quoteMint, baseVault, quoteVault := hop[0], hop[1], hop[3], hop[4]

		// Holding the quote mint makes the hop a buy, holding the base a sell.
		inMint, inVault, outMint, outVault := quoteMint, quoteVault, baseMint, baseVault
		if held == baseMint {
			inMint, inVault, outMint, outVault = baseMint, baseVault, quoteMint, quoteVault
		} else if held != quoteMint {
			return nil
		}

		var in, out uint64
		for _, t := range transfers {
			switch {
			case t.mint == inMint && t.destination == inVault:
				in += t.amount
			case t.mint == outMint && t.source == outVault && (vaults[t.destination] || e.isUserSide(t.destination)):
				out += t.amount
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
