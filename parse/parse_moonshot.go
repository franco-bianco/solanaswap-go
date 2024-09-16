package parse

import (
	"bytes"
	"fmt"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

type MoonshotTradeInstruction struct {
	Data *MoonshotTradeParams
}

type MoonshotTradeParams struct {
	TokenAmount      uint64
	CollateralAmount uint64
	FixedSide        uint8
	SlippageBps      uint64
}

type MoonshotTradeInstructionWithMint struct {
	Instruction MoonshotTradeInstruction
	Mint        solana.PublicKey
	TradeType   TradeType
}

type TradeType int

const (
	TradeTypeBuy TradeType = iota
	TradeTypeSell
)

var (
	MOONSHOT_BUY_INSTRUCTION  = ag_binary.TypeID([8]byte{102, 6, 61, 18, 1, 218, 235, 234})
	MOONSHOT_SELL_INSTRUCTION = ag_binary.TypeID([8]byte{51, 230, 133, 164, 1, 127, 131, 173})
)

// processMoonshotSwaps processes the moonshot swaps
func (p *Parser) processMoonshotSwaps() []SwapData {
	var swaps []SwapData

	for _, instruction := range p.txInfo.Message.Instructions {
		if p.isMoonshotTrade(instruction) {
			swapData, err := p.parseMoonshotTradeInstruction(instruction)
			if err != nil {
				continue
			}
			swaps = append(swaps, *swapData)
		}
	}

	return swaps
}

// isMoonshotTrade checks if the instruction is a moonshot trade
func (p *Parser) isMoonshotTrade(instruction solana.CompiledInstruction) bool {
	programID := p.txInfo.Message.AccountKeys[instruction.ProgramIDIndex]
	if !programID.Equals(MOONSHOT_PROGRAM_ID) {
		return false
	}

	if len(instruction.Data) != 33 || len(instruction.Accounts) != 11 {
		return false
	}

	return true
}

// parseMoonshotTradeInstruction parses the moonshot trade instruction
func (p *Parser) parseMoonshotTradeInstruction(instruction solana.CompiledInstruction) (*SwapData, error) {
	var (
		swapData        SwapData
		instructionData MoonshotTradeInstruction
	)

	decodedBytes, err := base58.Decode(instruction.Data.String())
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58 instruction data: %v", err)
	}

	discriminator := decodedBytes[:8]
	var tradeType TradeType

	switch {
	case bytes.Equal(discriminator, MOONSHOT_BUY_INSTRUCTION[:]):
		tradeType = TradeTypeBuy
		fmt.Printf("buy\n")
	case bytes.Equal(discriminator, MOONSHOT_SELL_INSTRUCTION[:]):
		tradeType = TradeTypeSell
		fmt.Printf("sell\n")
	default:
		return nil, fmt.Errorf("unknown moonshot trade instruction")
	}

	decoder := ag_binary.NewBorshDecoder(decodedBytes[8:])

	err = decoder.Decode(&instructionData)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling moonshot trade data: %s", err)
	}

	mint := p.txInfo.Message.AccountKeys[instruction.Accounts[6]]

	instructionWithMint := &MoonshotTradeInstructionWithMint{
		Instruction: instructionData,
		Mint:        mint,
		TradeType:   tradeType,
	}

	swapData = SwapData{
		Type: MOONSHOT,
		Data: instructionWithMint,
	}

	return &swapData, nil
}
