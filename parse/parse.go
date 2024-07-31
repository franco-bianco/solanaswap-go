package parse

import (
	"bytes"
	"fmt"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"

	ag_binary "github.com/gagliardetto/binary"
)

const pumpFunProgramID = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"

func ParseTx(tx rpc.GetParsedTransactionResult, client *rpc.Client) (*PumpFunTxData, error) {
	if tx.Meta.Err != nil {
		return nil, nil // ignore failed transactions
	}

	// ensure that it is a pumpfun tx
	if !isPumpfunTx(tx.Transaction.Message.Instructions) {
		return nil, nil
	}

	txData := PumpFunTxData{
		TransactionHash: tx.Transaction.Signatures[0].String(),
	}

	var innerInstrs []*rpc.ParsedInstruction
	for _, instr := range tx.Meta.InnerInstructions {
		innerInstrs = append(innerInstrs, instr.Instructions...)
	}

	// parse transaction
	for _, instr := range innerInstrs {
		eventData, err := parseForEvents(instr)
		if err != nil {
			continue
		}
		if eventData != nil {
			txData.Events = append(txData.Events, eventData)
		}
	}

	return &txData, nil
}

// parseForEvents parses the instruction data for pumpfun events
func parseForEvents(instruction *rpc.ParsedInstruction) (interface{}, error) {

	if len(instruction.Data) < 16 {
		return nil, nil
	}

	decodedBytes, err := base58.Decode(instruction.Data.String())
	if err != nil {
		return nil, fmt.Errorf("error decoding instruction data: %s", err)
	}

	discriminator := decodedBytes[:16]
	decoder := ag_binary.NewBorshDecoder(decodedBytes[16:])

	switch {
	case bytes.Equal(discriminator, TradeEventDiscriminator[:]):
		return handleTradeEvent(decoder)
	case bytes.Equal(discriminator, CreateEventDiscriminator[:]):
		return handleCreateEvent(decoder)
	default:
		return nil, nil
	}
}

// isPumpfunTx checks if the transaction is a pumpfun transaction
func isPumpfunTx(instrs []*rpc.ParsedInstruction) bool {
	var isPumpfunTx bool
	for _, instr := range instrs {
		if instr.ProgramId.String() == pumpFunProgramID {
			isPumpfunTx = true
			break
		}
	}
	return isPumpfunTx
}
