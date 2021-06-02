package mkrlwe

import (
	"sort"

	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/rlwe"
	"github.com/ldsec/lattigo/v2/utils"
)

// Dot computes the dot product of two decomposed polynomials in ring^d and store the result in res
func Dot(p1 *MKDecomposedPoly, p2 *MKDecomposedPoly, r *ring.Ring) *ring.Poly {
	if len(p1.Poly) != len(p2.Poly) {
		panic("Cannot compute dot product on vectors of different size !")
	}

	res := r.NewPoly()

	for i := uint64(0); i < uint64(len(p1.Poly)); i++ {

		r.MulCoeffsMontgomeryAndAdd(p1.Poly[i], p2.Poly[i], res)
	}

	return res
}

// DotLvl computes the dot product of two decomposed polynomials in ringQ^d up to q_level and store the result in res
func DotLvl(level uint64, p1 *MKDecomposedPoly, p2 *MKDecomposedPoly, r *ring.Ring) *ring.Poly {
	if len(p1.Poly) != len(p2.Poly) {
		panic("Cannot compute dot product on vectors of different size !")
	}

	res := r.NewPoly()

	for i := uint64(0); i < uint64(len(p1.Poly)); i++ {

		r.MulCoeffsMontgomeryAndAddLvl(level, p1.Poly[i], p2.Poly[i], res)
	}

	return res
}

// DotQP combines the dot product of a decomposed polynomial in RQ^d up to q_level and a decomposed polynomial in R_P^d with a switching key, and returns the two results
func DotQP(level uint64, p1 *MKDecomposedPoly, p2 *MKDecomposedPoly, swk *rlwe.SwitchingKey, r1 *ring.Ring, r2 *ring.Ring) (*ring.Poly, *ring.Poly) {
	res1 := r1.NewPoly() // in R_Q
	res2 := r2.NewPoly() // in R_P

	for i := uint64(0); i < uint64(len(p1.Poly)); i++ {

		r1.MulCoeffsMontgomeryAndAddLvl(level, p1.Poly[i], swk.Value[i][0], res1)
		r2.MulCoeffsMontgomeryAndAdd(p2.Poly[i], swk.Value[i][1], res2)
	}
	return res1, res2

}

// DotSwk combines the dot product of a two switching keys in RQ^d up to q_level and in R_P^d with a switching key, and returns the two results
func DotSwk(level uint64, swk1, swk2 *rlwe.SwitchingKey, r1 *ring.Ring, r2 *ring.Ring, beta uint64) (*ring.Poly, *ring.Poly) {
	res1 := r1.NewPoly() // in R_Q
	res2 := r2.NewPoly() // in R_P

	if len(swk1.Value) != len(swk2.Value) {
		panic("Cannot compute dot product on vectors of different size !")
	}

	for i := uint64(0); i < beta; i++ {

		r1.MulCoeffsMontgomeryAndAddLvl(level, swk1.Value[i][0], swk2.Value[i][0], res1)
		r2.MulCoeffsMontgomeryAndAdd(swk1.Value[i][1], swk2.Value[i][1], res2)
	}
	return res1, res2

}

// MergeSlices merges two slices of uint64 and places the result in s3
// the resulting slice is sorted in ascending order
func MergeSlices(s1, s2 []uint64) []uint64 {

	s3 := make([]uint64, len(s1))

	copy(s3, s1)

	for _, el := range s2 {

		if Contains(s3, el) < 0 {
			s3 = append(s3, el)
		}
	}

	sort.Slice(s3, func(i, j int) bool { return s3[i] < s3[j] })

	return s3
}

// Contains return the element's index if the element is in the slice. -1 otherwise
func Contains(s []uint64, e uint64) int {

	for i, el := range s {
		if el == e {
			return i
		}
	}
	return -1
}

// GetRandomPoly samples a polynomial with a uniform distribution in the given ring
func GetRandomPoly(params *rlwe.Parameters, r *ring.Ring) *ring.Poly {

	prng, err := utils.NewPRNG()
	if err != nil {
		panic(err)
	}

	return GetUniformSampler(params, r, prng).ReadNew()
}

// EqualsSlice returns true if both slices are equal
func EqualsSlice(s1, s2 []uint64) bool {

	if len(s1) != len(s2) {
		return false
	}

	for i, e := range s1 {
		if e != s2[i] {
			return false
		}
	}

	return true
}

// EqualsPoly returns true if both polynomials are equal
func EqualsPoly(p1 *ring.Poly, p2 *ring.Poly) bool {

	if len(p1.Coeffs) != len(p2.Coeffs) {
		return false
	}

	for i, e := range p1.Coeffs {

		if !EqualsSlice(e, p2.Coeffs[i]) {
			return false
		}
	}

	return true
}
