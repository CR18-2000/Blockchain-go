package rlwe

import (
	"github.com/tuneinsight/lattigo/v3/ring"
)

// PublicKeyIsCorrect returns true if pk is a correct RLWE public-key for secret-key sk and parameters params.
func PublicKeyIsCorrect(pk *PublicKey, sk *SecretKey, params Parameters, log2Bound int) bool {

	pk = pk.CopyNew()
	ringQ, ringP, ringQP := params.RingQ(), params.RingP(), params.RingQP()
	levelQ, levelP := params.QCount()-1, params.PCount()-1

	// [-as + e] + [as]
	ringQP.MulCoeffsMontgomeryAndAddLvl(levelQ, levelP, sk.Value, pk.Value[1], pk.Value[0])
	ringQP.InvNTTLvl(levelQ, levelP, pk.Value[0], pk.Value[0])
	ringQP.InvMFormLvl(levelQ, levelP, pk.Value[0], pk.Value[0])

	if log2Bound <= ringQ.Log2OfInnerSum(pk.Value[0].Q.Level(), pk.Value[0].Q) {
		return false
	}

	if ringP != nil && log2Bound <= ringP.Log2OfInnerSum(pk.Value[0].P.Level(), pk.Value[0].P) {
		return false
	}

	return true
}

// RotationKeyIsCorrect returns true if swk is a correct RLWE switching-key for galois element galEl, secret-key sk and parameters params.
func RotationKeyIsCorrect(swk *SwitchingKey, galEl uint64, skIdeal *SecretKey, params Parameters, log2Bound int) bool {
	swk = swk.CopyNew()
	skIn := skIdeal.CopyNew()
	skOut := skIdeal.CopyNew()
	galElInv := ring.ModExp(galEl, uint64(4*params.N()-1), uint64(4*params.N()))
	ringQ, ringP := params.RingQ(), params.RingP()

	ringQ.PermuteNTT(skIdeal.Value.Q, galElInv, skOut.Value.Q)
	if ringP != nil {
		ringP.PermuteNTT(skIdeal.Value.P, galElInv, skOut.Value.P)
	}

	return SwitchingKeyIsCorrect(swk, skIn, skOut, params, log2Bound)
}

// SwitchingKeyIsCorrect returns true if swk is a correct RLWE switching-key for input key skIn, output key skOut and parameters params.
func SwitchingKeyIsCorrect(swk *SwitchingKey, skIn, skOut *SecretKey, params Parameters, log2Bound int) bool {
	swk = swk.CopyNew()
	skIn = skIn.CopyNew()
	skOut = skOut.CopyNew()
	levelQ, levelP := params.QCount()-1, params.PCount()-1
	ringQ, ringP, ringQP := params.RingQ(), params.RingP(), params.RingQP()
	decompPw2 := params.DecompPw2(levelQ, levelP)

	// Decrypts
	// [-asIn + w*P*sOut + e, a] + [asIn]
	for i := range swk.Value {
		for j := range swk.Value[i] {
			ringQP.MulCoeffsMontgomeryAndAddLvl(levelQ, levelP, swk.Value[i][j].Value[1], skOut.Value, swk.Value[i][j].Value[0])
		}
	}

	// Sums all basis together (equivalent to multiplying with CRT decomposition of 1)
	// sum([1]_w * [RNS*PW2*P*sOut + e]) = PWw*P*sOut + sum(e)
	for i := range swk.Value { // RNS decomp
		if i > 0 {
			for j := range swk.Value[i] { // PW2 decomp
				ringQP.AddLvl(levelQ, levelP, swk.Value[0][j].Value[0], swk.Value[i][j].Value[0], swk.Value[0][j].Value[0])
			}
		}
	}

	if levelP != -1 {
		// sOut * P
		ringQ.MulScalarBigint(skIn.Value.Q, ringP.ModulusAtLevel[levelP], skIn.Value.Q)
	}

	for i := 0; i < decompPw2; i++ {

		// P*s^i + sum(e) - P*s^i = sum(e)
		ringQ.Sub(swk.Value[0][i].Value[0].Q, skIn.Value.Q, swk.Value[0][i].Value[0].Q)

		// Checks that the error is below the bound
		// Worst error bound is N * floor(6*sigma) * #Keys
		ringQP.InvNTTLvl(levelQ, levelP, swk.Value[0][i].Value[0], swk.Value[0][i].Value[0])
		ringQP.InvMFormLvl(levelQ, levelP, swk.Value[0][i].Value[0], swk.Value[0][i].Value[0])

		// Worst bound of inner sum
		// N*#Keys*(N * #Parties * floor(sigma*6) + #Parties * floor(sigma*6) + N * #Parties  +  #Parties * floor(6*sigma))

		if log2Bound < ringQ.Log2OfInnerSum(levelQ, swk.Value[0][i].Value[0].Q) {
			return false
		}

		if levelP != -1 {
			if log2Bound < ringP.Log2OfInnerSum(levelP, swk.Value[0][i].Value[0].P) {
				return false
			}
		}

		// sOut * P * PW2
		ringQ.MulScalar(skIn.Value.Q, 1<<params.Pow2Base(), skIn.Value.Q)
	}

	return true
}