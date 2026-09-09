package model

// AvailabilityObservation is one final outcome of a real request. ID is generated
// by the server for each HTTP request, independently of caller-supplied IDs.
type AvailabilityObservation struct {
	ID              string
	StartedAt       int64
	CompletedAt     int64
	GroupID         int
	ModelName       string
	Outcome         string
	FirstResponseMs *int64
	Reason          string
}

const (
	AvailabilitySuccess  = "success"
	AvailabilityFailure  = "failure"
	AvailabilityExcluded = "excluded"
	AvailabilityUnknown  = "unknown"

	AvailabilityAsyncPendingReason       = "async_pending"
	AvailabilityProcessInterruptedReason = "process_interrupted"
)

type AvailabilityCatalogGroup struct {
	ID          int
	Name        string
	Description string
	Models      []string
}

// Catalog is an immutable request-time snapshot of subscription display ownership.
// ChannelGroups includes disabled channels so a final failure keeps its ownership.
type AvailabilityCatalog struct {
	Groups        []AvailabilityCatalogGroup
	ChannelGroups map[int]int
	NamedGroups   map[string][]int
	ChannelModels map[int][]string
	ChannelNames  map[int][]string
}

type AvailabilityMetric struct {
	State           string   `json:"state"`
	SuccessRate     *float64 `json:"success_rate"`
	FirstResponseMs *float64 `json:"first_response_ms"`
	LowSample       bool     `json:"low_sample"`
	Coverage        string   `json:"coverage"`
	LastObservedAt  *int64   `json:"last_observed_at"`
	HasFailures     bool     `json:"has_failures"`
}

type AvailabilityBucket struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	AvailabilityMetric
}

type AvailabilityModel struct {
	Name    string               `json:"name"`
	Active  bool                 `json:"active"`
	Current AvailabilityMetric   `json:"current"`
	Summary AvailabilityMetric   `json:"summary"`
	Buckets []AvailabilityBucket `json:"buckets"`
}

type AvailabilityGroup struct {
	ID          int                  `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Current     AvailabilityMetric   `json:"current"`
	Summary     AvailabilityMetric   `json:"summary"`
	Buckets     []AvailabilityBucket `json:"buckets"`
	Models      []AvailabilityModel  `json:"models"`
}

type AvailabilityReport struct {
	Range          string              `json:"range"`
	WindowStart    int64               `json:"window_start"`
	WindowEnd      int64               `json:"window_end"`
	CurrentStart   int64               `json:"current_start"`
	GeneratedAt    int64               `json:"generated_at"`
	CoverageStart  int64               `json:"coverage_start"`
	RefreshSeconds int                 `json:"refresh_seconds"`
	BucketSeconds  int64               `json:"bucket_seconds"`
	Groups         []AvailabilityGroup `json:"groups"`
}
