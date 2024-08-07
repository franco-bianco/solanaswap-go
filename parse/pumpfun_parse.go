package parse

import (
	"fmt"
	"log"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

var PumpfunProgramID = solana.MustPrivateKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")

type PumpfunTradeEvent struct {
	Mint                 solana.PublicKey
	SolAmount            uint64
	TokenAmount          uint64
	IsBuy                bool
	User                 solana.PublicKey
	Timestamp            int64
	VirtualSolReserves   uint64
	VirtualTokenReserves uint64
}

type PumpfunCreateEvent struct {
	Name         string
	Symbol       string
	Uri          string
	Mint         solana.PublicKey
	BondingCurve solana.PublicKey
	User         solana.PublicKey
}

type PumpfunTxData struct {
	Signature string `json:"signature"`
	Events    []interface{}
}

func ParsePumpfunEvents(tx *rpc.GetTransactionResult) (*PumpfunTxData, error) {

	if tx.Meta.Err != nil { // reject failed transactions
		return nil, fmt.Errorf("transaction failed")
	}

	txInfo, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil, err
	}

	// compile instructions
	instrs := make([]solana.CompiledInstruction, 0, len(txInfo.Message.Instructions)+len(tx.Meta.InnerInstructions)*2)
	instrs = append(instrs, txInfo.Message.Instructions...)
	for _, innerInstr := range tx.Meta.InnerInstructions {
		instrs = append(instrs, innerInstr.Instructions...)
	}

	// compile account keys
	allAccountKeys := append(txInfo.Message.AccountKeys, tx.Meta.LoadedAddresses.Writable...)
	allAccountKeys = append(allAccountKeys, tx.Meta.LoadedAddresses.ReadOnly...)

	if !isPumpfunTx(instrs, allAccountKeys) {
		return nil, fmt.Errorf("not a pumpfun transaction")
	}

	pumpfunTxData := &PumpfunTxData{
		Signature: txInfo.Signatures[0].String(),
	}

	for _, instr := range instrs {
		eventData, err := parseForEvents(instr)
		if err != nil {
			log.Printf("error parsing event: %s\n", err)
			continue
		}
		if eventData != nil {
			pumpfunTxData.Events = append(pumpfunTxData.Events, eventData)
		}
	}

	return pumpfunTxData, nil
}

// isPumpfunTx checks if the transaction is a pumpfun transaction
func isPumpfunTx(instrs []solana.CompiledInstruction, accountKeys []solana.PublicKey) bool {
	var isPumpfunTx bool
	for _, instr := range instrs {
		if int(instr.ProgramIDIndex) >= len(accountKeys) {
			continue // Skip invalid program ID indexes
		}
		progId := accountKeys[instr.ProgramIDIndex]
		if progId.Equals(PumpfunProgramID.PublicKey()) {
			isPumpfunTx = true
			break
		}
	}
	return isPumpfunTx
}
