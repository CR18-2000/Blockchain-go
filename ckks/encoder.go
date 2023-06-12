package ckks

import (
	"fmt"
	"math/big"

	"github.com/tuneinsight/lattigo/v4/ring"

	"github.com/tuneinsight/lattigo/v4/rlwe"
	"github.com/tuneinsight/lattigo/v4/rlwe/ringqp"
	"github.com/tuneinsight/lattigo/v4/utils"
	"github.com/tuneinsight/lattigo/v4/utils/bignum"
	"github.com/tuneinsight/lattigo/v4/utils/sampling"
)

// GaloisGen is an integer of order N/2 modulo M and that spans Z_M with the integer -1.
// The j-th ring automorphism takes the root zeta to zeta^(5j).
const GaloisGen uint64 = ring.GaloisGen

// Encoder is a struct that implements the encoding and decoding operations. It provides methods to encode/decode
// []complex128/[]*bignum.Complex and []float64/[]*big.Float types into/from Plaintext types.
//
// Two different encodings domains are provided:
//
//   - Coefficients: The coefficients are directly embedded on the plaintext. This encoding only allows to encode []float64/[]*big.Float slices,
//     but of size up to N (N being the ring degree) and does not preserve the point-wise multiplication. A ciphertext multiplication will result
//     in a negacyclic polynomial convolution in the plaintext domain. This encoding does not provide native slot cyclic rotation.
//     Other operations, like addition or constant multiplication, behave as usual.
//
//   - Slots: The coefficients are first subjected to a special Fourier transform before being embedded in the plaintext by using Coeffs encoding.
//     This encoding can embed []complex128/[]*bignum.Complex and []float64/[]*big.Float slices of size at most N/2 (N being the ring degree) and
//     leverages the convolution property of the DFT to preserve point-wise complex multiplication in the plaintext domain, i.e. a ciphertext
//     multiplication will result in an element-wise multiplication in the plaintext domain. It also enables cyclic rotations on plaintext slots.
//     Other operations, like constant multiplication, behave as usual. It is considered the default encoding method for CKKS.
//
// The figure bellow illustrates the relationship between these two encodings:
//
//	                                                    Z_Q[X]/(X^N+1)
//		Coefficients: ---------------> Real^{N} ---------> Plaintext
//	                                      |
//	                                      |
//		Slots: Complex^{N/2} -> iDFT -----┘
type Encoder struct {
	prec uint

	parameters   Parameters
	bigintCoeffs []*big.Int
	qHalf        *big.Int
	buff         *ring.Poly
	m            int
	rotGroup     []int

	prng sampling.PRNG

	roots     interface{}
	buffCmplx interface{}
}

func (ecd *Encoder) ShallowCopy() *Encoder {

	prng, err := sampling.NewPRNG()
	if err != nil {
		panic(err)
	}

	var buffCmplx interface{}

	if prec := ecd.prec; prec <= 53 {
		buffCmplx = make([]complex128, ecd.m>>1)
	} else {
		tmp := make([]*bignum.Complex, ecd.m>>2)

		for i := 0; i < ecd.m>>2; i++ {
			tmp[i] = &bignum.Complex{bignum.NewFloat(0, prec), bignum.NewFloat(0, prec)}
		}

		buffCmplx = tmp
	}

	return &Encoder{
		prec:         ecd.prec,
		parameters:   ecd.parameters,
		bigintCoeffs: make([]*big.Int, len(ecd.bigintCoeffs)),
		qHalf:        new(big.Int),
		buff:         ecd.buff.CopyNew(),
		m:            ecd.m,
		rotGroup:     ecd.rotGroup,
		prng:         prng,
		roots:        ecd.roots,
		buffCmplx:    buffCmplx,
	}
}

// NewEncoder creates a new Encoder from the target parameters.
// Optional field `precision` can be given. If precision is empty
// or <= 53, then float64 and complex128 types will be used to
// perform the encoding. Else *big.Float and *bignum.Complex will be used.
func NewEncoder(parameters Parameters, precision ...uint) (ecd *Encoder) {

	m := int(parameters.RingQ().NthRoot())

	rotGroup := make([]int, m>>2)
	fivePows := 1
	for i := 0; i < m>>2; i++ {
		rotGroup[i] = fivePows
		fivePows *= int(GaloisGen)
		fivePows &= (m - 1)
	}

	prng, err := sampling.NewPRNG()
	if err != nil {
		panic(err)
	}

	var prec uint
	if len(precision) != 0 && precision[0] != 0 {
		prec = precision[0]
	} else {
		prec = parameters.PlaintextPrecision()
	}

	ecd = &Encoder{
		prec:         prec,
		parameters:   parameters,
		bigintCoeffs: make([]*big.Int, m>>1),
		qHalf:        bignum.NewInt(0),
		buff:         parameters.RingQ().NewPoly(),
		m:            m,
		rotGroup:     rotGroup,
		prng:         prng,
	}

	if prec <= 53 {

		ecd.roots = GetRootsComplex128(ecd.m)
		ecd.buffCmplx = make([]complex128, ecd.m>>2)

	} else {

		tmp := make([]*bignum.Complex, ecd.m>>2)

		for i := 0; i < ecd.m>>2; i++ {
			tmp[i] = &bignum.Complex{bignum.NewFloat(0, prec), bignum.NewFloat(0, prec)}
		}

		ecd.roots = GetRootsBigComplex(ecd.m, prec)
		ecd.buffCmplx = tmp
	}

	return
}

// Prec returns the precision in bits used by the target Encoder.
// A precision <= 53 will use float64, else *big.Float.
func (ecd *Encoder) Prec() uint {
	return ecd.prec
}

// Parameters returns the Parameters used by the target Encoder.
func (ecd *Encoder) Parameters() rlwe.ParametersInterface {
	return ecd.parameters
}

// Encode encodes a set of values on the target plaintext.
// Encoding is done at the level and scale of the plaintext.
// Encoding domain is done according to the metadata of the plaintext.
// User must ensure that 1 <= len(values) <= 2^pt.PlaintextLogDimensions < 2^logN.
// Accepted values.(type) for `rlwe.EncodingDomain = rlwe.FrequencyDomain` is []complex128 of []float64.
// Accepted values.(type) for `rlwe.EncodingDomain = rlwe.CoefficientDomain` is []float64.
// The imaginary part of []complex128 will be discarded if ringType == ring.ConjugateInvariant.
func (ecd *Encoder) Encode(values interface{}, pt *rlwe.Plaintext) (err error) {

	switch pt.EncodingDomain {
	case rlwe.FrequencyDomain:

		return ecd.Embed(values, pt.MetaData, pt.Value)

	case rlwe.TimeDomain:

		switch values := values.(type) {
		case []float64:

			if len(values) > ecd.parameters.N() {
				return fmt.Errorf("cannot Encode: maximum number of values is %d but len(values) is %d", ecd.parameters.N(), len(values))
			}

			Float64ToFixedPointCRT(ecd.parameters.RingQ().AtLevel(pt.Level()), values, pt.PlaintextScale.Float64(), pt.Value.Coeffs)

		case []*big.Float:

			if len(values) > ecd.parameters.N() {
				return fmt.Errorf("cannot Encode: maximum number of values is %d but len(values) is %d", ecd.parameters.N(), len(values))
			}

			BigFloatToFixedPointCRT(ecd.parameters.RingQ().AtLevel(pt.Level()), values, &pt.PlaintextScale.Value, pt.Value.Coeffs)

		default:
			return fmt.Errorf("cannot Encode: supported values.(type) for %T encoding domain is []float64 or []*big.Float, but %T was given", rlwe.TimeDomain, values)
		}

		ecd.parameters.RingQ().AtLevel(pt.Level()).NTT(pt.Value, pt.Value)

	default:
		return fmt.Errorf("cannot Encode: invalid rlwe.EncodingType, accepted types are rlwe.FrequencyDomain and rlwe.TimeDomain but is %T", pt.EncodingDomain)
	}

	return
}

// Decode decodes the input plaintext on a new slice of complex128.
// This method is the same as .DecodeSlots(*).
func (ecd *Encoder) Decode(pt *rlwe.Plaintext, values interface{}) (err error) {
	return ecd.DecodePublic(pt, values, nil)
}

// DecodePublic decodes the input plaintext on a new slice of complex128.
// Adds, before the decoding step, noise following the given distribution parameters.
// If the underlying ringType is ConjugateInvariant, the imaginary part (and its related error) are zero.
func (ecd *Encoder) DecodePublic(pt *rlwe.Plaintext, values interface{}, noiseFlooding ring.DistributionParameters) (err error) {
	return ecd.decodePublic(pt, values, noiseFlooding)
}

// Embed is a generic method to encode a set of values on the target polyOut interface.
// This method it as the core of the slot encoding.
// values: values.(type) can be either []complex128, []*bignum.Complex, []float64 or []*big.Float.
//
//	The imaginary part of []complex128 or []*bignum.Complex will be discarded if ringType == ring.ConjugateInvariant.
//
// logslots: user must ensure that 1 <= len(values) <= 2^logSlots < 2^logN.
// scale: the scaling factor used do discretize float64 to fixed point integers.
// montgomery: if true then the value written on polyOut are put in the Montgomery domain.
// polyOut: polyOut.(type) can be either ringqp.Poly or *ring.Poly.
//
//	The encoding encoding is done at the level of polyOut.
//
// Values written on  polyOut are always in the NTT domain.
func (ecd *Encoder) Embed(values interface{}, metadata rlwe.MetaData, polyOut interface{}) (err error) {
	if ecd.prec <= 53 {
		return ecd.embedDouble(values, metadata, polyOut)
	}

	return ecd.embedArbitrary(values, metadata, polyOut)
}

func (ecd *Encoder) embedDouble(values interface{}, metadata rlwe.MetaData, polyOut interface{}) (err error) {

	if maxLogCols := ecd.parameters.PlaintextLogDimensions()[1]; metadata.PlaintextLogDimensions[1] < 0 || metadata.PlaintextLogDimensions[1] > maxLogCols {
		return fmt.Errorf("cannot Embed: logSlots (%d) must be greater or equal to %d and smaller than %d", metadata.PlaintextLogDimensions[1], 0, maxLogCols)
	}

	slots := 1 << metadata.PlaintextLogDimensions[1]
	var lenValues int

	buffCmplx := ecd.buffCmplx.([]complex128)

	switch values := values.(type) {

	case []complex128:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		if ecd.parameters.RingType() == ring.ConjugateInvariant {
			for i := range values {
				buffCmplx[i] = complex(real(values[i]), 0)
			}
		} else {
			copy(buffCmplx[:len(values)], values)
		}

	case []*bignum.Complex:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		if ecd.parameters.RingType() == ring.ConjugateInvariant {
			for i := range values {
				if values[i] != nil {
					f64, _ := values[i][0].Float64()
					buffCmplx[i] = complex(f64, 0)
				} else {
					buffCmplx[i] = 0
				}
			}
		} else {
			for i := range values {
				if values[i] != nil {
					buffCmplx[i] = values[i].Complex128()
				} else {
					buffCmplx[i] = 0
				}
			}
		}

	case []float64:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		for i := range values {
			buffCmplx[i] = complex(values[i], 0)
		}

	case []*big.Float:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		for i := range values {
			if values[i] != nil {
				f64, _ := values[i].Float64()
				buffCmplx[i] = complex(f64, 0)
			} else {
				buffCmplx[i] = 0
			}
		}
	default:
		return fmt.Errorf("cannot Embed: values.(Type) must be []complex128, []*bignum.Complex, []float64 or []*big.Float, but is %T", values)
	}

	// Zeroes all other values
	for i := lenValues; i < slots; i++ {
		buffCmplx[i] = 0
	}

	// IFFT
	if err = ecd.IFFT(buffCmplx[:slots], metadata.PlaintextLogDimensions[1]); err != nil {
		return
	}

	// Maps Y = X^{N/n} -> X and quantizes.
	switch p := polyOut.(type) {
	case ringqp.Poly:
		Complex128ToFixedPointCRT(ecd.parameters.RingQ().AtLevel(p.Q.Level()), buffCmplx[:slots], metadata.PlaintextScale.Float64(), p.Q.Coeffs)
		rlwe.NTTSparseAndMontgomery(ecd.parameters.RingQ().AtLevel(p.Q.Level()), metadata, p.Q)

		if p.P != nil {
			Complex128ToFixedPointCRT(ecd.parameters.RingP().AtLevel(p.P.Level()), buffCmplx[:slots], metadata.PlaintextScale.Float64(), p.P.Coeffs)
			rlwe.NTTSparseAndMontgomery(ecd.parameters.RingP().AtLevel(p.P.Level()), metadata, p.P)
		}
	case *ring.Poly:
		Complex128ToFixedPointCRT(ecd.parameters.RingQ().AtLevel(p.Level()), buffCmplx[:slots], metadata.PlaintextScale.Float64(), p.Coeffs)
		rlwe.NTTSparseAndMontgomery(ecd.parameters.RingQ().AtLevel(p.Level()), metadata, p)
	default:
		return fmt.Errorf("cannot Embed: invalid polyOut.(Type) must be ringqp.Poly or *ring.Poly")
	}

	return
}

func (ecd *Encoder) embedArbitrary(values interface{}, metadata rlwe.MetaData, polyOut interface{}) (err error) {

	if maxLogCols := ecd.parameters.PlaintextLogDimensions()[1]; metadata.PlaintextLogDimensions[1] < 0 || metadata.PlaintextLogDimensions[1] > maxLogCols {
		return fmt.Errorf("cannot Embed: logSlots (%d) must be greater or equal to %d and smaller than %d", metadata.PlaintextLogDimensions[1], 0, maxLogCols)
	}

	slots := 1 << metadata.PlaintextLogDimensions[1]
	var lenValues int

	buffCmplx := ecd.buffCmplx.([]*bignum.Complex)

	switch values := values.(type) {

	case []complex128:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		if ecd.parameters.RingType() == ring.ConjugateInvariant {
			for i := range values {
				buffCmplx[i][0].SetFloat64(real(values[i]))
				buffCmplx[i][1].SetFloat64(0)
			}
		} else {
			for i := range values {
				buffCmplx[i][0].SetFloat64(real(values[i]))
				buffCmplx[i][1].SetFloat64(imag(values[i]))
			}
		}

	case []*bignum.Complex:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		if ecd.parameters.RingType() == ring.ConjugateInvariant {
			for i := range values {
				if values[i] != nil {
					buffCmplx[i][0].Set(values[i][0])
				} else {
					buffCmplx[i][0].SetFloat64(0)
				}

				buffCmplx[i][1].SetFloat64(0)
			}
		} else {
			for i := range values {
				if values[i] != nil {
					buffCmplx[i].Set(values[i])
				} else {
					buffCmplx[i][0].SetFloat64(0)
					buffCmplx[i][1].SetFloat64(0)
				}
			}
		}

	case []float64:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		for i := range values {
			buffCmplx[i][0].SetFloat64(values[i])
			buffCmplx[i][1].SetFloat64(0)
		}

	case []*big.Float:

		lenValues = len(values)

		if maxCols := ecd.parameters.PlaintextDimensions()[1]; lenValues > maxCols || lenValues > slots {
			return fmt.Errorf("cannot Embed: ensure that #values (%d) <= slots (%d) <= maxCols (%d)", len(values), slots, maxCols)
		}

		for i := range values {
			if values[i] != nil {
				buffCmplx[i][0].Set(values[i])
			} else {
				buffCmplx[i][0].SetFloat64(0)
			}

			buffCmplx[i][1].SetFloat64(0)
		}
	default:
		return fmt.Errorf("cannot Embed: values.(Type) must be []complex128, []*bignum.Complex, []float64 or []*big.Float, but is %T", values)
	}

	// Zeroes all other values
	for i := lenValues; i < slots; i++ {
		buffCmplx[i][0].SetFloat64(0)
		buffCmplx[i][1].SetFloat64(0)
	}

	if err = ecd.IFFT(buffCmplx[:slots], metadata.PlaintextLogDimensions[1]); err != nil {
		return
	}

	// Maps Y = X^{N/n} -> X and quantizes.
	switch p := polyOut.(type) {

	case *ring.Poly:

		ComplexArbitraryToFixedPointCRT(ecd.parameters.RingQ().AtLevel(p.Level()), buffCmplx[:slots], &metadata.PlaintextScale.Value, p.Coeffs)
		rlwe.NTTSparseAndMontgomery(ecd.parameters.RingQ().AtLevel(p.Level()), metadata, p)

	case ringqp.Poly:

		ComplexArbitraryToFixedPointCRT(ecd.parameters.RingQ().AtLevel(p.Q.Level()), buffCmplx[:slots], &metadata.PlaintextScale.Value, p.Q.Coeffs)
		rlwe.NTTSparseAndMontgomery(ecd.parameters.RingQ().AtLevel(p.Q.Level()), metadata, p.Q)

		if p.P != nil {
			ComplexArbitraryToFixedPointCRT(ecd.parameters.RingP().AtLevel(p.P.Level()), buffCmplx[:slots], &metadata.PlaintextScale.Value, p.P.Coeffs)
			rlwe.NTTSparseAndMontgomery(ecd.parameters.RingP().AtLevel(p.P.Level()), metadata, p.P)
		}

	default:
		return fmt.Errorf("cannot Embed: invalid polyOut.(Type) must be ringqp.Poly or *ring.Poly")
	}

	return
}

func (ecd *Encoder) plaintextToComplex(level int, scale rlwe.Scale, logSlots int, p *ring.Poly, values interface{}) {

	isreal := ecd.parameters.RingType() == ring.ConjugateInvariant
	if level == 0 {
		polyToComplexNoCRT(p.Coeffs[0], values, scale, logSlots, isreal, ecd.parameters.RingQ().AtLevel(level))
	} else {
		polyToComplexCRT(p, ecd.bigintCoeffs, values, scale, logSlots, isreal, ecd.parameters.RingQ().AtLevel(level))
	}
}

func (ecd *Encoder) plaintextToFloat(level int, scale rlwe.Scale, logSlots int, p *ring.Poly, values interface{}) {
	if level == 0 {
		ecd.polyToFloatNoCRT(p.Coeffs[0], values, scale, logSlots, ecd.parameters.RingQ().AtLevel(level))
	} else {
		ecd.polyToFloatCRT(p, values, scale, logSlots, ecd.parameters.RingQ().AtLevel(level))
	}
}

func (ecd *Encoder) decodePublic(pt *rlwe.Plaintext, values interface{}, noiseFlooding ring.DistributionParameters) (err error) {

	logSlots := pt.PlaintextLogDimensions[1]
	slots := 1 << logSlots

	if maxLogCols := ecd.parameters.PlaintextLogDimensions()[1]; logSlots > maxLogCols || logSlots < 0 {
		return fmt.Errorf("cannot Decode: ensure that %d <= logSlots (%d) <= %d", 0, logSlots, maxLogCols)
	}

	if pt.IsNTT {
		ecd.parameters.RingQ().AtLevel(pt.Level()).INTT(pt.Value, ecd.buff)
	} else {
		ring.CopyLvl(pt.Level(), pt.Value, ecd.buff)
	}

	if noiseFlooding != nil {
		ring.NewSampler(ecd.prng, ecd.parameters.RingQ(), noiseFlooding, pt.IsMontgomery).AtLevel(pt.Level()).ReadAndAdd(ecd.buff)
	}

	switch values.(type) {
	case []complex128, []float64, []*bignum.Complex, []*big.Float:
	default:
		return fmt.Errorf("cannot decode: values.(type) accepted are []complex128, []float64, []*bignum.Complex, []*big.Float but is %T", values)
	}

	switch pt.EncodingDomain {
	case rlwe.FrequencyDomain:

		if ecd.prec <= 53 {

			buffCmplx := ecd.buffCmplx.([]complex128)

			ecd.plaintextToComplex(pt.Level(), pt.PlaintextScale, logSlots, ecd.buff, buffCmplx)

			if err = ecd.FFT(buffCmplx[:slots], logSlots); err != nil {
				return
			}

			switch values := values.(type) {
			case []float64:

				slots := utils.Min(len(values), slots)

				for i := 0; i < slots; i++ {
					values[i] = real(buffCmplx[i])
				}
			case []complex128:
				copy(values, buffCmplx)

			case []*big.Float:
				slots := utils.Min(len(values), slots)

				for i := 0; i < slots; i++ {

					if values[i] == nil {
						values[i] = new(big.Float)
					}

					values[i].SetFloat64(real(buffCmplx[i]))
				}

			case []*bignum.Complex:

				slots := utils.Min(len(values), slots)

				for i := 0; i < slots; i++ {

					if values[i] == nil {
						values[i] = &bignum.Complex{
							new(big.Float),
							new(big.Float),
						}
					} else {
						if values[i][0] == nil {
							values[i][0] = new(big.Float)
						}

						if values[i][1] == nil {
							values[i][1] = new(big.Float)
						}
					}

					values[i][0].SetFloat64(real(buffCmplx[i]))
					values[i][1].SetFloat64(imag(buffCmplx[i]))
				}
			}
		} else {

			buffCmplx := ecd.buffCmplx.([]*bignum.Complex)

			ecd.plaintextToComplex(pt.Level(), pt.PlaintextScale, logSlots, ecd.buff, buffCmplx[:slots])

			if err = ecd.FFT(buffCmplx[:slots], logSlots); err != nil {
				return
			}

			switch values := values.(type) {
			case []float64:

				slots := utils.Min(len(values), slots)

				for i := 0; i < slots; i++ {
					values[i], _ = buffCmplx[i][0].Float64()
				}

			case []complex128:

				slots := utils.Min(len(values), slots)

				for i := 0; i < slots; i++ {
					values[i] = buffCmplx[i].Complex128()
				}

			case []*big.Float:
				slots := utils.Min(len(values), slots)

				for i := 0; i < slots; i++ {

					if values[i] == nil {
						values[i] = new(big.Float)
					}

					values[i].Set(buffCmplx[i][0])
				}

			case []*bignum.Complex:

				slots := utils.Min(len(values), slots)

				for i := 0; i < slots; i++ {

					if values[i] == nil {
						values[i] = &bignum.Complex{
							new(big.Float),
							new(big.Float),
						}
					} else {
						if values[i][0] == nil {
							values[i][0] = new(big.Float)
						}

						if values[i][1] == nil {
							values[i][1] = new(big.Float)
						}
					}

					values[i][0].Set(buffCmplx[i][0])
					values[i][1].Set(buffCmplx[i][1])
				}
			}
		}

	case rlwe.TimeDomain:
		ecd.plaintextToFloat(pt.Level(), pt.PlaintextScale, logSlots, ecd.buff, values)
	default:
		return fmt.Errorf("cannot decode: invalid rlwe.EncodingType, accepted types are rlwe.FrequencyDomain and rlwe.TimeDomain but is %T", pt.EncodingDomain)
	}

	return
}

func (ecd *Encoder) IFFT(values interface{}, logN int) (err error) {
	switch values := values.(type) {
	case []complex128:
		switch roots := ecd.roots.(type) {
		case []complex128:
			if true {
				SpecialIFFTDouble(values, 1<<logN, ecd.m, ecd.rotGroup, ecd.roots.([]complex128))
			} else {
				SpecialiFFTDoubleUnrolled8(values, 1<<logN, ecd.m, ecd.rotGroup, ecd.roots.([]complex128))
			}
		default:
			return fmt.Errorf("cannot IFFT: values.(type)=%T doesn't roots.(type) = %T", values, roots)
		}

	case []*bignum.Complex:
		switch roots := ecd.roots.(type) {
		case []*bignum.Complex:
			SpecialIFFTArbitrary(values, 1<<logN, ecd.m, ecd.rotGroup, ecd.roots.([]*bignum.Complex))
		default:
			return fmt.Errorf("cannot IFFT: values.(type)=%T doesn't roots.(type) = %T", values, roots)
		}
	default:
		return fmt.Errorf("cannot IFFT: invalid values.(type), accepted types are []complex128 and []*bignum.Complex but is %T", values)
	}
	return

}

func (ecd *Encoder) FFT(values interface{}, logN int) (err error) {
	switch values := values.(type) {
	case []complex128:
		switch roots := ecd.roots.(type) {
		case []complex128:
			if logN < 3 {
				SpecialFFTDouble(values, 1<<logN, ecd.m, ecd.rotGroup, roots)
			} else {
				SpecialFFTDoubleUL8(values, 1<<logN, ecd.m, ecd.rotGroup, roots)
			}
		default:
			return fmt.Errorf("cannot IFFT: values.(type)=%T doesn't roots.(type) = %T", values, roots)
		}

	case []*bignum.Complex:
		switch roots := ecd.roots.(type) {
		case []*bignum.Complex:
			SpecialFFTArbitrary(values, 1<<logN, ecd.m, ecd.rotGroup, roots)
		default:
			return fmt.Errorf("cannot IFFT: values.(type)=%T doesn't roots.(type) = %T", values, roots)
		}

	default:
		return fmt.Errorf("cannot IFFT: invalid values.(type), accepted types are []complex128 and []*bignum.Complex but is %T", values)
	}
	return
}

func polyToComplexNoCRT(coeffs []uint64, values interface{}, scale rlwe.Scale, logSlots int, isreal bool, ringQ *ring.Ring) {

	slots := 1 << logSlots
	maxCols := int(ringQ.NthRoot() >> 2)
	gap := maxCols / slots
	Q := ringQ.SubRings[0].Modulus
	var c uint64

	switch values := values.(type) {
	case []complex128:
		for i, idx := 0, 0; i < slots; i, idx = i+1, idx+gap {
			c = coeffs[idx]
			if c >= Q>>1 {
				values[i] = complex(-float64(Q-c), 0)
			} else {
				values[i] = complex(float64(c), 0)
			}
		}

		if !isreal {
			for i, idx := 0, maxCols; i < slots; i, idx = i+1, idx+gap {
				c = coeffs[idx]
				if c >= Q>>1 {
					values[i] += complex(0, -float64(Q-c))
				} else {
					values[i] += complex(0, float64(c))
				}
			}
		} else {
			// [X]/(X^N+1) to [X+X^-1]/(X^N+1)
			slots := 1 << logSlots
			for i := 1; i < slots; i++ {
				values[i] -= complex(0, real(values[slots-i]))
			}
		}

		divideComplex128SliceUnrolled8(values, complex(scale.Float64(), 0))

	case []*bignum.Complex:

		for i, idx := 0, 0; i < slots; i, idx = i+1, idx+gap {

			if values[i] == nil {
				values[i] = &bignum.Complex{
					new(big.Float),
					nil,
				}
			} else {
				if values[i][0] == nil {
					values[i][0] = new(big.Float)
				}
			}

			if c = coeffs[idx]; c >= Q>>1 {
				values[i][0].SetInt64(-int64(Q - c))
			} else {
				values[i][0].SetInt64(int64(c))
			}
		}

		if !isreal {
			for i, idx := 0, maxCols; i < slots; i, idx = i+1, idx+gap {

				if values[i][1] == nil {
					values[i][1] = new(big.Float)
				}

				if c = coeffs[idx]; c >= Q>>1 {
					values[i][1].SetInt64(-int64(Q - c))
				} else {
					values[i][1].SetInt64(int64(c))
				}
			}
		} else {
			slots := 1 << logSlots

			for i := 1; i < slots; i++ {
				values[i][1].Sub(values[i][1], values[slots-i][0])
			}
		}

		s := &scale.Value

		for i := range values {
			values[i][0].Quo(values[i][0], s)
			values[i][1].Quo(values[i][1], s)
		}

	default:
		panic(fmt.Errorf("cannot polyToComplexNoCRT: values.(Type) must be []complex128 or []*bignum.Complex but is %T", values))
	}
}

func polyToComplexCRT(poly *ring.Poly, bigintCoeffs []*big.Int, values interface{}, scale rlwe.Scale, logSlots int, isreal bool, ringQ *ring.Ring) {

	maxCols := int(ringQ.NthRoot() >> 2)
	slots := 1 << logSlots
	gap := maxCols / slots

	ringQ.PolyToBigint(poly, gap, bigintCoeffs)

	Q := ringQ.ModulusAtLevel[ringQ.Level()]

	qHalf := new(big.Int)
	qHalf.Set(Q)
	qHalf.Rsh(qHalf, 1)

	var sign int

	switch values := values.(type) {

	case []complex128:
		scalef64 := scale.Float64()

		var c *big.Int
		for i := 0; i < slots; i++ {
			c = bigintCoeffs[i]
			c.Mod(c, Q)
			if sign = c.Cmp(qHalf); sign == 1 || sign == 0 {
				c.Sub(c, Q)
			}
			values[i] = complex(scaleDown(c, scalef64), 0)
		}

		if !isreal {
			for i, j := 0, slots; i < slots; i, j = i+1, j+1 {
				c = bigintCoeffs[j]
				c.Mod(c, Q)
				if sign = c.Cmp(qHalf); sign == 1 || sign == 0 {
					c.Sub(c, Q)
				}
				values[i] += complex(0, scaleDown(c, scalef64))
			}
		} else {
			// [X]/(X^N+1) to [X+X^-1]/(X^N+1)
			slots := 1 << logSlots
			for i := 1; i < slots; i++ {
				values[i] -= complex(0, real(values[slots-i]))
			}
		}
	case []*bignum.Complex:

		var c *big.Int
		for i := 0; i < slots; i++ {
			c = bigintCoeffs[i]
			c.Mod(c, Q)
			if sign = c.Cmp(qHalf); sign == 1 || sign == 0 {
				c.Sub(c, Q)
			}

			if values[i] == nil {
				values[i] = &bignum.Complex{
					new(big.Float),
					nil,
				}
			} else {
				if values[i][0] == nil {
					values[i][0] = new(big.Float)
				}
			}

			values[i][0].SetInt(c)
		}

		if !isreal {
			for i, j := 0, slots; i < slots; i, j = i+1, j+1 {
				c = bigintCoeffs[j]
				c.Mod(c, Q)
				if sign = c.Cmp(qHalf); sign == 1 || sign == 0 {
					c.Sub(c, Q)
				}

				if values[i][1] == nil {
					values[i][1] = new(big.Float)
				}

				values[i][1].SetInt(c)
			}
		} else {
			// [X]/(X^N+1) to [X+X^-1]/(X^N+1)
			slots := 1 << logSlots
			for i := 1; i < slots; i++ {
				values[i][1].Sub(values[i][1], values[slots-i][0])
			}
		}

		s := &scale.Value

		for i := range values {
			values[i][0].Quo(values[i][0], s)
			values[i][1].Quo(values[i][1], s)
		}

	default:
		panic(fmt.Errorf("cannot polyToComplexNoCRT: values.(Type) must be []complex128 or []*bignum.Complex but is %T", values))
	}
}

func (ecd *Encoder) polyToFloatCRT(p *ring.Poly, values interface{}, scale rlwe.Scale, logSlots int, r *ring.Ring) {

	var slots int
	switch values := values.(type) {
	case []float64:
		slots = utils.Min(len(p.Coeffs[0]), len(values))
	case []complex128:
		slots = utils.Min(len(p.Coeffs[0]), len(values))
	case []*big.Float:
		slots = utils.Min(len(p.Coeffs[0]), len(values))
	case []*bignum.Complex:
		slots = utils.Min(len(p.Coeffs[0]), len(values))
	}

	bigintCoeffs := ecd.bigintCoeffs

	ecd.parameters.RingQ().PolyToBigint(ecd.buff, 1, bigintCoeffs)

	Q := r.ModulusAtLevel[r.Level()]

	ecd.qHalf.Set(Q)
	ecd.qHalf.Rsh(ecd.qHalf, 1)

	var sign int
	for i := 0; i < slots; i++ {
		// Centers the value around the current modulus
		bigintCoeffs[i].Mod(bigintCoeffs[i], Q)

		sign = bigintCoeffs[i].Cmp(ecd.qHalf)
		if sign == 1 || sign == 0 {
			bigintCoeffs[i].Sub(bigintCoeffs[i], Q)
		}
	}

	switch values := values.(type) {

	case []float64:
		sf64 := scale.Float64()
		for i := 0; i < slots; i++ {
			values[i] = scaleDown(bigintCoeffs[i], sf64)
		}
	case []complex128:
		sf64 := scale.Float64()
		for i := 0; i < slots; i++ {
			values[i] = complex(scaleDown(bigintCoeffs[i], sf64), 0)
		}
	case []*big.Float:
		s := &scale.Value
		for i := 0; i < slots; i++ {

			if values[i] == nil {
				values[i] = new(big.Float)
			}

			values[i].SetInt(bigintCoeffs[i])
			values[i].Quo(values[i], s)
		}
	case []*bignum.Complex:
		s := &scale.Value
		for i := 0; i < slots; i++ {

			if values[i] == nil {
				values[i] = &bignum.Complex{
					new(big.Float),
					new(big.Float),
				}
			} else {
				if values[i][0] == nil {
					values[i][0] = new(big.Float)
				}
			}

			values[i][0].SetInt(bigintCoeffs[i])
			values[i][0].Quo(values[i][0], s)
		}
	default:
		panic(fmt.Errorf("cannot polyToComplexNoCRT: values.(Type) must be []complex128, []*bignum.Complex, []float64 or []*big.Float but is %T", values))

	}
}

func (ecd *Encoder) polyToFloatNoCRT(coeffs []uint64, values interface{}, scale rlwe.Scale, logSlots int, r *ring.Ring) {

	Q := r.SubRings[0].Modulus

	var slots int
	switch values := values.(type) {
	case []float64:
		slots = utils.Min(len(coeffs), len(values))
	case []complex128:
		slots = utils.Min(len(coeffs), len(values))
	case []*big.Float:
		slots = utils.Min(len(coeffs), len(values))
	case []*bignum.Complex:
		slots = utils.Min(len(coeffs), len(values))
	}

	switch values := values.(type) {

	case []float64:

		sf64 := scale.Float64()

		for i := 0; i < slots; i++ {
			if coeffs[i] >= Q>>1 {
				values[i] = -float64(Q-coeffs[i]) / sf64
			} else {
				values[i] = float64(coeffs[i]) / sf64
			}
		}

	case []complex128:

		sf64 := scale.Float64()

		for i := 0; i < slots; i++ {
			if coeffs[i] >= Q>>1 {
				values[i] = complex(-float64(Q-coeffs[i])/sf64, 0)
			} else {
				values[i] = complex(float64(coeffs[i])/sf64, 0)
			}
		}

	case []*big.Float:

		s := &scale.Value

		for i := 0; i < slots; i++ {

			if values[i] == nil {
				values[i] = new(big.Float)
			}

			if coeffs[i] >= Q>>1 {
				values[i].SetInt64(-int64(Q - coeffs[i]))
			} else {
				values[i].SetInt64(int64(coeffs[i]))
			}

			values[i].Quo(values[i], s)
		}

	case []*bignum.Complex:

		s := &scale.Value

		for i := 0; i < slots; i++ {

			if values[i] == nil {
				values[i] = &bignum.Complex{
					new(big.Float),
					nil,
				}
			} else {
				if values[i][0] == nil {
					values[i][0] = new(big.Float)
				}
			}

			if coeffs[i] >= Q>>1 {
				values[i][0].SetInt64(-int64(Q - coeffs[i]))
			} else {
				values[i][0].SetInt64(int64(coeffs[i]))
			}

			values[i][0].Quo(values[i][0], s)
		}

	default:
		panic(fmt.Errorf("cannot polyToComplexNoCRT: values.(Type) must be []complex128, []*bignum.Complex, []float64 or []*big.Float but is %T", values))
	}
}

type encoder[T float64 | complex128 | *big.Float | *bignum.Complex, U *ring.Poly | ringqp.Poly | *rlwe.Plaintext] struct {
	*Encoder
}

func (e *encoder[T, U]) Encode(values []T, metadata rlwe.MetaData, output U) (err error) {
	return e.Encoder.Embed(values, metadata, output)
}
