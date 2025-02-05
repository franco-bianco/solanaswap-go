package solanaswapgo

import (
	"bytes"
	"fmt"

	"github.com/mr-tron/base58"
)

var (
	OKX_SWAP_DISCRIMINATOR                 = [8]byte{248, 198, 158, 145, 225, 117, 135, 200}
	OKX_SWAP2_DISCRIMINATOR                = [8]byte{65, 75, 63, 76, 235, 91, 91, 136}
	OKX_COMMISSION_SPL_SWAP2_DISCRIMINATOR = [8]byte{173, 131, 78, 38, 150, 165, 123, 15}
)

func (p *Parser) processOKXSwaps(instructionIndex int) []SwapData {
	p.Log.Infof("starting okx swap parsing for instruction index: %d", instructionIndex)

	parentInstruction := p.txInfo.Message.Instructions[instructionIndex]
	programID := p.allAccountKeys[parentInstruction.ProgramIDIndex]

	if !programID.Equals(OKX_DEX_ROUTER_PROGRAM_ID) {
		p.Log.Warnf("instruction %d skipped: not okx dex router program", instructionIndex)
		return nil
	}

	if len(parentInstruction.Data) < 8 {
		p.Log.Warnf("instruction %d skipped: data too short (%d)", instructionIndex, len(parentInstruction.Data))
		return nil
	}

	decodedBytes, err := base58.Decode(parentInstruction.Data.String())
	if err != nil {
		p.Log.Errorf("failed to decode okx swap instruction %d: %s", instructionIndex, err)
		return nil
	}

	discriminator := decodedBytes[:8]
	p.Log.Infof("decoded okx swap instruction %d with discriminator: %x", instructionIndex, discriminator)

	var swaps []SwapData

	switch {
	case bytes.Equal(discriminator, OKX_SWAP_DISCRIMINATOR[:]):
		p.Log.Infof("processing okx swap type: okx_swap for instruction %d", instructionIndex)
		return p.processOKXRouterSwaps(instructionIndex)

	case bytes.Equal(discriminator, OKX_SWAP2_DISCRIMINATOR[:]):
		p.Log.Infof("processing okx swap type: okx_swap2 for instruction %d", instructionIndex)
		swaps = append(swaps, p.processOKXRouterSwaps(instructionIndex)...)

	case bytes.Equal(discriminator, OKX_COMMISSION_SPL_SWAP2_DISCRIMINATOR[:]):
		p.Log.Infof("processing okx swap type: okx_commission_spl_swap2 for instruction %d", instructionIndex)
		swaps = append(swaps, p.processOKXRouterSwaps(instructionIndex)...)

	default:
		p.Log.Warnf("unknown okx swap discriminator %x for instruction %d", discriminator, instructionIndex)
		return nil
	}

	p.Log.Infof("returning %d swaps for instruction %d", len(swaps), instructionIndex)
	return swaps
}

func (p *Parser) processOKXRouterSwaps(instructionIndex int) []SwapData {
	var swaps []SwapData
	seen := make(map[string]bool)
	processedProtocols := make(map[SwapType]bool)

	innerInstructions := p.getInnerInstructions(instructionIndex)
	p.Log.Infof("processing okx router swaps for instruction %d: %d inner instructions", instructionIndex, len(innerInstructions))
	if len(innerInstructions) == 0 {
		p.Log.Warnf("no inner instructions for instruction %d", instructionIndex)
		return swaps
	}

	for _, inner := range innerInstructions {
		progID := p.allAccountKeys[inner.ProgramIDIndex]

		switch {
		case progID.Equals(RAYDIUM_V4_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CPMM_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_AMM_PROGRAM_ID) ||
			progID.Equals(RAYDIUM_CONCENTRATED_LIQUIDITY_PROGRAM_ID):
			if processedProtocols[RAYDIUM] {
				continue
			}
			p.Log.Debugf("instruction %d: processing raydium inner instruction", instructionIndex)
			if raydSwaps := p.processRaydSwaps(instructionIndex); len(raydSwaps) > 0 {
				for _, swap := range raydSwaps {
					key := getSwapKey(swap)
					if !seen[key] {
						p.Log.Debugf("adding raydium swap: %s", key)
						swaps = append(swaps, swap)
						seen[key] = true
					}
				}
				processedProtocols[RAYDIUM] = true
			}

		case progID.Equals(ORCA_PROGRAM_ID):
			if processedProtocols[ORCA] {
				continue
			}
			p.Log.Debugf("instruction %d: processing orca inner instruction", instructionIndex)
			if orcaSwaps := p.processOrcaSwaps(instructionIndex); len(orcaSwaps) > 0 {
				for _, swap := range orcaSwaps {
					key := getSwapKey(swap)
					if !seen[key] {
						p.Log.Debugf("adding orca swap: %s", key)
						swaps = append(swaps, swap)
						seen[key] = true
					}
				}
				processedProtocols[ORCA] = true
			}

		case progID.Equals(METEORA_PROGRAM_ID) ||
			progID.Equals(METEORA_POOLS_PROGRAM_ID):
			if processedProtocols[METEORA] {
				continue
			}
			p.Log.Debugf("instruction %d: processing meteora inner instruction", instructionIndex)
			if meteoraSwaps := p.processMeteoraSwaps(instructionIndex); len(meteoraSwaps) > 0 {
				for _, swap := range meteoraSwaps {
					key := getSwapKey(swap)
					if !seen[key] {
						p.Log.Debugf("adding meteora swap: %s", key)
						swaps = append(swaps, swap)
						seen[key] = true
					}
				}
				processedProtocols[METEORA] = true
			}

		case progID.Equals(PUMP_FUN_PROGRAM_ID):
			if processedProtocols[PUMP_FUN] {
				continue
			}
			p.Log.Debugf("instruction %d: processing pumpfun inner instruction", instructionIndex)
			if pumpfunSwaps := p.processPumpfunSwaps(instructionIndex); len(pumpfunSwaps) > 0 {
				for _, swap := range pumpfunSwaps {
					key := getSwapKey(swap)
					if !seen[key] {
						p.Log.Debugf("adding pumpfun swap: %s", key)
						swaps = append(swaps, swap)
						seen[key] = true
					}
				}
				processedProtocols[PUMP_FUN] = true
			}

		default:
			p.Log.Debugf("instruction %d: skipping unknown inner instruction", instructionIndex)
		}
	}

	p.Log.Infof("processed okx router swaps: %d unique swaps", len(swaps))
	return swaps
}

// getSwapKey generates a unique key for a swap based on its type and amounts
func getSwapKey(swap SwapData) string {
	switch data := swap.Data.(type) {
	case *TransferCheck:
		return fmt.Sprintf("%s-%s-%s", swap.Type, data.Info.TokenAmount.Amount, data.Info.Mint)
	case *TransferData:
		return fmt.Sprintf("%s-%d-%s", swap.Type, data.Info.Amount, data.Mint)
	default:
		return fmt.Sprintf("%s-%v", swap.Type, data)
	}
}
