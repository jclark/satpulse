package mon

type syncState int

const (
	noSync syncState = iota
	inSync
)

// String returns a human-readable representation of the sync state
func (s syncState) String() string {
	switch s {
	case noSync:
		return "out of sync"
	case inSync:
		return "in sync"
	default:
		return "unknown"
	}
}
