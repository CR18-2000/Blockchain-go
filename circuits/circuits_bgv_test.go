package circuits

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/tuneinsight/lattigo/v4/bgv"
	"github.com/tuneinsight/lattigo/v4/ring"
	"github.com/tuneinsight/lattigo/v4/rlwe"
	"github.com/tuneinsight/lattigo/v4/utils"

	"github.com/stretchr/testify/require"
	"github.com/tuneinsight/lattigo/v4/utils/sampling"
)

// func GetTestName(opname string, p Parameters, lvl int) string {
// 	return fmt.Sprintf("%s/LogN=%d/logQ=%d/logP=%d/LogSlots=%dx%d/logT=%d/Qi=%d/Pi=%d/lvl=%d",
// 		opname,
// 		p.LogN(),
// 		int(math.Round(p.LogQ())),
// 		int(math.Round(p.LogP())),
// 		p.LogMaxDimensions().Rows,
// 		p.LogMaxDimensions().Cols,
// 		int(math.Round(p.LogT())),
// 		p.QCount(),
// 		p.PCount(),
// 		lvl)
// }

func TestBGV(t *testing.T) {

	var err error

	paramsLiterals := testParams

	if *flagParamString != "" {
		var jsonParams bgv.ParametersLiteral
		if err = json.Unmarshal([]byte(*flagParamString), &jsonParams); err != nil {
			t.Fatal(err)
		}
		paramsLiterals = []bgv.ParametersLiteral{jsonParams} // the custom test suite reads the parameters from the -params flag
	}

	for _, p := range paramsLiterals[:] {

		for _, plaintextModulus := range testPlaintextModulus[:] {

			p.PlaintextModulus = plaintextModulus

			var params bgv.Parameters
			if params, err = bgv.NewParametersFromLiteral(p); err != nil {
				t.Error(err)
				t.Fail()
			}

			var tc *bgvTestContext
			if tc, err = genBGVTestParams(params); err != nil {
				t.Error(err)
				t.Fail()
			}

			for _, testSet := range []func(tc *bgvTestContext, t *testing.T){
				testBGVLinearTransformation,
			} {
				testSet(tc, t)
				runtime.GC()
			}
		}
	}
}

type bgvTestContext struct {
	params      bgv.Parameters
	ringQ       *ring.Ring
	ringT       *ring.Ring
	prng        sampling.PRNG
	uSampler    *ring.UniformSampler
	encoder     *bgv.Encoder
	kgen        *rlwe.KeyGenerator
	sk          *rlwe.SecretKey
	pk          *rlwe.PublicKey
	encryptorPk *rlwe.Encryptor
	encryptorSk *rlwe.Encryptor
	decryptor   *rlwe.Decryptor
	evaluator   *bgv.Evaluator
	testLevel   []int
}

func genBGVTestParams(params bgv.Parameters) (tc *bgvTestContext, err error) {

	tc = new(bgvTestContext)
	tc.params = params

	if tc.prng, err = sampling.NewPRNG(); err != nil {
		return nil, err
	}

	tc.ringQ = params.RingQ()
	tc.ringT = params.RingT()

	tc.uSampler = ring.NewUniformSampler(tc.prng, tc.ringT)
	tc.kgen = bgv.NewKeyGenerator(tc.params)
	tc.sk, tc.pk = tc.kgen.GenKeyPairNew()
	tc.encoder = bgv.NewEncoder(tc.params)

	if tc.encryptorPk, err = bgv.NewEncryptor(tc.params, tc.pk); err != nil {
		return
	}

	if tc.encryptorSk, err = bgv.NewEncryptor(tc.params, tc.sk); err != nil {
		return
	}

	if tc.decryptor, err = bgv.NewDecryptor(tc.params, tc.sk); err != nil {
		return
	}

	var rlk *rlwe.RelinearizationKey
	if rlk, err = tc.kgen.GenRelinearizationKeyNew(tc.sk); err != nil {
		return
	}

	evk := rlwe.NewMemEvaluationKeySet(rlk)
	tc.evaluator = bgv.NewEvaluator(tc.params, evk)

	tc.testLevel = []int{0, params.MaxLevel()}

	return
}

func newBGVTestVectorsLvl(level int, scale rlwe.Scale, tc *bgvTestContext, encryptor *rlwe.Encryptor) (coeffs ring.Poly, plaintext *rlwe.Plaintext, ciphertext *rlwe.Ciphertext) {
	coeffs = tc.uSampler.ReadNew()
	for i := range coeffs.Coeffs[0] {
		coeffs.Coeffs[0][i] = uint64(i)
	}

	plaintext = bgv.NewPlaintext(tc.params, level)
	plaintext.Scale = scale
	tc.encoder.Encode(coeffs.Coeffs[0], plaintext)
	if encryptor != nil {
		var err error
		ciphertext, err = encryptor.EncryptNew(plaintext)
		if err != nil {
			panic(err)
		}
	}

	return coeffs, plaintext, ciphertext
}

func verifyBGVTestVectors(tc *bgvTestContext, decryptor *rlwe.Decryptor, coeffs ring.Poly, element rlwe.OperandInterface[ring.Poly], t *testing.T) {

	coeffsTest := make([]uint64, tc.params.MaxSlots())

	switch el := element.(type) {
	case *rlwe.Plaintext:
		require.NoError(t, tc.encoder.Decode(el, coeffsTest))
	case *rlwe.Ciphertext:

		pt := decryptor.DecryptNew(el)

		require.NoError(t, tc.encoder.Decode(pt, coeffsTest))

		if *flagPrintNoise {
			require.NoError(t, tc.encoder.Encode(coeffsTest, pt))
			ct, err := tc.evaluator.SubNew(el, pt)
			require.NoError(t, err)
			vartmp, _, _ := rlwe.Norm(ct, decryptor)
			t.Logf("STD(noise): %f\n", vartmp)
		}

	default:
		t.Error("invalid test object to verify")
	}

	require.True(t, utils.EqualSlice(coeffs.Coeffs[0], coeffsTest))
}

func testBGVLinearTransformation(tc *bgvTestContext, t *testing.T) {

	level := tc.params.MaxLevel()
	t.Run(GetTestName("Evaluator/LinearTransformationBSGS=true", tc.params, level), func(t *testing.T) {

		params := tc.params

		values, _, ciphertext := newBGVTestVectorsLvl(level, tc.params.DefaultScale(), tc, tc.encryptorSk)

		diagonals := make(Diagonals[uint64])

		totSlots := values.N()

		diagonals[-15] = make([]uint64, totSlots)
		diagonals[-4] = make([]uint64, totSlots)
		diagonals[-1] = make([]uint64, totSlots)
		diagonals[0] = make([]uint64, totSlots)
		diagonals[1] = make([]uint64, totSlots)
		diagonals[2] = make([]uint64, totSlots)
		diagonals[3] = make([]uint64, totSlots)
		diagonals[4] = make([]uint64, totSlots)
		diagonals[15] = make([]uint64, totSlots)

		for i := 0; i < totSlots; i++ {
			diagonals[-15][i] = 1
			diagonals[-4][i] = 1
			diagonals[-1][i] = 1
			diagonals[0][i] = 1
			diagonals[1][i] = 1
			diagonals[2][i] = 1
			diagonals[3][i] = 1
			diagonals[4][i] = 1
			diagonals[15][i] = 1
		}

		ltparams := LinearTransformationParameters{
			DiagonalsIndexList:       []int{-15, -4, -1, 0, 1, 2, 3, 4, 15},
			Level:                    ciphertext.Level(),
			Scale:                    tc.params.DefaultScale(),
			LogDimensions:            ciphertext.LogDimensions,
			LogBabyStepGianStepRatio: 1,
		}

		// Allocate the linear transformation
		linTransf := NewLinearTransformation(params, ltparams)

		// Encode on the linear transformation
		require.NoError(t, EncodeIntegerLinearTransformation[uint64](ltparams, tc.encoder, diagonals, linTransf))

		galEls := GaloisElementsForLinearTransformation(params, ltparams)

		gks, err := tc.kgen.GenGaloisKeysNew(galEls, tc.sk)
		require.NoError(t, err)
		eval := tc.evaluator.WithKey(rlwe.NewMemEvaluationKeySet(nil, gks...))
		ltEval := NewEvaluator(eval)

		require.NoError(t, ltEval.LinearTransformation(ciphertext, linTransf, ciphertext))

		tmp := make([]uint64, totSlots)
		copy(tmp, values.Coeffs[0])

		subRing := tc.params.RingT().SubRings[0]

		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, -15), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, -4), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, -1), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 1), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 2), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 3), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 4), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 15), values.Coeffs[0])

		verifyBGVTestVectors(tc, tc.decryptor, values, ciphertext, t)
	})

	t.Run(GetTestName("Evaluator/LinearTransformationBSGS=false", tc.params, level), func(t *testing.T) {

		params := tc.params

		values, _, ciphertext := newBGVTestVectorsLvl(level, tc.params.DefaultScale(), tc, tc.encryptorSk)

		diagonals := make(map[int][]uint64)

		totSlots := values.N()

		diagonals[-15] = make([]uint64, totSlots)
		diagonals[-4] = make([]uint64, totSlots)
		diagonals[-1] = make([]uint64, totSlots)
		diagonals[0] = make([]uint64, totSlots)
		diagonals[1] = make([]uint64, totSlots)
		diagonals[2] = make([]uint64, totSlots)
		diagonals[3] = make([]uint64, totSlots)
		diagonals[4] = make([]uint64, totSlots)
		diagonals[15] = make([]uint64, totSlots)

		for i := 0; i < totSlots; i++ {
			diagonals[-15][i] = 1
			diagonals[-4][i] = 1
			diagonals[-1][i] = 1
			diagonals[0][i] = 1
			diagonals[1][i] = 1
			diagonals[2][i] = 1
			diagonals[3][i] = 1
			diagonals[4][i] = 1
			diagonals[15][i] = 1
		}

		ltparams := LinearTransformationParameters{
			DiagonalsIndexList:       []int{-15, -4, -1, 0, 1, 2, 3, 4, 15},
			Level:                    ciphertext.Level(),
			Scale:                    tc.params.DefaultScale(),
			LogDimensions:            ciphertext.LogDimensions,
			LogBabyStepGianStepRatio: -1,
		}

		// Allocate the linear transformation
		linTransf := NewLinearTransformation(params, ltparams)

		// Encode on the linear transformation
		require.NoError(t, EncodeIntegerLinearTransformation[uint64](ltparams, tc.encoder, diagonals, linTransf))

		galEls := GaloisElementsForLinearTransformation(params, ltparams)

		gks, err := tc.kgen.GenGaloisKeysNew(galEls, tc.sk)
		require.NoError(t, err)
		eval := tc.evaluator.WithKey(rlwe.NewMemEvaluationKeySet(nil, gks...))
		ltEval := NewEvaluator(eval)

		require.NoError(t, ltEval.LinearTransformation(ciphertext, linTransf, ciphertext))

		tmp := make([]uint64, totSlots)
		copy(tmp, values.Coeffs[0])

		subRing := tc.params.RingT().SubRings[0]

		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, -15), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, -4), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, -1), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 1), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 2), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 3), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 4), values.Coeffs[0])
		subRing.Add(values.Coeffs[0], utils.RotateSlotsNew(tmp, 15), values.Coeffs[0])

		verifyBGVTestVectors(tc, tc.decryptor, values, ciphertext, t)
	})
}
