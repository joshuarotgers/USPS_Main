package opt

import "math"

// StopNode holds minimal info for routing heuristics
type StopNode struct {
	Lat float64
	Lng float64
}

// ImproveOrder2Opt applies a simple 2-opt heuristic to reduce total distance.
func ImproveOrder2Opt(nodes []StopNode, order []int, iterations int) []int {
	if iterations <= 0 {
		iterations = 1
	}
	best := append([]int(nil), order...)
	bestDist := pathDistance(nodes, best)
	n := len(order)
	for it := 0; it < iterations; it++ {
		improved := false
		for i := 1; i < n-2; i++ {
			for k := i + 1; k < n-1; k++ {
				newOrder := twoOptSwap(best, i, k)
				d := pathDistance(nodes, newOrder)
				if d+1e-3 < bestDist {
					best = newOrder
					bestDist = d
					improved = true
				}
			}
		}
		if !improved {
			break
		}
	}
	return best
}

func twoOptSwap(ord []int, i, k int) []int {
	out := make([]int, len(ord))
	copy(out, ord[:i])
	// reverse i..k
	pos := i
	for j := k; j >= i; j-- {
		out[pos] = ord[j]
		pos++
	}
	copy(out[pos:], ord[k+1:])
	return out
}

func pathDistance(nodes []StopNode, order []int) float64 {
	total := 0.0
	for i := 0; i < len(order)-1; i++ {
		a := nodes[order[i]]
		b := nodes[order[i+1]]
		total += haversineMeters(a.Lat, a.Lng, b.Lat, b.Lng)
	}
	return total
}

// Haversine duplicate to avoid import cycles
func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
