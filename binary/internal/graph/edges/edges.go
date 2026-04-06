package edges

// Edge type constants. Additional types (e.g. "contains", "foreign_key") are
// used as string literals throughout the adapters; they'll be promoted here
// incrementally as they become shared vocabulary.
const (
	// TypeRoutesTo represents a Kubernetes Service → Pod routing edge,
	// resolved via label-selector matching at discovery time.
	TypeRoutesTo = "routes_to"
)

// Connection and boundary data
type Edge struct {
	Id     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Label  string `json:"label,omitempty"`
}
