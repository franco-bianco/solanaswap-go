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
	PROTOCOL_RAYDIUM           = "raydium"
	PROTOCOL_ORCA              = "orca"
	PROTOCOL_METEORA           = "meteora"
	PROTOCOL_PUMPFUN           = "pumpfun"
	PROTOCOL_RAYDIUM_LAUNCHPAD = "raydiumLaunchpad"
)

type TokenTransfer struct {
	mint     string
	amount   uint64
	decimals uint8
}

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
			progID.Equals(MAESTRO_PROGRAM_ID):
			if innerSwaps := p.processRouterSwaps(i); len(innerSwaps) > 0 {
				parsedSwaps = append(parsedSwaps, innerSwaps...)
			}
		case progID.Equals(OKX_DEX_ROUTER_PROGRAM_ID):
			skip = true
			parsedSwaps = append(parsedSwaps, p.processOKXSwaps(i)...)
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
			parsedSwaps = append(parsedSwaps, p.processRaydSwaps(i, RAYDIUM)...)
		case progID.Equals(ORCA_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processOrcaSwaps(i)...)
		case progID.Equals(METEORA_PROGRAM_ID) || progID.Equals(METEORA_POOLS_PROGRAM_ID) || progID.Equals(METEORA_DLMM_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processMeteoraSwaps(i)...)
		case progID.Equals(PUMPFUN_AMM_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processPumpfunAMMSwaps(i)...)
		case progID.Equals(PUMP_FUN_PROGRAM_ID) ||
			progID.Equals(solana.MustPublicKeyFromBase58("BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW")):
			parsedSwaps = append(parsedSwaps, p.processPumpfunSwaps(i)...)
		case progID.Equals(RAYDIUM_LAUNCHPAD_PROGRAM_ID):
			parsedSwaps = append(parsedSwaps, p.processRaydSwaps(i, RAYDIUM_LAUNCHPAD)...)
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
	if len(swapDatas) == 0 {
		return nil, fmt.Errorf("no swap data provided")
	}

	swapInfo := &SwapInfo{
		Signatures: p.txInfo.Signatures,
	}

	if p.containsDCAProgram() {
		swapInfo.Signers = []solana.PublicKey{p.allAccountKeys[2]}
	} else {
		swapInfo.Signers = []solana.PublicKey{p.allAccountKeys[0]}
	}

	jupiterSwaps := make([]SwapData, 0)
	pumpfunSwaps := make([]SwapData, 0)
	otherSwaps := make([]SwapData, 0)

	for _, swapData := range swapDatas {
		switch swapData.Type {
		case JUPITER:
			jupiterSwaps = append(jupiterSwaps, swapData)
		case PUMP_FUN:
			pumpfunSwaps = append(pumpfunSwaps, swapData)
		default:
			otherSwaps = append(otherSwaps, swapData)
		}
	}

	if len(jupiterSwaps) > 0 {
		jupiterInfo, err := parseJupiterEvents(jupiterSwaps)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Jupiter events: %w", err)
		}

		swapInfo.TokenInMint = jupiterInfo.TokenInMint
		swapInfo.TokenInAmount = jupiterInfo.TokenInAmount
		swapInfo.TokenInDecimals = jupiterInfo.TokenInDecimals
		swapInfo.TokenOutMint = jupiterInfo.TokenOutMint
		swapInfo.TokenOutAmount = jupiterInfo.TokenOutAmount
		swapInfo.TokenOutDecimals = jupiterInfo.TokenOutDecimals
		swapInfo.AMMs = jupiterInfo.AMMs

		return swapInfo, nil
	}

	if len(pumpfunSwaps) > 0 {
		// Check if it's a PumpfunTradeEvent
		if tradeEvent, ok := pumpfunSwaps[0].Data.(*PumpfunTradeEvent); ok {
			if tradeEvent.IsBuy {
				swapInfo.TokenInMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenInAmount = tradeEvent.SolAmount
				swapInfo.TokenInDecimals = 9
				swapInfo.TokenOutMint = tradeEvent.Mint
				swapInfo.TokenOutAmount = tradeEvent.TokenAmount
				swapInfo.TokenOutDecimals = p.splDecimalsMap[tradeEvent.Mint.String()]
			} else {
				swapInfo.TokenInMint = tradeEvent.Mint
				swapInfo.TokenInAmount = tradeEvent.TokenAmount
				swapInfo.TokenInDecimals = p.splDecimalsMap[tradeEvent.Mint.String()]
				swapInfo.TokenOutMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenOutAmount = tradeEvent.SolAmount
				swapInfo.TokenOutDecimals = 9
			}
			swapInfo.AMMs = append(swapInfo.AMMs, string(pumpfunSwaps[0].Type))
			swapInfo.Timestamp = time.Unix(int64(tradeEvent.Timestamp), 0)
			return swapInfo, nil
		} else {
			// Detailed logging for investigation
			p.Log.Infof("Processing PumpFun AMM swaps, count: %d", len(pumpfunSwaps))

			// Check if it's a buy transaction
			isBuy := p.isPumpFunAMMBuyTransaction(pumpfunSwaps)

			if isBuy {
				p.Log.Infof("Detected PumpFun BUY transaction")

				var tokenTransfer *TokenTransfer
				var totalSolAmount uint64

				for _, swap := range pumpfunSwaps {
					transfer := getTransferFromSwapData(swap)
					if transfer == nil {
						continue
					}

					if transfer.mint == NATIVE_SOL_MINT_PROGRAM_ID.String() {
						totalSolAmount += transfer.amount
						p.Log.Infof("Added SOL amount: %d, total now: %d",
							transfer.amount, totalSolAmount)
					} else {
						tokenTransfer = transfer
						p.Log.Infof("Found token transfer: %s, amount: %d",
							tokenTransfer.mint, tokenTransfer.amount)
					}
				}

				if tokenTransfer != nil {
					// It's a buy: SOL -> Token
					swapInfo.TokenInMint = NATIVE_SOL_MINT_PROGRAM_ID
					swapInfo.TokenInAmount = totalSolAmount
					swapInfo.TokenInDecimals = 9
					swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(tokenTransfer.mint)
					swapInfo.TokenOutAmount = tokenTransfer.amount
					swapInfo.TokenOutDecimals = tokenTransfer.decimals
					swapInfo.AMMs = append(swapInfo.AMMs, string(PUMP_FUN))
					swapInfo.Timestamp = time.Now()

					p.Log.Infof("Generated swap info for BUY transaction: %+v", swapInfo)
					return swapInfo, nil
				}
			} else {
				// It's a sell or not identifiable as a buy - process normally
				p.Log.Infof("Processing as a SELL or normal transaction")
				otherSwaps = append(otherSwaps, pumpfunSwaps...)
			}
		}
	}

	if len(otherSwaps) > 0 {
		// Track the chronological order of transfers
		var allTransfers []TokenTransfer

		for _, swapData := range otherSwaps {
			transfer := getTransferFromSwapData(swapData)
			if transfer != nil {
				allTransfers = append(allTransfers, *transfer)
			}
		}

		// If we have at least a starting and ending transfer
		if len(allTransfers) >= 2 {
			// Use first transfer as input and last as output for a multi-hop swap
			inputTransfer := allTransfers[0]
			outputTransfer := allTransfers[len(allTransfers)-1]

			// For multi-hop routes like RAY -> OXY -> WSOL, we want RAY as input and WSOL as output
			swapInfo.TokenInMint = solana.MustPublicKeyFromBase58(inputTransfer.mint)
			swapInfo.TokenInAmount = inputTransfer.amount
			swapInfo.TokenInDecimals = inputTransfer.decimals
			swapInfo.TokenOutMint = solana.MustPublicKeyFromBase58(outputTransfer.mint)
			swapInfo.TokenOutAmount = outputTransfer.amount
			swapInfo.TokenOutDecimals = outputTransfer.decimals

			// Collect unique AMMs used in this swap
			seenAMMs := make(map[string]bool)
			for _, swapData := range otherSwaps {
				if !seenAMMs[string(swapData.Type)] {
					swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
					seenAMMs[string(swapData.Type)] = true
				}
			}

			swapInfo.Timestamp = time.Now()
			return swapInfo, nil
		}
	}

	return nil, fmt.Errorf("no valid swaps found")
}

func getTransferFromSwapData(swapData SwapData) *TokenTransfer {
	switch data := swapData.Data.(type) {
	case *TransferData:
		return &TokenTransfer{
			mint:     data.Mint,
			amount:   data.Info.Amount,
			decimals: data.Decimals,
		}
	case *TransferCheck:
		amt, err := strconv.ParseUint(data.Info.TokenAmount.Amount, 10, 64)
		if err != nil {
			return nil
		}
		return &TokenTransfer{
			mint:     data.Info.Mint,
			amount:   amt,
			decimals: data.Info.TokenAmount.Decimals,
		}
	}
	return nil
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
			if raydSwaps := p.processRaydSwaps(instructionIndex, RAYDIUM); len(raydSwaps) > 0 {
				swaps = append(swaps, raydSwaps...)
			}

		case progID.Equals(ORCA_PROGRAM_ID) && !processedProtocols[PROTOCOL_ORCA]:
			processedProtocols[PROTOCOL_ORCA] = true
			if orcaSwaps := p.processOrcaSwaps(instructionIndex); len(orcaSwaps) > 0 {
				swaps = append(swaps, orcaSwaps...)
			}

		case (progID.Equals(METEORA_PROGRAM_ID) ||
			progID.Equals(METEORA_POOLS_PROGRAM_ID) ||
			progID.Equals(METEORA_DLMM_PROGRAM_ID)) && !processedProtocols[PROTOCOL_METEORA]:
			processedProtocols[PROTOCOL_METEORA] = true
			if meteoraSwaps := p.processMeteoraSwaps(instructionIndex); len(meteoraSwaps) > 0 {
				swaps = append(swaps, meteoraSwaps...)
			}

		case progID.Equals(PUMPFUN_AMM_PROGRAM_ID) && !processedProtocols[PROTOCOL_PUMPFUN]:
			processedProtocols[PROTOCOL_PUMPFUN] = true
			if pumpfunAMMSwaps := p.processPumpfunAMMSwaps(instructionIndex); len(pumpfunAMMSwaps) > 0 {
				swaps = append(swaps, pumpfunAMMSwaps...)
			}

		case (progID.Equals(PUMP_FUN_PROGRAM_ID) ||
			progID.Equals(solana.MustPublicKeyFromBase58("BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW"))) && !processedProtocols[PROTOCOL_PUMPFUN]:
			processedProtocols[PROTOCOL_PUMPFUN] = true
			if pumpfunSwaps := p.processPumpfunSwaps(instructionIndex); len(pumpfunSwaps) > 0 {
				swaps = append(swaps, pumpfunSwaps...)
			}

		case progID.Equals(RAYDIUM_LAUNCHPAD_PROGRAM_ID) && !processedProtocols[PROTOCOL_RAYDIUM_LAUNCHPAD]:
			processedProtocols[PROTOCOL_RAYDIUM_LAUNCHPAD] = true
			if raydLaunchpadSwaps := p.processRaydSwaps(instructionIndex, RAYDIUM_LAUNCHPAD); len(raydLaunchpadSwaps) > 0 {
				swaps = append(swaps, raydLaunchpadSwaps...)
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
