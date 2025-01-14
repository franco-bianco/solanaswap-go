package solanaswapgo

import (
	"fmt"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

var (
	PumpfunTradeEventDiscriminator  = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 189, 219, 127, 211, 78, 230, 97, 238}
	PumpfunCreateEventDiscriminator = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 27, 114, 169, 77, 222, 235, 99, 118}
)

type PumpfunTradeEvent struct {
	Mint                 solana.PublicKey
	SolAmount            uint64
	TokenAmount          uint64
	IsBuy                bool
	User                 solana.PublicKey
	Timestamp            int64
	VirtualSolReserves   uint64
	VirtualTokenReserves uint64
}

type PumpfunCreateEvent struct {
	Name         string
	Symbol       string
	Uri          string
	Mint         solana.PublicKey
	BondingCurve solana.PublicKey
	User         solana.PublicKey
}

func (p *Parser) processPumpfunSwaps(instructionIndex int) []SwapData {
	var swaps []SwapData
	for _, innerInstructionSet := range p.txMeta.InnerInstructions {
		if innerInstructionSet.Index == uint16(instructionIndex) {
			for _, innerInstruction := range innerInstructionSet.Instructions {
				if p.isPumpFunTradeEventInstruction(innerInstruction) {
					eventData, err := p.parsePumpfunTradeEventInstruction(innerInstruction)
					if err != nil {
						p.Log.Errorf("error processing Pumpfun trade event: %s", err)
					}
					if eventData != nil {
						swaps = append(swaps, SwapData{Type: PUMP_FUN, Data: eventData})
					}
				}
			}
		}
	}
	return swaps
}

func (p *Parser) parsePumpfunTradeEventInstruction(instruction solana.CompiledInstruction) (*PumpfunTradeEvent, error) {
	decodedBytes, err := base58.Decode(instruction.Data.String())
	if err != nil {
		return nil, fmt.Errorf("error decoding instruction data: %s", err)
	}
	decoder := ag_binary.NewBorshDecoder(decodedBytes[16:])

	return handlePumpfunTradeEvent(decoder)
}

func handlePumpfunTradeEvent(decoder *ag_binary.Decoder) (*PumpfunTradeEvent, error) {
	var trade PumpfunTradeEvent
	if err := decoder.Decode(&trade); err != nil {
		return nil, fmt.Errorf("error unmarshaling TradeEvent: %s", err)
	}

	return &trade, nil
}
