package solanaswapgo

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/sirupsen/logrus"
)

// TokenInfo holds information about a specific SPL token.
type TokenInfo struct {
	MintAddress    string
	Owner          string
	AmountUIAmount string
}

const (
	PROTOCOL_RAYDIUM = "raydium"
	PROTOCOL_ORCA    = "orca"
	PROTOCOL_METEORA = "meteora"
	PROTOCOL_PUMPFUN = "pumpfun"
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
	splTokenInfoMap map[string]TokenInfo // Map of mint address to TokenInfo
	splDecimalsMap  map[string]uint8     // Map of mint address to token decimals
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
		// Log the error but don't fail parser initialization
		// as token info might not always be critical.
		parser.Log.Warnf("failed to extract SPL Token Info: %v", err)
	}

	if err := parser.extractSPLDecimals(); err != nil {
		// Log the error but don't fail parser initialization,
		// decimals might be found through other means or not be essential for all parsing types.
		parser.Log.Warnf("failed to extract SPL decimals: %v", err)
	}

	return parser, nil
}

// extractSPLTokenInfo populates the splTokenInfoMap with data from pre and post token balances.
func (p *Parser) extractSPLTokenInfo() error {
	p.splTokenInfoMap = make(map[string]TokenInfo)

	if p.txMeta == nil {
		return fmt.Errorf("transaction metadata is nil")
	}

	// Process PreTokenBalances
	for _, balance := range p.txMeta.PreTokenBalances {
		mintAddress := balance.Mint.String()
		owner := balance.Owner.String()
		amountUIAmount := balance.UITokenAmount.Amount

		p.splTokenInfoMap[mintAddress] = TokenInfo{
			MintAddress:    mintAddress,
			Owner:          owner,
			AmountUIAmount: amountUIAmount,
		}
	}

	// Process PostTokenBalances, potentially overwriting pre-balances
	// This ensures that the latest state (post-transaction) is stored if a token appears in both.
	for _, balance := range p.txMeta.PostTokenBalances {
		mintAddress := balance.Mint.String()
		owner := balance.Owner.String()
		amountUIAmount := balance.UITokenAmount.Amount

		p.splTokenInfoMap[mintAddress] = TokenInfo{
			MintAddress:    mintAddress,
			Owner:          owner,
			AmountUIAmount: amountUIAmount,
		}
	}

	return nil
}

// extractSPLDecimals populates the splDecimalsMap with data from pre and post token balances.
func (p *Parser) extractSPLDecimals() error {
	p.splDecimalsMap = make(map[string]uint8)

	if p.txMeta == nil {
		return fmt.Errorf("transaction metadata is nil")
	}

	// Process PreTokenBalances
	for _, balance := range p.txMeta.PreTokenBalances {
		mintAddress := balance.Mint.String()
		decimals := balance.UITokenAmount.Decimals
		p.splDecimalsMap[mintAddress] = decimals
	}

	// Process PostTokenBalances, potentially overwriting pre-balances
	// This is generally fine as decimals for a mint should be constant.
	for _, balance := range p.txMeta.PostTokenBalances {
		mintAddress := balance.Mint.String()
		decimals := balance.UITokenAmount.Decimals
		p.splDecimalsMap[mintAddress] = decimals
	}

	return nil
}

type SwapData struct {
	Type SwapType
	Data interface{}
}

func (p *Parser) ParseTransaction() ([]SwapData, error) {
	var parsedSwaps []SwapData
	specificProtocolFound := false

	// First loop: Check for major aggregators, bots, or specific protocols that might handle routing internally.
	for i, outerInstruction := range p.txInfo.Message.Instructions {
		progID := p.allAccountKeys[outerInstruction.ProgramIDIndex]
		switch {
		case progID.Equals(JUPITER_PROGRAM_ID):
			p.Log.Debugf("Jupiter program found at instruction %d", i)
			swaps := p.processJupiterSwaps(i)
			if len(swaps) > 0 {
				parsedSwaps = append(parsedSwaps, swaps...)
				specificProtocolFound = true
			}
		case progID.Equals(MOONSHOT_PROGRAM_ID):
			p.Log.Debugf("Moonshot program found at instruction %d", i)
			swaps := p.processMoonshotSwaps() // Assuming this might not need index or handles it internally
			if len(swaps) > 0 {
				parsedSwaps = append(parsedSwaps, swaps...)
				specificProtocolFound = true
			}
		case progID.Equals(BANANA_GUN_PROGRAM_ID) ||
			progID.Equals(MINTECH_PROGRAM_ID) ||
			progID.Equals(BLOOM_PROGRAM_ID) ||
			progID.Equals(NOVA_PROGRAM_ID) ||
			progID.Equals(MAESTRO_PROGRAM_ID):
			p.Log.Debugf("Bot/Router program %s found at instruction %d", progID.String(), i)
			swaps := p.processRouterSwaps(i)
			if len(swaps) > 0 {
				parsedSwaps = append(parsedSwaps, swaps...)
				specificProtocolFound = true // Routers are specific enough
			}
		case progID.Equals(OKX_DEX_ROUTER_PROGRAM_ID):
			p.Log.Debugf("OKX Router program found at instruction %d", i)
			swaps := p.processOKXSwaps(i)
			if len(swaps) > 0 {
				parsedSwaps = append(parsedSwaps, swaps...)
				specificProtocolFound = true
			}
		}
		if specificProtocolFound && len(parsedSwaps) > 0 {
			// If a high-level protocol like Jupiter is found and yields swaps,
			// we might assume it handled the whole transaction's swap logic.
			// This can be refined based on Jupiter's behavior with multiple swaps.
			// For now, if Jupiter (or similar) provides swaps, we return them.
			// This helps avoid double processing or incorrect generic parsing.
			p.Log.Debugf("Specific protocol %s yielded swaps, returning early.", progID.String())
			return parsedSwaps, nil
		}
	}

	// Second loop: If no major aggregator/router handled it, check for direct interactions with common DEXs.
	if !specificProtocolFound {
		for i, outerInstruction := range p.txInfo.Message.Instructions {
			progID := p.allAccountKeys[outerInstruction.ProgramIDIndex]
			switch {
			case progID.Equals(RAYDIUM_V4_PROGRAM_ID) ||
				progID.Equals(RAYDIUM_CPMM_PROGRAM_ID) ||
				progID.Equals(RAYDIUM_AMM_PROGRAM_ID) ||
				progID.Equals(RAYDIUM_CONCENTRATED_LIQUIDITY_PROGRAM_ID) ||
				progID.Equals(solana.MustPublicKeyFromBase58("AP51WLiiqTdbZfgyRMs35PsZpdmLuPDdHYmrB23pEtMU")): // Another Raydium ID?
				p.Log.Debugf("Raydium program %s found at instruction %d", progID.String(), i)
				swaps := p.processRaydSwaps(i)
				if len(swaps) > 0 {
					parsedSwaps = append(parsedSwaps, swaps...)
					specificProtocolFound = true
				}
			case progID.Equals(ORCA_PROGRAM_ID):
				p.Log.Debugf("Orca program found at instruction %d", i)
				swaps := p.processOrcaSwaps(i)
				if len(swaps) > 0 {
					parsedSwaps = append(parsedSwaps, swaps...)
					specificProtocolFound = true
				}
			case progID.Equals(METEORA_PROGRAM_ID) || progID.Equals(METEORA_POOLS_PROGRAM_ID) || progID.Equals(METEORA_DLMM_PROGRAM_ID):
				p.Log.Debugf("Meteora program %s found at instruction %d", progID.String(), i)
				swaps := p.processMeteoraSwaps(i)
				if len(swaps) > 0 {
					parsedSwaps = append(parsedSwaps, swaps...)
					specificProtocolFound = true
				}
			case progID.Equals(PUMPFUN_AMM_PROGRAM_ID):
				p.Log.Debugf("Pumpfun AMM program found at instruction %d", i)
				swaps := p.processPumpfunAMMSwaps(i)
				if len(swaps) > 0 {
					parsedSwaps = append(parsedSwaps, swaps...)
					specificProtocolFound = true
				}
			case progID.Equals(PUMP_FUN_PROGRAM_ID) ||
				progID.Equals(solana.MustPublicKeyFromBase58("BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW")): // Pumpfun main or related
				p.Log.Debugf("Pumpfun program %s found at instruction %d", progID.String(), i)
				swaps := p.processPumpfunSwaps(i)
				if len(swaps) > 0 {
					parsedSwaps = append(parsedSwaps, swaps...)
					specificProtocolFound = true
				}
			}
			if specificProtocolFound && len(parsedSwaps) > 0 {
				// Similar to above, if a direct DEX interaction yields swaps,
				// we assume it's the primary action for this instruction block.
				// Consider if multiple direct DEX calls in one tx need combined parsing.
				p.Log.Debugf("Direct DEX interaction with %s yielded swaps.", progID.String())
				// Unlike Jupiter, we might not return immediately, to allow for multiple DEX interactions.
				// However, this could also lead to issues if not handled carefully by ProcessSwapData.
				// For now, let's keep it simple: if any specific protocol yields data, we rely on that.
				// The generic parser is a last resort.
			}
		}
	}

	// Third stage: If no specific protocol parsers (Jupiter, Raydium, Bots, etc.) found any swaps,
	// try to parse generic transfers from inner instructions for each outer instruction.
	if len(parsedSwaps) == 0 {
		p.Log.Debugf("No specific protocol swaps found, attempting generic inner swap parsing.")
		for i := range p.txInfo.Message.Instructions {
			// Outer instruction itself might not be SPL token, but its inner instructions could be.
			// progID := p.allAccountKeys[outerInstruction.ProgramIDIndex]
			// p.Log.Debugf("Attempting generic inner swap parsing for outer instruction %d, program %s", i, progID.String())
			genericSwaps := p.parseGenericInnerSwaps(i)
			if len(genericSwaps) > 0 {
				parsedSwaps = append(parsedSwaps, genericSwaps...)
				// Unlike specific protocols, we don't set specificProtocolFound = true here
				// because these are generic, and we might find more across other outer instructions.
			}
		}
	}

	if len(parsedSwaps) > 0 {
		p.Log.Debugf("Found %d potential swaps in total.", len(parsedSwaps))
	} else {
		p.Log.Debugf("No swaps found in transaction.")
	}
	return parsedSwaps, nil
}

// GenericSwapData holds information about a swap identified from generic token transfers.
type GenericSwapData struct {
	SourceMint     string
	SourceAmount   uint64
	SourceDecimals uint8
	Owner          string // The account that owned the source tokens (likely the signer)

	DestinationMint     string
	DestinationAmount   uint64
	DestinationDecimals uint8
}

// parseGenericInnerSwaps attempts to identify swaps from SPL token transfers in inner instructions.
func (p *Parser) parseGenericInnerSwaps(instructionIndex int) []SwapData {
	var swaps []SwapData
	innerInstructions := p.getInnerInstructions(instructionIndex)
	if len(innerInstructions) == 0 {
		return swaps
	}

	var transfers []TransferData // Using existing TransferData for simplicity
	signer := p.txInfo.Message.AccountKeys[0].String() // Assume first signer is the primary actor

	for _, inner := range innerInstructions {
		progID := p.allAccountKeys[inner.ProgramIDIndex]
		if !progID.Equals(solana.TokenProgramID) {
			continue
		}

		data := inner.Data
		if len(data) == 0 {
			continue
		}

		instructionType := data[0]
		// Instruction type 3: Transfer, 12: TransferChecked
		if instructionType == 3 || instructionType == 12 {
			// Basic SPL Token Transfer instruction structure:
			// Accounts: Source (from), Destination (to), Authority
			// Data: InstructionType (1 byte), Amount (8 bytes for Transfer, or Amount + Decimals for TransferChecked)
			if len(inner.Accounts) < 3 { // Source, Destination, Authority
				p.Log.Warnf("Generic transfer parsing: Not enough accounts for SPL transfer in inner instruction: %d", len(inner.Accounts))
				continue
			}

			sourceAccount := p.allAccountKeys[inner.Accounts[0]].String()
			destAccount := p.allAccountKeys[inner.Accounts[1]].String()
			// authorityAccount := p.allAccountKeys[inner.Accounts[2]].String() // Not always the tx signer

			// Try to get mint and amount from Pre/Post Token Balances by looking at account changes
			// This is more reliable than trying to parse instruction data directly without fullborsh
			var amount uint64
			var mint string
			var decimals uint8
			var ownerOfSource string

			// Find source account in PreTokenBalances to get mint and pre-amount
			// Find source account in PostTokenBalances to get post-amount
			// Amount transferred = pre-amount - post-amount
			// This logic is complex because Pre/PostTokenBalances are at tx level, not instruction level.
			// A simpler approach for now: rely on PreTokenBalances for initial state,
			// and assume the amount in the instruction is the amount transferred.
			// This requires parsing the instruction data for amount.

			// For Transfer (type 3): Amount is bytes 1-9 (uint64)
			// For TransferChecked (type 12): Amount is bytes 1-9 (uint64), Decimals is byte 9
			if instructionType == 3 && len(data) >= 9 {
				amount = solana.Encoding.Binary.GetUint64(data[1:9], solana.Encoding.LittleEndian)
			} else if instructionType == 12 && len(data) >= 10 {
				amount = solana.Encoding.Binary.GetUint64(data[1:9], solana.Encoding.LittleEndian)
				decimals = data[9]
			} else {
				p.Log.Warnf("Generic transfer parsing: Data length insufficient for SPL transfer type %d: %d", instructionType, len(data))
				continue
			}

			// Attempt to find the mint for the source account.
			// We need to associate token accounts with their mints.
			// PreTokenBalances is one way.
			foundMint := false
			for _, ptb := range p.txMeta.PreTokenBalances {
				if ptb.Owner.String() == signer && ptb.AccountIndex == inner.Accounts[0] { // inner.Accounts[0] is source
					mint = ptb.Mint.String()
					if decimals == 0 && instructionType == 3 { // If Transfer (not TransferChecked), use known decimals
						decimals = ptb.UITokenAmount.Decimals
					}
					ownerOfSource = ptb.Owner.String()
					foundMint = true
					break
				}
				// Also check PostTokenBalances in case it's a newly created account receiving tokens
				// (though for a source, PreTokenBalances is more relevant)
			}
			if !foundMint {
				// Fallback: check PostTokenBalances if mint not found in Pre (e.g. token created and transferred in same tx)
				// This is less likely for source_account of a transfer, but good to have
				for _, ptb := range p.txMeta.PostTokenBalances {
					if ptb.Owner.String() == signer && ptb.AccountIndex == inner.Accounts[0] {
						mint = ptb.Mint.String()
						if decimals == 0 && instructionType == 3 {
							decimals = ptb.UITokenAmount.Decimals
						}
						ownerOfSource = ptb.Owner.String()
						foundMint = true
						break
					}
				}
			}

			if !foundMint {
				// If mint still not found, we might try to get it from splTokenInfoMap if populated for the account
				// This is getting complicated. For now, if mint isn't easily found via balances, skip.
				// A robust solution would involve mapping all accounts in AccountKeys to their mints if they are token accounts.
				p.Log.Debugf("Generic transfer parsing: Mint not found for source account %s (index %d)", sourceAccount, inner.Accounts[0])
				continue
			}

			if amount == 0 { // Skip zero amount transfers
				continue
			}

			transfers = append(transfers, TransferData{
				ProgramID: progID.String(), // SPL Token Program
				Info: TransferInfo{
					Source:      sourceAccount,
					Destination: destAccount,
					Amount:      amount,
					// Mint is crucial, needs to be figured out
				},
				Mint:     mint,
				Decimals: decimals,
				Owner:    ownerOfSource, // Owner of the source token account
			})
		}
	}

	if len(transfers) < 2 {
		return swaps // Need at least two transfers for a potential swap
	}

	// Heuristic: Find a transfer from signer and a transfer to signer with different mints
	var outTransfer, inTransfer *TransferData
	for i, t1 := range transfers {
		// Check if source is owned by signer (or is the signer if SOL transfer - though this is SPL focused)
		// For SPL, check if the *token account owner* is the signer.
		// The current `Owner` field in TransferData is set to `ownerOfSource`.
		if t1.Owner == signer { // Potential outgoing transfer
			for j, t2 := range transfers {
				if i == j {
					continue
				}
				// Check if destination account is owned by signer
				// This requires knowing the owner of the destination SPL token account.
				// This info is in Pre/PostTokenBalances.
				destOwner := ""
				for _, ptb := range p.txMeta.PreTokenBalances {
					if ptb.AccountIndex == p.txInfo.Message.AccountKeys.Index(solana.MustPublicKeyFromBase58(t2.Info.Destination)) {
						destOwner = ptb.Owner.String()
						break
					}
				}
				if destOwner == "" {
					for _, ptb := range p.txMeta.PostTokenBalances {
						if ptb.AccountIndex == p.txInfo.Message.AccountKeys.Index(solana.MustPublicKeyFromBase58(t2.Info.Destination)) {
							destOwner = ptb.Owner.String()
							break
						}
					}
				}


				if destOwner == signer && t1.Mint != t2.Mint { // Potential incoming transfer
					outTransfer = &transfers[i]
					inTransfer = &transfers[j]
					break
				}
			}
			if outTransfer != nil && inTransfer != nil {
				break
			}
		}
	}

	if outTransfer != nil && inTransfer != nil {
		p.Log.Debugf("Generic swap identified: %s -> %s", outTransfer.Mint, inTransfer.Mint)
		swaps = append(swaps, SwapData{
			Type: GENERIC_SWAP,
			Data: GenericSwapData{
				SourceMint:        outTransfer.Mint,
				SourceAmount:      outTransfer.Info.Amount,
				SourceDecimals:    p.splDecimalsMap[outTransfer.Mint], // Use pre-populated decimals
				Owner:             outTransfer.Owner,                  // Should be signer
				DestinationMint:   inTransfer.Mint,
				DestinationAmount: inTransfer.Info.Amount,
				DestinationDecimals: p.splDecimalsMap[inTransfer.Mint], // Use pre-populated decimals
			},
		})
	}

	return swaps
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
	genericSwaps := make([]*GenericSwapData, 0)
	otherSwapsData := make([]SwapData, 0) // Renamed to avoid confusion with the loop variable

	for _, swapData := range swapDatas {
		switch swapData.Type {
		case JUPITER:
			jupiterSwaps = append(jupiterSwaps, swapData)
		case PUMP_FUN:
			pumpfunSwaps = append(pumpfunSwaps, swapData)
		case GENERIC_SWAP:
			if data, ok := swapData.Data.(GenericSwapData); ok { // Ensure correct type, not pointer
				genericSwaps = append(genericSwaps, &data)
			} else if dataPtr, ok := swapData.Data.(*GenericSwapData); ok { // Also handle pointer type just in case
				genericSwaps = append(genericSwaps, dataPtr)
			} else {
				p.Log.Warnf("Failed to cast GENERIC_SWAP data: %+v", swapData.Data)
			}
		default:
			otherSwapsData = append(otherSwapsData, swapData)
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
		switch data := pumpfunSwaps[0].Data.(type) {
		case *PumpfunTradeEvent:
			if data.IsBuy {
				swapInfo.TokenInMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenInAmount = data.SolAmount
				swapInfo.TokenInDecimals = 9
				swapInfo.TokenOutMint = data.Mint
				swapInfo.TokenOutAmount = data.TokenAmount
				if dec, ok := p.splDecimalsMap[data.Mint.String()]; ok {
					swapInfo.TokenOutDecimals = dec
				} else {
					p.Log.Warnf("PumpFun: Decimals not found for mint %s. Defaulting to 0.", data.Mint.String())
					swapInfo.TokenOutDecimals = 0 // Or handle error appropriately
				}
			} else {
				swapInfo.TokenInMint = data.Mint
				swapInfo.TokenInAmount = data.TokenAmount
				if dec, ok := p.splDecimalsMap[data.Mint.String()]; ok {
					swapInfo.TokenInDecimals = dec
				} else {
					p.Log.Warnf("PumpFun: Decimals not found for mint %s. Defaulting to 0.", data.Mint.String())
					swapInfo.TokenInDecimals = 0 // Or handle error appropriately
				}
				swapInfo.TokenOutMint = NATIVE_SOL_MINT_PROGRAM_ID
				swapInfo.TokenOutAmount = data.SolAmount
				swapInfo.TokenOutDecimals = 9
			}
			swapInfo.AMMs = append(swapInfo.AMMs, string(pumpfunSwaps[0].Type))
			swapInfo.Timestamp = time.Unix(int64(data.Timestamp), 0)
			return swapInfo, nil
		default:
			// If pumpfunSwaps[0].Data is not *PumpfunTradeEvent, add to otherSwapsData for generic processing
			p.Log.Warnf("PumpFun: Unexpected data type %T in pumpfunSwaps, attempting generic processing.", pumpfunSwaps[0].Data)
			otherSwapsData = append(otherSwapsData, pumpfunSwaps...)
		}
	}

	if len(genericSwaps) > 0 {
		// Assuming the first generic swap is representative if multiple are found.
		// parseGenericInnerSwaps currently aims to produce one GenericSwapData per outer instruction if a swap is found.
		gsData := genericSwaps[0]
		p.Log.Debugf("Processing GENERIC_SWAP: %s -> %s", gsData.SourceMint, gsData.DestinationMint)

		var err error
		swapInfo.TokenInMint, err = solana.PublicKeyFromBase58(gsData.SourceMint)
		if err != nil {
			return nil, fmt.Errorf("invalid source mint address for generic swap %s: %w", gsData.SourceMint, err)
		}
		swapInfo.TokenInAmount = gsData.SourceAmount
		swapInfo.TokenInDecimals = gsData.SourceDecimals // Already determined in parseGenericInnerSwaps

		swapInfo.TokenOutMint, err = solana.PublicKeyFromBase58(gsData.DestinationMint)
		if err != nil {
			return nil, fmt.Errorf("invalid destination mint address for generic swap %s: %w", gsData.DestinationMint, err)
		}
		swapInfo.TokenOutAmount = gsData.DestinationAmount
		swapInfo.TokenOutDecimals = gsData.DestinationDecimals // Already determined in parseGenericInnerSwaps

		swapInfo.AMMs = []string{string(GENERIC_SWAP)}
		swapInfo.Timestamp = time.Now() // Generic swaps don't have an easily extractable timestamp from instruction
		return swapInfo, nil
	}

	// Process otherSwapsData only if no specific protocol swaps were processed
	if len(otherSwapsData) > 0 {
		p.Log.Debugf("Processing %d other swaps.", len(otherSwapsData))
		var uniqueTokens []TokenTransfer
		seenTokens := make(map[string]bool)

		for _, swapData := range otherSwapsData { // Iterate over otherSwapsData
			transfer := getTransferFromSwapData(swapData)
			if transfer != nil && !seenTokens[transfer.mint] {
				uniqueTokens = append(uniqueTokens, *transfer)
				seenTokens[transfer.mint] = true
			}
		}

		if len(uniqueTokens) >= 2 {
			// The existing logic for 'otherSwaps' seems to pick the first and last unique tokens.
			// This might be okay for simple A->B swaps but could be inaccurate for more complex routing.
			inputTransfer := uniqueTokens[0]
			outputTransfer := uniqueTokens[len(uniqueTokens)-1]

			seenInputs := make(map[string]bool)
			seenOutputs := make(map[string]bool)
			var totalInputAmount uint64 = 0
			var totalOutputAmount uint64 = 0

			for _, swapData := range otherSwapsData { // Iterate over otherSwapsData
				transfer := getTransferFromSwapData(swapData)
				if transfer == nil {
					continue
				}

				amountStr := fmt.Sprintf("%d-%s", transfer.amount, transfer.mint)
				if transfer.mint == inputTransfer.mint && !seenInputs[amountStr] {
					totalInputAmount += transfer.amount
					seenInputs[amountStr] = true
				}
				if transfer.mint == outputTransfer.mint && !seenOutputs[amountStr] {
					totalOutputAmount += transfer.amount
					seenOutputs[amountStr] = true
				}
			}

			var errIn, errOut error
			swapInfo.TokenInMint, errIn = solana.PublicKeyFromBase58(inputTransfer.mint)
			if errIn != nil {
				return nil, fmt.Errorf("invalid input mint address for other swap %s: %w", inputTransfer.mint, errIn)
			}
			swapInfo.TokenInAmount = totalInputAmount
			swapInfo.TokenInDecimals = inputTransfer.decimals

			swapInfo.TokenOutMint, errOut = solana.PublicKeyFromBase58(outputTransfer.mint)
			if errOut != nil {
				return nil, fmt.Errorf("invalid output mint address for other swap %s: %w", outputTransfer.mint, errOut)
			}
			swapInfo.TokenOutAmount = totalOutputAmount
			swapInfo.TokenOutDecimals = outputTransfer.decimals

			seenAMMs := make(map[string]bool)
			for _, swapData := range otherSwapsData { // Iterate over otherSwapsData
				if !seenAMMs[string(swapData.Type)] {
					swapInfo.AMMs = append(swapInfo.AMMs, string(swapData.Type))
					seenAMMs[string(swapData.Type)] = true
				}
			}

			swapInfo.Timestamp = time.Now() // Fallback timestamp
			return swapInfo, nil
		}
	}

	return nil, fmt.Errorf("no valid swaps found or processed")
}

func getTransferFromSwapData(swapData SwapData) *TokenTransfer {
	// It's important that GENERIC_SWAP data is not processed by this function,
	// as GenericSwapData is not a TransferData or TransferCheck.
	// The new logic in ProcessSwapData should prevent GENERIC_SWAP from reaching here.
	switch data := swapData.Data.(type) {
	case *TransferData:
		// Ensure Mint is a string, if it's a PublicKey, convert it.
		// Assuming data.Mint is already string based on TokenTransfer struct.
		return &TokenTransfer{
			mint:     data.Mint, // Should be string
			amount:   data.Info.Amount,
			decimals: data.Decimals,
		}
	case *TransferCheck:
		amt, err := strconv.ParseUint(data.Info.TokenAmount.Amount, 10, 64)
		if err != nil {
			// p.Log.Warnf("Failed to parse amount in TransferCheck: %v", err) // Consider logging if p is accessible
			return nil
		}
		return &TokenTransfer{
			mint:     data.Info.Mint, // Should be string
			amount:   amt,
			decimals: data.Info.TokenAmount.Decimals,
		}
	// Add other cases if new data types can be passed here.
	// default:
	// p.Log.Warnf("getTransferFromSwapData received unhandled type: %T", swapData.Data) // Consider logging
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
			if raydSwaps := p.processRaydSwaps(instructionIndex); len(raydSwaps) > 0 {
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
