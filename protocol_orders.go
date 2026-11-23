package solanaswap

import "github.com/gagliardetto/solana-go"

// Keeper-filled orders swap on behalf of an order owner who does not sign.
var (
	// Jupiter Recurring (DCA) emits Filled(user_key, dca_key, input_mint,
	// output_mint, in_amount, out_amount, fee_mint, fee) through self-CPI.
	jupiterRecurringFilled = discriminator{0x86, 0x04, 0x11, 0x3f, 0xdd, 0x2d, 0xb1, 0xad}

	// Jupiter Trigger (limit orders) fill_order and flash_fill_order take
	// accounts [taker, maker, order, ...].
	JupiterTriggerProgramID    = solana.MustPublicKeyFromBase58("j1o2qRpjcyUwEvwtcfhEQefh773ZgjxcVRry7LDqg5X")
	jupiterTriggerFill         = discriminator{0xe8, 0x7a, 0x73, 0x19, 0xc7, 0x8f, 0x88, 0xa2}
	jupiterTriggerFlashFill    = discriminator{0xfc, 0x68, 0x12, 0x86, 0xa4, 0x4e, 0x12, 0x8c}
	jupiterTriggerMakerAccount = 1
)

// orderOwner returns the owner of a keeper-filled order in the
// transaction, if any.
func (e *extractor) orderOwner() (solana.PublicKey, bool) {
	for _, ix := range e.v.flat {
		if ix.programID == JupiterTriggerProgramID &&
			(hasDiscriminator(ix.data, jupiterTriggerFill) || hasDiscriminator(ix.data, jupiterTriggerFlashFill)) {
			if maker := ix.account(jupiterTriggerMakerAccount); !maker.IsZero() {
				return maker, true
			}
		}
		if ix.programID != JupiterRecurringProgramID {
			continue
		}
		for _, data := range cpiEvents(ix, jupiterRecurringFilled) {
			if r := newBorshReader(data); r.ok {
				if user := r.pubkey(); r.ok {
					return user, true
				}
			}
		}
	}
	return solana.PublicKey{}, false
}
