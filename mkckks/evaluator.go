package mkckks

import (
	"sort"

	"github.com/ldsec/lattigo/v2/ckks"
	"github.com/ldsec/lattigo/v2/mkrlwe"
	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/rlwe"
	"github.com/ldsec/lattigo/v2/utils"
)

// MKEvaluator is a wrapper for the ckks evaluator
type MKEvaluator interface {
	Add(c1 *MKCiphertext, c2 *MKCiphertext) *MKCiphertext
	Sub(c1 *MKCiphertext, c2 *MKCiphertext) *MKCiphertext
	AddPlaintext(pt *ckks.Plaintext, c *MKCiphertext) *MKCiphertext
	SubPlaintext(pt *ckks.Plaintext, c *MKCiphertext) *MKCiphertext
	Neg(c *MKCiphertext) *MKCiphertext
	MultPlaintext(pt *ckks.Plaintext, c *MKCiphertext) *MKCiphertext
	Mul(c1 *MKCiphertext, c2 *MKCiphertext) *MKCiphertext
	RelinInPlace(ct *MKCiphertext, evalKeys []*mkrlwe.MKEvaluationKey, publicKeys []*mkrlwe.MKPublicKey)
	Rescale(c *MKCiphertext, out *MKCiphertext)
	Rotate(c *MKCiphertext, n int, keys []*mkrlwe.MKEvalGalKey) *MKCiphertext
	NewPlaintextFromValue([]complex128) *ckks.Plaintext
	DropLevel(ct *MKCiphertext, levels uint64)
	ConvertToMKCiphertext(ct []*ckks.Ciphertext, ids []uint64) []*MKCiphertext
	ConvertToCKKSCiphertext(mkCT *MKCiphertext) []*ckks.Ciphertext
}

type mkEvaluator struct {
	ckksEval        ckks.Evaluator
	params          *ckks.Parameters
	ringQ           *ring.Ring
	ringP           *ring.Ring
	samplerGaussian *ring.GaussianSampler
	convertor       *ring.FastBasisExtender
	encoder         ckks.Encoder
}

// NewMKEvaluator returns an evaluator for the multi key ckks scheme.
func NewMKEvaluator(params *ckks.Parameters) MKEvaluator {

	if params == nil {
		panic("Cannot create evaluator with uninitialized parameters")
	}

	ringQ := mkrlwe.GetRingQ(&params.Parameters)
	ringP := mkrlwe.GetRingP(&params.Parameters)

	prng, err := utils.NewPRNG()
	if err != nil {
		panic(err)
	}

	sampler := mkrlwe.GetGaussianSampler(&params.Parameters, ringQ, prng)
	convertor := ring.NewFastBasisExtender(ringQ, ringP)

	return &mkEvaluator{
		ckksEval:        ckks.NewEvaluator(*params, rlwe.EvaluationKey{}),
		params:          params,
		ringQ:           ringQ,
		ringP:           ringP,
		samplerGaussian: sampler,
		convertor:       convertor,
		encoder:         ckks.NewEncoder(*params),
	}
}

// ConvertToMKCiphertext takes a slice of ckks ciphertexts and their ID and convert them to multi key ciphertexts
// ciphers will be ordered with respect to their IDs (ascending order)
func (eval *mkEvaluator) ConvertToMKCiphertext(ct []*ckks.Ciphertext, ids []uint64) []*MKCiphertext {

	if len(ids) != len(ct) {
		panic("ids and ciphertexts must be of ssame length")
	}

	res := make([]*MKCiphertext, len(ct))

	for i, c := range ct {

		if c.Degree() != 1 {
			panic("Cannot convert ciphertexts of degree different than 1")
		}

		newCipher := new(MKCiphertext)
		newCipher.Ciphertexts = c
		newCipher.PeerID = []uint64{ids[i]}
		res[i] = newCipher
	}

	return res
}

// ConvertToCKKSCiphertext transforms and MKCiphertext into ckks Ciphertexts containing c0 and a ciphertext part.
// ciphers are outputed in ascending id order
func (eval *mkEvaluator) ConvertToCKKSCiphertext(mkCT *MKCiphertext) []*ckks.Ciphertext {

	res := make([]*ckks.Ciphertext, len(mkCT.PeerID))

	c0 := mkCT.Ciphertexts.Value[0]

	for i, v := range mkCT.Ciphertexts.Value {

		if i != 0 {
			newCipher := new(ckks.Ciphertext)
			newCipher.Element = new(ckks.Element)
			newCipher.Element.IsNTT = true
			newCipher.Value = []*ring.Poly{c0, v}
			newCipher.SetScale(mkCT.Ciphertexts.Scale())
			res[i-1] = newCipher
		}
	}

	return res
}

// Add adds the ciphertexts component wise and expend their list of involved peers. A new ciphertext is returned
func (eval *mkEvaluator) Add(c0 *MKCiphertext, c1 *MKCiphertext) *MKCiphertext {

	padded1, padded2 := PadCiphers(c0, c1, eval.params)

	out := NewMKCiphertext(padded1.PeerID, eval.ringQ, eval.params, padded1.Ciphertexts.Level(), c0.Ciphertexts.Scale())
	out.Ciphertexts = eval.AddNew(padded1.Ciphertexts, padded2.Ciphertexts)
	return out
}

// AddNew adds op0 to op1 and returns the result in a newly created element.
func (eval *mkEvaluator) AddNew(op0, op1 ckks.Operand) (ctOut *ckks.Ciphertext) {
	ctOut = eval.newCiphertextBinary(op0, op1)
	eval.AddWithNil(op0, op1, ctOut)
	return
}

// newCiphertextBinary takes two ckks operands  and returns a ckks ciphertext with max degree, max scale, min level
func (eval *mkEvaluator) newCiphertextBinary(op0, op1 ckks.Operand) (ctOut *ckks.Ciphertext) {

	maxDegree := utils.MaxUint64(op0.Degree(), op1.Degree())
	maxScale := utils.MaxFloat64(op0.Scale(), op1.Scale())
	minLevel := utils.MinUint64(op0.Level(), op1.Level())

	return ckks.NewCiphertext(*eval.params, maxDegree, minLevel, maxScale)
}

// Add adds op0 to op1 and returns the result in ctOut.
func (eval *mkEvaluator) AddWithNil(op0, op1 ckks.Operand, ctOut *ckks.Ciphertext) {
	maxDegree := uint64(0)
	isNil := false
	if op0 == nil || op1 == nil {
		isNil = true
		if op0 == nil {
			maxDegree = op1.Degree()
		}
		if op1 == nil {
			maxDegree = op0.Degree()
		}
	}
	if !isNil {
		el0, el1, elOut := eval.getElem(op0, op1, ctOut, utils.MaxUint64(op0.Degree(), op1.Degree()))

		eval.evaluateInPlace(el0, el1, elOut, false, eval.ringQ.AddLvl)
	} else {
		el0, el1, elOut := eval.getElem(op0, op1, ctOut, maxDegree)
		eval.evaluateInPlace(el0, el1, elOut, false, eval.ringQ.AddLvl)
	}
}

// Sub returns the component wise substraction of 2 ciphertexts
func (eval *mkEvaluator) Sub(c0 *MKCiphertext, c1 *MKCiphertext) *MKCiphertext {

	padded1, padded2 := PadCiphers(c0, c1, eval.params)

	out := NewMKCiphertext(padded1.PeerID, eval.ringQ, eval.params, padded1.Ciphertexts.Level(), padded1.Ciphertexts.Scale())

	out.Ciphertexts = eval.SubNew(padded1.Ciphertexts, padded2.Ciphertexts)
	return out
}

// AddPlaintext adds the paintext to the ciphertexts component wise
func (eval *mkEvaluator) AddPlaintext(pt *ckks.Plaintext, c *MKCiphertext) *MKCiphertext {

	out := NewMKCiphertext(c.PeerID, eval.ringQ, eval.params, c.Ciphertexts.Level(), c.Ciphertexts.Scale())

	out.Ciphertexts = eval.AddNew(c.Ciphertexts, pt)

	return out
}

// SubPlaintext subtracts the plaintext from the ciphertext component wise
func (eval *mkEvaluator) SubPlaintext(pt *ckks.Plaintext, c *MKCiphertext) *MKCiphertext {

	out := NewMKCiphertext(c.PeerID, eval.ringQ, eval.params, c.Ciphertexts.Level(), c.Ciphertexts.Scale())

	out.Ciphertexts = eval.SubNew(c.Ciphertexts, pt)

	return out
}

// Neg returns the additive inverse of a cyphertext
func (eval *mkEvaluator) Neg(c *MKCiphertext) *MKCiphertext {

	out := NewMKCiphertext(c.PeerID, eval.ringQ, eval.params, c.Ciphertexts.Level(), c.Ciphertexts.Scale())

	out.Ciphertexts = eval.NegNew(c.Ciphertexts)

	return out
}

// MultPlaintext multiplies a plaintext and a ciphertext
func (eval *mkEvaluator) MultPlaintext(pt *ckks.Plaintext, c *MKCiphertext) *MKCiphertext {

	out := NewMKCiphertext(c.PeerID, eval.ringQ, eval.params, c.Ciphertexts.Level(), c.Ciphertexts.Scale())

	out.Ciphertexts.SetScale(pt.Scale() * c.Ciphertexts.Scale())

	val := make([]*ring.Poly, len(c.PeerID)+1)

	level := utils.MinUint64(c.Ciphertexts.Level(), pt.Level())

	tmp := eval.ringQ.NewPoly()
	eval.ringQ.MFormLvl(level, pt.Value[0], tmp)

	for i, v := range c.Ciphertexts.Value {
		val[i] = eval.ringQ.NewPoly()
		if v == nil {
			val[i] = tmp
		} else {
			eval.ringQ.MulCoeffsMontgomeryLvl(level, tmp, v, val[i])
		}
	}

	out.Ciphertexts.SetValue(val)

	return out
}

// Mul will compute the tensor product and output a ciphertext with degree (k+1)**2
func (eval *mkEvaluator) Mul(c1 *MKCiphertext, c2 *MKCiphertext) *MKCiphertext {

	padded1, padded2 := PadCiphers(c1, c2, eval.params)

	nbrElements := padded1.Ciphertexts.Degree() + 1 // k+1

	outputDegree := nbrElements * nbrElements // (k+1)**2

	el1 := padded1.Ciphertexts
	el2 := padded2.Ciphertexts
	level := utils.MinUint64(el1.Level(), el2.Level())

	out := new(MKCiphertext)
	out.Ciphertexts = new(ckks.Ciphertext)
	out.Ciphertexts.Element = new(ckks.Element)
	out.Ciphertexts.Value = make([]*ring.Poly, outputDegree)
	out.PeerID = padded1.PeerID
	out.Ciphertexts.IsNTT = true

	out.Ciphertexts.SetScale(el1.Scale() * el2.Scale())

	if !el1.IsNTT {
		panic("cannot MulRelin: op0 must be in NTT")
	}

	if !el2.IsNTT {
		panic("cannot MulRelin: op1 must be in NTT")
	}

	ringQ := eval.ringQ

	tmp1 := ringQ.NewPoly()

	resCipher := out.Ciphertexts.Value

	for i, v1 := range el1.Value {

		if v1 != nil {
			ringQ.MFormLvl(level, v1, tmp1)
		}

		for j, v2 := range el2.Value {

			index := int(nbrElements)*i + j
			if v1 != nil && v2 != nil {
				resCipher[index] = ringQ.NewPoly()
				ringQ.MulCoeffsMontgomeryLvl(level, tmp1, v2, resCipher[index])
			}
			if v1 == nil {
				resCipher[index] = nil
			}
			if v2 == nil {
				resCipher[index] = nil
			}

		}
	}

	return out
}

// Relinearize a ciphertext after a multiplication
func (eval *mkEvaluator) RelinInPlace(ct *MKCiphertext, evalKeys []*mkrlwe.MKEvaluationKey, publicKeys []*mkrlwe.MKPublicKey) {

	sort.Slice(evalKeys, func(i, j int) bool { return evalKeys[i].PeerID < evalKeys[j].PeerID })
	sort.Slice(publicKeys, func(i, j int) bool { return publicKeys[i].PeerID < publicKeys[j].PeerID })

	checkParticipantsEvalKey(ct.PeerID, evalKeys)
	checkParticipantsPubKey(ct.PeerID, publicKeys)
	mkrlwe.Relinearization(evalKeys, publicKeys, &ct.Ciphertexts.Value, &eval.params.Parameters, ct.Ciphertexts.Level())
}

// Rescale takes a ciphertext at level l reduces it until it reaches its original
// this function is the same as in ckks/evaluator.go
func (eval *mkEvaluator) Rescale(c *MKCiphertext, out *MKCiphertext) {

	eval.ckksEval.Rescale(c.Ciphertexts, eval.params.Scale(), c.Ciphertexts)
}

// DropLevel drops the level of the given ciphertext by levels. No rescaling is applied
func (eval *mkEvaluator) DropLevel(ct *MKCiphertext, levels uint64) {

	eval.ckksEval.DropLevel(ct.Ciphertexts, levels)
}

// Rotate rotate the columns of the ciphertext by n to the left and return the result in a new ciphertext
func (eval *mkEvaluator) Rotate(c *MKCiphertext, n int, keys []*mkrlwe.MKEvalGalKey) *MKCiphertext {

	sort.Slice(keys, func(i, j int) bool { return keys[i].PeerID < keys[j].PeerID })

	checkParticipantsGalKey(c.PeerID, keys)

	out := NewMKCiphertext(c.PeerID, eval.ringQ, eval.params, c.Ciphertexts.Level(), c.Ciphertexts.Scale())

	galEl := eval.params.GaloisElementForColumnRotationBy(n)

	level := c.Ciphertexts.Level()

	ringQP := mkrlwe.GetRingQP(&eval.params.Parameters)

	k := uint64(len(c.PeerID))

	res := make([]*ring.Poly, k+1)

	restmpQ := make([]*ring.Poly, k+1)
	restmpP := make([]*ring.Poly, k+1)
	restmpQ[0] = eval.ringQ.NewPoly()
	restmpP[0] = eval.ringP.NewPoly()

	for i := uint64(0); i < k+1; i++ {
		res[i] = eval.ringQ.NewPoly()
	}

	gal0Q := mkrlwe.NewDecomposedPoly(eval.ringQ, eval.params.Beta())
	gal0P := mkrlwe.NewDecomposedPoly(eval.ringP, eval.params.Beta())
	gal1Q := mkrlwe.NewDecomposedPoly(eval.ringQ, eval.params.Beta())
	gal1P := mkrlwe.NewDecomposedPoly(eval.ringP, eval.params.Beta())

	for i := uint64(1); i <= k; i++ {

		prepareGaloisEvaluationKey(i, level, uint64(len(eval.ringQ.Modulus)), eval.params.Beta(), keys, gal0Q, gal0P, gal1Q, gal1P)

		permutedCipher := eval.ringQ.NewPoly() // apply rotation to the ciphertext
		index := ring.PermuteNTTIndex(galEl, ringQP.N)
		ring.PermuteNTTWithIndexLvl(level, c.Ciphertexts.Value[i], index, permutedCipher)

		decomposedPermutedQ, decomposedPermutedP := mkrlwe.GInverse(permutedCipher, &eval.params.Parameters, level)

		res0P := mkrlwe.Dot(decomposedPermutedP, gal0P, eval.ringP) // dot product and add in c0''
		res0Q := mkrlwe.DotLvl(level, decomposedPermutedQ, gal0Q, eval.ringQ)

		eval.ringP.Add(restmpP[0], res0P, restmpP[0])
		eval.ringQ.AddLvl(level, restmpQ[0], res0Q, restmpQ[0])

		restmpP[i] = mkrlwe.Dot(decomposedPermutedP, gal1P, eval.ringP) // dot product and put in ci''
		restmpQ[i] = mkrlwe.DotLvl(level, decomposedPermutedQ, gal1Q, eval.ringQ)

	}

	// finalize computation of c0'
	index := ring.PermuteNTTIndex(galEl, ringQP.N)
	ring.PermuteNTTWithIndexLvl(level, c.Ciphertexts.Value[0], index, res[0])

	tmpModDown := eval.ringQ.NewPoly()
	eval.convertor.ModDownSplitNTTPQ(level, restmpQ[0], restmpP[0], tmpModDown)
	eval.ringQ.AddLvl(level, res[0], tmpModDown, res[0])

	// finalize computation of ci'
	for i := uint64(1); i <= k; i++ {

		eval.convertor.ModDownSplitNTTPQ(level, restmpQ[i], restmpP[i], tmpModDown)
		eval.ringQ.CopyLvl(level, tmpModDown, res[i])
	}

	out.Ciphertexts.SetValue(res)

	return out
}

// prepare galois evaluation keys for operations in split crt basis
func prepareGaloisEvaluationKey(j, level, modulus, beta uint64, galKeys []*mkrlwe.MKEvalGalKey, gal0Q, gal0P, gal1Q, gal1P *mkrlwe.MKDecomposedPoly) {

	for u := uint64(0); u < beta; u++ {

		gal0Q.Poly[u].Coeffs = galKeys[j-1].Key[0].Poly[u].Coeffs[:level+1]

		gal0P.Poly[u].Coeffs = galKeys[j-1].Key[0].Poly[u].Coeffs[modulus:]

		gal1Q.Poly[u].Coeffs = galKeys[j-1].Key[1].Poly[u].Coeffs[:level+1]

		gal1P.Poly[u].Coeffs = galKeys[j-1].Key[1].Poly[u].Coeffs[modulus:]

	}

}

// NewPlaintextFromValue returns a plaintext from the provided values
func (eval *mkEvaluator) NewPlaintextFromValue(values []complex128) *ckks.Plaintext {

	return eval.encoder.EncodeNTTAtLvlNew(eval.params.MaxLevel(), values, eval.params.LogSlots())
}

func getCiphertextIndex(peerID uint64, ct *MKCiphertext) uint64 {

	for i, id := range ct.PeerID {

		if id == peerID {
			return uint64(i + 1)
		}
	}

	return 0
}

func checkParticipantsEvalKey(peerID []uint64, evalKeys []*mkrlwe.MKEvaluationKey) {

	for i, id := range peerID {
		if id != evalKeys[i].PeerID {
			panic("Incorrect evaluation keys for the given ciphertexts")
		}
	}
}

func checkParticipantsPubKey(peerID []uint64, pubKeys []*mkrlwe.MKPublicKey) {

	for i, id := range peerID {
		if id != pubKeys[i].PeerID {
			panic("Incorrect public keys for the given ciphertexts")
		}
	}
}

func checkParticipantsGalKey(peerID []uint64, galKeys []*mkrlwe.MKEvalGalKey) {

	for i, id := range peerID {
		if id != galKeys[i].PeerID {
			panic("Incorrect galois evaluation keys for the given ciphertexts")
		}
	}
}

func (eval *mkEvaluator) getElem(op0, op1, opOut ckks.Operand, opOutMinDegree uint64) (el0, el1, elOut *ckks.Element) {

	return mkrlwe.GetThreeElementsckksWithNil(op0, op1, opOut)
}

// evaluateInPlace performs an evaluation of the evaluate function between c0 and c1, and stores the result into ctOut by optimizing the computation if there are nils. Subtraction needs to be handled ad-hoc and is indicated with isSub bool
func (eval *mkEvaluator) evaluateInPlace(c0, c1, ctOut *ckks.Element, isSub bool, evaluate func(uint64, *ring.Poly, *ring.Poly, *ring.Poly)) {

	if c0 == nil && c1 == nil {
		ctOut = nil
		return
	}

	if c0 == nil {
		ctOut = c1
	}

	if c1 == nil {
		ctOut = c0
	}

	var tmp0, tmp1 *ckks.Element
	level := utils.MinUint64(utils.MinUint64(c0.Level(), c1.Level()), ctOut.Level())

	maxDegree := utils.MaxUint64(c0.Degree(), c1.Degree())
	minDegree := utils.MinUint64(c0.Degree(), c1.Degree())

	// Else resizes the receiver element
	ctOut.Resize(*eval.params, maxDegree)
	eval.ckksEval.DropLevel(&ckks.Ciphertext{Element: ctOut}, ctOut.Level()-utils.MinUint64(c0.Level(), c1.Level()))
	// Checks whether or not the receiver element is the same as one of the input elements
	// and acts accordingly to avoid unnecessary element creation or element overwriting,
	// and scales properly the element before the evaluation.
	if ctOut == c0 {

		if c0.Scale() > c1.Scale() {
			tmp1 = new(ckks.Element)

			if uint64(c0.Scale()/c1.Scale()) != 0 {
				eval.ckksEval.MultByConst(&ckks.Ciphertext{Element: c1}, uint64(c0.Scale()/c1.Scale()), &ckks.Ciphertext{Element: tmp1})
			}

		} else if c1.Scale() > c0.Scale() {

			if uint64(c1.Scale()/c0.Scale()) != 0 {
				eval.ckksEval.MultByConst(&ckks.Ciphertext{Element: c0}, uint64(c1.Scale()/c0.Scale()), &ckks.Ciphertext{Element: c0})
			}
			c0.SetScale(c1.Scale())

			tmp1 = c1

		} else {
			tmp1 = c1
		}

		tmp0 = c0

	} else if ctOut == c1 {
		if c1.Scale() > c0.Scale() {

			tmp0 = new(ckks.Element)
			if uint64(c1.Scale()/c0.Scale()) != 0 {
				eval.ckksEval.MultByConst(&ckks.Ciphertext{Element: c0}, uint64(c1.Scale()/c0.Scale()), &ckks.Ciphertext{Element: tmp0})
			}
		} else if c0.Scale() > c1.Scale() {

			if uint64(c0.Scale()/c1.Scale()) != 0 {
				eval.ckksEval.MultByConst(&ckks.Ciphertext{Element: c1}, uint64(c0.Scale()/c1.Scale()), &ckks.Ciphertext{Element: ctOut})
			}

			ctOut.SetScale(c0.Scale())
			tmp0 = c0

		} else {
			tmp0 = c0
		}

		tmp1 = c1

	} else {
		if c1.Scale() > c0.Scale() {
			tmp0 = new(ckks.Element)

			if uint64(c1.Scale()/c0.Scale()) != 0 {
				eval.ckksEval.MultByConst(&ckks.Ciphertext{Element: c0}, uint64(c1.Scale()/c0.Scale()), &ckks.Ciphertext{Element: tmp0})
			}

			tmp1 = c1

		} else if c0.Scale() > c1.Scale() {
			tmp1 = new(ckks.Element)

			if uint64(c0.Scale()/c1.Scale()) != 0 {
				eval.ckksEval.MultByConst(&ckks.Ciphertext{Element: c1}, uint64(c0.Scale()/c1.Scale()), &ckks.Ciphertext{Element: tmp1})
			}

			tmp0 = c0

		} else {
			tmp0 = c0
			tmp1 = c1
		}
	}

	for i := uint64(0); i < minDegree+1; i++ {

		if tmp0.Value[i] == nil && tmp1.Value[i] == nil {
			ctOut.Value[i] = nil
		}
		if tmp0.Value[i] == nil && tmp1.Value[i] != nil {

			if !isSub {
				ctOut.Value[i] = tmp1.Value[i]
			} else {
				eval.ringQ.Neg(tmp1.Value[i], ctOut.Value[i])
			}
		}
		if tmp0.Value[i] != nil && tmp1.Value[i] == nil {
			ctOut.Value[i] = tmp0.Value[i]
		}
		if tmp0.Value[i] != nil && tmp1.Value[i] != nil {
			evaluate(level, tmp0.Value[i], tmp1.Value[i], ctOut.Value[i])
		}
	}

	ctOut.SetScale(utils.MaxFloat64(c0.Scale(), c1.Scale()))

	// If the inputs degrees differ, it copies the remaining degree on the receiver.
	// Also checks that the receiver is not one of the inputs to avoid unnecessary work.

	if c0.Degree() > c1.Degree() && tmp0 != ctOut {
		for i := minDegree + 1; i < maxDegree+1; i++ {
			eval.ringQ.CopyLvl(level, tmp0.Value[i], ctOut.Value[i])
		}
	} else if c1.Degree() > c0.Degree() && tmp1 != ctOut {
		for i := minDegree + 1; i < maxDegree+1; i++ {
			eval.ringQ.CopyLvl(level, tmp1.Value[i], ctOut.Value[i])
		}
	}
}

// SubNew subtracts op1 from op0 and returns the result in a newly created element.
func (eval *mkEvaluator) SubNew(op0, op1 ckks.Operand) (ctOut *ckks.Ciphertext) {
	ctOut = eval.newCiphertextBinary(op0, op1)
	eval.SubWithNil(op0, op1, ctOut)
	return
}

// SubWithNil subtracts op1 from op0 and returns the result in ctOut.
func (eval *mkEvaluator) SubWithNil(op0, op1 ckks.Operand, ctOut *ckks.Ciphertext) {

	el0, el1, elOut := eval.getElem(op0, op1, ctOut, utils.MaxUint64(op0.Degree(), op1.Degree()))

	eval.evaluateInPlace(el0, el1, elOut, true, eval.ringQ.SubLvl)
	level := uint64(0)
	if el0 == nil || el1 == nil || elOut == nil {
		level = mkrlwe.MinLevelWithNil(el0, el1, elOut)
	} else {
		level = utils.MinUint64(utils.MinUint64(el0.Level(), el1.Level()), elOut.Level())
		if el0.Degree() < el1.Degree() {
			for i := el0.Degree() + 1; i < el1.Degree()+1; i++ {
				eval.ringQ.NegLvl(level, elOut.Value[i], elOut.Value[i])
			}
		}
	}

}

// NegNew negates ct0 and returns the result in a newly created element.
func (eval *mkEvaluator) NegNew(ct0 *ckks.Ciphertext) (ctOut *ckks.Ciphertext) {
	if ct0 == nil {
		return nil
	}
	ctOut = ckks.NewCiphertext(*eval.params, ct0.Degree(), ct0.Level(), ct0.Scale())
	eval.NegWithNil(ct0, ctOut)
	return
}

// Neg negates the value of ct0 and returns the result in ctOut.
func (eval *mkEvaluator) NegWithNil(ct0 *ckks.Ciphertext, ctOut *ckks.Ciphertext) {

	level := utils.MinUint64(ct0.Level(), ctOut.Level())

	if ct0.Degree() != ctOut.Degree() {
		panic("cannot Negate: invalid receiver Ciphertext does not match input Ciphertext degree")
	}

	for i := range ct0.Value {
		eval.ringQ.NegLvl(level, ct0.Value[i], ctOut.Value[i])
	}
}
