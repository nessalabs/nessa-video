package plot

import (
	"math"
	"math/cmplx"
)

// lanczos holds the coefficients for g = 7, n = 9 (Numerical Recipes and
// Godfrey), good to about 15 digits.
var lanczos = [...]float64{
	0.99999999999980993, 676.5203681218851, -1259.1392167224028,
	771.32342877765313, -176.61502916214059, 12.507343278686905,
	-0.13857109526572012, 9.9843695780195716e-6, 1.5056327351493116e-7,
}

// Gamma is the gamma function for complex z, by the Lanczos
// approximation, with the reflection formula left of Re z = 1/2.
func Gamma(z complex128) complex128 {
	if real(z) < 0.5 {
		// Γ(z) Γ(1-z) = π / sin(πz)
		s := cmplx.Sin(complex(math.Pi, 0) * z)
		if s == 0 {
			return cmplx.Inf()
		}
		return complex(math.Pi, 0) / (s * Gamma(1-z))
	}
	z--
	x := complex(lanczos[0], 0)
	for i := 1; i < len(lanczos); i++ {
		x += complex(lanczos[i], 0) / (z + complex(float64(i), 0))
	}
	t := z + 7.5
	return complex(math.Sqrt(2*math.Pi), 0) * cmplx.Pow(t, z+0.5) * cmplx.Exp(-t) * x
}

// bernoulli are B2, B4, …, B14 divided by (2k)!.
var bernoulli = func() []float64 {
	b := []float64{1.0 / 6, -1.0 / 30, 1.0 / 42, -1.0 / 30, 5.0 / 66, -691.0 / 2730, 7.0 / 6}
	out := make([]float64, len(b))
	fact := 1.0
	for k := range b {
		n := float64(2 * (k + 1))
		fact *= (n - 1) * n
		out[k] = b[k] / fact
	}
	return out
}()

// zetaN is how many terms the Euler–Maclaurin sum adds directly.
const zetaN = 20

// Zeta is the Riemann zeta function for complex s. Right of Re s = 0 it
// uses Euler–Maclaurin summation with N = 20 and Bernoulli terms up to
// B14, accurate to about 1e-9 for |Im s| up to 40; left of it the
// functional equation. s = 1 is the pole.
func Zeta(s complex128) complex128 {
	if s == 1 {
		return cmplx.Inf()
	}
	if real(s) < 0 {
		// ζ(s) = 2^s π^(s-1) sin(πs/2) Γ(1-s) ζ(1-s)
		return cmplx.Pow(2, s) * cmplx.Pow(complex(math.Pi, 0), s-1) *
			cmplx.Sin(complex(math.Pi/2, 0)*s) * Gamma(1-s) * Zeta(1-s)
	}
	var sum complex128
	for n := 1; n < zetaN; n++ {
		sum += cmplx.Exp(-s * complex(math.Log(float64(n)), 0))
	}
	N := complex(float64(zetaN), 0)
	lnN := complex(math.Log(zetaN), 0)
	nPow := cmplx.Exp(-s * lnN) // N^-s
	sum += N * nPow / (s - 1)   // N^(1-s)/(s-1)
	sum += nPow / 2
	return sum + emTail(s, nPow, N)
}

// emTail is the Bernoulli part of Euler–Maclaurin:
// Σ_{k=1..7} B2k/(2k)! · s(s+1)…(s+2k-2) · N^(-s-2k+1).
func emTail(s, nPow, N complex128) complex128 {
	var out complex128
	poch := s      // s(s+1)…(s+2k-2) for k = 1
	pw := nPow / N // N^(-s-1)
	inv2 := 1 / (N * N)
	for k, b := range bernoulli {
		if k > 0 {
			// from s…(s+2k-4) to s…(s+2k-2): two more factors
			m := float64(2*k - 1)
			poch *= (s + complex(m, 0)) * (s + complex(m+1, 0))
			pw *= inv2
		}
		out += complex(b, 0) * poch * pw
	}
	return out
}
