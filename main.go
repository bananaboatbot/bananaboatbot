package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Set up and parse commandline flags
	luaFile := flag.String("lua", "", "Path to Lua script")
	ringSize := flag.Int("ring-size", 100, "Number of entries in log ringbuffer")
	webAddr := flag.String("addr", "localhost:9781", "Listening address for WebUI")
	flag.Parse()

	// Set up custom logger for maintaining log in ringbuffer
	logger := NewLogger(&LoggerConfig{
		ringSize: *ringSize,
	})
	log.SetOutput(logger)

	// Create BananaBoatBot
	b := NewBananaBoatBot(*luaFile)

	// Setup handlers for webserver
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		b.ReloadLua()
	})
	http.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		w.Write(logger.ShowRing())
	})
	http.HandleFunc("/quit", func(w http.ResponseWriter, r *http.Request) {
		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = p.Signal(os.Interrupt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	http.Handle("/metrics", promhttp.Handler())
	// Start webserver
	go http.ListenAndServe(*webAddr, nil)

	// Invoke shutdown-related tasks on exit
	defer b.Close()

	// Process input from IRC servers
	go func() {
		for {
			b.LoopOnce()
		}
	}()

	// Catch interrupt signal and exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan
}
