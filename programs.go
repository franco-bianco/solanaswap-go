package solanaswap

import (
	"cmp"
	"slices"

	"github.com/gagliardetto/solana-go"
)

// ProgramKind classifies the programs the parser recognises.
type ProgramKind int

const (
	// KindDEX is a program that executes swaps against its own liquidity:
	// AMMs, CLMMs, order books, proprietary market makers and launchpad
	// bonding curves.
	KindDEX ProgramKind = iota + 1
	// KindAggregator routes a swap through one or more DEX programs.
	KindAggregator
	// KindBot is a trading bot or front-end program that wraps DEX calls.
	KindBot
)

func (k ProgramKind) String() string {
	switch k {
	case KindDEX:
		return "dex"
	case KindAggregator:
		return "aggregator"
	case KindBot:
		return "bot"
	}
	return "unknown"
}

// Program describes a recognised on-chain program.
type Program struct {
	Name string
	ID   solana.PublicKey
	Kind ProgramKind
}

// swapDecoder extracts the swaps executed by ix, an invocation of a known
// program. It returns nil when ix is not a swap (for example an
// add-liquidity or admin instruction) or when it cannot decode it, in which
// case the generic transfer-based decoder is used for DEX programs.
type swapDecoder func(e *extractor, ix *instruction) []Swap

type programSpec struct {
	Program
	decode swapDecoder
}

// Program IDs that have dedicated handling.
var (
	PumpFunProgramID          = solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	PumpSwapProgramID         = solana.MustPublicKeyFromBase58("pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA")
	JupiterV6ProgramID        = solana.MustPublicKeyFromBase58("JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4")
	MoonshotProgramID         = solana.MustPublicKeyFromBase58("MoonCVVNZFSYkqNXP6bxHLPL6QQJiMagDL3qcqUQTrG")
	RaydiumLaunchLabProgramID = solana.MustPublicKeyFromBase58("LanMV9sAd7wArD4vJFi2qDdfnVhFxYSUg6eADduJ3uj")
)

var registry = newRegistry(
	// Launchpads and bonding curves.
	programSpec{Program{"Pump.fun", PumpFunProgramID, KindDEX}, decodePumpFun},
	programSpec{Program{"PumpSwap", PumpSwapProgramID, KindDEX}, nil},
	programSpec{Program{"Moonshot", MoonshotProgramID, KindDEX}, decodeMoonshot},
	programSpec{Program{"Raydium LaunchLab", RaydiumLaunchLabProgramID, KindDEX}, nil},

	// AMMs, CLMMs and order books.
	dex("Raydium AMM v4", "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8"),
	dex("Raydium CPMM", "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C"),
	dex("Raydium CLMM", "CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK"),
	dex("Orca Whirlpool", "whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"),
	dex("Meteora DLMM", "LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo"),
	dex("Meteora DAMM v1", "Eo7WjKq67rjJQSZxS6z3YkapzY3eMj6Xy8X5EQVn5UaB"),
	dex("Meteora DAMM v2", "cpamdpZCGKUy5JxQXB4dcpGPiikHawvSWAd6mEn1sGG"),
	dex("Meteora DBC", "dbcij3LWUppWqq96dh6gJWwBifmcGfLSB5D4DuSMaqN"),
	dex("Phoenix", "PhoeNiXZ8ByJGLkxNfZRnkUfjvmuYqLR89jjFHGqdXY"),

	// Aggregators.
	programSpec{Program{"Jupiter", JupiterV6ProgramID, KindAggregator}, decodeJupiter},
	aggregator("OKX DEX Router", "6m2CDdhRgxpH4WjvdzxAYbGxwdGUz5MziiL5jek2kBma"),
	aggregator("OKX DEX Router v2", "proVF4pMXVaYqmy4NjniPh4pqKNfMmsihgd4wdkCX3u"),
	aggregator("Raydium Router", "routeUGWgWzqBWFcrCfv8tritsqukccJPu3q5GPP3xS"),

	// Trading bots.
	bot("Photon", "BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW"),
	bot("Banana Gun", "BANANAjs7FJiPQqJTGFzkZJndT9o7UmKiYYGaJz6frGu"),
	bot("Maestro", "MaestroAAe9ge5HTc64VbBQZ6fP77pwvrhM8i1XWSAx"),
	bot("Bloom", "b1oomGGqPKGD6errbyfbVMBuzSC8WtAAYo8MwNafWW1"),
	bot("Nova", "NoVA1TmDUqksaj2hB1nayFkPysjJbFiU76dT4qPw2wm"),
	bot("MinTech", "minTcHYRLVPubRK8nt6sqe2ZpWrGDLQoNLipDJCGocY"),
)

func dex(name, id string) programSpec {
	return programSpec{Program: Program{name, solana.MustPublicKeyFromBase58(id), KindDEX}}
}

func aggregator(name, id string) programSpec {
	return programSpec{Program: Program{name, solana.MustPublicKeyFromBase58(id), KindAggregator}}
}

func bot(name, id string) programSpec {
	return programSpec{Program: Program{name, solana.MustPublicKeyFromBase58(id), KindBot}}
}

func newRegistry(specs ...programSpec) map[solana.PublicKey]programSpec {
	m := make(map[solana.PublicKey]programSpec, len(specs))
	for _, s := range specs {
		if _, dup := m[s.ID]; dup {
			panic("solanaswap: duplicate program " + s.ID.String())
		}
		m[s.ID] = s
	}
	return m
}

// LookupProgram returns the recognised program with the given ID.
func LookupProgram(id solana.PublicKey) (Program, bool) {
	s, ok := registry[id]
	return s.Program, ok
}

// Programs returns every recognised program, sorted by kind and name.
func Programs() []Program {
	out := make([]Program, 0, len(registry))
	for _, s := range registry {
		out = append(out, s.Program)
	}
	slices.SortFunc(out, func(a, b Program) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Name, b.Name))
	})
	return out
}

// protocolName returns the registry name for id, or the base58 ID itself for
// unrecognised programs.
func protocolName(id solana.PublicKey) string {
	if s, ok := registry[id]; ok {
		return s.Name
	}
	return id.String()
}
