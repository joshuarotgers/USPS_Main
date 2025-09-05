package opt

import "sync"

type key struct{
    Tenant string
    PlanDate string
    Algo string
}

var (
    mu sync.Mutex
    store = map[key]Metrics{}
)

func RecordMetrics(tenant, planDate, algo string, m Metrics) {
    mu.Lock()
    store[key{Tenant:tenant, PlanDate:planDate, Algo:algo}] = m
    mu.Unlock()
}

func GetMetrics(tenant, planDate string) map[string]Metrics {
    mu.Lock()
    defer mu.Unlock()
    out := map[string]Metrics{}
    for k, v := range store {
        if k.Tenant == tenant && k.PlanDate == planDate {
            out[k.Algo] = v
        }
    }
    return out
}

