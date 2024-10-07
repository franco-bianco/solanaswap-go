package parse

import (
	"bytes"
	"fmt"
	"strconv"

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

func (p *Parser) processMoonshotSwaps() []SwapData {
	var swaps []SwapData

	for _, instruction := range p.txInfo.Message.Instructions {
		if p.isMoonshotTrade(instruction) {
			swapData, err := p.parseMoonshotTradeInstruction(instruction)
			if err != nil {
				p.Log.Errorf("error parsing moonshot trade instruction: %s", err)
				continue
			}
			swaps = append(swaps, *swapData)
		}
	}

	return swaps
}

func (p *Parser) isMoonshotTrade(instruction solana.CompiledInstruction) bool {
	return p.txInfo.Message.AccountKeys[instruction.ProgramIDIndex].Equals(MOONSHOT_PROGRAM_ID) && len(instruction.Data) == 33 && len(instruction.Accounts) == 11
}

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

	var instructionData MoonshotTradeInstruction
	if err := ag_binary.NewBorshDecoder(decodedBytes[8:]).Decode(&instructionData); err != nil {
		return nil, fmt.Errorf("error unmarshaling moonshot trade data: %s", err)
	}

	mint := NATIVE_SOL_MINT_PROGRAM_ID
	if tradeType == TradeTypeBuy {
		mint = p.txInfo.Message.AccountKeys[instruction.Accounts[6]]
	}

	tokenBalanceChanges, err := p.getTokenBalanceChanges(mint)
	if err != nil {
		return nil, fmt.Errorf("error getting token balance changes: %s", err)
	}

	tokenBalanceChangesAbs := uint64(abs(tokenBalanceChanges))

	instructionWithMint := &MoonshotTradeInstructionWithMint{
		Instruction: instructionData,
		Mint:        mint,
		TradeType:   tradeType,
	}

	if tradeType == TradeTypeBuy {
		instructionWithMint.Instruction.Data.TokenAmount = tokenBalanceChangesAbs
	} else {
		instructionWithMint.Mint = p.txInfo.Message.AccountKeys[instruction.Accounts[6]]
		instructionWithMint.Instruction.Data.CollateralAmount = tokenBalanceChangesAbs
	}

	return &SwapData{
		Type: MOONSHOT,
		Data: instructionWithMint,
	}, nil
}

func (p *Parser) getTokenBalanceChanges(mint solana.PublicKey) (int64, error) {

	// handle native SOL balance change for sells (as SOL is the output). We are getting the balance change for the signer's SOL account (index 0)
	if mint == NATIVE_SOL_MINT_PROGRAM_ID {
		if len(p.tx.Meta.PostBalances) == 0 || len(p.tx.Meta.PreBalances) == 0 {
			return 0, fmt.Errorf("insufficient balance information for SOL")
		}
		change := int64(p.tx.Meta.PostBalances[0]) - int64(p.tx.Meta.PreBalances[0])
		return change, nil
	}

	// handle SPL token balance change for buys (as SPL token is the output). We are getting the balance change for the sender's token account (index 1)
	var preAmount, postAmount int64
	var postBalanceFound bool

	// find pre-balance for account 1
	for _, preBalance := range p.tx.Meta.PreTokenBalances {
		if preBalance.AccountIndex == 1 && preBalance.Mint.Equals(mint) {
			preAmountStr := preBalance.UiTokenAmount.Amount
			var err error
			preAmount, err = strconv.ParseInt(preAmountStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("error parsing pre balance amount: %s", err)
			}
			break
		}
	}

	// find post-balance for account 1
	for _, postBalance := range p.tx.Meta.PostTokenBalances {
		if postBalance.AccountIndex == 1 && postBalance.Mint.Equals(mint) {
			postAmountStr := postBalance.UiTokenAmount.Amount
			var err error
			postAmount, err = strconv.ParseInt(postAmountStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("error parsing post balance amount: %s", err)
			}
			postBalanceFound = true
			break
		}
	}

	if !postBalanceFound { // not necessary to check for preBalanceFound
		return 0, fmt.Errorf("could not find post balance for account 1")
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
