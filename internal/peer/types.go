package peer

type ServerState int

const (
	StateIdle ServerState = iota
	StateConnecting
	StateActive
	StateRetrying
	StateUnknown
)

var stateName = map[ServerState]string{
	StateIdle:       "idle",
	StateConnecting: "connecting",
	StateActive:     "active",
	StateRetrying:   "retrying",
	StateUnknown:    "unknown",
}

func (ss ServerState) String() string {
	if name, ok := stateName[ss]; ok {
		return name
	}

	return stateName[StateUnknown]
}
