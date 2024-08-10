package parse

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/sirupsen/logrus"
)

type Parser struct {
	tx              *rpc.GetTransactionResult
	txInfo          *solana.Transaction
	allAccountKeys  solana.PublicKeySlice
	splTokenInfoMap map[string]TokenInfo // map[authority]TokenInfo
	splDecimalsMap  map[string]uint8     // map[mint]decimals
	Log             *logrus.Logger
}

func NewTransactionParser(tx *rpc.GetTransactionResult) (*Parser, error) {

	txInfo, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	allAccountKeys := append(txInfo.Message.AccountKeys, tx.Meta.LoadedAddresses.Writable...)
	allAccountKeys = append(allAccountKeys, tx.Meta.LoadedAddresses.ReadOnly...)

	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
	})

	parser := &Parser{
		tx:             tx,
		txInfo:         txInfo,
		allAccountKeys: allAccountKeys,
		Log:            log,
	}

	if err := parser.extractSPLTokenInfo(); err != nil {
		return nil, fmt.Errorf("failed to extract SPL Token Addresses: %w", err)
	}

	if err := parser.extractSPLDecimals(); err != nil {
		return nil, fmt.Errorf("failed to extract SPL decimals: %w", err)
	}

	return parser, nil
}

type SwapData struct {
	Type SwapType
	Data interface{}
}

func (p *Parser) ParseTransaction() ([]SwapData, error) {
	var parsedSwaps []SwapData

	containsJupiterSwap := false
	for i, outerInstruction := range p.txInfo.Message.Instructions {
		progID := p.allAccountKeys[outerInstruction.ProgramIDIndex]
		if progID.Equals(JUPITER_PROGRAM_ID) {
			containsJupiterSwap = true
			parsedSwaps = append(parsedSwaps, p.processJupiterSwaps(i)...)
		}
	}
	if containsJupiterSwap {
		return parsedSwaps, nil // avoid processing further because Jupiter is an aggregator and other swaps are already included in the Jupiter swap
	}

	for i, outerInstruction := range p.txInfo.Message.Instructions {
		progID := p.allAccountKeys[outerInstruction.ProgramIDIndex]
		switch {
		case progID.Equals(RAYDIUM_V4_PROGRAM_ID) || progID.Equals(RAYDIUM_CPMM_PROGRAM_ID) || progID.Equals(RAYDIUM_CPMM_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processRaydSwaps(i)...)
		case progID.Equals(ORCA_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processOrcaSwaps(i)...)
		case progID.Equals(METEORA_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processMeteoraSwaps(i)...)
		case progID.Equals(PUMP_FUN_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processPumpfunSwaps(i)...)
		}
	}

	return parsedSwaps, nil
}

type SwapInfo struct {
	Signers    []solana.PublicKey
	Signatures []solana.Signature
	AMMs       []string
	Timestamp  time.Time

	TokenInMint     solana.PublicKey
	TokenInAmount   uint64
	TokenInDecimals uint8

	TokenOutMint     solana.PublicKey
	TokenOutAmount   uint64
	TokenOutDecimals uint8
}

func (p *Parser) ProcessSwapData(swapDatas []SwapData) (*SwapInfo, error) {

	txInfo, err := p.tx.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	swapInfo := &SwapInfo{
		Signers:    txInfo.Message.Signers(),
		Signatures: txInfo.Signatures,
		// TODO: add timestamp (get from block)
	}

	for i, swapData := range swapDatas {
		switch swapData.Type {
		case JUPITER:
			if i == 0 {
				swapInfo.TokenInMint = swapData.Data.(*JupiterSwapEventData).InputMint
				swapInfo.TokenInAmount = swapData.Data.(*JupiterSwapEventData).InputAmount
				swapInfo.TokenInDecimals = swapData.Data.(*JupiterSwapEventData).InputMintDecimals
				swapInfo.TokenOutMint = swapData.Data.(*JupiterSwapEventData).OutputMint
				swapInfo.TokenOutAmount = swapData.Data.(*JupiterSwapEventData).OutputAmount
				swapInfo.TokenOutDecimals = swapData.Data.(*JupiterSwapEventData).OutputMintDecimals
			} else {
				swapInfo.TokenOutMint = swapData.Data.(*JupiterSwapEventData).OutputMint
				swapInfo.TokenOutAmount = swapData.Data.(*JupiterSwapEventData).OutputAmount
				swapInfo.TokenOutDecimals = swapData.Data.(*JupiterSwapEventData).OutputMintDecimals
			}
		case PUMP_FUN:
			swapInfo.TokenInMint = NATIVE_SOL_MINT_PROGRAM_ID // TokenIn info is always SOL for Pumpfun
			swapInfo.TokenInAmount = swapData.Data.(*PumpfunTradeEvent).SolAmount / 1e9
			swapInfo.TokenInDecimals = 9
			swapInfo.TokenOutMint = swapData.Data.(*PumpfunTradeEvent).Mint
			swapInfo.TokenOutAmount = swapData.Data.(*PumpfunTradeEvent).TokenAmount
			swapInfo.TokenOutDecimals = p.splDecimalsMap[swapInfo.TokenOutMint.String()]
			swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
			swapInfo.Timestamp = time.Unix(int64(swapData.Data.(*PumpfunTradeEvent).Timestamp), 0)
			return swapInfo, nil // Pumpfun only has one swap event
		case METEORA:
			if i == 0 {
				tokenInAmount, _ := strconv.ParseInt(swapData.Data.(*TransferCheck).Info.TokenAmount.Amount, 10, 64)
				swapInfo.TokenInMint = solana.MustPublicKeyFromBase58(swapData.Data.(*TransferCheck).Info.Mint)
				swapInfo.TokenInAmount = uint64(tokenInAmount)
				swapInfo.TokenInDecimals = swapData.Data.(*TransferCheck).Info.TokenAmount.Decimals
			} else {
				tokenOutAmount, _ := strconv.ParseFloat(swapData.Data.(*TransferCheck).Info.TokenAmount.Amount, 64)
				swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(swapData.Data.(*TransferCheck).Info.Mint)
				swapInfo.TokenOutAmount = uint64(tokenOutAmount)
				swapInfo.TokenOutDecimals = swapData.Data.(*TransferCheck).Info.TokenAmount.Decimals
			}
		case RAYDIUM, ORCA:
			if i == 0 {
				swapInfo.TokenInMint = solana.MustPublicKeyFromBase58(swapData.Data.(*TransferData).Mint)
				swapInfo.TokenInAmount = swapData.Data.(*TransferData).Info.Amount
				swapInfo.TokenInDecimals = swapData.Data.(*TransferData).Decimals
			} else {
				swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(swapData.Data.(*TransferData).Mint)
				swapInfo.TokenOutAmount = swapData.Data.(*TransferData).Info.Amount
				swapInfo.TokenOutDecimals = swapData.Data.(*TransferData).Decimals
			}
		}
		swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
	}

	return swapInfo, nil
}
