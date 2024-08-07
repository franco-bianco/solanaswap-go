package raydium_parse

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

var (
	RaydiumV4ProgramID              = solana.MustPublicKeyFromBase58("675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8")
	AssociatedTokenAccountProgramID = solana.MustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
	NativeSolMint                   = solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
)

type TransferInfo struct {
	Amount      uint64 `json:"amount"`
	Authority   string `json:"authority"`
	Destination string `json:"destination"`
	Source      string `json:"source"`
}

type TransferData struct {
	Info     TransferInfo `json:"info"`
	Type     string       `json:"type"`
	Mint     string       `json:"mint"`
	Decimals uint8        `json:"decimals"`
}

type RaydiumTransactionData struct {
	Signature string          `json:"signature"`
	Signers   []string        `json:"signers"`
	Transfers []*TransferData `json:"transfers"`
}

type RaydiumParser struct {
	tx                *rpc.GetTransactionResult
	txInfo            *solana.Transaction
	allAccountKeys    solana.PublicKeySlice
	splTokenAddresses map[string]TokenInfo
}

func NewRaydiumTransactionParser(tx *rpc.GetTransactionResult) (*RaydiumParser, error) {

	txInfo, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	allAccountKeys := append(txInfo.Message.AccountKeys, tx.Meta.LoadedAddresses.Writable...)
	allAccountKeys = append(allAccountKeys, tx.Meta.LoadedAddresses.ReadOnly...)

	parser := &RaydiumParser{
		tx:             tx,
		txInfo:         txInfo,
		allAccountKeys: allAccountKeys,
	}

	parser.splTokenAddresses, err = parser.extractSPLTokenAddresses()
	if err != nil {
		return nil, fmt.Errorf("failed to extract SPL Token Addresses: %w", err)
	}

	return parser, nil
}

func (p *RaydiumParser) ParseRaydiumTransaction() (*RaydiumTransactionData, error) {
	raydiumInstructionCount := 0

	signers := p.txInfo.Message.Signers()
	var signersStr []string
	for _, signer := range signers {
		signersStr = append(signersStr, signer.String())
	}

	var transferDatas []*TransferData
	for i, instr := range p.txInfo.Message.Instructions {
		if p.allAccountKeys[instr.ProgramIDIndex].Equals(RaydiumV4ProgramID) {
			raydiumInstructionCount++

			for _, innerInstr := range p.tx.Meta.InnerInstructions {
				if innerInstr.Index == uint16(i) {
					for _, innerInstr := range innerInstr.Instructions {
						transferData, err := p.processInstruction(innerInstr)
						if err != nil {
							return nil, fmt.Errorf("failed to process instruction: %w", err)
						}
						transferDatas = append(transferDatas, transferData)
					}
				}
			}
		}
	}

	if raydiumInstructionCount == 0 {
		return nil, fmt.Errorf("no Raydium instructions found in the transaction")
	}

	return &RaydiumTransactionData{
		Signature: p.txInfo.Signatures[0].String(),
		Signers:   signersStr,
		Transfers: transferDatas,
	}, nil
}

func (p *RaydiumParser) processInstruction(instr solana.CompiledInstruction) (*TransferData, error) {
	if !p.isTransferInstruction(instr) {
		return nil, fmt.Errorf("unsupported instruction")
	}

	transferData, err := p.processTransfer(instr)
	if err != nil {
		return nil, fmt.Errorf("failed to process transfer instruction: %w", err)
	}

	return transferData, nil
}

func (p *RaydiumParser) processTransfer(instr solana.CompiledInstruction) (*TransferData, error) {
	if len(instr.Data) < 9 {
		return nil, fmt.Errorf("invalid transfer instruction: data too short")
	}

	if len(instr.Accounts) < 3 {
		return nil, fmt.Errorf("invalid transfer instruction: not enough accounts")
	}

	amount := binary.LittleEndian.Uint64(instr.Data[1:9])

	if int(instr.Accounts[0]) >= len(p.allAccountKeys) || int(instr.Accounts[1]) >= len(p.allAccountKeys) || int(instr.Accounts[2]) >= len(p.allAccountKeys) {
		return nil, fmt.Errorf("invalid account index")
	}

	transferData := &TransferData{
		Info: TransferInfo{
			Amount:      amount,
			Source:      p.allAccountKeys[instr.Accounts[0]].String(),
			Destination: p.allAccountKeys[instr.Accounts[1]].String(),
			Authority:   p.allAccountKeys[instr.Accounts[2]].String(),
		},
		Type:     "transfer",
		Mint:     p.splTokenAddresses[p.allAccountKeys[instr.Accounts[1]].String()].Mint,
		Decimals: p.splTokenAddresses[p.allAccountKeys[instr.Accounts[1]].String()].Decimals,
	}

	if transferData.Mint == "" {
		transferData.Mint = "Unknown"
	}

	return transferData, nil
}

func (p *RaydiumParser) isTransferInstruction(instr solana.CompiledInstruction) bool {
	return len(instr.Data) >= 9 && (instr.Data[0] == 3 || instr.Data[0] == 12)
}

type TokenInfo struct {
	Mint     string
	Decimals uint8
}

func (p *RaydiumParser) extractSPLTokenAddresses() (map[string]TokenInfo, error) {
	splTokenAddresses := make(map[string]TokenInfo)

	for _, accountInfo := range p.tx.Meta.PostTokenBalances {
		if !accountInfo.Mint.IsZero() {
			accountKey := p.allAccountKeys[accountInfo.AccountIndex].String()
			splTokenAddresses[accountKey] = TokenInfo{
				Mint:     accountInfo.Mint.String(),
				Decimals: accountInfo.UiTokenAmount.Decimals,
			}
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

		source := p.allAccountKeys[instr.Accounts[0]].String()
		destination := p.allAccountKeys[instr.Accounts[1]].String()

		if _, exists := splTokenAddresses[source]; !exists {
			splTokenAddresses[source] = TokenInfo{Mint: "", Decimals: 0}
		}
		if _, exists := splTokenAddresses[destination]; !exists {
			splTokenAddresses[destination] = TokenInfo{Mint: "", Decimals: 0}
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

	for account, info := range splTokenAddresses {
		if info.Mint == "" {
			splTokenAddresses[account] = TokenInfo{
				Mint:     NativeSolMint.String(),
				Decimals: 9, // Native SOL has 9 decimal places
			}
		}
	}

	return splTokenAddresses, nil
}
