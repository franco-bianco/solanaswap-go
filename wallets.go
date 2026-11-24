package solanaswap

import "github.com/gagliardetto/solana-go"

// tipAccounts are the published tip accounts of block engines and landing services
// (November 2026). Payments to them are reported as FeeKindTip.
var tipAccounts = keySet(
	// Jito
	"96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5",
	"HFqU5x63VTqvQss8hp11i4wVV8bD44PvwucfZ2bU7gRe",
	"Cw8CFyM9FkoMi7K7Crf6HNQqf4uEMzpKw6QNghXLvLkY",
	"ADaUMid9yfUytqMBgopwjb2DTLSokTSzL1zt6iGPaS49",
	"DfXygSm4jCyNCybVYYK6DwvWqjKee8pbDmJGcLWNDXjh",
	"ADuUkR4vqLUMWXxW9gh6D6L8pMSawimctcNZ5pGwDcEt",
	"DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL",
	"3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKZ6jT",
	// Helius Sender
	"4ACfpUFoaSD9bfPdeu6DBt89gB6ENTeHBXCAi87NhDEE",
	"D2L6yPZ2FmmmTKPgzaMKdhu6EWZcTpLy1Vhx8uvZe7NZ",
	"9bnz4RShgq1hAnLnZbP8kbgBg1kEmcJBYQq3gQbmnSta",
	"5VY91ws6B2hMmBFRsXkoAAdsPHBJwRfBht4DXox3xkwn",
	"2nyhqdwKcJZR2vcqCyrYsaPVdAnFoJjiksCXJ7hfEYgD",
	"2q5pghRs6arqVjRvT5gfgWfWcHWmw1ZuCzphgd5KfWGJ",
	"wyvPkWjVZz1M8fHQnMMCDTQDbkManefNNhweYk5WkcF",
	"3KCKozbAaF75qEU33jtzozcJ29yJuaLJTy2jFdzUY8bT",
	"4vieeGHPYPG2MmyPRcYjdiDmmhN3ww7hsFNap8pVN3Ey",
	"4TQLFNWK8AovT1gFvda5jfw2oJeRMKEmw7aH6MGBJ3or",
	// Nozomi (Temporal)
	"nEFs3jph8HJt7honu3k7XtGUufMnwAvSXmXcKSPxryP",
	"TEMPaMeCRFAS9EKF53Jd6KpHxgL47uWLcpFArU1Fanq",
	"noz3jAjPiHuBPqiSPkkugaJDkJscPuRhYnSpbi8UvC4",
	"noz3str9KXfpKknefHji8L1mPgimezaiUyCHYMDv1GE",
	"noz6uoYCDijhu1V7cutCpwxNiSovEwLdRHPwmgCGDNo",
	"noz9EPNcT7WH6Sou3sr3GGjHQYVkN3DNirpbvDkv9YJ",
	"nozc5yT15LazbLTFVZzoNZCwjh3yUtW86LoUyqsBu4L",
	"nozFrhfnNGoyqwVuwPAW4aaGqempx4PU6g6D9CJMv7Z",
	"nozievPk7HyK1Rqy1MPJwVQ7qQg2QoJGyP71oeDwbsu",
	"noznbgwYnBLDHu8wcQVCEw6kDrXkPdKkydGJGNXGvL7",
	"nozNVWs5N8mgzuD3qigrCG2UoKxZttxzZ85pvAQVrbP",
	"nozpEGbwx4BcGp6pvEdAh1JoC2CQGZdU6HbNP1v2p6P",
	"nozrhjhkCr3zXT3BiT4WCodYCUFeQvcdUkM7MqhKqge",
	"nozrwQtWhEdrA6W8dkbt9gnUaMs52PdAv5byipnadq3",
	"nozUacTVWub3cL4mJmGCYjKZTnE9RbdY5AP46iQgbPJ",
	"nozWCyTPppJjRuw2fpzDhhWbW355fzosWSzrrMYB1Qk",
	"nozWNju6dY353eMkMqURqwQEoM3SFgEKC6psLCSfUne",
	"nozxNBgWohjR75vdspfxR5H9ceC7XXH99xpxhVGt3Bb",
	// bloXroute
	"3UQUKjhMKaY2S6bjcQD6yHB7utcZt5bfarRCmctpRtUd",
	"FogxVNs6Mm2w9rnGL1vkARSwJxvLE8mujTv3LK8RnUhF",
	"bLx7MvxGaKdKL7mEbpk9tC79z6MnBSJoJkuaEAPu6Nd",
	"bLx7XBqSg3LUPVf1bRgCnkJmgVZR8QEgDJBPqcRLHvp",
	"bLx8KeZxinPwy6kkUgyzMLeqb2ARNsWjADG1dhSsVba",
	"bLxADBknoNj8WAGw2W6GBYeq848Xx6ajhaymV1YvrHm",
	"bLxAc88vRBwvcUQJEgcxNfBLvHPikY4csNsUmPeWea2",
	"bLxQ88oCiTsL8Xj4YWekKi1hjrgmbE3J3FFZ2xZHR3h",
	"bLxS7NoLuynNRJ4mCnEE2YbtwJFttYsEyp2ME7rp2yt",
	"bLxW6mCov7VEbrKc3S9tcBRcfSzRnLCbNp3Dfn3SJG5",
	"bLxXSGXs4mYPTC5okZXed1qzvjNwNJ48QJ82hT2V7w7",
	"bLxYi3vojbbB7hVzVDVTdBLVPhp7GJ3ZB3BwdK5sFXi",
	"bLxhLPgBXtUpX4b1bH3HatuMGMSKT9GnwtuCGiMSAqe",
	"bLxpY1mniuFW4PgkNA4JiNxoeKHFszryi6tNgyZAiAA",
	"bLxuETxd2tgWxBALNwPzAfHhsik4BzD3nrEBCiPNZQD",
	"bLxuL2gK5FW7xfahvwLrxLyW76vcCpNsKQY2CmnE6kV",
	"bLxv4Hnub7nDJWHs8s17o9bGU65Bnx6Yqp2fqtMgHmm",
	// 0slot
	"6fQaVhYZA4w3MBSXjJ81Vf6W1EDYeUPXpgVQ6UQyU1Av",
	"DiTmWENJsHQdawVUUKnUXkconcpW4Jv52TnMWhkncF6t",
	"HRyRhQ86t3H4aAtgvHVpUJmw64BDrb61gRiKcdKUXs5c",
	"Eb2KpSC8uMt9GmzyAEm5Eb1AAAgTjRaXWFjKyFXHZxF3",
	"FCjUJZ1qozm1e8romw216qyfQMaaWKxWsuySnumVCCNe",
	"7y4whZmw388w1ggjToDLSBLv47drw5SUXcLk6jtmwixd",
	"J9BMEWFbCBEjtQ1fG5Lo9kouX1HfrKQxeUxetwXrifBw",
	"8U1JPQh3mVQ4F5jwRdFTBzvNRQaYFQppHQYoH38DJGSQ",
	"ENxTEjSQ1YabmUpXAdCgevnHQ9MHdLv8tzFiuiYJqa13",
	"6rYLG55Q9RpsPGvqdPNJs4z5WTxJVatMB8zV3WJhs5EK",
	"Cix2bHfqPcKcM233mzxbLk14kSggUUiz2A87fJtGivXr",
	"4HiwLEP2Bzqj3hM2ENxJuzhcPCdsafwiet3oGkMkuQY4",
	"7toBU3inhmrARGngC7z6SjyP85HgGMmCTEwGNRAcYnEK",
	"8mR3wB1nh4D6J9RUCugxUpc6ya8w38LPxZ3ZjcBhgzws",
	"6SiVU5WEwqfFapRuYCndomztEwDjvS5xgtEof3PLEGm9",
	"TpdxgNJBWZRL8UXF5mrEsyWxDWx9HQexA9P1eTWQ42p",
	"D8f3WkQu6dCF33cZxuAsrKHrGsqGP2yvAHf8mX6RXnwf",
	"GQPFicsy3P3NXxB5piJohoxACqTvWE9fKpLgdsMduoHE",
	"Ey2JEr8hDkgN8qKJGrLf2yFjRhW7rab99HVxwi5rcvJE",
	"4iUgjMT8q2hNZnLuhpqZ1QtiV8deFPy2ajvvjEpKKgsS",
	"3Rz8uD83QsU8wKvZbgWAPvCNDU6Fy8TSZTMcPm3RB6zt",
	// NextBlock
	"NextbLoCkVtMGcV47JzewQdvBpLqT9TxQFozQkN98pE",
	"NexTbLoCkWykbLuB1NkjXgFWkX9oAtcoagQegygXXA2",
	"NeXTBLoCKs9F1y5PJS9CKrFNNLU1keHW71rfh7KgA1X",
	"NexTBLockJYZ7QD7p2byrUa6df8ndV2WSd8GkbWqfbb",
	"neXtBLock1LeC67jYd1QdAa32kbVeubsfPNTJC1V5At",
	"nEXTBLockYgngeRmRrjDV31mGSekVPqZoMGhQEZtPVG",
	"NEXTbLoCkB51HpLBLojQfpyVAMorm3zzKg7w9NFdqid",
	"nextBLoCkPMgmG8ZgJtABeScP35qLa2AMCNKntAP7Xc",
	// Astralane
	"astra4uejePWneqNaJKuFFA8oonqCE1sqF6b45kDMZm",
	// Solana Vibe Station
	"svsMoWJBwLcs8JgfN8VaF111tAc199KpYeTKRwxRtip",
	// GMGN
	"gmgnfGwFvnqZm4MtaunuVjErQNHdAC7H4wZYjBQkvKW",
)

// botFeeWallets names the fee wallets of trading bots and wallet apps. Bots that call
// DEX programs straight from the user's wallet are only recognisable by these payments.
var botFeeWallets = map[solana.PublicKey]string{
	solana.MustPublicKeyFromBase58("AVUCZyuT35YSuj4RH7fwiyPu82Djn2Hfg7y2ND2XcnZH"): "Photon",
	solana.MustPublicKeyFromBase58("9yMwSPk9mrXSN7yDHUuZurAh1sjbJsfpUqjZ7SvVtdco"): "Trojan",
	solana.MustPublicKeyFromBase58("65gDv7pZQCZELsNpNYSFEBtNFpWZAbxmRFB6BGMqFkHH"): "Trojan",
	solana.MustPublicKeyFromBase58("ZG98FUCjb8mJ824Gbs6RsgVmr1FhXb2oNiJHa2dwmPd"):  "BonkBot",
	solana.MustPublicKeyFromBase58("MaestroUL88UBnZr3wfoN7hqmNWFi3ZYCGqZoJJHE36"):  "Maestro",
	solana.MustPublicKeyFromBase58("7HeD6sLLqAnKVRuSfc1Ko3BSPMNKWgGTiWLKXJF31vKM"): "Bloom",
	solana.MustPublicKeyFromBase58("3spK1TmrAnFUDRN2bErDAVw225tLE7uFpQeP9WNXP2nY"): "Banana Gun",
	solana.MustPublicKeyFromBase58("47hEzz83VFR23rLTEeVm9A7eFzjJwjvdupPPmX3cePqF"): "Banana Gun",
	solana.MustPublicKeyFromBase58("5BqYhuD4q1YD3DMAYkc1FeTu9vqQVYYdfBAmkZjamyZg"): "Axiom",
	solana.MustPublicKeyFromBase58("BfFX9rUm8qTZiZjmeq9BktWVTNuG3YWMc5AvkrCKJike"): "Axiom",
	solana.MustPublicKeyFromBase58("2ApLdwLrGayEmxgpLX9BTR47Q2QprfMg5SpjrLeaK8s7"): "Axiom",
	solana.MustPublicKeyFromBase58("dBhdrmwBkRa66XxBuAK4WZeZnsZ6bHeHCCLXa3a8bTJ"):  "GMGN",
	solana.MustPublicKeyFromBase58("9yj3zvLS3fDMqi1F8zhkaWfq8TZpZWHe6cz1Sgt7djXf"): "Phantom",
	solana.MustPublicKeyFromBase58("R4rNJHaffSUotNmqSKNEfDcJE8A7zJUkaoM5Jkd7cYX"):  "Fomo",
}

func keySet(addrs ...string) map[solana.PublicKey]bool {
	m := make(map[solana.PublicKey]bool, len(addrs))
	for _, a := range addrs {
		m[solana.MustPublicKeyFromBase58(a)] = true
	}
	return m
}
