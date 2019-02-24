package log

import (
	"bytes"
	"container/ring"
	"io"
	"os"
)

// Logger contains custom elements of our logger
type Logger struct {
	config *LoggerConfig
	ring   *ring.Ring
	writer io.Writer
}

// LoggerConfig contains configuration for the logger
type LoggerConfig struct {
	RingSize int
}

// Write handles writes
func (l *Logger) Write(b []byte) (wrote int, err error) {
	// Convert message to string, set it to ring buffer
	l.ring.Value = string(b)
	// Move ringbuffer to next value
	l.ring = l.ring.Next()
	// Write message to stdout
	return l.writer.Write(b)
}

// ShowRing returns ringbuffer as []byte
func (l *Logger) ShowRing() (log []byte) {
	// Create bytes.Buffer
	var b bytes.Buffer
	// Iterate over ringbuffer
	l.ring.Do(func(p interface{}) {
		// Write value to bytes.Buffer
		b.WriteString(p.(string))
	})
	// Return bytes.Buffer as []byte
	return b.Bytes()
}

// NewLogger creates a new Logger
func NewLogger(config *LoggerConfig) *Logger {
	// Create logger
	l := &Logger{
		config: config,
		ring:   ring.New(config.RingSize),
		writer: os.Stdout,
	}
	// Populate ringbuffer with empty strings
	for i := 0; i < config.RingSize; i++ {
		l.ring.Value = ""
		l.ring = l.ring.Next()
	}
	// Return logger
	return l
}
