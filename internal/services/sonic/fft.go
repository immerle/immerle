package sonic

import (
	"math"
	"math/bits"
	"math/cmplx"
)

// fft computes an in-place iterative radix-2 FFT. len(x) must be a power of two.
func fft(x []complex128) {
	n := len(x)
	shift := 64 - uint(bits.Len(uint(n))-1)
	for i := range x {
		if j := int(bits.Reverse64(uint64(i)) >> shift); j > i {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], w*x[start+k+size/2]
				x[start+k], x[start+k+size/2] = a+b, a-b
				w *= step
			}
		}
	}
}
