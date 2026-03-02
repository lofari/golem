package runner

// EventType identifies the kind of TUI event.
type EventType int

const (
	EventIterStart  EventType = iota // Iteration beginning
	EventOutputLine                  // A line of claude output
	EventIterEnd                     // Iteration finished
	EventLoopDone                    // Loop finished
)

// Event carries information from the builder loop to the TUI.
type Event struct {
	Type    EventType
	Iter    int            // EventIterStart, EventIterEnd
	MaxIter int            // EventIterStart
	Task    string         // EventIterEnd
	Outcome string         // EventIterEnd
	Line    string         // EventOutputLine
	Err     error          // EventIterEnd (if failed), EventLoopDone
	Result  *BuilderResult // EventLoopDone
}
