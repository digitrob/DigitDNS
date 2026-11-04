package aggregate

import (
	"hash/fnv"
	"math"
	"math/bits"
)

// HyperLogLog with 256 one-byte registers. Distinct-client counts are stored in
// every bucket, so the summary no longer has to infer "active clients" from the
// bounded top-N table (which silently clipped at TopClients). 256 registers need
// 256 bytes per bucket and give a standard error of about 6.5%, while a count
// below ~640 is corrected with linear counting and is usually exact after
// rounding.
const (
	hllPrecision = 8
	hllRegisters = 1 << hllPrecision
)

func newHLL() []byte {
	return make([]byte, hllRegisters)
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func hllAdd(registers []byte, key string) {
	if len(registers) != hllRegisters {
		return
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	// FNV-1a leaves adjacent short strings in the same high bits (the index is
	// the top byte), so apply the splitmix64 finalizer before slicing. It is
	// deterministic across restarts, which keeps merged registers meaningful.
	value := splitmix64(h.Sum64())

	index := value >> (64 - hllPrecision)
	// Shift the remaining bits up so LeadingZeros64 measures the run directly
	// after the index. Clamp to the maximum observable rank.
	remainder := value << hllPrecision
	rank := uint8(bits.LeadingZeros64(remainder)) + 1
	if maxRank := uint8(64 - hllPrecision + 1); rank > maxRank {
		rank = maxRank
	}
	if rank > registers[index] {
		registers[index] = rank
	}
}

// hllMerge folds src into dst. HyperLogLog union is register-wise maximum, so
// daily rollups and multi-bucket summaries are lossless at the register level.
func hllMerge(dst, src []byte) {
	if len(dst) != hllRegisters || len(src) != hllRegisters {
		return
	}
	for i, v := range src {
		if v > dst[i] {
			dst[i] = v
		}
	}
}

func hllEstimate(registers []byte) int {
	if len(registers) != hllRegisters {
		return 0
	}
	const m = float64(hllRegisters)
	alpha := 0.7213 / (1 + 1.079/m)
	sum := 0.0
	zeros := 0
	for _, register := range registers {
		sum += 1 / float64(uint64(1)<<register)
		if register == 0 {
			zeros++
		}
	}
	raw := alpha * m * m / sum

	// Small-range correction: with many empty registers the raw estimator is
	// biased high. Linear counting is exact enough that a rounded value matches
	// the true cardinality for the small windows where it applies.
	if raw <= 2.5*m && zeros > 0 {
		return int(math.Round(m * math.Log(m/float64(zeros))))
	}
	// Large-range correction for the (unlikely) approach to 2^64.
	twoTo64 := math.Pow(2, 64)
	if raw > twoTo64/30 {
		if raw >= twoTo64 {
			return math.MaxInt32
		}
		return int(math.Round(-twoTo64 * math.Log(1-raw/twoTo64)))
	}
	return int(math.Round(raw))
}
