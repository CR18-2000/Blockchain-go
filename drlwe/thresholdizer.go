package drlwe

import (
	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/rlwe"
	"github.com/ldsec/lattigo/v2/utils"
)

// ThreshPublicKey is a type for Shamir public keys in a thresholdizer.
type ThreshPublicKey struct {
	*ring.Poly
}

// ThreshSecretKey is a type for a Shamir Secret Key in a thresholdizer.
// type ThreshSecretKey struct {
// 	*ring.Poly
// }

// ThreshSecretShare is a type for a share of a secret key in a thresholdizer.
type ThreshSecretShare struct {
	*ring.Poly
}

// PartyID is a type for a Party's identifier, in order to avoid having to broad-
// cast polynomials
type PartyID struct {
	String string
}

// ShareGenPoly is a type for a share-generating polynomial.
type ShareGenPoly struct {
	coeffs []*ring.Poly
}

// ThresholdizerProtocol is an interface describing the local steps of a generic
// DRLWE thresholdizer.
type ThresholdizerProtocol interface {
	GenKeyFromID(id PartyID) *ThreshPublicKey

	AllocateShareGenPoly() *ShareGenPoly
	InitShareGenPoly(gen *ShareGenPoly, sk *rlwe.SecretKey, threshold uint64)

	AllocateSecretShare() *ThreshSecretShare
	GenShareForParty(gen *ShareGenPoly, party_key *ThreshPublicKey, share_out *ThreshSecretShare)

	GenThreshSecretKey(aggregate *ThreshSecretShare, tsk *rlwe.SecretKey)

	AggregateShares(share1, share2, outShare *ThreshSecretShare)
}

// CombinerProtocol is an interface describing the local steps of a generic
// DRLWE combiner
type CombinerProtocol interface {
	GenFinalShare(active_pks []*ThreshPublicKey, tpk *ThreshPublicKey, tsk, out_key *rlwe.SecretKey)
	Equal(tpk1, tpk2 *ThreshPublicKey) bool
}

//--------------------------------THRESHOLDIZING--------------------------------

//Thresholdizer is the structure containing the parameters for a thresholdizer.
type Thresholdizer struct {
	ringQP    *ring.Ring
	samplerQP *ring.UniformSampler
}

// NewThresholdizer creates a new Thresholdizer instance from parameters.
func NewThresholdizer(params rlwe.Parameters) *Thresholdizer {

	thresholdizer := new(Thresholdizer)
	var err error
	thresholdizer.ringQP = params.RingQP()

	prng, err := utils.NewPRNG()
	if err != nil {
		panic("Error in Thresholdizer initalization : error in PRNG generation")
	}

	thresholdizer.samplerQP = ring.NewUniformSampler(prng, thresholdizer.ringQP)

	return thresholdizer
}

// GenKeyFromID generates a threshold public key from an id. Useful to avoid
// having to broadcast polynomials during the setup.
func (thresholdizer *Thresholdizer) GenKeyFromID(id PartyID) *ThreshPublicKey {

	idPRNG, err := utils.NewKeyedPRNG([]byte(id.String))

	if err != nil {
		panic(err)
	}
	gen := ring.NewUniformSampler(idPRNG, thresholdizer.ringQP)
	return &ThreshPublicKey{gen.ReadNew()}
}

// AllocateShareGenPoly allocates a ShareGenPoly, which is used to generate shares.
func (thresholdizer *Thresholdizer) AllocateShareGenPoly() *ShareGenPoly {
	return new(ShareGenPoly)
}

// InitShareGenPoly initiates a ShareGenPoly by sampling a random polynomial of
// degree threshold-1 with a constant term equal to the given secret key's value.
func (thresholdizer *Thresholdizer) InitShareGenPoly(gen *ShareGenPoly, sk *rlwe.SecretKey, threshold uint64) {
	gen.coeffs = make([]*ring.Poly, int(threshold))

	gen.coeffs[0] = sk.Value.CopyNew()

	for i := uint64(1); i < threshold; i++ {
		gen.coeffs[i] = thresholdizer.samplerQP.ReadNew()
	}
	return
}

// GenShareForParty generates a secret share for a given threshold public key.
// Stores the result in share_out. This result should be sent to the given
// threshold public key's owner.
func (thresholdizer *Thresholdizer) GenShareForParty(gen *ShareGenPoly, party_key *ThreshPublicKey, share_out *ThreshSecretShare) {
	thresholdizer.ringQP.EvalPolMontgomeryNTT(gen.coeffs, party_key.Poly, share_out.Poly)
}

// AllocateSecretShare allocates a Threshold secret share.
func (thresholdizer *Thresholdizer) AllocateSecretShare() *ThreshSecretShare {
	return &ThreshSecretShare{thresholdizer.ringQP.NewPoly()}
}

// AllocateSecretKey allocates a threshold secret key.
//func (thresholdizer *Thresholdizer) AllocateSecretKey() *ThreshSecretKey {
//	return &ThreshSecretKey{thresholdizer.ringQP.NewPoly()}
//}

// GenThreshSecretKey generates a threshold secret key from an aggregate of secret
// shares. This secret key must be stored until the combining phase is completed.
func (thresholdizer *Thresholdizer) GenThreshSecretKey(aggregate *ThreshSecretShare, tsk *rlwe.SecretKey) {
	thresholdizer.ringQP.Copy(aggregate.Poly, tsk.Value)

}

// AggregateShares aggregates two secret shares(by adding them), and stores them
// in outShare.
func (thresholdizer *Thresholdizer) AggregateShares(share1, share2, outShare *ThreshSecretShare) {
	thresholdizer.ringQP.Add(share1.Poly, share2.Poly, outShare.Poly)
}

//-------------------------------COMBINING -------------------------------------

// Combiner is a structure that holds the parameters for the combining phase of
// a threshold secret sharing protocol.
type Combiner struct {
	ringQP    *ring.Ring
	threshold uint64
}

//NewCombiner creates a new Combiner.
func NewCombiner(params rlwe.Parameters, threshold uint64) *Combiner {
	combiner := new(Combiner)
	combiner.ringQP = params.RingQP()

	return combiner
}

// GenFinalShare generates an additive share of a cohort's secret key from a slice con-
// taining all active player's threshold public keys and a party's public and
// secret keys. Stores the result in out_key.
func (combiner *Combiner) GenFinalShare(active_pks []*ThreshPublicKey, tpk *ThreshPublicKey, tsk, out_key *rlwe.SecretKey) {

	if uint64(len(active_pks)) < combiner.threshold {
		panic("Not enough active players to combine threshold shares.")
	}

	r := combiner.ringQP
	keyDiff := combiner.ringQP.NewPoly()
	r.Copy(tsk.Value, out_key.Value)
	for _, key := range active_pks {
		//Lagrange Interpolation with the public threshold key of other active players
		if !combiner.Equal(key, tpk) {
			r.MulCoeffsMontgomeryConstant(out_key.Value, key.Poly, out_key.Value)
			combiner.ringQP.SubNoMod(key.Poly, tpk.Poly, keyDiff)
			combiner.ringQP.InvMultPolyMontgomeryNTT(keyDiff, keyDiff)
			r.MulCoeffsMontgomeryConstant(out_key.Value, keyDiff, out_key.Value)
		}
	}

	r.Reduce(out_key.Value, out_key.Value)
}

//Equal compares two ThreshPublicKey for equality.
func (combiner *Combiner) Equal(tpk1, tpk2 *ThreshPublicKey) bool {
	return combiner.ringQP.Equal(tpk1.Poly, tpk2.Poly)
}

//--------------------------------Combiner with cache---------------------------

// Combiner is a structure that holds the parameters for the combining phase of
// a threshold secret sharing protocol, augmented with a stateful cache.
type CombinerCache struct {
	*Combiner
	inverses map[*ThreshPublicKey]*ring.Poly
}

// NewCombinerCache creates a new combiner with cache from parameters.
func NewCombinerCache(combiner *Combiner, tpk *ThreshPublicKey, pks []*ThreshPublicKey) *CombinerCache {
	combinercache := new(CombinerCache)
	combinercache.inverses = make(map[*ThreshPublicKey]*ring.Poly)
	combinercache.Combiner = combiner
	combinercache.CacheInverses(tpk, pks)

	return combinercache
}

// CacheInverses caches the inverses of the differences between tpk and each of
// pks (as needed for Lagrange interpolation)
func (combiner *CombinerCache) CacheInverses(tpk *ThreshPublicKey, pks []*ThreshPublicKey) {
	for _, key := range pks {
		if !combiner.Equal(tpk, key) {
			tmp := combiner.ringQP.NewPoly()
			combiner.getDiffInverse(tpk, key, tmp)
		}
	}
}

// getDiffInverse computes the difference between the two given keys and stores
// its multiplicative inverse in pol_out, caching it as well.
func (combiner *CombinerCache) getDiffInverse(this_key *ThreshPublicKey, that_key *ThreshPublicKey, pol_out *ring.Poly) {

	inv, found := combiner.inverses[that_key]
	if found {
		//Inverse is in the cache
		combiner.ringQP.Copy(inv, pol_out)
	} else {
		//Inverse not in the cache, we have to compute it
		keyDiff := combiner.ringQP.NewPoly()
		combiner.ringQP.SubNoMod(that_key.Poly, this_key.Poly, keyDiff)
		if !combiner.ringQP.IsInvertible(keyDiff) {
			panic("Error : keys yield a non-invertible difference")
		}
		combiner.ringQP.InvMultPolyMontgomeryNTT(keyDiff, pol_out)

		//Cache the result.
		combiner.inverses[that_key] = combiner.ringQP.NewPoly()
		combiner.ringQP.Copy(pol_out, combiner.inverses[that_key])
	}
}

// GenFinalShare generates an additive share of a cohort's secret key from the values
// in the cache and a party's secret keys. Assumes the inverse corresponding
// to every active's party is in the cache. Stores the result in out_key.
func (combiner *CombinerCache) GenFinalShare(tsk, out_key *rlwe.SecretKey) {

	r := combiner.ringQP
	r.Copy(tsk.Value, out_key.Value)
	for key, inv := range combiner.inverses {
		//Lagrange Interpolation with the threshold public key of other active players
		r.MulCoeffsMontgomeryConstant(out_key.Value, key.Poly, out_key.Value)
		r.MulCoeffsMontgomeryConstant(out_key.Value, inv, out_key.Value)
	}

	r.Reduce(out_key.Value, out_key.Value)
}

// ClearCache replaces the cache of a combiner by an empty one.
func (combiner *CombinerCache) ClearCache() {
	combiner.inverses = make(map[*ThreshPublicKey]*ring.Poly)
}
