package solanaswapgo

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/sirupsen/logrus"
)

const (
	PROTOCOL_RAYDIUM = "raydium"
	PROTOCOL_ORCA    = "orca"
	PROTOCOL_METEORA = "meteora"
	PROTOCOL_PUMPFUN = "pumpfun"
)

type Parser struct {
	txMeta          *rpc.TransactionMeta
	txInfo          *solana.Transaction
	allAccountKeys  solana.PublicKeySlice
	splTokenInfoMap map[string]TokenInfo
	splDecimalsMap  map[string]uint8
	Log             *logrus.Logger
}

func NewTransactionParser(tx *rpc.GetTransactionResult) (*Parser, error) {
	txInfo, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return NewTransactionParserFromTransaction(txInfo, tx.Meta)
}

func NewTransactionParserFromTransaction(tx *solana.Transaction, txMeta *rpc.TransactionMeta) (*Parser, error) {
	allAccountKeys := append(tx.Message.AccountKeys, txMeta.LoadedAddresses.Writable...)
	allAccountKeys = append(allAccountKeys, txMeta.LoadedAddresses.ReadOnly...)

	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
	})

	parser := &Parser{
		txMeta:         txMeta,
		txInfo:         tx,
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

	skip := false
	for i, outerInstruction := range p.txInfo.Message.Instructions {
		progID := p.allAccountKeys[outerInstruction.ProgramIDIndex]
		switch {
		case progID.Equals(JUPITER_PROGRAM_ID):
			skip = true
			parsedSwaps = append(parsedSwaps, p.processJupiterSwaps(i)...)
		case progID.Equals(MOONSHOT_PROGRAM_ID):
			skip = true
			parsedSwaps = append(parsedSwaps, p.processMoonshotSwaps()...)
		case progID.Equals(BANANA_GUN_PROGRAM_ID) ||
			progID.Equals(MINTECH_PROGRAM_ID) ||
			progID.Equals(BLOOM_PROGRAM_ID) ||
			progID.Equals(NOVA_PROGRAM_ID) ||
			progID.Equals(MAESTRO_PROGRAM_ID) ||
			progID.Equals(OKX_DEX_ROUTER_PROGRAM_ID):
			if innerSwaps := p.processRouterSwaps(i); len(innerSwaps) > 0 {
				parsedSwaps = append(parsedSwaps, innerSwaps...)
			}
		}
	}
	if skip {
		return parsedSwaps, nil
	}

	for i, outerInstruction := range p.txInfo.Message.Instructions {
		progID := p.allAccountKeys[outerInstruction.ProgramIDIndex]
		switch {
		case progID.Equals(RAYDIUM_V4_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CPMM_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_AMM_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CONCENTRATED_LIQUIDITY_PROGRAM_ID) ||
			progID.Equals(solana.MustPublicKeyFromBase58("AP51WLiiqTdbZfgyRMs35PsZpdmLuPDdHYmrB23pEtMU")):
			parsedSwaps = append(parsedSwaps, p.processRaydSwaps(i)...)
		case progID.Equals(ORCA_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processOrcaSwaps(i)...)
		case progID.Equals(METEORA_PROGRAM_ID) || progID.Equals(METEORA_POOLS_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processMeteoraSwaps(i)...)
		case progID.Equals(PUMP_FUN_PROGRAM_ID) ||
			progID.Equals(solana.MustPublicKeyFromBase58("BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW")):
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
	swapInfo := &SwapInfo{
		Signers:    p.txInfo.Message.Signers(),
		Signatures: p.txInfo.Signatures,
	}

	for i, swapData := range swapDatas {
		switch swapData.Type {
		case JUPITER:
			intermediateInfo, err := parseJupiterEvents(swapDatas)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Jupiter events: %w", err)
			}
			jupiterSwapInfo, err := p.convertToSwapInfo(intermediateInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to convert to swap info: %w", err)
			}
			jupiterSwapInfo.Signatures = swapInfo.Signatures
			return jupiterSwapInfo, nil
		case PUMP_FUN:
			if swapData.Data.(*PumpfunTradeEvent).IsBuy {
				swapInfo.TokenInMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenInAmount = swapData.Data.(*PumpfunTradeEvent).SolAmount
				swapInfo.TokenInDecimals = 9
				swapInfo.TokenOutMint = swapData.Data.(*PumpfunTradeEvent).Mint
				swapInfo.TokenOutAmount = swapData.Data.(*PumpfunTradeEvent).TokenAmount
				swapInfo.TokenOutDecimals = p.splDecimalsMap[swapInfo.TokenOutMint.String()]
			} else {
				swapInfo.TokenInMint = swapData.Data.(*PumpfunTradeEvent).Mint
				swapInfo.TokenInAmount = swapData.Data.(*PumpfunTradeEvent).TokenAmount
				swapInfo.TokenInDecimals = p.splDecimalsMap[swapInfo.TokenInMint.String()]
				swapInfo.TokenOutMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenOutAmount = swapData.Data.(*PumpfunTradeEvent).SolAmount
				swapInfo.TokenOutDecimals = 9
			}
			swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
			swapInfo.Timestamp = time.Unix(int64(swapData.Data.(*PumpfunTradeEvent).Timestamp), 0)
			return swapInfo, nil
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
			case *TransferData:
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
			case *TransferData:
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
			}
		case MOONSHOT:
			swapData := swapData.Data.(*MoonshotTradeInstructionWithMint)
			switch swapData.TradeType {
			case TradeTypeBuy:
				swapInfo.TokenInMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenInAmount = swapData.CollateralAmount
				swapInfo.TokenInDecimals = 9
				swapInfo.TokenOutMint = swapData.Mint
				swapInfo.TokenOutAmount = swapData.TokenAmount
				swapInfo.TokenOutDecimals = 9
			case TradeTypeSell:
				swapInfo.TokenInMint = swapData.Mint
				swapInfo.TokenInAmount = swapData.TokenAmount
				swapInfo.TokenInDecimals = 9
				swapInfo.TokenOutMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenOutAmount = swapData.CollateralAmount
				swapInfo.TokenOutDecimals = 9
			default:
				return nil, fmt.Errorf("invalid trade type: %d", swapData.TradeType)
			}
		}
		swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
	}

	return swapInfo, nil
}

func (p *Parser) processRouterSwaps(instructionIndex int) []SwapData {
	var swaps []SwapData

	innerInstructions := p.getInnerInstructions(instructionIndex)
	if len(innerInstructions) == 0 {
		return swaps
	}

	processedProtocols := make(map[string]bool)

	for _, inner := range innerInstructions {
		progID := p.allAccountKeys[inner.ProgramIDIndex]

		switch {
		case (progID.Equals(RAYDIUM_V4_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CPMM_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_AMM_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CONCENTRATED_LIQUIDITY_PROGRAM_ID)) && !processedProtocols[PROTOCOL_RAYDIUM]:
			processedProtocols[PROTOCOL_RAYDIUM] = true
			if raydSwaps := p.processRaydSwaps(instructionIndex); len(raydSwaps) > 0 {
				swaps = append(swaps, raydSwaps...)
			}

		case progID.Equals(ORCA_PROGRAM_ID) && !processedProtocols[PROTOCOL_ORCA]:
			processedProtocols[PROTOCOL_ORCA] = true
			if orcaSwaps := p.processOrcaSwaps(instructionIndex); len(orcaSwaps) > 0 {
				swaps = append(swaps, orcaSwaps...)
			}

		case (progID.Equals(METEORA_PROGRAM_ID) ||
			progID.Equals(METEORA_POOLS_PROGRAM_ID)) && !processedProtocols[PROTOCOL_METEORA]:
			processedProtocols[PROTOCOL_METEORA] = true
			if meteoraSwaps := p.processMeteoraSwaps(instructionIndex); len(meteoraSwaps) > 0 {
				swaps = append(swaps, meteoraSwaps...)
			}

		case (progID.Equals(PUMP_FUN_PROGRAM_ID) ||
			progID.Equals(solana.MustPublicKeyFromBase58("BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW"))) && !processedProtocols[PROTOCOL_PUMPFUN]:
			processedProtocols[PROTOCOL_PUMPFUN] = true
			if pumpfunSwaps := p.processPumpfunSwaps(instructionIndex); len(pumpfunSwaps) > 0 {
				swaps = append(swaps, pumpfunSwaps...)
			}
		}
	}

	return swaps
}

func (p *Parser) getInnerInstructions(index int) []solana.CompiledInstruction {
	if p.txMeta == nil || p.txMeta.InnerInstructions == nil {
		return nil
	}

	for _, inner := range p.txMeta.InnerInstructions {
		if inner.Index == uint16(index) {
			return inner.Instructions
		}
	}

	return nil
}
