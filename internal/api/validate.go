package api

import (
	"fmt"
	"gpsnav/internal/model"
	"strings"
)

func validateOptimizeRequest(req *model.OptimizeRequest) error {
	if req.Algorithm != "" && req.Algorithm != "greedy" && req.Algorithm != "alns" {
		return fmt.Errorf("invalid algorithm: %s", req.Algorithm)
	}
	if req.TimeBudgetMs < 0 {
		return fmt.Errorf("timeBudgetMs must be >= 0")
	}
	if req.MaxIterations < 0 {
		return fmt.Errorf("maxIterations must be >= 0")
	}
	if req.Cooling != 0 && (req.Cooling <= 0 || req.Cooling >= 1) {
		return fmt.Errorf("cooling must be in (0,1)")
	}
	if len(req.RemovalWeights) > 0 && len(req.RemovalWeights) != 2 {
		return fmt.Errorf("removalWeights must have length 2")
	}
	if len(req.InsertionWeights) > 0 && len(req.InsertionWeights) != 2 {
		return fmt.Errorf("insertionWeights must have length 2")
	}
	if req.Objectives != nil {
		allowed := map[string]struct{}{"drivetime": {}, "lateness": {}, "failed": {}, "distance": {}}
		for k, v := range req.Objectives {
			if v < 0 {
				return fmt.Errorf("objective %s must be >= 0", k)
			}
			if _, ok := allowed[strings.ToLower(k)]; !ok {
				return fmt.Errorf("unknown objective key: %s (allowed: driveTime,lateness,failed,distance)", k)
			}
		}
	}
	return nil
}
