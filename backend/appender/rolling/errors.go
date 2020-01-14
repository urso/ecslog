package rolling

import "errors"

// ErrNoFile indicates that a message can not be logged, because the appender
// was not able to open a file for writing so far. The log message will be lost.
var ErrNoFile = errors.New("No log file open")

var ErrClosed = errors.New("rolling file appender has been closed")
