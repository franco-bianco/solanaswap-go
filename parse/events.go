package parse

import (
	"bytes"
	"fmt"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

var (
	PumpfunTradeEventDiscriminator  = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 189, 219, 127, 211, 78, 230, 97, 238}
	PumpfunCreateEventDiscriminator = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 27, 114, 169, 77, 222, 235, 99, 118}
	JupiterRouteEventDiscriminator  = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 64, 198, 205, 232, 38, 8, 113, 226}
)

func parseForEvents(instruction solana.CompiledInstruction) (interface{}, error) {
	if len(instruction.Data) < 16 {
		return nil, nil
	}

	decodedBytes, err := base58.Decode(instruction.Data.String())
	if err != nil {
		return nil, fmt.Errorf("error decoding instruction data: %s", err)
	}

	discriminator := decodedBytes[:16]
	decoder := ag_binary.NewBorshDecoder(decodedBytes[16:])

	switch {
	case bytes.Equal(discriminator, PumpfunTradeEventDiscriminator[:]):
		return handlePumpfunTradeEvent(decoder)
	case bytes.Equal(discriminator, PumpfunCreateEventDiscriminator[:]):
		return handlePumpfunCreateEvent(decoder)
	case bytes.Equal(discriminator, JupiterRouteEventDiscriminator[:]):
		return handleJupiterRouteEvent(decoder)
	default:
		return nil, nil
	}
}

// handlePumpfunTradeEvent handles the Pumpfun trade event
func handlePumpfunTradeEvent(decoder *ag_binary.Decoder) (*PumpfunTradeEvent, error) {
	var trade PumpfunTradeEvent
	if err := decoder.Decode(&trade); err != nil {
		return nil, fmt.Errorf("error unmarshaling TradeEvent: %s", err)
	}

	return &trade, nil
}

// handlePumpfunCreateEvent handles the Pumpfun create event
func handlePumpfunCreateEvent(decoder *ag_binary.Decoder) (*PumpfunCreateEvent, error) {
	var create PumpfunCreateEvent
	if err := decoder.Decode(&create); err != nil {
		return nil, fmt.Errorf("error unmarshaling CreateEvent: %s", err)
	}

	return &create, nil
}

// handleJupiterRouteEvent handles the Jupiter route event
func handleJupiterRouteEvent(decoder *ag_binary.Decoder) (*JupiterSwapEvent, error) {
	var event JupiterSwapEvent
	if err := decoder.Decode(&event); err != nil {
		return nil, fmt.Errorf("error unmarshaling JupiterSwapEvent: %s", err)
	}
	return &event, nil
}
