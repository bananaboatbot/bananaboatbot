package main

import (
	"bytes"
	"container/ring"
	"os"
)

// Logger contains custom elements of our logger
type Logger struct {
	config *LoggerConfig
	ring   *ring.Ring
}

// LoggerConfig contains configuration for the logger
type LoggerConfig struct {
	ringSize int
}

// Handle writes
func (l *Logger) Write(b []byte) (wrote int, err error) {
	// Convert message to string, set it to ring buffer
	l.ring.Value = string(b)
	// Move ringbuffer to next value
	l.ring = l.ring.Next()
	// Write message to stdout
	return os.Stdout.Write(b)
}

// Returns ringbuffer as []byte
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

// Creates a new Logger
func NewLogger(config *LoggerConfig) *Logger {
	// Create logger
	l := &Logger{
		config: config,
		ring:   ring.New(config.ringSize),
	}
	// Populate ringbuffer with empty strings
	for i := 0; i < config.ringSize; i++ {
		l.ring.Value = ""
		l.ring = l.ring.Next()
	}
	// Return logger
	return l
}
