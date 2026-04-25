package flux

// Priority constants for UserEndpoint.
// Lower value = higher preference (preferred endpoint is selected first).
const (
	PriorityHigh   = 100  // High priority - most preferred
	PriorityNormal = 500  // Normal priority - default
	PriorityLow    = 1000 // Low priority - least preferred
)
