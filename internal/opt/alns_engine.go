package opt

import (
	"math"
	"math/rand"
	"time"
)

type TW struct{ Start, End time.Time }

type Demand struct{ Weight, Volume float64 }

type Node struct {
	ID         string
	Lat, Lng   float64
	ServiceSec int
	TW         *TW
	Demand     Demand
	Skills     []string
}

type Vehicle struct {
	ID          string
	CapWeight   float64
	CapVolume   float64
	Skills      []string
	StartLatLng *[2]float64 // optional depot
	EndLatLng   *[2]float64 // optional depot
}

type Problem struct {
	Nodes                   []Node
	Vehicles                []Vehicle
	SpeedKph                float64
	Objectives              map[string]float64 // weights: driveTime, distance, lateness, failed
	HosMaxDriveSec          int                // optional HoS continuous drive limit (seconds)
	BreakSec                int                // planned break duration in seconds if HosMaxDriveSec exceeded
	IterationsLimit         int                // optional iteration cap
	InitialTemp             float64            // initial temperature for SA
	Cooling                 float64            // cooling factor per iteration
	InitialRemovalWeights   []float64          // [random, shaw]
	InitialInsertionWeights []float64          // [greedy, regret2]
}

type RoutePlan struct {
	VehicleID string
	Order     []int // indices into Nodes
}

type Solution struct {
	Plans []RoutePlan
	Cost  float64
}

type Metrics struct {
	RemovalSelects        [2]int // random, shaw
	InsertSelects         [2]int // greedy, regret2
	Iterations            int
	Improvements          int
	AcceptedWorse         int
	BestCost              float64
	FinalCost             float64
	FinalRemovalWeights   [2]float64
	FinalInsertionWeights [2]float64
	Snapshots             []WeightSnapshot
}

type WeightSnapshot struct {
	Iteration int
	Removal   [2]float64
	Insertion [2]float64
}

// Solve runs a simple ALNS-like heuristic with regret insertion and random removal.
func Solve(p Problem, seed int64, timeBudget time.Duration) (Solution, Metrics) {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))
	if p.SpeedKph <= 0 {
		p.SpeedKph = 50
	}
	// seed solution via greedy assignment
	curr := greedySeed(p)
	best := curr
	// operator weights (removal + insertion)
	remW := []float64{1, 1} // random, shaw
	insW := []float64{1, 1} // greedy, regret2
	if len(p.InitialRemovalWeights) == 2 {
		remW = []float64{p.InitialRemovalWeights[0], p.InitialRemovalWeights[1]}
	}
	if len(p.InitialInsertionWeights) == 2 {
		insW = []float64{p.InitialInsertionWeights[0], p.InitialInsertionWeights[1]}
	}
	temp := 1.0
	if p.InitialTemp > 0 {
		temp = p.InitialTemp
	}
	cool := 0.995
	if p.Cooling > 0 && p.Cooling < 1 {
		cool = p.Cooling
	}
	m := Metrics{BestCost: best.Cost}
	deadline := time.Now().Add(timeBudget)
	snapshotEvery := 50
	for time.Now().Before(deadline) {
		m.Iterations++
		if p.IterationsLimit > 0 && m.Iterations >= p.IterationsLimit {
			break
		}
		k := 1 + rng.Intn(3)
		// select operators by roulette wheel
		op := selectOp(remW, rng)
		m.RemovalSelects[op]++
		ip := selectOp(insW, rng)
		m.InsertSelects[ip]++
		var removedIdx []int
		switch op {
		case 0:
			removedIdx = pickRandomNodes(curr, k, rng)
		case 1:
			removedIdx = shawRemoval(p, curr, k, rng)
		}
		curr = removeNodes(curr, removedIdx)
		switch ip {
		case 0:
			curr = greedyInsert(p, curr, removedIdx)
		case 1:
			curr = regretInsert(p, curr, removedIdx)
		}
		// local improvements per iteration
		curr = twoOptImprove(p, curr)
		curr = crossExchangeImprove(p, curr)
		curr = twoOptStarImprove(p, curr) // inter-route segment swaps
		curr.Cost = cost(p, curr)
		// acceptance criterion (simulated annealing-like)
		delta := curr.Cost - best.Cost
		if delta < 0 || rng.Float64() < math.Exp(-delta/(temp+1e-9)) {
			// improvement or accepted worse
			if curr.Cost < best.Cost {
				best = curr
				remW[op] += 0.1
				insW[ip] += 0.1
				m.Improvements++
				m.BestCost = best.Cost
			} else {
				remW[op] += 0.01
				insW[ip] += 0.01
				m.AcceptedWorse++
			}
		} else {
			// slight penalty for non-acceptance
			remW[op] = math.Max(0.01, remW[op]*0.999)
			insW[ip] = math.Max(0.01, insW[ip]*0.999)
		}
		temp *= cool
		// snapshot weights
		if m.Iterations%snapshotEvery == 0 {
			m.Snapshots = append(m.Snapshots, WeightSnapshot{Iteration: m.Iterations, Removal: [2]float64{remW[0], remW[1]}, Insertion: [2]float64{insW[0], insW[1]}})
		}
	}
	m.FinalCost = best.Cost
	m.FinalRemovalWeights = [2]float64{remW[0], remW[1]}
	m.FinalInsertionWeights = [2]float64{insW[0], insW[1]}
	return best, m
}

func greedySeed(p Problem) Solution {
	n := len(p.Nodes)
	used := make([]bool, n)
	plans := make([]RoutePlan, len(p.Vehicles))
	for vi := range plans {
		plans[vi] = RoutePlan{VehicleID: p.Vehicles[vi].ID, Order: []int{}}
	}
	for assigned := 0; assigned < n; {
		progress := false
		for vi := range p.Vehicles {
			bestIdx, bestDelta := -1, math.MaxFloat64
			for i := 0; i < n; i++ {
				if used[i] {
					continue
				}
				if !feasibleAdd(p, plans[vi], p.Vehicles[vi], i) {
					continue
				}
				d := deltaCostAppend(p, plans[vi], p.Vehicles[vi], i)
				if d < bestDelta {
					bestDelta = d
					bestIdx = i
				}
			}
			if bestIdx >= 0 {
				plans[vi].Order = append(plans[vi].Order, bestIdx)
				used[bestIdx] = true
				assigned++
				progress = true
				if assigned == n {
					break
				}
			}
		}
		if !progress {
			break
		}
	}
	sol := Solution{Plans: plans}
	sol.Cost = cost(p, sol)
	return sol
}

func pickRandomNodes(sol Solution, k int, rng *rand.Rand) []int {
	present := map[int]bool{}
	for _, pl := range sol.Plans {
		for _, idx := range pl.Order {
			present[idx] = true
		}
	}
	all := []int{}
	for idx := range present {
		all = append(all, idx)
	}
	if len(all) == 0 {
		return nil
	}
	removed := []int{}
	for i := 0; i < k && len(all) > 0; i++ {
		j := rng.Intn(len(all))
		removed = append(removed, all[j])
		all = append(all[:j], all[j+1:]...)
	}
	return removed
}

func removeNodes(sol Solution, removed []int) Solution {
	if len(removed) == 0 {
		return sol
	}
	rm := map[int]bool{}
	for _, i := range removed {
		rm[i] = true
	}
	out := Solution{Plans: make([]RoutePlan, len(sol.Plans))}
	for i := range sol.Plans {
		out.Plans[i].VehicleID = sol.Plans[i].VehicleID
		for _, idx := range sol.Plans[i].Order {
			if !rm[idx] {
				out.Plans[i].Order = append(out.Plans[i].Order, idx)
			}
		}
	}
	return out
}

func regretInsert(p Problem, sol Solution, removed []int) Solution {
	if len(removed) == 0 {
		return sol
	}
	// Greedy regret-2 insertion across all vehicles/positions with simple TW feasibility
	nodes := removed
	for len(nodes) > 0 {
		bestNode := -1
		bestPlan := -1
		bestPos := -1
		bestCost := math.MaxFloat64
		second := math.MaxFloat64
		for ni, idx := range nodes {
			best1 := math.MaxFloat64
			best2 := math.MaxFloat64
			bp, bpos := -1, -1
			for vi, pl := range sol.Plans {
				for pos := 0; pos <= len(pl.Order); pos++ {
					if !feasibleAddAt(p, pl, p.Vehicles[vi], idx, pos) {
						continue
					}
					c := deltaCostInsert(p, pl, p.Vehicles[vi], idx, pos)
					if c < best1 {
						best2 = best1
						best1 = c
						bp = vi
						bpos = pos
					} else if c < best2 {
						best2 = c
					}
				}
			}
			regret := best2 - best1
			if regret < 0 {
				regret = 0
			}
			if best1 < math.MaxFloat64 {
				if regret > (second-bestCost) || (bestNode == -1) {
					bestNode = ni
					bestPlan = bp
					bestPos = bpos
					bestCost = best1
					second = best2
				}
			}
		}
		if bestNode == -1 { // no feasible insertion; append to shortest plan
			shortest := 0
			for i := range sol.Plans {
				if len(sol.Plans[i].Order) < len(sol.Plans[shortest].Order) {
					shortest = i
				}
			}
			sol.Plans[shortest].Order = append(sol.Plans[shortest].Order, nodes[0])
			nodes = nodes[1:]
			continue
		}
		// insert chosen node
		pl := &sol.Plans[bestPlan]
		if bestPos == len(pl.Order) {
			pl.Order = append(pl.Order, nodes[bestNode])
		} else {
			pl.Order = append(pl.Order[:bestPos+1], pl.Order[bestPos:]...)
			pl.Order[bestPos] = nodes[bestNode]
		}
		nodes = append(nodes[:bestNode], nodes[bestNode+1:]...)
	}
	sol.Cost = cost(p, sol)
	// local improvement pass (or-opt 1-node)
	sol = orOptLocalImprove(p, sol)
	return sol
}

// greedyInsert inserts nodes by cheapest feasible append/insertion
func greedyInsert(p Problem, sol Solution, removed []int) Solution {
	if len(removed) == 0 {
		return sol
	}
	nodes := removed
	for len(nodes) > 0 {
		bestPlan, bestPos, bestNode := -1, -1, 0
		bestCost := math.MaxFloat64
		for ni, idx := range nodes {
			for vi, pl := range sol.Plans {
				for pos := 0; pos <= len(pl.Order); pos++ {
					if !feasibleAddAt(p, pl, p.Vehicles[vi], idx, pos) {
						continue
					}
					c := deltaCostInsert(p, pl, p.Vehicles[vi], idx, pos)
					if c < bestCost {
						bestCost = c
						bestPlan = vi
						bestPos = pos
						bestNode = ni
					}
				}
			}
		}
		if bestPlan == -1 {
			// append to shortest
			shortest := 0
			for i := range sol.Plans {
				if len(sol.Plans[i].Order) < len(sol.Plans[shortest].Order) {
					shortest = i
				}
			}
			sol.Plans[shortest].Order = append(sol.Plans[shortest].Order, nodes[0])
			nodes = nodes[1:]
			continue
		}
		pl := &sol.Plans[bestPlan]
		if bestPos == len(pl.Order) {
			pl.Order = append(pl.Order, nodes[bestNode])
		} else {
			pl.Order = append(pl.Order[:bestPos+1], pl.Order[bestPos:]...)
			pl.Order[bestPos] = nodes[bestNode]
		}
		nodes = append(nodes[:bestNode], nodes[bestNode+1:]...)
	}
	sol.Cost = cost(p, sol)
	return sol
}

func cost(p Problem, s Solution) float64 {
	wDrive := p.Objectives["driveTime"]
	if wDrive == 0 {
		wDrive = 1
	}
	wDist := p.Objectives["distance"]
	wLate := p.Objectives["lateness"]
	wFail := p.Objectives["failed"]
	total := 0.0
	for vi, pl := range s.Plans {
		v := p.Vehicles[vi]
		t := 0.0
		var curLat, curLng float64
		if v.StartLatLng != nil {
			curLat, curLng = v.StartLatLng[0], v.StartLatLng[1]
		} else if len(pl.Order) > 0 {
			cur := p.Nodes[pl.Order[0]]
			curLat, curLng = cur.Lat, cur.Lng
		}
		for _, idx := range pl.Order {
			nd := p.Nodes[idx]
			dist := haversine(curLat, curLng, nd.Lat, nd.Lng)
			drive := dist / (p.SpeedKph / 3.6)
			t += drive
			arr := t
			late := 0.0
			if nd.TW != nil && !nd.TW.End.IsZero() {
				end := nd.TW.End.Sub(time.Unix(0, 0)).Seconds() // relative zero
				if arr > end {
					late = arr - end
				}
			}
			t += float64(nd.ServiceSec)
			total += wDrive*drive + wDist*dist + wLate*late
			curLat, curLng = nd.Lat, nd.Lng
		}
	}
	// failed nodes: if any node not present
	present := map[int]bool{}
	for _, pl := range s.Plans {
		for _, idx := range pl.Order {
			present[idx] = true
		}
	}
	failed := 0.0
	for i := range p.Nodes {
		if !present[i] {
			failed++
		}
	}
	total += wFail * failed * 3600 // heavy penalty
	return total
}

func feasibleAdd(p Problem, pl RoutePlan, v Vehicle, idx int) bool {
	// Capacity check (simple sum)
	w := 0.0
	vol := 0.0
	for _, i := range pl.Order {
		w += p.Nodes[i].Demand.Weight
		vol += p.Nodes[i].Demand.Volume
	}
	w += p.Nodes[idx].Demand.Weight
	vol += p.Nodes[idx].Demand.Volume
	if v.CapWeight > 0 && w > v.CapWeight {
		return false
	}
	if v.CapVolume > 0 && vol > v.CapVolume {
		return false
	}
	// Skills (subset)
	if len(p.Nodes[idx].Skills) > 0 && len(v.Skills) > 0 {
		needed := make(map[string]bool)
		for _, s := range p.Nodes[idx].Skills {
			needed[s] = true
		}
		for _, s := range v.Skills {
			delete(needed, s)
		}
		if len(needed) > 0 {
			return false
		}
	}
	return true
}

func feasibleAddAt(p Problem, pl RoutePlan, v Vehicle, idx, pos int) bool {
	if !feasibleAdd(p, pl, v, idx) {
		return false
	}
	// basic TW feasibility: arrival at inserted node must be before its TW end
	if pos < 0 || pos > len(pl.Order) {
		return false
	}
	// full schedule propagation feasibility after insertion
	tmp := RoutePlan{VehicleID: pl.VehicleID, Order: make([]int, 0, len(pl.Order)+1)}
	tmp.Order = append(tmp.Order, pl.Order[:pos]...)
	tmp.Order = append(tmp.Order, idx)
	tmp.Order = append(tmp.Order, pl.Order[pos:]...)
	_, feasible := schedulePlan(p, tmp, v)
	return feasible
}

func deltaCostAppend(p Problem, pl RoutePlan, _ Vehicle, idx int) float64 {
	// cost to append node idx at end of pl
	if len(pl.Order) == 0 {
		return 0
	}
	last := p.Nodes[pl.Order[len(pl.Order)-1]]
	nd := p.Nodes[idx]
	dist := haversine(last.Lat, last.Lng, nd.Lat, nd.Lng)
	return dist
}

func deltaCostInsert(p Problem, pl RoutePlan, v Vehicle, idx, pos int) float64 {
	// approximate delta: prev->new + new->next - prev->next + service
	var prevLat, prevLng float64
	if pos == 0 {
		if v.StartLatLng != nil {
			prevLat, prevLng = v.StartLatLng[0], v.StartLatLng[1]
		} else if len(pl.Order) > 0 {
			n := p.Nodes[pl.Order[0]]
			prevLat, prevLng = n.Lat, n.Lng
		}
	} else {
		n := p.Nodes[pl.Order[pos-1]]
		prevLat, prevLng = n.Lat, n.Lng
	}
	var nextLat, nextLng float64
	if pos < len(pl.Order) {
		n := p.Nodes[pl.Order[pos]]
		nextLat, nextLng = n.Lat, n.Lng
	} else {
		nextLat, nextLng = prevLat, prevLng
	}
	nd := p.Nodes[idx]
	add := haversine(prevLat, prevLng, nd.Lat, nd.Lng) + haversine(nd.Lat, nd.Lng, nextLat, nextLng)
	rem := haversine(prevLat, prevLng, nextLat, nextLng)
	return add - rem + float64(nd.ServiceSec)
}

// schedulePlan computes arrival times and simple feasibility for a plan with optional HoS breaks.
// Returns total cost components (driveSec, distanceM, latenessSec) and feasibility flag.
func schedulePlan(p Problem, pl RoutePlan, v Vehicle) (struct{ drive, dist, late float64 }, bool) {
	speed := p.SpeedKph / 3.6
	var curLat, curLng float64
	if v.StartLatLng != nil {
		curLat, curLng = v.StartLatLng[0], v.StartLatLng[1]
	} else if len(pl.Order) > 0 {
		cur := p.Nodes[pl.Order[0]]
		curLat, curLng = cur.Lat, cur.Lng
	}
	t := 0.0
	distTotal := 0.0
	lateTotal := 0.0
	driveSinceBreak := 0.0
	for _, idx := range pl.Order {
		nd := p.Nodes[idx]
		d := haversine(curLat, curLng, nd.Lat, nd.Lng)
		drive := d / speed
		// planned break if HoS exceeded
		if p.HosMaxDriveSec > 0 && int(driveSinceBreak+drive) > p.HosMaxDriveSec {
			// add break
			t += float64(p.BreakSec)
			driveSinceBreak = 0
		}
		t += drive
		driveSinceBreak += drive
		arr := t
		if nd.TW != nil && !nd.TW.Start.IsZero() {
			ws := nd.TW.Start.Sub(time.Unix(0, 0)).Seconds()
			if arr < ws {
				arr = ws
				t = arr
			}
		}
		if nd.TW != nil && !nd.TW.End.IsZero() {
			we := nd.TW.End.Sub(time.Unix(0, 0)).Seconds()
			if arr > we {
				return struct{ drive, dist, late float64 }{t, distTotal + d, lateTotal + (arr - we)}, false
			}
		}
		// service
		t += float64(nd.ServiceSec)
		distTotal += d
		curLat, curLng = nd.Lat, nd.Lng
	}
	return struct{ drive, dist, late float64 }{t, distTotal, lateTotal}, true
}

// orOptLocalImprove attempts relocating single nodes within each plan if it reduces cost and remains feasible.
func orOptLocalImprove(p Problem, sol Solution) Solution {
	improved := true
	for improved {
		improved = false
		for vi := range sol.Plans {
			pl := sol.Plans[vi]
			best := pl
			bestCost := math.MaxFloat64
			for i := 0; i < len(pl.Order); i++ {
				for j := 0; j <= len(pl.Order); j++ {
					if j == i || j == i+1 {
						continue
					}
					cand := RoutePlan{VehicleID: pl.VehicleID, Order: append([]int(nil), pl.Order...)}
					// remove i, insert at j
					node := cand.Order[i]
					cand.Order = append(cand.Order[:i], cand.Order[i+1:]...)
					if j > len(cand.Order) {
						j = len(cand.Order)
					}
					cand.Order = append(cand.Order[:j], append([]int{node}, cand.Order[j:]...)...)
					if _, ok := schedulePlan(p, cand, p.Vehicles[vi]); !ok {
						continue
					}
					// approximate cost
					cSol := Solution{Plans: append([]RoutePlan(nil), sol.Plans...)}
					cSol.Plans[vi] = cand
					c := cost(p, cSol)
					if c+1e-6 < bestCost {
						best = cand
						bestCost = c
					}
				}
			}
			if bestCost+1e-6 < cost(p, Solution{Plans: sol.Plans}) {
				sol.Plans[vi] = best
				improved = true
			}
		}
	}
	sol.Cost = cost(p, sol)
	return sol
}

// twoOptImprove applies 2-opt within each plan when feasible
func twoOptImprove(p Problem, sol Solution) Solution {
	for vi := range sol.Plans {
		pl := sol.Plans[vi]
		n := len(pl.Order)
		improved := true
		for improved {
			improved = false
			for i := 1; i < n-2; i++ {
				for k := i + 1; k < n-1; k++ {
					cand := RoutePlan{VehicleID: pl.VehicleID, Order: append([]int(nil), pl.Order...)}
					// reverse segment [i,k]
					for a, b := i, k; a < b; a, b = a+1, b-1 {
						cand.Order[a], cand.Order[b] = cand.Order[b], cand.Order[a]
					}
					if _, ok := schedulePlan(p, cand, p.Vehicles[vi]); !ok {
						continue
					}
					c1 := pathDistanceNodes(p, pl)
					c2 := pathDistanceNodes(p, cand)
					if c2+1e-6 < c1 {
						pl = cand
						improved = true
					}
				}
			}
		}
		sol.Plans[vi] = pl
	}
	sol.Cost = cost(p, sol)
	return sol
}

func pathDistanceNodes(p Problem, pl RoutePlan) float64 {
	total := 0.0
	if len(pl.Order) == 0 {
		return 0
	}
	var curLat, curLng float64
	cur := p.Nodes[pl.Order[0]]
	curLat, curLng = cur.Lat, cur.Lng
	for _, idx := range pl.Order {
		nd := p.Nodes[idx]
		total += haversine(curLat, curLng, nd.Lat, nd.Lng)
		curLat, curLng = nd.Lat, nd.Lng
	}
	return total
}

// crossExchangeImprove swaps nodes between routes if cost decreases and feasible
func crossExchangeImprove(p Problem, sol Solution) Solution {
	m := len(sol.Plans)
	if m < 2 {
		return sol
	}
	improved := true
	for improved {
		improved = false
		for a := 0; a < m; a++ {
			for b := a + 1; b < m; b++ {
				pa := sol.Plans[a]
				pb := sol.Plans[b]
				for i := 0; i < len(pa.Order); i++ {
					for j := 0; j < len(pb.Order); j++ {
						ca := RoutePlan{VehicleID: pa.VehicleID, Order: append([]int(nil), pa.Order...)}
						cb := RoutePlan{VehicleID: pb.VehicleID, Order: append([]int(nil), pb.Order...)}
						ca.Order[i], cb.Order[j] = cb.Order[j], ca.Order[i]
						if _, ok := schedulePlan(p, ca, p.Vehicles[a]); !ok {
							continue
						}
						if _, ok := schedulePlan(p, cb, p.Vehicles[b]); !ok {
							continue
						}
						before := pathDistanceNodes(p, pa) + pathDistanceNodes(p, pb)
						after := pathDistanceNodes(p, ca) + pathDistanceNodes(p, cb)
						if after+1e-6 < before {
							sol.Plans[a] = ca
							sol.Plans[b] = cb
							improved = true
						}
					}
				}
			}
		}
	}
	sol.Cost = cost(p, sol)
	return sol
}

// twoOptStarImprove performs inter-route segment exchanges (2-opt*) limited to segment length 1..2
func twoOptStarImprove(p Problem, sol Solution) Solution {
	m := len(sol.Plans)
	if m < 2 {
		return sol
	}
	improved := true
	for improved {
		improved = false
		for a := 0; a < m; a++ {
			for b := a + 1; b < m; b++ {
				pa := sol.Plans[a]
				pb := sol.Plans[b]
				for i := 0; i < len(pa.Order); i++ {
					for j := 0; j < len(pb.Order); j++ {
						for la := 1; la <= 2 && i+la <= len(pa.Order); la++ {
							for lb := 1; lb <= 2 && j+lb <= len(pb.Order); lb++ {
								ca := RoutePlan{VehicleID: pa.VehicleID, Order: append([]int(nil), pa.Order...)}
								cb := RoutePlan{VehicleID: pb.VehicleID, Order: append([]int(nil), pb.Order...)}
								segA := append([]int(nil), ca.Order[i:i+la]...)
								segB := append([]int(nil), cb.Order[j:j+lb]...)
								ca.Order = append(append(ca.Order[:i], segB...), ca.Order[i+la:]...)
								cb.Order = append(append(cb.Order[:j], segA...), cb.Order[j+lb:]...)
								if _, ok := schedulePlan(p, ca, p.Vehicles[a]); !ok {
									continue
								}
								if _, ok := schedulePlan(p, cb, p.Vehicles[b]); !ok {
									continue
								}
								before := pathDistanceNodes(p, pa) + pathDistanceNodes(p, pb)
								after := pathDistanceNodes(p, ca) + pathDistanceNodes(p, cb)
								if after+1e-6 < before {
									sol.Plans[a] = ca
									sol.Plans[b] = cb
									improved = true
								}
							}
						}
					}
				}
			}
		}
	}
	sol.Cost = cost(p, sol)
	return sol
}

// shawRemoval selects k nodes related by geography/time windows.
func shawRemoval(p Problem, sol Solution, k int, rng *rand.Rand) []int {
	// pick a random seed node from current assignment
	assigned := []int{}
	for _, pl := range sol.Plans {
		assigned = append(assigned, pl.Order...)
	}
	if len(assigned) == 0 {
		return nil
	}
	seedIdx := assigned[rng.Intn(len(assigned))]
	// score relatedness for all nodes
	type pair struct {
		idx   int
		score float64
	}
	rel := []pair{}
	sN := p.Nodes[seedIdx]
	for _, idx := range assigned {
		if idx == seedIdx {
			continue
		}
		n := p.Nodes[idx]
		geo := haversine(sN.Lat, sN.Lng, n.Lat, n.Lng)
		tw := 0.0
		if sN.TW != nil && n.TW != nil {
			tw = twOverlap(*sN.TW, *n.TW)
		}
		score := geo - 1000.0*tw // prefer close in geo and overlapping TW
		rel = append(rel, pair{idx: idx, score: score})
	}
	// sort by score ascending
	for i := 0; i < len(rel); i++ {
		for j := i + 1; j < len(rel); j++ {
			if rel[j].score < rel[i].score {
				rel[i], rel[j] = rel[j], rel[i]
			}
		}
	}
	removed := []int{seedIdx}
	for i := 0; i < len(rel) && len(removed) < k; i++ {
		removed = append(removed, rel[i].idx)
	}
	return removed
}

func twOverlap(a, b TW) float64 {
	// return overlap seconds fraction
	start := a.Start
	if b.Start.After(start) {
		start = b.Start
	}
	end := a.End
	if b.End.Before(end) {
		end = b.End
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Seconds()
}

func selectOp(weights []float64, rng *rand.Rand) int {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	if sum <= 0 {
		return 0
	}
	r := rng.Float64() * sum
	acc := 0.0
	for i, w := range weights {
		acc += w
		if r <= acc {
			return i
		}
	}
	return len(weights) - 1
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
