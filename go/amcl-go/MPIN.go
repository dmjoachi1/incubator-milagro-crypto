/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

/* MPIN API Functions */

package amcl

import "time"

import "fmt"

/* Configure mode of operation */

const PERMITS bool = true
const PINERROR bool = true
const FULL bool = true
const SINGLE_PASS bool = false

const MPIN_EFS int = int(MODBYTES)
const MPIN_EGS int = int(MODBYTES)
const MPIN_PAS int = 16
const MPIN_BAD_PARAMS int = -11
const MPIN_INVALID_POINT int = -14
const MPIN_WRONG_ORDER int = -18
const MPIN_BAD_PIN int = -19

/* Configure your PIN here */

const MPIN_MAXPIN int32 = 10000 /* PIN less than this */
const MPIN_PBLEN int32 = 14     /* Number of bits in PIN */
const MPIN_TS int = 10          /* 10 for 4 digit PIN, 14 for 6-digit PIN - 2^TS/TS approx = sqrt(MAXPIN) */
const MPIN_TRAP int = 200       /* 200 for 4 digit PIN, 2000 for 6-digit PIN  - approx 2*sqrt(MAXPIN) */

/* Hash number (optional) and string to point on curve */

func Hashit(n int32, ID []byte) []byte {
	H := NewHASH()
	if n != 0 {
		H.Process_num(n)
	}
	H.Process_array(ID)
	h := H.Hash()
	return h[:]
}

func mapit(h []byte) *ECP {
	q := NewBIGints(Modulus)
	x := fromBytes(h[:])
	x.mod(q)
	var P *ECP
	for true {
		P = NewECPbigint(x, 0)
		if !P.is_infinity() {
			break
		}
		x.inc(1)
		x.norm()
	}
	return P
}

/* needed for SOK */
func mapit2(h []byte) *ECP2 {
	q := NewBIGints(Modulus)
	x := fromBytes(h[:])
	one := NewBIGint(1)
	var X *FP2
	var Q, T, K *ECP2
	x.mod(q)
	for true {
		X = NewFP2bigs(one, x)
		Q = NewECP2fp2(X)
		if !Q.is_infinity() {
			break
		}
		x.inc(1)
		x.norm()
	}
	/* Fast Hashing to G2 - Fuentes-Castaneda, Knapp and Rodriguez-Henriquez */
	Fra := NewBIGints(CURVE_Fra)
	Frb := NewBIGints(CURVE_Frb)
	X = NewFP2bigs(Fra, Frb)
	x = NewBIGints(CURVE_Bnx)

	T = NewECP2()
	T.copy(Q)
	T.mul(x)
	T.neg()
	K = NewECP2()
	K.copy(T)
	K.dbl()
	K.add(T)
	K.affine()

	K.frob(X)
	Q.frob(X)
	Q.frob(X)
	Q.frob(X)
	Q.add(T)
	Q.add(K)
	T.frob(X)
	T.frob(X)
	Q.add(T)
	Q.affine()
	return Q
}

/* return time in slots since epoch */
func MPIN_today() int {
	now := time.Now()
	return int(now.Unix()) / (60 * 1440)
}

/* these next two functions help to implement elligator squared - http://eprint.iacr.org/2014/043 */
/* maps a random u to a point on the curve */
func emap(u *BIG, cb int) *ECP {
	var P *ECP
	x := NewBIGcopy(u)
	p := NewBIGints(Modulus)
	x.mod(p)
	for true {
		P = NewECPbigint(x, cb)
		if !P.is_infinity() {
			break
		}
		x.inc(1)
		x.norm()
	}
	return P
}

/* returns u derived from P. Random value in range 1 to return value should then be added to u */
func unmap(u *BIG, P *ECP) int {
	s := P.getS()
	var R *ECP
	r := 0
	x := P.getX()
	u.copy(x)
	for true {
		u.dec(1)
		u.norm()
		r++
		R = NewECPbigint(u, s)
		if !R.is_infinity() {
			break
		}
	}
	return r
}

func MPIN_HASH_ID(ID []byte) []byte {
	return Hashit(0, ID)
}

/* these next two functions implement elligator squared - http://eprint.iacr.org/2014/043 */
/* Elliptic curve point E in format (0x04,x,y} is converted to form {0x0-,u,v} */
/* Note that u and v are indistinguisible from random strings */
func MPIN_ENCODING(rng *RAND, E []byte) int {
	var T [MPIN_EFS]byte

	for i := 0; i < MPIN_EFS; i++ {
		T[i] = E[i+1]
	}
	u := fromBytes(T[:])
	for i := 0; i < MPIN_EFS; i++ {
		T[i] = E[i+MPIN_EFS+1]
	}
	v := fromBytes(T[:])

	P := NewECPbigs(u, v)
	if P.is_infinity() {
		return MPIN_INVALID_POINT
	}

	p := NewBIGints(Modulus)
	u = randomnum(p, rng)

	su := int(rng.GetByte())
	su %= 2

	W := emap(u, su)
	P.sub(W)
	sv := P.getS()
	rn := unmap(v, P)
	m := int(rng.GetByte())
	m %= rn
	v.inc(m + 1)
	E[0] = byte(su + 2*sv)
	u.toBytes(T[:])
	for i := 0; i < MPIN_EFS; i++ {
		E[i+1] = T[i]
	}
	v.toBytes(T[:])
	for i := 0; i < MPIN_EFS; i++ {
		E[i+MPIN_EFS+1] = T[i]
	}

	return 0
}

func MPIN_DECODING(D []byte) int {
	var T [MPIN_EFS]byte

	if (D[0] & 0x04) != 0 {
		return MPIN_INVALID_POINT
	}

	for i := 0; i < MPIN_EFS; i++ {
		T[i] = D[i+1]
	}
	u := fromBytes(T[:])
	for i := 0; i < MPIN_EFS; i++ {
		T[i] = D[i+MPIN_EFS+1]
	}
	v := fromBytes(T[:])

	su := int(D[0] & 1)
	sv := int((D[0] >> 1) & 1)
	W := emap(u, su)
	P := emap(v, sv)
	P.add(W)
	u = P.getX()
	v = P.getY()
	D[0] = 0x04
	u.toBytes(T[:])
	for i := 0; i < MPIN_EFS; i++ {
		D[i+1] = T[i]
	}
	v.toBytes(T[:])
	for i := 0; i < MPIN_EFS; i++ {
		D[i+MPIN_EFS+1] = T[i]
	}

	return 0
}

/* R=R1+R2 in group G1 */
func MPIN_RECOMBINE_G1(R1 []byte, R2 []byte, R []byte) int {
	P := ECP_fromBytes(R1)
	Q := ECP_fromBytes(R2)

	if P.is_infinity() || Q.is_infinity() {
		return MPIN_INVALID_POINT
	}

	P.add(Q)

	P.toBytes(R[:])
	return 0
}

/* W=W1+W2 in group G2 */
func MPIN_RECOMBINE_G2(W1 []byte, W2 []byte, W []byte) int {
	P := ECP2_fromBytes(W1)
	Q := ECP2_fromBytes(W2)

	if P.is_infinity() || Q.is_infinity() {
		return MPIN_INVALID_POINT
	}

	P.add(Q)

	P.toBytes(W)
	return 0
}

/* create random secret S */
func MPIN_RANDOM_GENERATE(rng *RAND, S []byte) int {
	r := NewBIGints(CURVE_Order)
	s := randomnum(r, rng)

	s.toBytes(S)
	return 0
}

/* Extract PIN from TOKEN for identity CID */
func MPIN_EXTRACT_PIN(CID []byte, pin int, TOKEN []byte) int {
	P := ECP_fromBytes(TOKEN)
	if P.is_infinity() {
		return MPIN_INVALID_POINT
	}
	h := Hashit(0, CID)
	R := mapit(h)

	R = R.pinmul(int32(pin)%MPIN_MAXPIN, MPIN_PBLEN)
	P.sub(R)

	P.toBytes(TOKEN)

	return 0
}

/* Implement step 2 on client side of MPin protocol */
func MPIN_CLIENT_2(X []byte, Y []byte, SEC []byte) int {
	r := NewBIGints(CURVE_Order)
	P := ECP_fromBytes(SEC)
	if P.is_infinity() {
		return MPIN_INVALID_POINT
	}

	px := fromBytes(X)
	py := fromBytes(Y)
	px.add(py)
	px.mod(r)
	px.rsub(r)

	G1mul(P, px).toBytes(SEC)
	return 0
}

/* Implement step 1 on client side of MPin protocol */
func MPIN_CLIENT_1(date int, CLIENT_ID []byte, rng *RAND, X []byte, pin int, TOKEN []byte, SEC []byte, xID []byte, xCID []byte, PERMIT []byte) int {
	r := NewBIGints(CURVE_Order)

	var x *BIG
	if rng != nil {
		x = randomnum(r, rng)
		x.toBytes(X)
	} else {
		x = fromBytes(X)
	}

	h := Hashit(0, CLIENT_ID)
	P := mapit(h)

	T := ECP_fromBytes(TOKEN)
	if T.is_infinity() {
		return MPIN_INVALID_POINT
	}

	W := P.pinmul(int32(pin)%MPIN_MAXPIN, MPIN_PBLEN)
	T.add(W)
	if date != 0 {
		W = ECP_fromBytes(PERMIT)
		if W.is_infinity() {
			return MPIN_INVALID_POINT
		}
		T.add(W)
		h = Hashit(int32(date), h)
		W = mapit(h)
		if xID != nil {
			P = G1mul(P, x)
			P.toBytes(xID)
			W = G1mul(W, x)
			P.add(W)
		} else {
			P.add(W)
			P = G1mul(P, x)
		}
		if xCID != nil {
			P.toBytes(xCID)
		}
	} else {
		if xID != nil {
			P = G1mul(P, x)
			P.toBytes(xID)
		}
	}

	T.toBytes(SEC)
	return 0
}

/* Extract Server Secret SST=S*Q where Q is fixed generator in G2 and S is master secret */
func MPIN_GET_SERVER_SECRET(S []byte, SST []byte) int {
	Q := NewECP2fp2s(NewFP2bigs(NewBIGints(CURVE_Pxa), NewBIGints(CURVE_Pxb)), NewFP2bigs(NewBIGints(CURVE_Pya), NewBIGints(CURVE_Pyb)))

	s := fromBytes(S)
	Q = G2mul(Q, s)
	Q.toBytes(SST)
	return 0
}

/*
 W=x*H(G);
 if RNG == NULL then X is passed in
 if RNG != NULL the X is passed out
 if type=0 W=x*G where G is point on the curve, else W=x*M(G), where M(G) is mapping of octet G to point on the curve
*/
func MPIN_GET_G1_MULTIPLE(rng *RAND, typ int, X []byte, G []byte, W []byte) int {
	var x *BIG
	r := NewBIGints(CURVE_Order)
	if rng != nil {
		x = randomnum(r, rng)
		x.toBytes(X)
	} else {
		x = fromBytes(X)
	}
	var P *ECP
	if typ == 0 {
		P = ECP_fromBytes(G)
		if P.is_infinity() {
			return MPIN_INVALID_POINT
		}
	} else {
		P = mapit(G)
	}

	G1mul(P, x).toBytes(W)
	return 0
}

/* Client secret CST=S*H(CID) where CID is client ID and S is master secret */
/* CID is hashed externally */
func MPIN_GET_CLIENT_SECRET(S []byte, CID []byte, CST []byte) int {
	return MPIN_GET_G1_MULTIPLE(nil, 1, S, CID, CST)
}

/* Time Permit CTT=S*(date|H(CID)) where S is master secret */
func MPIN_GET_CLIENT_PERMIT(date int, S []byte, CID []byte, CTT []byte) int {
	h := Hashit(int32(date), CID)
	P := mapit(h)

	s := fromBytes(S)
	G1mul(P, s).toBytes(CTT)
	return 0
}

/* Outputs H(CID) and H(T|H(CID)) for time permits. If no time permits set HID=HTID */
func MPIN_SERVER_1(date int, CID []byte, HID []byte, HTID []byte) {
	h := Hashit(0, CID)
	P := mapit(h)

	if date != 0 {
		if HID != nil {
			P.toBytes(HID)
		}
		h = Hashit(int32(date), h)
		R := mapit(h)
		P.add(R)
		P.toBytes(HTID)
	} else {
		P.toBytes(HID)
	}
}

/* Implement step 2 of MPin protocol on server side */
func MPIN_SERVER_2(date int, HID []byte, HTID []byte, Y []byte, SST []byte, xID []byte, xCID []byte, mSEC []byte, E []byte, F []byte) int {
	//	q:=NewBIGints(Modulus)
	Q := NewECP2fp2s(NewFP2bigs(NewBIGints(CURVE_Pxa), NewBIGints(CURVE_Pxb)), NewFP2bigs(NewBIGints(CURVE_Pya), NewBIGints(CURVE_Pyb)))

	sQ := ECP2_fromBytes(SST)
	if sQ.is_infinity() {
		return MPIN_INVALID_POINT
	}

	var R *ECP
	if date != 0 {
		R = ECP_fromBytes(xCID)
	} else {
		if xID == nil {
			return MPIN_BAD_PARAMS
		}
		R = ECP_fromBytes(xID)
	}
	if R.is_infinity() {
		return MPIN_INVALID_POINT
	}

	y := fromBytes(Y)
	var P *ECP
	if date != 0 {
		P = ECP_fromBytes(HTID)
	} else {
		if HID == nil {
			return MPIN_BAD_PARAMS
		}
		P = ECP_fromBytes(HID)
	}

	if P.is_infinity() {
		return MPIN_INVALID_POINT
	}

	P = G1mul(P, y)
	P.add(R)
	R = ECP_fromBytes(mSEC)
	if R.is_infinity() {
		return MPIN_INVALID_POINT
	}

	var g *FP12
	//		FP12 g1=new FP12(0);

	g = ate2(Q, R, sQ, P)
	g = fexp(g)

	if !g.isunity() {
		if HID != nil && xID != nil && E != nil && F != nil {
			g.toBytes(E)
			if date != 0 {
				P = ECP_fromBytes(HID)
				if P.is_infinity() {
					return MPIN_INVALID_POINT
				}
				R = ECP_fromBytes(xID)
				if R.is_infinity() {
					return MPIN_INVALID_POINT
				}

				P = G1mul(P, y)
				P.add(R)
			}
			g = ate(Q, P)
			g = fexp(g)
			g.toBytes(F)
		}
		return MPIN_BAD_PIN
	}

	return 0
}

/* Pollards kangaroos used to return PIN error */
func MPIN_KANGAROO(E []byte, F []byte) int {
	ge := FP12_fromBytes(E)
	gf := FP12_fromBytes(F)
	var distance [MPIN_TS]int
	t := NewFP12copy(gf)

	var table []*FP12
	var i int
	s := 1
	for m := 0; m < MPIN_TS; m++ {
		distance[m] = s
		table = append(table, NewFP12copy(t))
		s *= 2
		t.usqr()
	}
	t.one()
	dn := 0
	for j := 0; j < MPIN_TRAP; j++ {
		i = t.geta().geta().getA().lastbits(8) % MPIN_TS
		t.mul(table[i])
		dn += distance[i]
	}
	gf.copy(t)
	gf.conj()
	steps := 0
	dm := 0
	res := 0
	for dm-dn < int(MPIN_MAXPIN) {
		steps++
		if steps > 4*MPIN_TRAP {
			break
		}
		i = ge.geta().geta().getA().lastbits(8) % MPIN_TS
		ge.mul(table[i])
		dm += distance[i]
		if ge.equals(t) {
			res = dm - dn
			break
		}
		if ge.equals(gf) {
			res = dn - dm
			break
		}

	}
	if steps > 4*MPIN_TRAP || dm-dn >= int(MPIN_MAXPIN) {
		res = 0
	} // Trap Failed  - probable invalid token
	return int(res)
}

/* Functions to support M-Pin Full */

func MPIN_PRECOMPUTE(TOKEN []byte, CID []byte, G1 []byte, G2 []byte) int {
	var P, T *ECP
	var g *FP12

	T = ECP_fromBytes(TOKEN)
	if T.is_infinity() {
		return MPIN_INVALID_POINT
	}

	P = mapit(CID)

	Q := NewECP2fp2s(NewFP2bigs(NewBIGints(CURVE_Pxa), NewBIGints(CURVE_Pxb)), NewFP2bigs(NewBIGints(CURVE_Pya), NewBIGints(CURVE_Pyb)))

	g = ate(Q, T)
	g = fexp(g)
	g.toBytes(G1)

	g = ate(Q, P)
	g = fexp(g)
	g.toBytes(G2)

	return 0
}

/* calculate common key on client side */
/* wCID = w.(A+AT) */
func MPIN_CLIENT_KEY(G1 []byte, G2 []byte, pin int, R []byte, X []byte, wCID []byte, CK []byte) int {
	H := NewHASH()
	var t [MPIN_EFS]byte

	g1 := FP12_fromBytes(G1)
	g2 := FP12_fromBytes(G2)
	z := fromBytes(R)
	x := fromBytes(X)

	W := ECP_fromBytes(wCID)
	if W.is_infinity() {
		return MPIN_INVALID_POINT
	}

	W = G1mul(W, x)

	f := NewFP2bigs(NewBIGints(CURVE_Fra), NewBIGints(CURVE_Frb))
	r := NewBIGints(CURVE_Order)
	q := NewBIGints(Modulus)

	m := NewBIGcopy(q)
	m.mod(r)

	a := NewBIGcopy(z)
	a.mod(m)

	b := NewBIGcopy(z)
	b.div(m)

	g2.pinpow(pin, int(MPIN_PBLEN))
	g1.mul(g2)

	c := g1.trace()
	g2.copy(g1)
	g2.frob(f)
	cp := g2.trace()
	g1.conj()
	g2.mul(g1)
	cpm1 := g2.trace()
	g2.mul(g1)
	cpm2 := g2.trace()

	c = c.xtr_pow2(cp, cpm1, cpm2, a, b)

	c.geta().getA().toBytes(t[:])
	H.Process_array(t[:])
	c.geta().getB().toBytes(t[:])
	H.Process_array(t[:])
	c.getb().getA().toBytes(t[:])
	H.Process_array(t[:])
	c.getb().getB().toBytes(t[:])
	H.Process_array(t[:])

	W.getX().toBytes(t[:])
	H.Process_array(t[:])
	W.getY().toBytes(t[:])
	H.Process_array(t[:])

	t = H.Hash()
	for i := 0; i < MPIN_PAS; i++ {
		CK[i] = t[i]
	}

	return 0
}

/* calculate common key on server side */
/* Z=r.A - no time permits involved */

func MPIN_SERVER_KEY(Z []byte, SST []byte, W []byte, xID []byte, xCID []byte, SK []byte) int {
	H := NewHASH()
	var t [MPIN_EFS]byte

	sQ := ECP2_fromBytes(SST)
	if sQ.is_infinity() {
		return MPIN_INVALID_POINT
	}
	R := ECP_fromBytes(Z)
	if R.is_infinity() {
		return MPIN_INVALID_POINT
	}

	var U *ECP
	if xCID != nil {
		U = ECP_fromBytes(xCID)
	} else {
		U = ECP_fromBytes(xID)
	}
	if U.is_infinity() {
		return MPIN_INVALID_POINT
	}

	w := fromBytes(W)
	U = G1mul(U, w)
	g := ate(sQ, R)
	g = fexp(g)

	c := g.trace()
	c.geta().getA().toBytes(t[:])
	H.Process_array(t[:])
	c.geta().getB().toBytes(t[:])
	H.Process_array(t[:])
	c.getb().getA().toBytes(t[:])
	H.Process_array(t[:])
	c.getb().getB().toBytes(t[:])
	H.Process_array(t[:])

	U.getX().toBytes(t[:])
	H.Process_array(t[:])
	U.getY().toBytes(t[:])
	H.Process_array(t[:])

	t = H.Hash()
	for i := 0; i < MPIN_PAS; i++ {
		SK[i] = t[i]
	}

	return 0
}

/* return time since epoch */
func MPIN_GET_TIME() int {
	now := time.Now()
	return int(now.Unix())
}

/* Generate Y = H(epoch, xCID/xID) */
func MPIN_GET_Y(TimeValue int, xCID []byte, Y []byte) {
	h := Hashit(int32(TimeValue), xCID)
	y := fromBytes(h)
	q := NewBIGints(CURVE_Order)
	y.mod(q)
	y.toBytes(Y)
}

/* One pass MPIN Client */
func MPIN_CLIENT(date int, CLIENT_ID []byte, RNG *RAND, X []byte, pin int, TOKEN []byte, SEC []byte, xID []byte, xCID []byte, PERMIT []byte, MESSAGE []byte, TimeValue int, Y []byte) int {
	rtn := 0

	var M []byte
	if date == 0 {
		M = xID
	} else {
		M = xCID
	}

	rtn = MPIN_CLIENT_1(date, CLIENT_ID, RNG, X, pin, TOKEN, SEC, xID, xCID, PERMIT)
	if rtn != 0 {
		return rtn
	}

	if MESSAGE != nil {
		M = append(M, MESSAGE...)
	}

	MPIN_GET_Y(TimeValue, M, Y)

	rtn = MPIN_CLIENT_2(X, Y, SEC)
	if rtn != 0 {
		return rtn
	}

	return 0
}

/* One pass MPIN Server */
func MPIN_SERVER(date int, HID []byte, HTID []byte, Y []byte, SST []byte, xID []byte, xCID []byte, SEC []byte, E []byte, F []byte, CID []byte, MESSAGE []byte, TimeValue int) int {
	rtn := 0

	var M []byte
	if date == 0 {
		M = xID
	} else {
		M = xCID
	}

	MPIN_SERVER_1(date, CID, HID, HTID)

	if MESSAGE != nil {
		M = append(M, MESSAGE...)
	}

	MPIN_GET_Y(TimeValue, M, Y)

	rtn = MPIN_SERVER_2(date, HID, HTID, Y, SST, xID, xCID, SEC, E, F)
	if rtn != 0 {
		return rtn
	}

	return 0
}

func MPIN_printBinary(array []byte) {
	for i := 0; i < len(array); i++ {
		fmt.Printf("%02x", array[i])
	}
	fmt.Printf("\n")
}
