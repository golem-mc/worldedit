package block

import "math"

// Hash3D returns a deterministic pseudo-random float64 in [0, 1) for integer coords and seed.
func Hash3D(seed int64, x, y, z int) float64 {
	h := uint64(seed) ^ uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0xbf58476d1ce4e5b9 ^ uint64(z)*0x94d049bb133111eb
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	return float64(h>>11) / float64(uint64(1)<<53)
}

func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }
func lerp(t, a, b float64) float64 {
	return a + t*(b-a)
}

func grad3(hash uint64, x, y, z float64) float64 {
	h := hash & 15
	u := x
	if h >= 8 {
		u = y
	}
	v := y
	if h >= 4 {
		if h == 12 || h == 14 {
			v = x
		} else {
			v = z
		}
	}
	res := 0.0
	if h&1 == 0 {
		res += u
	} else {
		res -= u
	}
	if h&2 == 0 {
		res += v
	} else {
		res -= v
	}
	return res
}

func hashInt(seed int64, x, y, z int) uint64 {
	h := uint64(seed) ^ uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0xbf58476d1ce4e5b9 ^ uint64(z)*0x94d049bb133111eb
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	return h ^ (h >> 31)
}

// Perlin3D evaluates multi-octave 3D Perlin noise normalized to [0, 1].
func Perlin3D(seed int64, x, y, z, freq float64, octaves int) float64 {
	if octaves < 1 {
		octaves = 1
	}
	if freq == 0 {
		freq = 1
	}
	total, amp, maxAmp, f := 0.0, 1.0, 0.0, freq
	for i := 0; i < octaves; i++ {
		total += perlinSingle(seed+int64(i)*31, x*f, y*f, z*f) * amp
		maxAmp += amp
		amp *= 0.5
		f *= 2.0
	}
	v := (total/maxAmp)*0.5 + 0.5
	return max(0, min(1, v))
}

func perlinSingle(seed int64, x, y, z float64) float64 {
	xi := int(math.Floor(x))
	yi := int(math.Floor(y))
	zi := int(math.Floor(z))
	xf := x - float64(xi)
	yf := y - float64(yi)
	zf := z - float64(zi)
	u, v, w := fade(xf), fade(yf), fade(zf)

	aaa := grad3(hashInt(seed, xi, yi, zi), xf, yf, zf)
	baa := grad3(hashInt(seed, xi+1, yi, zi), xf-1, yf, zf)
	aba := grad3(hashInt(seed, xi, yi+1, zi), xf, yf-1, zf)
	bba := grad3(hashInt(seed, xi+1, yi+1, zi), xf-1, yf-1, zf)
	aab := grad3(hashInt(seed, xi, yi, zi+1), xf, yf, zf-1)
	bab := grad3(hashInt(seed, xi+1, yi, zi+1), xf-1, yf, zf-1)
	abb := grad3(hashInt(seed, xi, yi+1, zi+1), xf, yf-1, zf-1)
	bbb := grad3(hashInt(seed, xi+1, yi+1, zi+1), xf-1, yf-1, zf-1)

	x1 := lerp(u, aaa, baa)
	x2 := lerp(u, aba, bba)
	y1 := lerp(v, x1, x2)
	x3 := lerp(u, aab, bab)
	x4 := lerp(u, abb, bbb)
	y2 := lerp(v, x3, x4)
	return lerp(w, y1, y2)
}

// Simplex3D evaluates 3D simplex-like gradient noise normalized to [0, 1].
func Simplex3D(seed int64, x, y, z, freq float64, octaves int) float64 {
	const f3 = 1.0 / 3.0
	if freq == 0 {
		freq = 1
	}
	if octaves < 1 {
		octaves = 1
	}
	// Skew coordinates for isotropic simplex sampling over gradient lattice.
	s := (x + y + z) * f3
	return Perlin3D(seed^0x51a7, x+s*0.5, y+s*0.5, z+s*0.5, freq, octaves)
}

// Voronoi3D evaluates 3D cellular Voronoi distance noise normalized to [0, 1].
func Voronoi3D(seed int64, x, y, z, freq float64) float64 {
	if freq == 0 {
		freq = 1
	}
	sx, sy, sz := x*freq, y*freq, z*freq
	xi, yi, zi := int(math.Floor(sx)), int(math.Floor(sy)), int(math.Floor(sz))
	minDist := math.MaxFloat64
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for dz := -1; dz <= 1; dz++ {
				cx, cy, cz := xi+dx, yi+dy, zi+dz
				px := float64(cx) + Hash3D(seed, cx, cy, cz)
				py := float64(cy) + Hash3D(seed+17, cx, cy, cz)
				pz := float64(cz) + Hash3D(seed+37, cx, cy, cz)
				ddx, ddy, ddz := sx-px, sy-py, sz-pz
				d := ddx*ddx + ddy*ddy + ddz*ddz
				if d < minDist {
					minDist = d
				}
			}
		}
	}
	return max(0, min(1, math.Sqrt(minDist)))
}

// RidgedMulti3D evaluates 3D ridged multifractal noise normalized to [0, 1].
func RidgedMulti3D(seed int64, x, y, z, freq float64, octaves int) float64 {
	if octaves < 1 {
		octaves = 1
	}
	if freq == 0 {
		freq = 1
	}
	total, amp, maxAmp, f := 0.0, 1.0, 0.0, freq
	for i := 0; i < octaves; i++ {
		n := perlinSingle(seed+int64(i)*53, x*f, y*f, z*f)
		ridge := 1.0 - math.Abs(n)
		ridge *= ridge
		total += ridge * amp
		maxAmp += amp
		amp *= 0.5
		f *= 2.0
	}
	return max(0, min(1, total/maxAmp))
}
