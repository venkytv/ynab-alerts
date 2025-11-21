package rules

import "log"

// DebugLogger receives verbose debug messages during rule evaluation.
type DebugLogger interface {
	Debugf(format string, args ...interface{})
}

type noopDebugLogger struct{}

func (noopDebugLogger) Debugf(string, ...interface{}) {}

var dbg DebugLogger = noopDebugLogger{}

// SetDebugLogger sets the logger used for debug output. Pass nil to disable.
func SetDebugLogger(l DebugLogger) {
	if l == nil {
		dbg = noopDebugLogger{}
		return
	}
	dbg = l
}

// LogDebugLogger writes debug lines to the standard logger with a prefix.
type LogDebugLogger struct{}

func (LogDebugLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[debug] "+format, args...)
}
