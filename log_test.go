package main

import (
	"bytes"
	"log"
	"testing"
)

func TestLogger(t *testing.T) {
	logger := NewLogger(&LoggerConfig{
		ringSize: 2,
		quiet:    true,
	})
	log.SetOutput(logger)
	log.SetFlags(0)
	log.Print("foo")
	log.Print("bar")
	log.Print("baz")
	res := logger.ShowRing()
	expected := []byte("bar\nbaz\n")
	if !bytes.Equal(res, expected) {
		t.Fatalf("%s != %s", res, expected)
	}
}
