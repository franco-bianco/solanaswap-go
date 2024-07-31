package parse

import (
	"fmt"

	ag_binary "github.com/gagliardetto/binary"
)

var (
	TradeEventDiscriminator  = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 189, 219, 127, 211, 78, 230, 97, 238}
	CreateEventDiscriminator = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 27, 114, 169, 77, 222, 235, 99, 118}
)

// handleTradeEvent handles the trade event
func handleTradeEvent(decoder *ag_binary.Decoder) (*TradeEvent, error) {
	var trade TradeEvent
	if err := decoder.Decode(&trade); err != nil {
		return nil, fmt.Errorf("error unmarshaling TradeEvent: %s", err)
	}

	return &trade, nil
}

// handleCreateEvent handles the create event
func handleCreateEvent(decoder *ag_binary.Decoder) (*CreateEvent, error) {
	var create CreateEvent
	if err := decoder.Decode(&create); err != nil {
		return nil, fmt.Errorf("error unmarshaling CreateEvent: %s", err)
	}

	return &create, nil
}
