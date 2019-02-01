package main

import (
	"flag"
	"net/http"
	"log"
	"os"
	"os/signal"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	luaFile := flag.String("lua", "", "Path to Lua script")
	ringSize := flag.Int("ring-size", 100, "Number of entries in log ringbuffer")
	webAddr := flag.String("addr", "localhost:9781", "Listening address for WebUI")
	flag.Parse()

	logger := NewLogger(&LoggerConfig{
		ringSize: *ringSize,
	})
	log.SetOutput(logger)

	b := NewBananaBoatBot(*luaFile)
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
	defer b.Close()
	go http.ListenAndServe(*webAddr, nil)
	go b.Loop()

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan
}
