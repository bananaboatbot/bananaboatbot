package log_test

import (
	"bytes"
	"log"
	"testing"

	blog "github.com/fatalbanana/bananaboatbot/log"
)

func TestLogger(t *testing.T) {
	logger := blog.NewLogger(&blog.LoggerConfig{
		RingSize: 2,
		Quiet:    true,
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
