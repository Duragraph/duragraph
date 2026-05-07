package command

// Metrics is the application-layer port for the subset of platform
// metrics that command handlers need to update. The infrastructure
// type *monitoring.Metrics satisfies this interface at wiring time
// (cmd/server/main.go). Keeping this thin port here preserves the
// application -> infrastructure boundary.
//
// Implementations may be nil in tests; handlers MUST guard with a
// `h.metrics != nil` check before calling. A literal-nil interface
// value passed as a constructor argument satisfies that guard.
type Metrics interface {
	IncAssistants(tenantID string)
	DecAssistants(tenantID string)
	IncThreads(tenantID string)
	DecThreads(tenantID string)
}
