package model

// Core domain types (simplified for stubs)

type OrderIn struct {
    ExternalRef   string            `json:"externalRef,omitempty"`
    Priority      int               `json:"priority,omitempty"`
    Attributes    map[string]any    `json:"attributes,omitempty"`
    Stops         []StopIn          `json:"stops"`
}

type StopIn struct {
    Type           string            `json:"type"`
    Address        string            `json:"address,omitempty"`
    Location       *GeoPoint         `json:"location"`
    TimeWindow     *TimeWindow       `json:"timeWindow,omitempty"`
    ServiceTimeSec int               `json:"serviceTimeSec,omitempty"`
    RequiredSkills []string          `json:"requiredSkills,omitempty"`
}

type GeoPoint struct {
    Lat float64 `json:"lat"`
    Lng float64 `json:"lng"`
}

type TimeWindow struct {
    Start string `json:"start"`
    End   string `json:"end"`
}

type OptimizeRequest struct {
    TenantID     string             `json:"tenantId"`
    PlanDate     string             `json:"planDate"`
    Algorithm    string             `json:"algorithm,omitempty"`
    TimeBudgetMs int                `json:"timeBudgetMs,omitempty"`
    MaxIterations int               `json:"maxIterations,omitempty"`
    InitTemp     float64            `json:"initTemp,omitempty"`
    Cooling      float64            `json:"cooling,omitempty"`
    RemovalWeights []float64        `json:"removalWeights,omitempty"`
    InsertionWeights []float64      `json:"insertionWeights,omitempty"`
    VehiclePool  []string           `json:"vehiclePool,omitempty"`
    Depots       []string           `json:"depots,omitempty"`
    IncludeOrders []string          `json:"includeOrders,omitempty"`
    Constraints  map[string]any     `json:"constraints,omitempty"`
    Objectives   map[string]float64 `json:"objectives,omitempty"`
    Reoptimize   bool               `json:"reoptimize,omitempty"`
    Freeze       *FreezeSpec        `json:"freeze,omitempty"`
}

type FreezeSpec struct {
    Routes    []string `json:"routes,omitempty"`
    UpToLegID string   `json:"upToLegId,omitempty"`
}

type Route struct {
    ID            string             `json:"id"`
    Version       int                `json:"version"`
    PlanDate      string             `json:"planDate,omitempty"`
    Status        string             `json:"status"`
    DriverID      string             `json:"driverId,omitempty"`
    VehicleID     string             `json:"vehicleId,omitempty"`
    Legs          []Leg              `json:"legs"`
    CostBreakdown map[string]float64 `json:"costBreakdown,omitempty"`
    AutoAdvance   *AutoAdvancePolicy `json:"autoAdvance,omitempty"`
    BreaksCount   int                `json:"breaksCount,omitempty"`
    TotalBreakSec int                `json:"totalBreakSec,omitempty"`
}

type Leg struct {
    ID           string `json:"id"`
    Seq          int    `json:"seq"`
    Kind         string `json:"kind,omitempty"`
    BreakSec     int    `json:"breakSec,omitempty"`
    FromStopID   string `json:"fromStopId,omitempty"`
    ToStopID     string `json:"toStopId,omitempty"`
    DistM        int    `json:"distM,omitempty"`
    DriveSec     int    `json:"driveSec,omitempty"`
    ETAArrival   string `json:"etaArrival,omitempty"`
    ETADeparture string `json:"etaDeparture,omitempty"`
    Status       string `json:"status,omitempty"`
}

type AssignmentRequest struct {
    DriverID string `json:"driverId"`
    VehicleID string `json:"vehicleId"`
    StartAt  string `json:"startAt,omitempty"`
}

type DriverEvent struct {
    Type     string         `json:"type"`
    DriverID string         `json:"driverId,omitempty"`
    RouteID  string         `json:"routeId,omitempty"`
    LegID    string         `json:"legId,omitempty"`
    StopID   string         `json:"stopId,omitempty"`
    TS       string         `json:"ts"`
    Payload  map[string]any `json:"payload,omitempty"`
}

type PoDRequest struct {
    TenantID string         `json:"tenantId"`
    OrderID  string         `json:"orderId"`
    StopID   string         `json:"stopId"`
    Type     string         `json:"type"`
    Media    *PoDMedia      `json:"media,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}

type PoDMedia struct {
    UploadURL string `json:"uploadUrl,omitempty"`
    SHA256    string `json:"sha256,omitempty"`
}

type SubscriptionRequest struct {
    TenantID string   `json:"tenantId"`
    URL      string   `json:"url"`
    Events   []string `json:"events"`
    Secret   string   `json:"secret"`
}

type Subscription struct {
    ID       string   `json:"id"`
    TenantID string   `json:"tenantId"`
    URL      string   `json:"url"`
    Events   []string `json:"events"`
    Secret   string   `json:"secret,omitempty"`
}

// Read models for API responses
type OrderOut struct {
    ID         string `json:"id"`
    TenantID   string `json:"tenantId"`
    ExternalRef string `json:"externalRef,omitempty"`
    Priority   int    `json:"priority"`
    Status     string `json:"status"`
}

type RoutePatch struct {
    Status      string `json:"status,omitempty"`
    LockedUntil string `json:"lockedUntil,omitempty"`
    AutoAdvance *AutoAdvancePolicy `json:"autoAdvance,omitempty"`
}

// AutoAdvancePolicy controls automatic progression to the next stop
type AutoAdvancePolicy struct {
    Enabled       bool   `json:"enabled,omitempty"`
    Trigger       string `json:"trigger,omitempty"` // pod_ack, depart, geofence_arrive
    MinDwellSec   int    `json:"minDwellSec,omitempty"`
    RequirePoD    bool   `json:"requirePoD,omitempty"`
    GracePeriodSec int   `json:"gracePeriodSec,omitempty"`
    MovingLock    bool   `json:"movingLock,omitempty"`
    HosMaxDriveSec int   `json:"hosMaxDriveSec,omitempty"`
}

type AdvanceRequest struct {
    Reason string `json:"reason,omitempty"`
    Force  bool   `json:"force,omitempty"`
}

type AdvanceResult struct {
    RouteID    string `json:"routeId"`
    FromLegID  string `json:"fromLegId,omitempty"`
    FromStopID string `json:"fromStopId,omitempty"`
    ToLegID    string `json:"toLegId,omitempty"`
    ToStopID   string `json:"toStopId,omitempty"`
    TS         string `json:"ts"`
    Changed    bool   `json:"changed"`
}

type AdvanceResponse struct {
    Result AdvanceResult `json:"result"`
    Route  Route         `json:"route"`
    Alerts []PolicyAlert `json:"alerts,omitempty"`
}

type PolicyAlert struct {
    Reason string `json:"reason"`
    TS     string `json:"ts"`
}

// HOS / Shift models
type HOSUpdate struct {
    Action string `json:"action"` // shift_start, shift_end, break_start, break_end
    TS     string `json:"ts"`
    Type   string `json:"type,omitempty"` // rest, meal, other
    Note   string `json:"note,omitempty"`
}

// Geofences
type GeofenceInput struct {
    Name    string            `json:"name,omitempty"`
    Type    string            `json:"type,omitempty"`
    RadiusM int               `json:"radiusM,omitempty"`
    Center  *GeoPoint         `json:"center,omitempty"`
    Rules   map[string]any    `json:"rules,omitempty"`
}

type Geofence struct {
    ID       string            `json:"id"`
    TenantID string            `json:"tenantId"`
    Name     string            `json:"name,omitempty"`
    Type     string            `json:"type,omitempty"`
    RadiusM  int               `json:"radiusM,omitempty"`
    Center   *GeoPoint         `json:"center,omitempty"`
    Rules    map[string]any    `json:"rules,omitempty"`
}

// Media presign
type PresignRequest struct {
    TenantID    string `json:"tenantId"`
    FileName    string `json:"fileName"`
    ContentType string `json:"contentType"`
    Bytes       int64  `json:"bytes,omitempty"`
    SHA256      string `json:"sha256,omitempty"`
}
