package dckks

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ldsec/lattigo/v2/ckks"
	"github.com/ldsec/lattigo/v2/drlwe"
	"github.com/ldsec/lattigo/v2/rlwe"
	"github.com/stretchr/testify/require"
)

var threshold uint64

func Test_DCKKS_ThresholdProtocol(t *testing.T) {

	var defaultParams = ckks.DefaultParams[:4] // the default test runs for ring degree N=2^12, 2^13, 2^14, 2^15
	if testing.Short() {
		defaultParams = ckks.DefaultParams[:2] // the short test runs for ring degree N=2^12, 2^13
	}
	if *flagLongTest {
		defaultParams = append(ckks.DefaultParams, ckks.DefaultBootstrapSchemeParams...) // the long test suite runs for all default parameters
	}
	if *flagParamString != "" {
		var jsonParams ckks.ParametersLiteral
		json.Unmarshal([]byte(*flagParamString), &jsonParams)
		defaultParams = []ckks.ParametersLiteral{jsonParams} // the custom test suite reads the parameters from the -params flag
	}

	parties = 8
	threshold = parties / 2
	for _, p := range defaultParams {
		params, err := ckks.NewParametersFromLiteral(p)
		if err != nil {
			panic(err)
		}

		var testCtx *testContext
		if testCtx, err = genTestParams(params); err != nil {
			panic(err)
		}

		testKeyGen(testCtx, t)
		testThreshold(testCtx, t)
	}
}

//Test if the shares generated by the combining phase yield a correct key switch.
func testThreshold(testCtx *testContext, t *testing.T) {
	sk0Shards := testCtx.sk0Shards
	//decryptorSk0 := testCtx.decryptorSk0

	t.Run(testString("Threshold/", parties, testCtx.params)+fmt.Sprintf("/threshold=%d", threshold), func(t *testing.T) {

		type Party struct {
			*Thresholdizer
			*Combiner
			*CombinerCache
			id        drlwe.PartyID
			gen       *drlwe.ShareGenPoly
			sk        *rlwe.SecretKey
			tsk       *rlwe.SecretKey
			pcksShare *drlwe.PCKSShare
			sk_t 			*rlwe.SecretKey
		}

		pcksPhase := func(params ckks.Parameters, tpk *rlwe.PublicKey, ct *ckks.Ciphertext, P []*Party) (encOut *ckks.Ciphertext) {

			// Collective key switching from the collective secret key to
			// the target public key

			pcks := NewPCKSProtocol(params, 3.19)

			for _, pi := range P {
				pi.pcksShare = pcks.AllocateShares(ct.Level())
			}

			for _, pi := range P {
				pcks.GenShare(pi.sk_t, tpk, ct, pi.pcksShare)
			}

			pcksCombined := pcks.AllocateShares(ct.Level())
			encOut = ckks.NewCiphertext(testCtx.params, 1, ct.Level(), ct.Scale())
			for _, pi := range P {
				pcks.AggregateShares(pi.pcksShare, pcksCombined, pcksCombined)
			}
			pcks.KeySwitch(pcksCombined, ct, encOut)

			return

		}

		P := make([]*Party, parties)
		for i := uint64(0); i < parties; i++ {
			p := new(Party)
			p.sk = sk0Shards[i]
			p.sk_t = ckks.NewSecretKey(testCtx.params)
			p.Thresholdizer = NewThresholdizer(testCtx.params)
			p.gen = p.Thresholdizer.AllocateShareGenPoly()
			p.Thresholdizer.InitShareGenPoly(p.gen, p.sk, threshold)
			p.Combiner = NewCombiner(testCtx.params, threshold)
			p.CombinerCache = NewCombinerCache(p.Combiner, nil, nil)
			P[i] = p
		}

		//Array of all party IDs
		ids := make([]drlwe.PartyID, parties)
		for i := uint64(0); i < parties; i++ {
			pid := drlwe.PartyID{fmt.Sprintf("Party %d", i)}
			ids[i] = pid
			P[i].id = ids[i]
		}

		// Checks that dckks types complies to the corresponding drlwe interfaces
		var _ drlwe.ThresholdizerProtocol = P[0].Thresholdizer
		var _ drlwe.CombinerProtocol = P[0].Combiner

		polynomial_shares := make([]map[drlwe.PartyID]*drlwe.ThreshSecretShare, parties)
		for i, pi := range P {
			// Every party generates a share for every other party
			polynomial_shares[i] = make(map[drlwe.PartyID]*drlwe.ThreshSecretShare)

			for _, id := range ids {

				share := pi.Thresholdizer.AllocateSecretShare()
				pi.Thresholdizer.GenShareForParty(pi.gen, pi.Thresholdizer.GenKeyFromID(id), share)
				polynomial_shares[i][id] = share
			}
		}

		//Each party aggregates what it has received into a secret key
		for _, pi := range P {
			tmp_share := new(drlwe.ThreshSecretShare)
			tmp_share.Poly = testCtx.dckksContext.ringQP.NewPoly()
			for j := 0; j < len(P); j++ {
				pi.Thresholdizer.AggregateShares(tmp_share, polynomial_shares[j][pi.id], tmp_share)
			}
			pi.tsk = ckks.NewSecretKey(testCtx.params)
			pi.Thresholdizer.GenThreshSecretKey(tmp_share, pi.tsk)
		}

		// Determining which parties are active. In a distributed context, a party
		// would receive the ids of active players and retrieve (or compute) the corresponding keys.
		P_active := P[:threshold]
		P_active_keys := make([]*drlwe.ThreshPublicKey, threshold)
		for i, p := range P_active {
			P_active_keys[i] = P[0].GenKeyFromID(p.id)
		}

		// Combining
		// Slow because each party has to generate its public key on-the-fly. In
		// practice the public key could be precomputed from an id by parties during setup
		for _, pi := range P_active {
			pi.Combiner.GenFinalShare(P_active_keys, pi.Thresholdizer.GenKeyFromID(pi.id), pi.tsk, pi.sk_t)
			temp_tsk_nocache := pi.sk_t.Value.CopyNew()
			pi.CombinerCache.CacheInverses(pi.Thresholdizer.GenKeyFromID(pi.id), P_active_keys)
			pi.CombinerCache.GenFinalShare(pi.tsk, pi.sk_t)
			//the cached and non-cached combiners should yield the same results
			require.True(t, testCtx.dckksContext.ringQP.Equal(temp_tsk_nocache, pi.sk_t.Value))
		}

		//Clearing caches
		for _, pi := range P_active {
			pi.CombinerCache.ClearCache()
		}

		coeffs, _, ciphertext := newTestVectors(testCtx, testCtx.encryptorPk0, 1, t)

		ciphertextSwitched := pcksPhase(testCtx.params, testCtx.pk1, ciphertext, P_active)

		verifyTestVectors(testCtx, testCtx.decryptorSk1, coeffs, ciphertextSwitched, t)

	})
}

func testKeyGen(testCtx *testContext, t *testing.T) {
	t.Run(testString("ThresholdKeyGen/", parties, testCtx.params), func(t *testing.T) {
		type Party struct {
			*Thresholdizer
			*Combiner
		}
		// Checks that GenKeyFromID is consistent among parties
		P := make([]*Party, parties)
		for i := uint64(0); i < parties; i++ {
			p := new(Party)
			p.Thresholdizer = NewThresholdizer(testCtx.params)
			p.Combiner = NewCombiner(testCtx.params, threshold)
			P[i] = p
		}
		arb_id := drlwe.PartyID{"An arbitrary ID"}
		pks := make([]*drlwe.ThreshPublicKey, parties)
		for i, p := range P {
			pks[i] = p.Thresholdizer.GenKeyFromID(arb_id)
		}
		for i, p := range P {
			if i > 0 {
				require.True(t, p.Combiner.Equal(pks[i-1], pks[i]))
			}
		}
	})
}
