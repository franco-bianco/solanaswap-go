package solanaswapgo

import (
	"bytes"
	"fmt"
	"strconv"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

type MoonshotTradeInstructionWithMint struct {
	TokenAmount      uint64
	CollateralAmount uint64
	Mint             solana.PublicKey
	TradeType        TradeType
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

// processMoonshotSwaps processes all Moonshot swap instructions in the transaction
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

// isMoonshotTrade checks if the instruction is a Moonshot trade
func (p *Parser) isMoonshotTrade(instruction solana.CompiledInstruction) bool {
	return p.txInfo.Message.AccountKeys[instruction.ProgramIDIndex].Equals(MOONSHOT_PROGRAM_ID) && len(instruction.Data) == 33 && len(instruction.Accounts) == 11
}

// parseMoonshotTradeInstruction parses a Moonshot trade instruction
func (p *Parser) parseMoonshotTradeInstruction(instruction solana.CompiledInstruction) (*SwapData, error) {
	decodedBytes, err := base58.Decode(instruction.Data.String())
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58 instruction data: %v", err)
	}

	discriminator := decodedBytes[:8]
	var tradeType TradeType

	switch {
	case bytes.Equal(discriminator, MOONSHOT_BUY_INSTRUCTION[:]):
		tradeType = TradeTypeBuy
	case bytes.Equal(discriminator, MOONSHOT_SELL_INSTRUCTION[:]):
		tradeType = TradeTypeSell
	default:
		return nil, fmt.Errorf("unknown moonshot trade instruction")
	}

	moonshotTokenMint := p.txInfo.Message.AccountKeys[instruction.Accounts[6]]

	moonshotTokenBalanceChanges, err := p.getTokenBalanceChanges(moonshotTokenMint)
	if err != nil {
		return nil, fmt.Errorf("error getting moonshot token balance changes: %s", err)
	}

	nativeSolBalanceChanges, err := p.getTokenBalanceChanges(NATIVE_SOL_MINT_PROGRAM_ID)
	if err != nil {
		return nil, fmt.Errorf("error getting native sol balance changes: %s", err)
	}

	instructionWithMint := &MoonshotTradeInstructionWithMint{
		TokenAmount:      uint64(abs(moonshotTokenBalanceChanges)),
		CollateralAmount: uint64(abs(nativeSolBalanceChanges)),
		Mint:             moonshotTokenMint,
		TradeType:        tradeType,
	}

	return &SwapData{
		Type: MOONSHOT,
		Data: instructionWithMint,
	}, nil
}

// getTokenBalanceChanges calculates the balance change for a given token mint for the signer
func (p *Parser) getTokenBalanceChanges(mint solana.PublicKey) (int64, error) {
	if mint == NATIVE_SOL_MINT_PROGRAM_ID {
		// For native SOL, we'll use the first account (index 0) which is typically the fee payer/signer
		if len(p.txMeta.PostBalances) == 0 || len(p.txMeta.PreBalances) == 0 {
			return 0, fmt.Errorf("insufficient balance information for SOL")
		}
		change := int64(p.txMeta.PostBalances[0]) - int64(p.txMeta.PreBalances[0])
		return change, nil
	}

	// Get the signer's public key (assuming it's the first account in the transaction)
	signer := p.txInfo.Message.AccountKeys[0]

	var preAmount, postAmount int64
	var balanceFound bool

	for _, preBalance := range p.txMeta.PreTokenBalances {
		if preBalance.Mint.Equals(mint) && preBalance.Owner.Equals(signer) {
			preAmount, _ = strconv.ParseInt(preBalance.UiTokenAmount.Amount, 10, 64)
			balanceFound = true
			break
		}
	}

	for _, postBalance := range p.txMeta.PostTokenBalances {
		if postBalance.Mint.Equals(mint) && postBalance.Owner.Equals(signer) {
			postAmount, _ = strconv.ParseInt(postBalance.UiTokenAmount.Amount, 10, 64)
			balanceFound = true
			break
		}
	}

	if !balanceFound {
		return 0, fmt.Errorf("could not find balance for specified mint and signer")
	}

	change := postAmount - preAmount
	return change, nil
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
