package raydium_parse

import "math"

type SwapInfo struct {
	Signature      string
	Signers        []string
	TokenInMint    string
	TokenInAmount  float64
	TokenOutMint   string
	TokenOutAmount float64
}

func ConvertToSwapInfo(data *RaydiumTransactionData) SwapInfo {
	swapInfo := SwapInfo{
		Signature: data.Signature,
		Signers:   data.Signers,
	}

	if len(data.Transfers) < 2 {
		return swapInfo // return early if there aren't enough transfers
	}

	inTransfer := data.Transfers[0]
	swapInfo.TokenInMint = inTransfer.Mint
	swapInfo.TokenInAmount = float64(inTransfer.Info.Amount) / math.Pow10(int(inTransfer.Decimals))

	outTransfer := data.Transfers[len(data.Transfers)-1]
	swapInfo.TokenOutMint = outTransfer.Mint
	swapInfo.TokenOutAmount = float64(outTransfer.Info.Amount) / math.Pow10(int(outTransfer.Decimals))

	return swapInfo
}
