package ring

// RNSScalar represents a scalar value in the Ring (i.e., a degree-0 polynomial) in RNS form.
type RNSScalar []uint64

// NewRNSScalar creates a new Scalar value.
func (r *Ring) NewRNSScalar() RNSScalar {
	return make(RNSScalar, len(r.Modulus))
}

// NewRNSScalarFromUInt64 creates a new Scalar initialized with value v.
func (r *Ring) NewRNSScalarFromUInt64(v uint64) RNSScalar {
	s := make(RNSScalar, len(r.Modulus))
	for i, qi := range r.Modulus {
		s[i] = v % qi
	}
	return s
}

// SubRNSScalar subtracts s2 to s1 and stores the result in sout.
func (r *Ring) SubRNSScalar(s1, s2, sout RNSScalar) {
	for i, qi := range r.Modulus {
		if s2[i] > s1[i] {
			sout[i] = s1[i] + qi - s2[i]
		} else {
			sout[i] = s1[i] - s2[i]
		}
	}
}

// MulRNSScalar multiplies s1 and s2 and stores the result in sout.
func (r *Ring) MulRNSScalar(s1, s2, sout RNSScalar) {
	for i, qi := range r.Modulus {
		sout[i] = MRedConstant(s1[i], s2[i], qi, r.MredParams[i])
	}
}

// InverseCRT computes the modular inverse of a scalar a expressed in a CRT decomposition.
// The inversion is done in-place and assumes that a is in Montgomery form.
func (r *Ring) InverseCRT(a []uint64) {
	for i, qi := range r.Modulus {
		a[i] = ModexpMontgomery(a[i], int(qi-2), qi, r.MredParams[i], r.BredParams[i])
	}
}

// EvalPolMontgomeryScalarNTT evaluate the polynomial pol at pk and writes the result in p3
func (r *Ring) EvalPolMontgomeryScalarNTT(pol []*Poly, pk uint64, p3 *Poly) {
	p3.Copy(pol[len(pol)-1])
	for i := len(pol) - 1; i > 0; i-- {
		r.MulScalar(p3, pk, p3)
		r.AddNoMod(p3, pol[i-1], p3)
	}
	r.Reduce(p3, p3)
}