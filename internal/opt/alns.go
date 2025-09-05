package opt

// Lightweight API surface for higher-level callers.

type Order struct {
	ID       string
	Priority int
}

type Constraints struct {
	// capacity, skills, time windows, HoS, zones, etc.
}

// Optimize returns a seed Solution using the richer Vehicle/Solution types.
func Optimize(_ []Order, vehicles []Vehicle, _ Constraints) Solution {
	plans := make([]RoutePlan, len(vehicles))
	for i, v := range vehicles {
		plans[i] = RoutePlan{VehicleID: v.ID, Order: []int{}}
	}
	return Solution{Plans: plans, Cost: 0}
}
