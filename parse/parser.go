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
		case progID.Equals(RAYDIUM_V4_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CPMM_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CONCENTRATED_LIQUIDITY_PROGRAM_ID) || // RaydConcentratedLiquiditySwapV2
			progID.Equals(solana.MustPublicKeyFromBase58("AP51WLiiqTdbZfgyRMs35PsZpdmLuPDdHYmrB23pEtMU")): // RaydConcentratedLiquiditySwap
			parsedSwaps = append(parsedSwaps, p.processRaydSwaps(i)...)
		case progID.Equals(ORCA_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processOrcaSwaps(i)...)
		case progID.Equals(METEORA_PROGRAM_ID) || progID.Equals(METEORA_POOLS_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processMeteoraSwaps(i)...)
		case progID.Equals(PUMP_FUN_PROGRAM_ID) ||
			progID.Equals(solana.MustPublicKeyFromBase58("BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW")): // PumpFun
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
			intermediateInfo, err := parseJupiterEvents(swapDatas)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Jupiter events: %w", err)
			}
			jupiterSwapInfo, err := convertToSwapInfo(intermediateInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to convert to swap info: %w", err)
			}
			jupiterSwapInfo.Signatures = swapInfo.Signatures
			jupiterSwapInfo.Signers = swapInfo.Signers
			return jupiterSwapInfo, nil
		case PUMP_FUN:
			if swapData.Data.(*PumpfunTradeEvent).IsBuy {
				swapInfo.TokenInMint = NATIVE_SOL_MINT_PROGRAM_ID // TokenIn info is always SOL for Pumpfun
				swapInfo.TokenInAmount = swapData.Data.(*PumpfunTradeEvent).SolAmount
				swapInfo.TokenInDecimals = 9
				swapInfo.TokenOutMint = swapData.Data.(*PumpfunTradeEvent).Mint
				swapInfo.TokenOutAmount = swapData.Data.(*PumpfunTradeEvent).TokenAmount
				swapInfo.TokenOutDecimals = p.splDecimalsMap[swapInfo.TokenOutMint.String()]
			} else {
				swapInfo.TokenInMint = swapData.Data.(*PumpfunTradeEvent).Mint
				swapInfo.TokenInAmount = swapData.Data.(*PumpfunTradeEvent).TokenAmount
				swapInfo.TokenInDecimals = p.splDecimalsMap[swapInfo.TokenInMint.String()]
				swapInfo.TokenOutMint = NATIVE_SOL_MINT_PROGRAM_ID // TokenOut info is always SOL for Pumpfun
				swapInfo.TokenOutAmount = swapData.Data.(*PumpfunTradeEvent).SolAmount
				swapInfo.TokenOutDecimals = 9
			}
			swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
			swapInfo.Timestamp = time.Unix(int64(swapData.Data.(*PumpfunTradeEvent).Timestamp), 0)
			return swapInfo, nil // Pumpfun only has one swap event
		case METEORA:
			switch swapData.Data.(type) {
			case *TransferCheck:
				swapData := swapData.Data.(*TransferCheck)
				if i == 0 {
					tokenInAmount, _ := strconv.ParseInt(swapData.Info.TokenAmount.Amount, 10, 64)
					swapInfo.TokenInMint = solana.MustPublicKeyFromBase58(swapData.Info.Mint)
					swapInfo.TokenInAmount = uint64(tokenInAmount)
					swapInfo.TokenInDecimals = swapData.Info.TokenAmount.Decimals
				} else {
					tokenOutAmount, _ := strconv.ParseFloat(swapData.Info.TokenAmount.Amount, 64)
					swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(swapData.Info.Mint)
					swapInfo.TokenOutAmount = uint64(tokenOutAmount)
					swapInfo.TokenOutDecimals = swapData.Info.TokenAmount.Decimals
				}
			case *TransferData: // Meteora Pools
				swapData := swapData.Data.(*TransferData)
				if i == 0 {
					swapInfo.TokenInMint = solana.MustPublicKeyFromBase58(swapData.Mint)
					swapInfo.TokenInAmount = swapData.Info.Amount
					swapInfo.TokenInDecimals = swapData.Decimals
				} else {
					if swapData.Info.Authority == swapInfo.Signers[0].String() && swapData.Mint == swapInfo.TokenInMint.String() {
						swapInfo.TokenInAmount += swapData.Info.Amount
					}
					swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(swapData.Mint)
					swapInfo.TokenOutAmount = swapData.Info.Amount
					swapInfo.TokenOutDecimals = swapData.Decimals
				}
			}
		case RAYDIUM, ORCA:
			switch swapData.Data.(type) {
			case *TransferData: // Raydium V4 and Orca
				swapData := swapData.Data.(*TransferData)
				if i == 0 {
					swapInfo.TokenInMint = solana.MustPublicKeyFromBase58(swapData.Mint)
					swapInfo.TokenInAmount = swapData.Info.Amount
					swapInfo.TokenInDecimals = swapData.Decimals
				} else {
					swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(swapData.Mint)
					swapInfo.TokenOutAmount = swapData.Info.Amount
					swapInfo.TokenOutDecimals = swapData.Decimals
				}
			case *TransferCheck: // Raydium CPMM
				swapData := swapData.Data.(*TransferCheck)
				if i == 0 {
					tokenInAmount, _ := strconv.ParseInt(swapData.Info.TokenAmount.Amount, 10, 64)
					swapInfo.TokenInMint = solana.MustPublicKeyFromBase58(swapData.Info.Mint)
					swapInfo.TokenInAmount = uint64(tokenInAmount)
					swapInfo.TokenInDecimals = swapData.Info.TokenAmount.Decimals
				} else {
					tokenOutAmount, _ := strconv.ParseFloat(swapData.Info.TokenAmount.Amount, 64)
					swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(swapData.Info.Mint)
					swapInfo.TokenOutAmount = uint64(tokenOutAmount)
					swapInfo.TokenOutDecimals = swapData.Info.TokenAmount.Decimals
				}
			}
		}
		swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
	}

	return swapInfo, nil
}
