package main

import (
	"bytes"
	"container/ring"
	"os"
)

type Logger struct {
	config *LoggerConfig
	ring *ring.Ring
}

type LoggerConfig struct {
	ringSize int
}

func (l *Logger) Write(b []byte) (wrote int, err error) {
	l.ring.Value = string(b)
	l.ring = l.ring.Next()
	return os.Stdout.Write(b)
}

func (l *Logger) ShowRing() (log []byte) {
	var b bytes.Buffer
	l.ring.Do(func(p interface{}) {
		b.WriteString(p.(string))
	})
	return b.Bytes()
}

func NewLogger(config *LoggerConfig) *Logger {
	l := &Logger{
		config: config,
		ring: ring.New(config.ringSize),
	}
	for i := 0; i < config.ringSize; i++ {
		l.ring.Value = ""
		l.ring = l.ring.Next()
	}
	return l
}
