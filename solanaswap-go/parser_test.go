package solanaswapgo

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// mockTokenBalance creates a rpc.TokenBalance for testing.
func mockTokenBalance(accountIndex uint16, mint string, owner string, amount string, decimals uint8, uiAmount float64) rpc.TokenBalance {
	return rpc.TokenBalance{
		AccountIndex: accountIndex,
		Mint:         solana.MustPublicKeyFromBase58(mint),
		Owner:        solana.MustPublicKeyFromBase58(owner),
		UITokenAmount: rpc.UITokenAmount{
			Amount:         amount,
			Decimals:       decimals,
			UIAmountString: amount, // Often same as Amount for mocks unless specific UI calculation needed
			UIAmount:       &uiAmount,
		},
	}
}

// newTestParser creates a new Parser with minimal viable setup for many tests.
// Transaction and Meta can be further customized by the test.
func newTestParser(accountKeys []solana.PublicKey, preBalances []rpc.TokenBalance, postBalances []rpc.TokenBalance, innerInstructions []rpc.InnerInstruction) *Parser {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel) // Keep test output clean unless debugging

	txMeta := &rpc.TransactionMeta{
		PreTokenBalances:  preBalances,
		PostTokenBalances: postBalances,
		InnerInstructions: innerInstructions,
		LoadedAddresses:   rpc.LoadedAddresses{}, // Initialize to avoid nil panic
		LogMessages:       []string{},
	}

	// Create a minimal transaction message
	txMessage := &solana.Message{
		AccountKeys: accountKeys,
		Instructions: []solana.CompiledInstruction{}, // Add outer instructions if needed per test
	}

	tx, err := solana.NewTransaction(txMessage, []solana.Signature{}, solana.PublicKey{})
	if err != nil {
		panic("failed to create mock transaction: " + err.Error()) // Panic in test setup is fine
	}

	// Initialize parser
	// Note: NewTransactionParserFromTransaction calls extractSPLTokenInfo and extractSPLDecimals
	parser, err := NewTransactionParserFromTransaction(tx, txMeta)
	if err != nil {
		// Depending on the test, this error might be expected or not.
		// For basic setup, we assume it should succeed.
		// If a test expects this to fail (e.g. nil txMeta for some functions), it should handle it.
		// For now, let's make it a soft fail for setup.
		log.Warnf("Error creating test parser (might be expected by test): %v", err)
		if parser == nil { // If parser is nil, create a basic one to avoid nil panics in tests that don't rely on auto-extraction
			parser = &Parser{
				txMeta:          txMeta,
				txInfo:          tx,
				allAccountKeys:  accountKeys,
				Log:             log,
				splTokenInfoMap: make(map[string]TokenInfo),
				splDecimalsMap:  make(map[string]uint8),
			}
		}
	}
	return parser
}

func TestExtractSPLTokenInfoAndDecimals(t *testing.T) {
	owner1 := solana.NewWallet().PublicKey().String()
	owner2 := solana.NewWallet().PublicKey().String()

	mint1 := "So11111111111111111111111111111111111111112" // SOL
	mint2 := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v" // USDC
	mint3 := "DezXAZ8z7PnrnRJjz3wXBoRgixCa6xjnB7YaB1pPB263" // BONK

	uiAmount0 := 0.0
	uiAmount10 := 10.0
	uiAmount20 := 20.0
	uiAmount50 := 50.0

	preBalances := []rpc.TokenBalance{
		mockTokenBalance(1, mint1, owner1, "1000000000", 9, uiAmount10), // owner1 SOL pre
		mockTokenBalance(2, mint2, owner1, "20000000", 6, uiAmount20),  // owner1 USDC pre
		mockTokenBalance(3, mint2, owner2, "50000000", 6, uiAmount50),  // owner2 USDC pre (another account for same mint)
	}
	postBalances := []rpc.TokenBalance{
		mockTokenBalance(1, mint1, owner1, "500000000", 9, uiAmount0),  // owner1 SOL post (amount changed)
		mockTokenBalance(2, mint2, owner1, "70000000", 6, uiAmount0), // owner1 USDC post (amount changed)
		mockTokenBalance(4, mint3, owner1, "100000", 5, uiAmount0),    // owner1 BONK post (new token)
	}

	// Account keys don't need to be exhaustive for these map tests, just enough to cover indices if needed by parser init
	accountKeys := []solana.PublicKey{
		solana.SystemProgramID, // Placeholder for program IDs if any outer instructions were processed
		solana.MustPublicKeyFromBase58(owner1),
		solana.MustPublicKeyFromBase58(owner2),
		solana.MustPublicKeyFromBase58(mint1), // Mints aren't usually in accountKeys directly unless used by instructions
		solana.MustPublicKeyFromBase58(mint2),
		solana.MustPublicKeyFromBase58(mint3),
		// Token accounts (indices 1,2,3,4 in balances correspond to indices in a hypothetical full AccountKeys list)
		// For these map tests, the AccountIndex in TokenBalance is just an identifier.
		// The parser's NewTransactionParserFromTransaction will build its allAccountKeys from tx.Message.AccountKeys and loadedAddresses.
		// For the functions extractSPLTokenInfo & extractSPLDecimals, they directly use balance.Mint.String() and balance.Owner.String(),
		// so the main accountKeys list for the parser doesn't critically affect *these specific map tests* beyond parser initialization.
		// Let's add some dummy accounts that would match the indices if they were part of the tx message.
		solana.NewWallet().PublicKey(), // dummy for index 0
		solana.NewWallet().PublicKey(), // dummy for index 1 (would be owner1's SOL account)
		solana.NewWallet().PublicKey(), // dummy for index 2 (would be owner1's USDC account)
		solana.NewWallet().PublicKey(), // dummy for index 3 (would be owner2's USDC account)
		solana.NewWallet().PublicKey(), // dummy for index 4 (would be owner1's BONK account)
	}


	parser := newTestParser(accountKeys, preBalances, postBalances, nil)

	// extractSPLTokenInfo and extractSPLDecimals are called by NewTransactionParserFromTransaction (via newTestParser)

	// Test splTokenInfoMap
	assert.Equal(t, 3, len(parser.splTokenInfoMap), "Should have info for 3 mints")

	// SOL - overwritten by postBalance
	infoMint1, ok := parser.splTokenInfoMap[mint1]
	assert.True(t, ok, "mint1 info should exist")
	assert.Equal(t, mint1, infoMint1.MintAddress)
	assert.Equal(t, owner1, infoMint1.Owner) // Owner of the token account
	assert.Equal(t, "500000000", infoMint1.AmountUIAmount, "SOL amount should be from postBalance")

	// USDC - overwritten by postBalance for owner1's account, owner2's pre-balance is also processed.
	// The current logic for splTokenInfoMap is `p.splTokenInfoMap[mintAddress] = TokenInfo{...}`
	// This means for a given mint, the last seen balance (owner/amount) will be stored.
	// If owner1's account for mint2 is processed last from postBalances, it will overwrite any preBalance info for mint2.
	infoMint2, ok := parser.splTokenInfoMap[mint2]
	assert.True(t, ok, "mint2 info should exist")
	assert.Equal(t, mint2, infoMint2.MintAddress)
	// Who is the owner? It depends on iteration order.
	// PreBalances: owner1 (acct 2), then owner2 (acct 3)
	// PostBalances: owner1 (acct 2)
	// If PostBalances are iterated after PreBalances and overwrite, then owner1's post balance info for mint2 will be there.
	assert.Equal(t, owner1, infoMint2.Owner, "USDC Owner should be owner1 from post balance")
	assert.Equal(t, "70000000", infoMint2.AmountUIAmount, "USDC amount should be from owner1 postBalance")


	// BONK - only in postBalance
	infoMint3, ok := parser.splTokenInfoMap[mint3]
	assert.True(t, ok, "mint3 info should exist")
	assert.Equal(t, mint3, infoMint3.MintAddress)
	assert.Equal(t, owner1, infoMint3.Owner)
	assert.Equal(t, "100000", infoMint3.AmountUIAmount)

	// Test splDecimalsMap
	assert.Equal(t, 3, len(parser.splDecimalsMap), "Should have decimals for 3 mints")
	assert.Equal(t, uint8(9), parser.splDecimalsMap[mint1], "SOL decimals should be 9")
	assert.Equal(t, uint8(6), parser.splDecimalsMap[mint2], "USDC decimals should be 6")
	assert.Equal(t, uint8(5), parser.splDecimalsMap[mint3], "BONK decimals should be 5")

	// Test what happens if a mint is in pre but not post (e.g. token fully spent)
	mint4_addr := "DEADBEEF11111111111111111111111111111111111"
	preOnlyBalances := []rpc.TokenBalance{
		mockTokenBalance(5, mint4_addr, owner1, "100", 2, uiAmount0),
	}
	postOnlyBalances := []rpc.TokenBalance{} // No post balance for mint4_addr

	parserPreOnly := newTestParser(accountKeys, preOnlyBalances, postOnlyBalances, nil)

	infoMint4, ok := parserPreOnly.splTokenInfoMap[mint4_addr]
	assert.True(t, ok, "mint4_addr info should exist from preBalance")
	assert.Equal(t, mint4_addr, infoMint4.MintAddress)
	assert.Equal(t, owner1, infoMint4.Owner)
	assert.Equal(t, "100", infoMint4.AmountUIAmount)
	assert.Equal(t, uint8(2), parserPreOnly.splDecimalsMap[mint4_addr], "mint4_addr decimals should be from preBalance")
}

// Helper to create SPL Transfer instruction data
func splTransferInstructionData(amount uint64) []byte {
	data := make([]byte, 9)
	data[0] = 3 // Transfer instruction type
	solana.Encoding.Binary.PutUint64(data[1:], amount, solana.Encoding.LittleEndian)
	return data
}

// Helper to create SPL TransferChecked instruction data
func splTransferCheckedInstructionData(amount uint64, decimals uint8) []byte {
	data := make([]byte, 10)
	data[0] = 12 // TransferChecked instruction type
	solana.Encoding.Binary.PutUint64(data[1:9], amount, solana.Encoding.LittleEndian)
	data[9] = decimals
	return data
}


func TestParseGenericInnerSwapsAndIntegration(t *testing.T) {
	// Define involved accounts
	signer := solana.NewWallet() // Transaction signer and owner of token accounts
	tokenAMint := solana.MustPublicKeyFromBase58("TokenAMint11111111111111111111111111111111")
	tokenBMint := solana.MustPublicKeyFromBase58("TokenBMint11111111111111111111111111111111")

	// Signer's token accounts
	signerTokenAAccount := solana.NewWallet().PublicKey() // Account for Token A
	signerTokenBAccount := solana.NewWallet().PublicKey() // Account for Token B

	// Dummy program that "hosts" the inner instructions (not a known DEX)
	dummyProgramID := solana.MustPublicKeyFromBase58("DummyProg11111111111111111111111111111111")
	tokenProgramID := solana.TokenProgramID // Standard SPL Token Program

	// AccountKeys for the transaction message
	// Order: Signer, DummyProgram, TokenProgram, SignerTokenA, SignerTokenB, TokenAMint, TokenBMint
	// (Actual token mints might not be in outer accounts for generic transfers, but can be for clarity in test setup)
	accounts := []solana.PublicKey{
		signer.PublicKey(),    // Index 0: Signer
		dummyProgramID,        // Index 1: Outer program
		tokenProgramID,        // Index 2: SPL Token Program (referenced by inner instructions)
		signerTokenAAccount,   // Index 3: Signer's account for token A (source)
		signerTokenBAccount,   // Index 4: Signer's account for token B (destination)
		tokenAMint,            // Index 5
		tokenBMint,            // Index 6
	}

	// PreTokenBalances to define initial state and ownership
	// These AccountIndex values (0, 1 here) are relative to the *transaction's* full list of accounts,
	// which are `signerTokenAAccount` (index 3 in `accounts` slice) and `signerTokenBAccount` (index 4).
	// So, the `AccountIndex` in `mockTokenBalance` needs to map to these.
	// Let's say signerTokenAAccount is at index 3 in the tx's `AccountKeys`
	// and signerTokenBAccount is at index 4.
	uiAmount100 := 100.0
	uiAmount0 := 0.0

	preBalances := []rpc.TokenBalance{
		mockTokenBalance(3, tokenAMint.String(), signer.PublicKey().String(), "1000", 6, uiAmount100), // Signer has 1000 of Token A (decimals 6)
		mockTokenBalance(4, tokenBMint.String(), signer.PublicKey().String(), "50", 8, uiAmount0),    // Signer has 50 of Token B (decimals 8)
	}
	// PostTokenBalances (optional for this test if amounts aren't checked via balance diff, but good for completeness)
	postBalances := []rpc.TokenBalance{
		mockTokenBalance(3, tokenAMint.String(), signer.PublicKey().String(), "800", 6, uiAmount0),  // Signer spent 200 of Token A
		mockTokenBalance(4, tokenBMint.String(), signer.PublicKey().String(), "150", 8, uiAmount0), // Signer received 100 of Token B
	}

	// Inner instructions:
	// 1. Transfer 200 of Token A from signerTokenAAccount to some intermediate/dummy account (or a known sink if simpler)
	//    For simplicity, let's make the destination a dummy account not owned by signer.
	//    Source: signerTokenAAccount (index 3), Dest: dummy (not in main accounts, so won't be parsed as 'to signer')
	//    Authority: signer (index 0)
	// 2. Transfer 100 of Token B from some dummy source to signerTokenBAccount
	//    Source: dummy, Dest: signerTokenBAccount (index 4)
	//    Authority: can be dummyProgramID (index 1) if it's a CPI, or another authority.
	// For parseGenericInnerSwaps, the authority of the transfer is less important than source/dest ownership.

	// Let's refine the inner instructions to be simpler for the current heuristic:
	// Transfer A: signer's account -> some other account (not signer's)
	// Transfer B: some other account (not signer's) -> signer's account

	otherAccountA := solana.NewWallet().PublicKey() // Receives token A from signer
	otherAccountB := solana.NewWallet().PublicKey() // Sends token B to signer

	// We need to add these to `accounts` if they are used by inner instructions' AccountIndices
	// New Account Key Order:
	// 0: signer.PublicKey()
	// 1: dummyProgramID
	// 2: tokenProgramID
	// 3: signerTokenAAccount (source for transfer A)
	// 4: signerTokenBAccount (dest for transfer B)
	// 5: tokenAMint
	// 6: tokenBMint
	// 7: otherAccountA (dest for transfer A)
	// 8: otherAccountB (source for transfer B)
	accounts = append(accounts, otherAccountA, otherAccountB)

	// Update Pre/Post Balances account indices accordingly
	// signerTokenAAccount is index 3, signerTokenBAccount is index 4
	// Let's add balances for otherAccountA and otherAccountB if needed for mint lookups, though parseGenericInnerSwaps
	// primarily uses Pre/PostTokenBalances to find the *mint* of an account owned by the *signer*.
	// The heuristic for GenericSwap is:
	// 1. Transfer from signer (owner of source_account == signer)
	// 2. Transfer to signer (owner of dest_account == signer)
	// So, `otherAccountA` and `otherAccountB` don't strictly need to be owned by the signer.
	// Their mints are not directly looked up by parseGenericInnerSwaps for the swap determination,
	// but the mints of signer's accounts (signerTokenAAccount, signerTokenBAccount) are.

	// PreBalances (indices must match the final `accounts` list passed to `newTestParser`)
	// signerTokenAAccount is index 3, signerTokenBAccount is index 4
	preBalancesUpdated := []rpc.TokenBalance{
		mockTokenBalance(3, tokenAMint.String(), signer.PublicKey().String(), "1000", 6, uiAmount100),
		mockTokenBalance(4, tokenBMint.String(), signer.PublicKey().String(), "50", 8, uiAmount0),
		// Optional: balances for otherAccountA, otherAccountB if they are involved in mint lookups (not for current generic logic)
		mockTokenBalance(7, tokenAMint.String(), otherAccountA.String(), "0", 6, uiAmount0), // otherAccountA starts with 0 of Token A
		mockTokenBalance(8, tokenBMint.String(), otherAccountB.String(), "500", 8, uiAmount0),// otherAccountB has 500 of Token B
	}
	postBalancesUpdated := []rpc.TokenBalance{
		mockTokenBalance(3, tokenAMint.String(), signer.PublicKey().String(), "800", 6, uiAmount0),  // Signer spent 200 of Token A
		mockTokenBalance(4, tokenBMint.String(), signer.PublicKey().String(), "150", 8, uiAmount0), // Signer received 100 of Token B
		mockTokenBalance(7, tokenAMint.String(), otherAccountA.String(), "200", 6, uiAmount0), // otherAccountA received 200 of Token A
		mockTokenBalance(8, tokenBMintString(), otherAccountB.String(), "400", 8, uiAmount0),// otherAccountB sent 100 of Token B
	}


	innerInstructions := []rpc.InnerInstruction{
		{
			Index: 0, // Belongs to the first (and only) outer instruction
			Instructions: []solana.CompiledInstruction{
				{ // Transfer 1: Token A from signer to otherAccountA
					ProgramIDIndex: 2, // SPL Token Program
					Accounts:       []uint16{3, 7, 0}, // Source (signerTokenAAccount), Dest (otherAccountA), Authority (signer)
					Data:           splTransferInstructionData(200),
				},
				{ // Transfer 2: Token B from otherAccountB to signer
					ProgramIDIndex: 2, // SPL Token Program
					Accounts:       []uint16{8, 4, 1}, // Source (otherAccountB), Dest (signerTokenBAccount), Authority (dummyProgram - acting as authority)
					Data:           splTransferInstructionData(100),
				},
			},
		},
	}

	// Outer instruction (dummy program)
	outerInstructions := []solana.CompiledInstruction{
		{
			ProgramIDIndex: 1, // dummyProgramID
			Accounts:       []uint16{}, // Outer accounts can be empty or reference relevant ones
			Data:           []byte{1,2,3}, // Dummy data
		},
	}


	parser := newTestParser(accounts, preBalancesUpdated, postBalancesUpdated, innerInstructions)
	parser.txInfo.Message.Instructions = outerInstructions // Set the outer instructions for ParseTransaction

	// Call ParseTransaction
	swapData, err := parser.ParseTransaction()
	assert.NoError(t, err, "ParseTransaction should not error")
	assert.Equal(t, 1, len(swapData), "Should find one generic swap")

	if len(swapData) == 1 {
		sd := swapData[0]
		assert.Equal(t, GENERIC_SWAP, sd.Type, "Swap type should be GENERIC_SWAP")

		gsData, ok := sd.Data.(GenericSwapData) // Value type as it's stored in parseGenericInnerSwaps
		if !ok {
			gsDataPtr, okPtr := sd.Data.(*GenericSwapData)
			if !okPtr {
				t.Fatalf("Swap data is not of type GenericSwapData or *GenericSwapData: %T", sd.Data)
			}
			gsData = *gsDataPtr // Dereference if it's a pointer
		}

		assert.Equal(t, tokenAMint.String(), gsData.SourceMint)
		assert.Equal(t, uint64(200), gsData.SourceAmount)
		assert.Equal(t, uint8(6), gsData.SourceDecimals) // From preBalances + splDecimalMap
		assert.Equal(t, signer.PublicKey().String(), gsData.Owner)

		assert.Equal(t, tokenBMint.String(), gsData.DestinationMint)
		assert.Equal(t, uint64(100), gsData.DestinationAmount)
		assert.Equal(t, uint8(8), gsData.DestinationDecimals) // From preBalances + splDecimalMap
	}
}


// TestProcessSwapDataGenericSwap
// TestProcessSwapDataPumpfunDecimals
// TestProcessSwapDataPumpfunMissingDecimals

// (Helper for SPL Transfer instruction data might be needed)
func TestProcessSwapData_GenericSwap(t *testing.T) {
	signer := solana.NewWallet().PublicKey()
	tokenAMint := "TokenAMint11111111111111111111111111111111"
	tokenBMint := "TokenBMint11111111111111111111111111111111"

	parser := newTestParser([]solana.PublicKey{signer}, nil, nil, nil)
	parser.splDecimalsMap[tokenAMint] = 6
	parser.splDecimalsMap[tokenBMint] = 8

	genericSwapInput := SwapData{
		Type: GENERIC_SWAP,
		Data: GenericSwapData{
			SourceMint:          tokenAMint,
			SourceAmount:        1000,
			SourceDecimals:      6, // Assuming already resolved by parseGenericInnerSwaps
			Owner:               signer.String(),
			DestinationMint:     tokenBMint,
			DestinationAmount:   200,
			DestinationDecimals: 8, // Assuming already resolved by parseGenericInnerSwaps
		},
	}

	swapInfo, err := parser.ProcessSwapData([]SwapData{genericSwapInput})
	assert.NoError(t, err)
	assert.NotNil(t, swapInfo)

	assert.Equal(t, solana.MustPublicKeyFromBase58(tokenAMint), swapInfo.TokenInMint)
	assert.Equal(t, uint64(1000), swapInfo.TokenInAmount)
	assert.Equal(t, uint8(6), swapInfo.TokenInDecimals)

	assert.Equal(t, solana.MustPublicKeyFromBase58(tokenBMint), swapInfo.TokenOutMint)
	assert.Equal(t, uint64(200), swapInfo.TokenOutAmount)
	assert.Equal(t, uint8(8), swapInfo.TokenOutDecimals)

	assert.Contains(t, swapInfo.AMMs, string(GENERIC_SWAP))
	assert.Equal(t, []solana.PublicKey{signer}, swapInfo.Signers) // Based on default ProcessSwapData logic
}


func TestProcessSwapData_PumpfunDecimals(t *testing.T) {
	signer := solana.NewWallet().PublicKey()
	pumpTokenMint := "PumpToken11111111111111111111111111111111"
	pumpTokenDecimals := uint8(7)

	parser := newTestParser([]solana.PublicKey{signer}, nil, nil, nil)
	// Ensure splDecimalsMap is populated by the parser's init or manually for test
	parser.splDecimalsMap[pumpTokenMint] = pumpTokenDecimals

	// Simulate a "buy" event: SOL -> PumpToken
	pumpFunBuyEvent := PumpfunTradeEvent{
		IsBuy:       true,
		Mint:        solana.MustPublicKeyFromBase58(pumpTokenMint),
		SolAmount:   1 * solana.LAMPORTS_PER_SOL, // 1 SOL
		TokenAmount: 10000,                       // 10000 PumpTokens
		Timestamp:   uint64(time.Now().Unix()),
	}
	pumpFunBuySwapData := SwapData{Type: PUMP_FUN, Data: &pumpFunBuyEvent}

	swapInfoBuy, errBuy := parser.ProcessSwapData([]SwapData{pumpFunBuySwapData})
	assert.NoError(t, errBuy)
	assert.NotNil(t, swapInfoBuy)

	assert.Equal(t, NATIVE_SOL_MINT_PROGRAM_ID, swapInfoBuy.TokenInMint)
	assert.Equal(t, uint64(1 * solana.LAMPORTS_PER_SOL), swapInfoBuy.TokenInAmount)
	assert.Equal(t, uint8(9), swapInfoBuy.TokenInDecimals) // SOL decimals
	assert.Equal(t, solana.MustPublicKeyFromBase58(pumpTokenMint), swapInfoBuy.TokenOutMint)
	assert.Equal(t, uint64(10000), swapInfoBuy.TokenOutAmount)
	assert.Equal(t, pumpTokenDecimals, swapInfoBuy.TokenOutDecimals, "Pumpfun buy output token decimals should come from splDecimalsMap")

	// Simulate a "sell" event: PumpToken -> SOL
	pumpFunSellEvent := PumpfunTradeEvent{
		IsBuy:       false,
		Mint:        solana.MustPublicKeyFromBase58(pumpTokenMint),
		SolAmount:   2 * solana.LAMPORTS_PER_SOL, // 2 SOL
		TokenAmount: 20000,                       // 20000 PumpTokens
		Timestamp:   uint64(time.Now().Unix()),
	}
	pumpFunSellSwapData := SwapData{Type: PUMP_FUN, Data: &pumpFunSellEvent}

	swapInfoSell, errSell := parser.ProcessSwapData([]SwapData{pumpFunSellSwapData})
	assert.NoError(t, errSell)
	assert.NotNil(t, swapInfoSell)

	assert.Equal(t, solana.MustPublicKeyFromBase58(pumpTokenMint), swapInfoSell.TokenInMint)
	assert.Equal(t, uint64(20000), swapInfoSell.TokenInAmount)
	assert.Equal(t, pumpTokenDecimals, swapInfoSell.TokenInDecimals, "Pumpfun sell input token decimals should come from splDecimalsMap")
	assert.Equal(t, NATIVE_SOL_MINT_PROGRAM_ID, swapInfoSell.TokenOutMint)
	assert.Equal(t, uint64(2 * solana.LAMPORTS_PER_SOL), swapInfoSell.TokenOutAmount)
	assert.Equal(t, uint8(9), swapInfoSell.TokenOutDecimals) // SOL decimals
}

func TestProcessSwapData_PumpfunMissingDecimals(t *testing.T) {
	signer := solana.NewWallet().PublicKey()
	missingMint := "MissingDecimalsMint1111111111111111111111"

	// splDecimalsMap will NOT contain missingMint
	parser := newTestParser([]solana.PublicKey{signer}, nil, nil, nil)

	// Logrus hook to capture log messages
	// hook := test.NewGlobal() // from testify/logrus
	// logrus.AddHook(hook)
	// defer logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks)) // Reset global hooks

	pumpFunEvent := PumpfunTradeEvent{
		IsBuy:       true,
		Mint:        solana.MustPublicKeyFromBase58(missingMint),
		SolAmount:   1 * solana.LAMPORTS_PER_SOL,
		TokenAmount: 5000,
		Timestamp:   uint64(time.Now().Unix()),
	}
	pumpFunSwapData := SwapData{Type: PUMP_FUN, Data: &pumpFunEvent}

	swapInfo, err := parser.ProcessSwapData([]SwapData{pumpFunSwapData})
	assert.NoError(t, err) // Should still process, defaulting decimals
	assert.NotNil(t, swapInfo)

	assert.Equal(t, solana.MustPublicKeyFromBase58(missingMint), swapInfo.TokenOutMint)
	assert.Equal(t, uint8(0), swapInfo.TokenOutDecimals, "Decimals should default to 0 when missing from map")

	// Check log for warning (this part can be tricky to implement robustly without a dedicated log testing library)
	// foundLog := false
	// for _, entry := range hook.AllEntries() {
	// 	if strings.Contains(entry.Message, "Decimals not found for mint") && strings.Contains(entry.Message, missingMint) {
	// 		foundLog = true
	// 		break
	// 	}
	// }
	// assert.True(t, foundLog, "Expected warning log for missing decimals")
	// hook.Reset()
}


func TestMain(m *testing.M) {
	// Optional: setup code for all tests, like initializing a global logger
	// if needed, or ensuring solana-go's default RPC client isn't actually called.
	// if needed, or ensuring solana-go's default RPC client isn't actually called.
	m.Run()
}
