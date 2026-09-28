package core

import "math"

// Value noise and fractal noise, for textures.
func noiseHash(x, y, z int) float64 {
	h := uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841 ^ uint32(z)*0xcb1ab31f
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return float64(h&0xffffff) / 0xffffff
}

func Noise3(x, y, z float64) float64 {
	xi, yi, zi := math.Floor(x), math.Floor(y), math.Floor(z)
	fx, fy, fz := x-xi, y-yi, z-zi
	fx, fy, fz = fx*fx*(3-2*fx), fy*fy*(3-2*fy), fz*fz*(3-2*fz)
	X, Y, Z := int(xi), int(yi), int(zi)
	lerp := func(a, b, t float64) float64 { return a + (b-a)*t }
	c := func(dx, dy, dz int) float64 { return noiseHash(X+dx, Y+dy, Z+dz) }
	return lerp(
		lerp(lerp(c(0, 0, 0), c(1, 0, 0), fx), lerp(c(0, 1, 0), c(1, 1, 0), fx), fy),
		lerp(lerp(c(0, 0, 1), c(1, 0, 1), fx), lerp(c(0, 1, 1), c(1, 1, 1), fx), fy), fz)
}

// SNoise is noise3 centered: -1..1.
func SNoise(x, y, z float64) float64 { return 2*Noise3(x, y, z) - 1 }

// FBM is fractal noise in [-0.5, 0.5].
func FBM(x, y, z float64, octaves int) float64 {
	sum, amp, norm := 0.0, 0.5, 0.0
	for i := 0; i < octaves; i++ {
		sum += amp * Noise3(x, y, z)
		norm += amp
		x, y, z = x*2.03, y*2.03, z*2.03
		amp /= 2
	}
	return sum/norm - 0.5
}

// CellNoise is a fixed pseudo-random value in [0, 1) per screen cell.
func CellNoise(x, y int) float64 {
	h := uint32(x)*0x9E3779B1 ^ uint32(y)*0x85EBCA77
	h ^= h >> 15
	h *= 0x2C1B3C6D
	h ^= h >> 12
	return float64(h&0xffff) / 65536
}

func Hash2(x, y, k int) uint32 {
	h := uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841 ^ uint32(k)*0xcb1ab31f
	h ^= h >> 13
	h *= 0x5bd1e995
	return h ^ h>>15
}

func Frac(h uint32, shift uint) float64 { return float64((h>>shift)&0xff) / 255 }
