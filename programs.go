package solanaswap

import (
	"cmp"
	"slices"

	"github.com/gagliardetto/solana-go"

	"github.com/franco-bianco/solanaswap-go/v2/internal/txview"
)

// ProgramKind classifies the programs the parser recognises.
type ProgramKind int

const (
	// KindDEX swaps against its own liquidity: AMMs, CLMMs, order books,
	// proprietary market makers and launchpad bonding curves.
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

// MarshalText implements encoding.TextMarshaler.
func (k ProgramKind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// Program describes a recognised on-chain program.
type Program struct {
	Name string
	ID   solana.PublicKey
	Kind ProgramKind
}

// swapDecoder decodes the swaps of one invocation of a known program. It returns nil for
// non-swap instructions or on failure; DEX programs then fall back to transfer decoding.
type swapDecoder func(e *extractor, ix *txview.Instruction) []Swap

type programSpec struct {
	Program
	decode swapDecoder
}

// Program IDs with dedicated handling.
var (
	pumpFunProgramID          = solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	pumpSwapProgramID         = solana.MustPublicKeyFromBase58("pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA")
	moonshotProgramID         = solana.MustPublicKeyFromBase58("MoonCVVNZFSYkqNXP6bxHLPL6QQJiMagDL3qcqUQTrG")
	jupiterV6ProgramID        = solana.MustPublicKeyFromBase58("JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4")
	jupiterOrderEngineID      = solana.MustPublicKeyFromBase58("61DFfeTKM7trxYcPQCM78bJ794ddZprZpAwAnLiwTpYH")
	jupiterRecurringProgramID = solana.MustPublicKeyFromBase58("DCA265Vj8a9CEuX1eb1LWRnDT7uK6q1xMipnNyatn23M")
	jupiterTriggerProgramID   = solana.MustPublicKeyFromBase58("j1o2qRpjcyUwEvwtcfhEQefh773ZgjxcVRry7LDqg5X")
	dFlowProgramID            = solana.MustPublicKeyFromBase58("DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH")
)

// registry lists the recognised programs, verified on chain in November 2026.
// Retired programs stay so historical transactions keep their labels.
var registry = newRegistry(
	// Launchpads and bonding curves.
	programSpec{Program{"Pump.fun", pumpFunProgramID, KindDEX}, (*extractor).decodePumpFun},
	programSpec{Program{"PumpSwap", pumpSwapProgramID, KindDEX}, (*extractor).decodePumpSwap},
	programSpec{Program{"Moonshot", moonshotProgramID, KindDEX}, (*extractor).decodeMoonshot},
	dex("Raydium LaunchLab", "LanMV9sAd7wArD4vJFi2qDdfnVhFxYSUg6eADduJ3uj"),
	dex("Meteora DBC", "dbcij3LWUppWqq96dh6gJWwBifmcGfLSB5D4DuSMaqN"),
	dex("Boop.fun", "boop8hVGQGqehUK2iVEMEnMrL5RbjywRzHKBmBE7ry4"),
	dex("Heaven", "HEAVENoP2qxoeuF8Dj2oT1GHEnu49U5mJYkdeC8BAX2o"),
	dex("Vertigo", "vrTGoBuy5rYSxAfV3jaRJWHH6nN9WK4NRExGxsk1bCJ"),

	// AMMs and CLMMs.
	dex("Raydium AMM v4", "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8"),
	dex("Raydium CPMM", "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C"),
	dex("Raydium CLMM", "CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK"),
	dex("Orca Whirlpool", "whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"),
	dex("Meteora DLMM", "LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo"),
	dex("Meteora DAMM v1", "Eo7WjKq67rjJQSZxS6z3YkapzY3eMj6Xy8X5EQVn5UaB"),
	dex("Meteora DAMM v2", "cpamdpZCGKUy5JxQXB4dcpGPiikHawvSWAd6mEn1sGG"),
	dex("Raydium Stable", "5quBtoiQqxF9Jv6KYKctB59NT3gtJD2Y65kdnB1Uev3h"),
	dex("Orca Token Swap v2", "9W959DqEETiGZocYWCQPaJ6sBmUzgfxXfqGeTEdp3aQP"),
	dex("Meteora DLMM Pro", "dPro1D5wiqnGMH8vMDG23jh35btTHoWdyqXjqkqxe8n"),
	dex("PancakeSwap", "HpNfyc2Saw7RKkQd8nEL4khUcuPhQ7WwY1B2qjx8jxFq"),
	dex("Byreal", "REALQqNEomY6cQGZJUGwywTBD2UmDT32rZcNnfxQ5N2"),
	dex("DefiTuna", "fUSioN9YKKSa3CUC2YUc4tPkHJ5Y6XW1yz8y6F7qWz9"),
	dex("Stabble Stable Swap", "swapNyd8XiQwJ6ianp9snpu4brUqFxadzvHebnAXjJZ"),
	dex("Stabble Weighted Swap", "swapFpHZwjELNnjvThjajtiVmkz3yPQEHjLtka2fwHW"),
	dex("Saber", "SSwpkEEcbUqx4vtoEByFjSkhKdCT862DNVb52nZg1UZ"),
	dex("Mercurial", "MERLuDFBMmsHnsBPZw2sDQZHvXFMwp8EdjudcU2HKky"),
	dex("Stableswap", "ghosty4ZU1Qk1HN7Ymz4pZ15QfspzJZgSYFkdKN6ZLK"),
	dex("Perena", "NUMERUNsFCP3kuNmWZuXtm1AaQCPj9uw6Guv2Ekoi5P"),
	dex("Sanctum Infinity", "5ocnV1qiCgaQR8Jb8xWnVbApfaygJ8tNoZfgPwsgx9kx"),
	dex("FluxBeam", "FLUXubRmkEi2q6K3Y9kBPg9248ggaZVsoSFhtJHSrm1X"),
	dex("GooseFX Gamma", "GAMMA7meSFWaBXF25oSUgmGRwaW6sCMFLmBNiMSdbHVT"),
	dex("Saros", "SSwapUtytfBdBn1b9NUGG6foMVPtcWgpRU32HToDUZr"),
	dex("Invariant", "HyaB3W9q6XdA5xwpU4XnSZV94htfmbmqJXZcEbRaJutt"),
	dex("Crema", "CLMM9tUoggJu2wagPkkqs9eFG4BWhVBZWkP1qv3Sp7tR"),
	dex("Lifinity v2", "2wT8Yq49kHgDzXuPxZSaeLaH1qbmGXtEyPy64bL7aD3c"),
	dex("Woofi", "WooFif76YGRNjk1pA8wCsN67aQsD9f9iLsz4NcJ1AVb"),

	// Order books.
	dex("Phoenix", "PhoeNiXZ8ByJGLkxNfZRnkUfjvmuYqLR89jjFHGqdXY"),
	dex("OpenBook v2", "opnb2LAfJYbRMAHHvqjCwQxanZn7ReEHp1k81EohpZb"),
	dex("Manifest", "MNFSTqtC93rEfYHB6hF82sKdZpUDFWkViLByLd1k1Ms"),
	dex("Archer", "Archer8kgiavM61GyusMzaaS2ft5sALtNsD1HxkUPMhy"),
	dex("Deriverse", "DRVSpZ2YUYYKgZP8XtLhAGtT1zYSCKzeHfb4DgRnrgqD"),
	dex("1DEX", "DEXYosS6oEGvk8uCDayvwEZz4qEyDJRf9nFgYCaqPMTm"),

	// Proprietary market makers and RFQ.
	dex("HumidiFi", "9H6tua7jkLhdm3w8BvgpTn5LZNU7g4ZynDmCiNN3q6Rp"),
	dex("SolFi v2", "SV2EYYJyRz2YhfXwXnhNAevDEui5Q6yrfyo13WtupPF"),
	dex("SolFi", "SoLFiHG9TfgtdUXUjWAxi3LtvYuFyDLVhBWxdMZxyCe"),
	dex("Tessera V", "TessVdML9pBGgG9yGks7o4HewRaXVAMuoVj4x83GLQH"),
	dex("GoonFi v2", "goonuddtQRrWqqn5nFyczVKaie28f3kDkHWkHtURSLE"),
	dex("ZeroFi", "ZERor4xhbUycZ6gb9ntrhqscUcZmAbQDjEAtCf4hbZY"),
	dex("Obric v2", "obriQD1zbpyLz95G5n7nJe6a4DPjpFwa5XYPoNm113y"),
	dex("AlphaQ", "ALPHAQmeA7bjrVuccPsYPiCvsi428SNwte66Srvs4pHA"),
	dex("BisonFi", "BiSoNHVpsVZW2F7rx2eQ59yQwKxzU5NvBcmKshCSUypi"),
	dex("Aquifer", "AQU1FRd7papthgdrwPTTq5JacJh8YtwEXaBfKU3bTz45"),
	dex("Scorch", "SCoRcH8c2dpjvcJD6FiPbCSQyQgu3PcUAWj2Xxx3mqn"),
	dex("Hadron", "HADRoNbLovyqhCsocfYQYB7QdfCAAinN9HTePvBCVDQ8"),
	dex("Kipseli", "3TK9D8aoBFYjYZtKCjciPrVrRStsnvo7KmpcJqDavpaU"),
	dex("Obsidian", "HBVw6bZtcCaezhcBrmfyXBSBRWCdv72271xQ4GPvms2z"),
	dex("Riptide", "riptK81hDxhe5pW5jSzSM9iRA8azgEgLJ4dXkPtBS7j"),
	dex("Denali", "DNL1tgEj3nJovHw9jtyCCQD3arssCJzkmpDizknwzey4"),
	programSpec{Program{"Jupiter RFQ", jupiterOrderEngineID, KindDEX}, (*extractor).decodeJupiterRFQ},

	// Aggregators and order programs.
	programSpec{Program{"Jupiter", jupiterV6ProgramID, KindAggregator}, (*extractor).decodeSwapEvents},
	programSpec{Program{"DFlow", dFlowProgramID, KindAggregator}, (*extractor).decodeSwapEvents},
	aggregator("OKX DEX Router v2", "proVF4pMXVaYqmy4NjniPh4pqKNfMmsihgd4wdkCX3u"),
	aggregator("OKX DEX Router", "6m2CDdhRgxpH4WjvdzxAYbGxwdGUz5MziiL5jek2kBma"), // closed 2026-07
	aggregator("Titan", "T1TANpTeScyeqVzzgNViGDNrkQ6qHz9KrSBS4aNXvGT"),
	aggregator("Raydium Router", "routeUGWgWzqBWFcrCfv8tritsqukccJPu3q5GPP3xS"),
	// Keeper-filled order programs swap through an aggregator for the order owner.
	programSpec{Program: Program{"Jupiter Recurring", jupiterRecurringProgramID, KindAggregator}},
	aggregator("Jupiter DCA", "DCAK36VfExkPdAkYUQg6ewgxyinvcEyPLyHjRbmveKFw"), // retired
	programSpec{Program: Program{"Jupiter Trigger", jupiterTriggerProgramID, KindAggregator}},
	aggregator("Jupiter Limit Order", "jupoNjAxXgZ4rjzxzPMP4oxduvQsQtZzyknqvzYNrNu"),

	// Trading bots. Some only charge their fee here and call the DEX directly; see botFeeWallets.
	bot("Axiom", "FLASHX8DrLbgeR8FcfNV1F5krxYcYMUdBkrP1EPBtxB9"),
	bot("GMGN", "GMgnVFR8Jb39LoXsEVzb3DvBy3ywCmdmJquHUy1Lrkqb"),
	bot("Photon", "BSfD6SHZigAfDWSjzD5Q41jw8LmKwtmjskPH9XW1mrRW"),
	bot("Trojan", "troyXT7Ty3s2rjJe4bqWaroUrS4Fjd8rbHHNHxcACF4"),
	bot("Trojan", "DM2dmBUi2he1bot4xF7PaSGLorUjWLm2DMFsWiTPuXzb"), // retired 2025
	bot("BonkBot", "BBRouter1cVunVXvkcqeKkZQcBK7ruan37PPm3xzWaXD"),
	bot("BonkBot", "CxvksNjwhdHDLr3qbCXNKVdeYACW8cs93vFqLqtgyFE5"),
	bot("BonkBot", "AnCKmodpxycRZoAFgVwGjWuxP9wJPamghfvUw1fwKdpK"), // retired 2025
	bot("Banana Gun", "BANANAjs7FJiPQqJTGFzkZJndT9o7UmKiYYGaJz6frGu"),
	bot("Maestro", "MaestroAAe9ge5HTc64VbBQZ6fP77pwvrhM8i1XWSAx"),
	bot("Bloom", "b1oomGGqPKGD6errbyfbVMBuzSC8WtAAYo8MwNafWW1"),
	bot("MinTech", "minTcHYRLVPubRK8nt6sqe2ZpWrGDLQoNLipDJCGocY"),
	bot("Shuriken", "HEzM8TqVBvsgNKgqSbNkbL8J4Qq4DiRhz4NFXDKAKZiq"),
	bot("Nova", "NoVA1TmDUqksaj2hB1nayFkPysjJbFiU76dT4qPw2wm"), // retired 2026
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
		// Names may repeat (a bot's current and retired programs); IDs may not.
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
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.ID.String(), b.ID.String()))
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
