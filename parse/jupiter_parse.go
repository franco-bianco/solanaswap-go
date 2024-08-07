package parse

import (
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

type JupiterParser struct {
	tx             *rpc.GetTransactionResult
	txInfo         *solana.Transaction
	allAccountKeys solana.PublicKeySlice
	splDecimalsMap map[string]uint8
}

type JupiterSwapEvent struct {
	Amm          solana.PublicKey
	InputMint    solana.PublicKey
	InputAmount  uint64
	OutputMint   solana.PublicKey
	OutputAmount uint64
}

func NewJupiterParser(tx *rpc.GetTransactionResult) (*JupiterParser, error) {

	if tx.Meta.Err != nil { // reject failed transactions
		return nil, fmt.Errorf("failed transaction")
	}

	txInfo, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, err
	}

	allAccountKeys := append(txInfo.Message.AccountKeys, tx.Meta.LoadedAddresses.Writable...)
	allAccountKeys = append(allAccountKeys, tx.Meta.LoadedAddresses.ReadOnly...)

	parser := &JupiterParser{
		tx:             tx,
		txInfo:         txInfo,
		allAccountKeys: allAccountKeys,
		splDecimalsMap: make(map[string]uint8),
	}

	return parser, nil
}

func (p *JupiterParser) ParseJupiterTransaction() (*SwapInfo, error) {

	instrs := make([]solana.CompiledInstruction, 0, len(p.txInfo.Message.Instructions)+len(p.tx.Meta.InnerInstructions)*2)
	instrs = append(instrs, p.txInfo.Message.Instructions...)
	for _, innerInstr := range p.tx.Meta.InnerInstructions {
		instrs = append(instrs, innerInstr.Instructions...)
	}

	events := make([]interface{}, 0, len(instrs))
	for _, instruction := range instrs {
		eventData, err := parseForEvents(instruction)
		if err != nil {
			return nil, fmt.Errorf("failed to parse events: %w", err)
		}
		if eventData != nil {
			events = append(events, eventData)
		}
	}

	if err := p.extractSPLDecimals(); err != nil {
		return nil, fmt.Errorf("failed to extract SPL decimals: %w", err)
	}

	swapInfo := SwapInfo{
		Signature: p.txInfo.Signatures[0].String(),
		Signers:   make([]string, 0, len(p.txInfo.Message.Signers())),
	}

	for i, event := range events {
		if jupiterEvent, ok := event.(*JupiterSwapEvent); ok {
			if i == 0 {
				swapInfo.TokenInMint = jupiterEvent.InputMint.String()
				swapInfo.TokenInAmount = float64(jupiterEvent.InputAmount) / math.Pow10(int(p.splDecimalsMap[swapInfo.TokenInMint]))
			}
			swapInfo.TokenOutMint = jupiterEvent.OutputMint.String()
			swapInfo.TokenOutAmount = float64(jupiterEvent.OutputAmount) / math.Pow10(int(p.splDecimalsMap[swapInfo.TokenOutMint]))
		}
	}

	for _, signer := range p.txInfo.Message.Signers() {
		swapInfo.Signers = append(swapInfo.Signers, signer.String())
	}

	return &swapInfo, nil
}

func (p *JupiterParser) extractSPLDecimals() error {
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
	if _, exists := mintToDecimals[NativeSolMint.String()]; !exists {
		mintToDecimals[NativeSolMint.String()] = 9 // Native SOL has 9 decimal places
	}

	p.splDecimalsMap = mintToDecimals

	return nil
}
