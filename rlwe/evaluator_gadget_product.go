package rlwe

import (
	"github.com/tuneinsight/lattigo/v4/ring"
	"github.com/tuneinsight/lattigo/v4/rlwe/ringqp"
	"github.com/tuneinsight/lattigo/v4/utils"
)

// GadgetProduct evaluates poly x Gadget -> RLWE where
//
// p0 = dot(decomp(cx) * gadget[0]) mod Q
// p1 = dot(decomp(cx) * gadget[1]) mod Q
//
// Expects the flag IsNTT of ct to correctly reflect the domain of cx.
func (eval *Evaluator) GadgetProduct(levelQ int, cx *ring.Poly, gadgetCt GadgetCiphertext, ct *Ciphertext) {

	levelQ = utils.MinInt(levelQ, gadgetCt.LevelQ())
	levelP := gadgetCt.LevelP()

	ctTmp := CiphertextQP{}
	ctTmp.Value = [2]ringqp.Poly{{Q: ct.Value[0], P: eval.BuffQP[1].P}, {Q: ct.Value[1], P: eval.BuffQP[2].P}}
	ctTmp.IsNTT = ct.IsNTT

	if levelP > 0 {
		eval.GadgetProductNoModDown(levelQ, cx, gadgetCt, ctTmp)
	} else {
		eval.GadgetProductSinglePAndBitDecompNoModDown(levelQ, cx, gadgetCt, ctTmp)
	}

	if ct.IsNTT && levelP != -1 {
		eval.BasisExtender.ModDownQPtoQNTT(levelQ, levelP, ct.Value[0], ctTmp.Value[0].P, ct.Value[0])
		eval.BasisExtender.ModDownQPtoQNTT(levelQ, levelP, ct.Value[1], ctTmp.Value[1].P, ct.Value[1])
	} else if !ct.IsNTT {

		ringQ := eval.params.RingQ().AtLevel(levelQ)

		if levelP != -1 {

			ringQ.InvNTTLazy(ct.Value[0], ct.Value[0])
			ringQ.InvNTTLazy(ct.Value[1], ct.Value[1])

			ringP := eval.params.RingP().AtLevel(levelP)

			ringP.InvNTTLazy(ctTmp.Value[0].P, ctTmp.Value[0].P)
			ringP.InvNTTLazy(ctTmp.Value[1].P, ctTmp.Value[1].P)

			eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ct.Value[0], ctTmp.Value[0].P, ct.Value[0])
			eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ct.Value[1], ctTmp.Value[1].P, ct.Value[1])
		} else {
			ringQ.InvNTT(ct.Value[0], ct.Value[0])
			ringQ.InvNTT(ct.Value[1], ct.Value[1])
		}
	}
}

// GadgetProductNoModDown applies the gadget prodcut to the polynomial cx:
//
// ct.Value[0] = dot(decomp(cx) * gadget[0]) mod QP (encrypted input is multiplied by P factor)
// ct.Value[1] = dot(decomp(cx) * gadget[1]) mod QP (encrypted input is multiplied by P factor)
//
// Expects the flag IsNTT of ct to correctly reflect the domain of cx.
func (eval *Evaluator) GadgetProductNoModDown(levelQ int, cx *ring.Poly, gadgetCt GadgetCiphertext, ct CiphertextQP) {

	levelP := gadgetCt.LevelP()

	ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)

	ringQ := ringQP.RingQ
	ringP := ringQP.RingP

	c2QP := eval.BuffQP[0]

	var cxNTT, cxInvNTT *ring.Poly
	if ct.IsNTT {
		cxNTT = cx
		cxInvNTT = eval.BuffInvNTT
		ringQ.InvNTT(cxNTT, cxInvNTT)
	} else {
		cxNTT = eval.BuffInvNTT
		cxInvNTT = cx
		ringQ.NTT(cxInvNTT, cxNTT)
	}

	decompRNS := eval.params.DecompRNS(levelQ, levelP)

	QiOverF := eval.params.QiOverflowMargin(levelQ) >> 1
	PiOverF := eval.params.PiOverflowMargin(levelP) >> 1

	el := gadgetCt.Value

	// Key switching with CRT decomposition for the Qi
	var reduce int
	for i := 0; i < decompRNS; i++ {

		eval.DecomposeSingleNTT(levelQ, levelP, levelP+1, i, cxNTT, cxInvNTT, c2QP.Q, c2QP.P)

		if i == 0 {
			ringQP.MulCoeffsMontgomeryConstant(el[i][0].Value[0], c2QP, ct.Value[0])
			ringQP.MulCoeffsMontgomeryConstant(el[i][0].Value[1], c2QP, ct.Value[1])
		} else {
			ringQP.MulCoeffsMontgomeryConstantAndAddNoMod(el[i][0].Value[0], c2QP, ct.Value[0])
			ringQP.MulCoeffsMontgomeryConstantAndAddNoMod(el[i][0].Value[1], c2QP, ct.Value[1])
		}

		if reduce%QiOverF == QiOverF-1 {
			ringQ.Reduce(ct.Value[0].Q, ct.Value[0].Q)
			ringQ.Reduce(ct.Value[1].Q, ct.Value[1].Q)
		}

		if reduce%PiOverF == PiOverF-1 {
			ringP.Reduce(ct.Value[0].P, ct.Value[0].P)
			ringP.Reduce(ct.Value[1].P, ct.Value[1].P)
		}

		reduce++
	}

	if reduce%QiOverF != 0 {
		ringQ.Reduce(ct.Value[0].Q, ct.Value[0].Q)
		ringQ.Reduce(ct.Value[1].Q, ct.Value[1].Q)
	}

	if reduce%PiOverF != 0 {
		ringP.Reduce(ct.Value[0].P, ct.Value[0].P)
		ringP.Reduce(ct.Value[1].P, ct.Value[1].P)
	}
}

// GadgetProductSinglePAndBitDecompNoModDown applies the key-switch to the polynomial cx:
//
// ct.Value[0] = dot(decomp(cx) * evakey[0]) mod QP (encrypted input is multiplied by P factor)
// ct.Value[1] = dot(decomp(cx) * evakey[1]) mod QP (encrypted input is multiplied by P factor)
//
// Expects the flag IsNTT of ct to correctly reflect the domain of cx.
func (eval *Evaluator) GadgetProductSinglePAndBitDecompNoModDown(levelQ int, cx *ring.Poly, gadgetCt GadgetCiphertext, ct CiphertextQP) {

	levelP := gadgetCt.LevelP()

	ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)

	ringQ := ringQP.RingQ
	ringP := ringQP.RingP

	var cxInvNTT *ring.Poly
	if ct.IsNTT {
		cxInvNTT = eval.BuffInvNTT
		ringQ.InvNTT(cx, cxInvNTT)
	} else {
		cxInvNTT = cx
	}

	decompRNS := eval.params.DecompRNS(levelQ, levelP)
	decompPw2 := eval.params.DecompPw2(levelQ, levelP)

	pw2 := eval.params.pow2Base

	mask := uint64(((1 << pw2) - 1))

	if mask == 0 {
		mask = 0xFFFFFFFFFFFFFFFF
	}

	cw := eval.BuffQP[0].Q.Coeffs[0]
	cwNTT := eval.BuffBitDecomp

	QiOverF := eval.params.QiOverflowMargin(levelQ) >> 1
	PiOverF := eval.params.PiOverflowMargin(levelP) >> 1

	el := gadgetCt.Value

	// Key switching with CRT decomposition for the Qi
	var reduce int
	for i := 0; i < decompRNS; i++ {
		for j := 0; j < decompPw2; j++ {

			ring.MaskVec(cxInvNTT.Coeffs[i], cw, j*pw2, mask)

			if i == 0 && j == 0 {
				for u := 0; u < levelQ+1; u++ {

					Table := ringQ.Tables[u]

					ringQ.NTTSingleLazy(Table, cw, cwNTT)
					ring.MulCoeffsMontgomeryConstantVec(el[i][j].Value[0].Q.Coeffs[u], cwNTT, ct.Value[0].Q.Coeffs[u], Table.Modulus, Table.MRedParams)
					ring.MulCoeffsMontgomeryConstantVec(el[i][j].Value[1].Q.Coeffs[u], cwNTT, ct.Value[1].Q.Coeffs[u], Table.Modulus, Table.MRedParams)
				}

				if ringP != nil {
					for u := 0; u < levelP+1; u++ {

						Table := ringP.Tables[u]

						ringP.NTTSingleLazy(Table, cw, cwNTT)
						ring.MulCoeffsMontgomeryConstantVec(el[i][j].Value[0].P.Coeffs[u], cwNTT, ct.Value[0].P.Coeffs[u], Table.Modulus, Table.MRedParams)
						ring.MulCoeffsMontgomeryConstantVec(el[i][j].Value[1].P.Coeffs[u], cwNTT, ct.Value[1].P.Coeffs[u], Table.Modulus, Table.MRedParams)
					}
				}

			} else {
				for u := 0; u < levelQ+1; u++ {

					Table := ringQ.Tables[u]

					ringQ.NTTSingleLazy(Table, cw, cwNTT)
					ring.MulCoeffsMontgomeryConstantAndAddNoModVec(el[i][j].Value[0].Q.Coeffs[u], cwNTT, ct.Value[0].Q.Coeffs[u], Table.Modulus, Table.MRedParams)
					ring.MulCoeffsMontgomeryConstantAndAddNoModVec(el[i][j].Value[1].Q.Coeffs[u], cwNTT, ct.Value[1].Q.Coeffs[u], Table.Modulus, Table.MRedParams)
				}

				if ringP != nil {
					for u := 0; u < levelP+1; u++ {

						Table := ringP.Tables[u]

						ringP.NTTSingleLazy(Table, cw, cwNTT)
						ring.MulCoeffsMontgomeryConstantAndAddNoModVec(el[i][j].Value[0].P.Coeffs[u], cwNTT, ct.Value[0].P.Coeffs[u], Table.Modulus, Table.MRedParams)
						ring.MulCoeffsMontgomeryConstantAndAddNoModVec(el[i][j].Value[1].P.Coeffs[u], cwNTT, ct.Value[1].P.Coeffs[u], Table.Modulus, Table.MRedParams)
					}
				}
			}

			if reduce%QiOverF == QiOverF-1 {
				ringQ.Reduce(ct.Value[0].Q, ct.Value[0].Q)
				ringQ.Reduce(ct.Value[1].Q, ct.Value[1].Q)
			}

			if reduce%PiOverF == PiOverF-1 {
				ringP.Reduce(ct.Value[0].P, ct.Value[0].P)
				ringP.Reduce(ct.Value[1].P, ct.Value[1].P)
			}

			reduce++
		}
	}

	if reduce%QiOverF != 0 {
		ringQ.Reduce(ct.Value[0].Q, ct.Value[0].Q)
		ringQ.Reduce(ct.Value[1].Q, ct.Value[1].Q)
	}

	if ringP != nil {
		if reduce%PiOverF != 0 {
			ringP.Reduce(ct.Value[0].P, ct.Value[0].P)
			ringP.Reduce(ct.Value[1].P, ct.Value[1].P)
		}
	}

}
