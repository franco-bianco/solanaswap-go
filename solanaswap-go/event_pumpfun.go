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

func (p *Parser) processPumpfunAMMSwaps(instructionIndex int) []SwapData {
	var swaps []SwapData
	for _, innerInstructionSet := range p.txMeta.InnerInstructions {
		if innerInstructionSet.Index == uint16(instructionIndex) {
			for _, innerInstruction := range innerInstructionSet.Instructions {
				switch {
				case p.isTransferCheck(innerInstruction):
					transfer := p.processTransferCheck(innerInstruction)
					if transfer != nil {
						swaps = append(swaps, SwapData{Type: PUMP_FUN, Data: transfer})
					}
				case p.isTransfer(innerInstruction):
					transfer := p.processTransfer(innerInstruction)
					if transfer != nil {
						swaps = append(swaps, SwapData{Type: PUMP_FUN, Data: transfer})
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

// Helper function to determine if a PumpFun swap is a buy transaction (SOL -> Token)
// Helper function to determine if a PumpFun swap is a buy transaction (SOL -> Token)
func (p *Parser) isPumpFunAMMBuyTransaction(swaps []SwapData) bool {
	if len(swaps) == 0 {
		return false
	}

	// Get the signer's public key
	signer := p.allAccountKeys[0].String()
	p.Log.Infof("Signer address: %s", signer)

	var tokenTransfer *TransferCheck

	// Find the token transfer (non-SOL)
	for _, swap := range swaps {
		if tc, ok := swap.Data.(*TransferCheck); ok {
			if tc.Info.Mint != NATIVE_SOL_MINT_PROGRAM_ID.String() {
				tokenTransfer = tc
				break
			}
		}
	}

	if tokenTransfer == nil {
		return false
	}

	// Check if the signer's address matches the authority of token transfer
	// For a buy, the user's authority should NOT match token transfer authority
	// Log all relevant addresses
	p.Log.Infof("Token transfer authority: %s", tokenTransfer.Info.Authority)
	p.Log.Infof("Token destination: %s", tokenTransfer.Info.Destination)

	// Check if the SOL transfers have the signer as authority
	var signerIsSolAuthority bool
	for _, swap := range swaps {
		if tc, ok := swap.Data.(*TransferCheck); ok {
			if tc.Info.Mint == NATIVE_SOL_MINT_PROGRAM_ID.String() {
				p.Log.Infof("SOL transfer authority: %s", tc.Info.Authority)
				if tc.Info.Authority == signer {
					signerIsSolAuthority = true
				}
			}
		}
	}

	// This is a buy if:
	// 1. Signer is NOT the authority of the token transfer (token is received)
	// 2. Signer IS the authority of SOL transfers (SOL is spent)
	isBuy := tokenTransfer.Info.Authority != signer && signerIsSolAuthority
	p.Log.Infof("Transaction detected as buy: %v", isBuy)

	return isBuy
}
