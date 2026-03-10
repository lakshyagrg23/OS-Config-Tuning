package main

const queueSize = 100

// WorkEvent is a decoded, string-based representation of a perf event.
// It is what gets pushed into the event queue and consumed by workers.
type WorkEvent struct {
	Pid      uint32
	Process  string
	Access   string
	FilePath string
}

// NewEventQueue returns a buffered channel used as the event queue.
func NewEventQueue() chan WorkEvent {
	return make(chan WorkEvent, queueSize)
}
