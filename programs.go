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
	MoonshotProgramID         = solana.MustPublicKeyFromBase58("MoonCVVNZFSYkqNXP6bxHLPL6QQJiMagDL3qcqUQTrG")
	RaydiumLaunchLabProgramID = solana.MustPublicKeyFromBase58("LanMV9sAd7wArD4vJFi2qDdfnVhFxYSUg6eADduJ3uj")
	JupiterV6ProgramID        = solana.MustPublicKeyFromBase58("JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4")
	JupiterOrderEngineID      = solana.MustPublicKeyFromBase58("61DFfeTKM7trxYcPQCM78bJ794ddZprZpAwAnLiwTpYH")
	JupiterRecurringProgramID = solana.MustPublicKeyFromBase58("DCA265Vj8a9CEuX1eb1LWRnDT7uK6q1xMipnNyatn23M")
	DFlowProgramID            = solana.MustPublicKeyFromBase58("DF1ow4tspfHX9JwWJsAb9epbkA8hmpSEAtxXy1V27QBH")
)

// registry lists the recognised programs. Program IDs were verified on
// chain (executable, with recent swap activity) in November 2026; retired
// programs are kept so historical transactions stay labelled.
var registry = newRegistry(
	// Launchpads and bonding curves.
	programSpec{Program{"Pump.fun", PumpFunProgramID, KindDEX}, decodePumpFun},
	programSpec{Program{"PumpSwap", PumpSwapProgramID, KindDEX}, decodePumpSwap},
	programSpec{Program{"Moonshot", MoonshotProgramID, KindDEX}, decodeMoonshot},
	programSpec{Program{"Raydium LaunchLab", RaydiumLaunchLabProgramID, KindDEX}, nil},
	dex("Meteora DBC", "dbcij3LWUppWqq96dh6gJWwBifmcGfLSB5D4DuSMaqN"),
	dex("Boop.fun", "boop8hVGQGqehUK2iVEMEnMrL5RbjywRzHKBmBE7ry4"),
	dex("Heaven", "HEAVENoP2qxoeuF8Dj2oT1GHEnu49U5mJYkdeC8BAX2o"),
	dex("Vertigo", "vrTGoBuy5rYSxAfV3jaRJWHH6nN9WK4NRExGxsk1bCJ"),

	// AMMs, CLMMs, order books and proprietary market makers.
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
	programSpec{Program{"Jupiter RFQ", JupiterOrderEngineID, KindDEX}, decodeJupiterRFQ},

	// Aggregators and order programs.
	programSpec{Program{"Jupiter", JupiterV6ProgramID, KindAggregator}, decodeSwapEvents},
	programSpec{Program{"DFlow", DFlowProgramID, KindAggregator}, decodeSwapEvents},
	aggregator("OKX DEX Router v2", "proVF4pMXVaYqmy4NjniPh4pqKNfMmsihgd4wdkCX3u"),
	aggregator("OKX DEX Router", "6m2CDdhRgxpH4WjvdzxAYbGxwdGUz5MziiL5jek2kBma"), // closed 2026-07
	aggregator("Titan", "T1TANpTeScyeqVzzgNViGDNrkQ6qHz9KrSBS4aNXvGT"),
	aggregator("Raydium Router", "routeUGWgWzqBWFcrCfv8tritsqukccJPu3q5GPP3xS"),
	orderProgram("Jupiter Recurring", "DCA265Vj8a9CEuX1eb1LWRnDT7uK6q1xMipnNyatn23M"),
	orderProgram("Jupiter DCA", "DCAK36VfExkPdAkYUQg6ewgxyinvcEyPLyHjRbmveKFw"), // retired
	programSpec{Program: Program{"Jupiter Trigger", JupiterTriggerProgramID, KindAggregator}},
	orderProgram("Jupiter Limit Order", "jupoNjAxXgZ4rjzxzPMP4oxduvQsQtZzyknqvzYNrNu"),

	// Trading bots. Several bots only charge their fee through these
	// programs and call the DEX directly; see also botFeeWallets.
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

// orderProgram registers a keeper-filled order program (DCA, limit
// orders). Its fills swap through an aggregator on the owner's behalf.
func orderProgram(name, id string) programSpec {
	return programSpec{Program: Program{name, solana.MustPublicKeyFromBase58(id), KindAggregator}}
}

func bot(name, id string) programSpec {
	return programSpec{Program: Program{name, solana.MustPublicKeyFromBase58(id), KindBot}}
}

func newRegistry(specs ...programSpec) map[solana.PublicKey]programSpec {
	m := make(map[solana.PublicKey]programSpec, len(specs))
	for _, s := range specs {
		// Names may repeat (a bot's current and retired programs).
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
