package parse

import (
	"encoding/json"
	"fmt"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

type JupiterSwapEvent struct {
	Amm          solana.PublicKey
	InputMint    solana.PublicKey
	InputAmount  uint64
	OutputMint   solana.PublicKey
	OutputAmount uint64
}

type JupiterSwapEventData struct {
	JupiterSwapEvent
	InputMintDecimals  uint8
	OutputMintDecimals uint8
}

var JupiterRouteEventDiscriminator = [16]byte{228, 69, 165, 46, 81, 203, 154, 29, 64, 198, 205, 232, 38, 8, 113, 226}

func (p *Parser) processJupiterSwaps(instructionIndex int) []SwapData {
	var swaps []SwapData
	for _, innerInstructionSet := range p.tx.Meta.InnerInstructions {
		if innerInstructionSet.Index == uint16(instructionIndex) {
			for _, innerInstruction := range innerInstructionSet.Instructions {
				if p.isJupiterRouteEventInstruction(innerInstruction) {
					eventData, err := p.parseJupiterRouteEventInstruction(innerInstruction)
					if err != nil {
						p.Log.Errorf("error processing Pumpfun trade event: %s", err)
					}
					if eventData != nil {
						swaps = append(swaps, SwapData{Type: JUPITER, Data: eventData})
					}
				}
			}
		}
	}
	return swaps
}

// containsDCAProgram checks if the transaction contains the Jupiter DCA program.
func (p *Parser) containsDCAProgram() bool {
	for _, accountKey := range p.allAccountKeys {
		if accountKey.Equals(JUPITER_DCA_PROGRAM_ID) {
			return true
		}
	}
	return false
}

func (p *Parser) parseJupiterRouteEventInstruction(instruction solana.CompiledInstruction) (*JupiterSwapEventData, error) {

	decodedBytes, err := base58.Decode(instruction.Data.String())
	if err != nil {
		return nil, fmt.Errorf("error decoding instruction data: %s", err)
	}
	decoder := ag_binary.NewBorshDecoder(decodedBytes[16:])

	jupSwapEvent, err := handleJupiterRouteEvent(decoder)
	if err != nil {
		return nil, fmt.Errorf("error decoding jupiter swap event: %s", err)
	}

	inputMintDecimals, exists := p.splDecimalsMap[jupSwapEvent.InputMint.String()]
	if !exists {
		inputMintDecimals = 0
	}

	outputMintDecimals, exists := p.splDecimalsMap[jupSwapEvent.OutputMint.String()]
	if !exists {
		outputMintDecimals = 0
	}

	return &JupiterSwapEventData{
		JupiterSwapEvent:   *jupSwapEvent,
		InputMintDecimals:  inputMintDecimals,
		OutputMintDecimals: outputMintDecimals,
	}, nil
}

func handleJupiterRouteEvent(decoder *ag_binary.Decoder) (*JupiterSwapEvent, error) {
	var event JupiterSwapEvent
	if err := decoder.Decode(&event); err != nil {
		return nil, fmt.Errorf("error unmarshaling JupiterSwapEvent: %s", err)
	}
	return &event, nil
}

func (p *Parser) extractSPLDecimals() error {
	mintToDecimals := make(map[string]uint8)

	for _, accountInfo := range p.tx.Meta.PostTokenBalances {
		if !accountInfo.Mint.IsZero() {
			mintAddress := accountInfo.Mint.String()
			mintToDecimals[mintAddress] = uint8(accountInfo.UiTokenAmount.Decimals)
		}
	}

	processInstruction := func(instr solana.CompiledInstruction) {
		if !p.allAccountKeys[instr.ProgramIDIndex].Equals(solana.TokenProgramID) {
			return
		}

		if len(instr.Data) == 0 || (instr.Data[0] != 3 && instr.Data[0] != 12) {
			return
		}

		if len(instr.Accounts) < 3 {
			return
		}

		mint := p.allAccountKeys[instr.Accounts[1]].String()
		if _, exists := mintToDecimals[mint]; !exists {
			mintToDecimals[mint] = 0
		}
	}

	for _, instr := range p.txInfo.Message.Instructions {
		processInstruction(instr)
	}
	for _, innerSet := range p.tx.Meta.InnerInstructions {
		for _, instr := range innerSet.Instructions {
			processInstruction(instr)
		}
	}

	// Add Native SOL if not present
	if _, exists := mintToDecimals[NATIVE_SOL_MINT_PROGRAM_ID.String()]; !exists {
		mintToDecimals[NATIVE_SOL_MINT_PROGRAM_ID.String()] = 9 // Native SOL has 9 decimal places
	}

	p.splDecimalsMap = mintToDecimals

	return nil
}

type jupiterSwapInfo struct {
	AMMs     []string
	TokenIn  map[string]uint64
	TokenOut map[string]uint64
	Decimals map[string]uint8
}

// parseJupiterEvents parses Jupiter swap events and returns an intermediate representation of the swap data. logic differs slightly due to the need to track intermediate tokens.
func parseJupiterEvents(events []SwapData) (*jupiterSwapInfo, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events provided")
	}

	intermediateInfo := &jupiterSwapInfo{
		AMMs:     make([]string, 0, len(events)),
		TokenIn:  make(map[string]uint64),
		TokenOut: make(map[string]uint64),
		Decimals: make(map[string]uint8),
	}

	for _, event := range events {
		if event.Type != "Jupiter" {
			continue
		}

		var jupiterEvent JupiterSwapEventData
		eventData, err := json.Marshal(event.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal event data: %v", err)
		}

		if err := json.Unmarshal(eventData, &jupiterEvent); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Jupiter event data: %v", err)
		}

		intermediateInfo.AMMs = append(intermediateInfo.AMMs, string(JUPITER))

		inputMint := jupiterEvent.InputMint.String()
		outputMint := jupiterEvent.OutputMint.String()

		intermediateInfo.TokenIn[inputMint] += jupiterEvent.InputAmount
		intermediateInfo.TokenOut[outputMint] += jupiterEvent.OutputAmount

		intermediateInfo.Decimals[inputMint] = jupiterEvent.InputMintDecimals
		intermediateInfo.Decimals[outputMint] = jupiterEvent.OutputMintDecimals
	}

	for mint, inAmount := range intermediateInfo.TokenIn {
		if outAmount, exists := intermediateInfo.TokenOut[mint]; exists && inAmount == outAmount {
			delete(intermediateInfo.TokenIn, mint)
			delete(intermediateInfo.TokenOut, mint)
		}
	}

	if len(intermediateInfo.TokenIn) == 0 || len(intermediateInfo.TokenOut) == 0 {
		return nil, fmt.Errorf("invalid swap: all tokens were removed as intermediates")
	}

	return intermediateInfo, nil
}

// convertToSwapInfo converts the intermediate Jupiter swap data to a SwapInfo struct.
func (p *Parser) convertToSwapInfo(intermediateInfo *jupiterSwapInfo) (*SwapInfo, error) {

	if len(intermediateInfo.TokenIn) != 1 || len(intermediateInfo.TokenOut) != 1 {
		return nil, fmt.Errorf("invalid swap: expected 1 input and 1 output token, got %d input(s) and %d output(s)",
			len(intermediateInfo.TokenIn),
			len(intermediateInfo.TokenOut),
		)
	}

	swapInfo := &SwapInfo{
		AMMs: intermediateInfo.AMMs,
	}

	for mint, amount := range intermediateInfo.TokenIn {
		swapInfo.TokenInMint, _ = solana.PublicKeyFromBase58(mint)
		swapInfo.TokenInAmount = amount
		swapInfo.TokenInDecimals = intermediateInfo.Decimals[mint]
	}

	for mint, amount := range intermediateInfo.TokenOut {
		swapInfo.TokenOutMint, _ = solana.PublicKeyFromBase58(mint)
		swapInfo.TokenOutAmount = amount
		swapInfo.TokenOutDecimals = intermediateInfo.Decimals[mint]
	}

	// set signers if it contains DCA program
	var signer solana.PublicKey
	if p.containsDCAProgram() {
		signer = p.allAccountKeys[2]
	} else {
		signer = p.allAccountKeys[0]
	}
	swapInfo.Signers = append(swapInfo.Signers, signer)

	return swapInfo, nil
}
