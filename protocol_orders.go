package solanaswap

import (
	"github.com/gagliardetto/solana-go"

	"github.com/franco-bianco/solanaswap-go/v2/internal/borsh"
	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

// Keeper-filled orders swap on behalf of an order owner who does not sign.
var (
	// Jupiter Recurring (DCA) emits Filled(user_key, dca_key, ...) through self-CPI.
	jupiterRecurringFilled = txview.Discriminator{0x86, 0x04, 0x11, 0x3f, 0xdd, 0x2d, 0xb1, 0xad}

	// Jupiter Trigger (limit orders) fill_order and flash_fill_order take [taker, maker, order, ...].
	jupiterTriggerFill         = txview.Discriminator{0xe8, 0x7a, 0x73, 0x19, 0xc7, 0x8f, 0x88, 0xa2}
	jupiterTriggerFlashFill    = txview.Discriminator{0xfc, 0x68, 0x12, 0x86, 0xa4, 0x4e, 0x12, 0x8c}
	jupiterTriggerMakerAccount = 1
)

// orderOwner returns the owner of a keeper-filled order in the transaction, if any.
func (e *extractor) orderOwner() (solana.PublicKey, bool) {
	for _, ix := range e.v.Flat {
		if ix.ProgramID == jupiterTriggerProgramID &&
			(jupiterTriggerFill.Matches(ix.Data) || jupiterTriggerFlashFill.Matches(ix.Data)) {
			if maker := ix.Account(jupiterTriggerMakerAccount); !maker.IsZero() {
				return maker, true
			}
		}
		if ix.ProgramID != jupiterRecurringProgramID {
			continue
		}
		for _, data := range ix.CPIEvents(jupiterRecurringFilled) {
			r := borsh.NewReader(data)
			if user := r.Pubkey(); r.OK() {
				return user, true
			}
		}
	}
	return solana.PublicKey{}, false
}
