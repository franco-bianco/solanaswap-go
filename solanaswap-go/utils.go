package solanaswapgo

import (
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

func (p *Parser) convertRPCToSolanaInstruction(rpcInst rpc.CompiledInstruction) solana.CompiledInstruction {
	return solana.CompiledInstruction{
		ProgramIDIndex: rpcInst.ProgramIDIndex,
		Accounts:       rpcInst.Accounts,
		Data:           rpcInst.Data,
	}
}
