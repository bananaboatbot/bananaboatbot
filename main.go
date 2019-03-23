package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/bananaboatbot/bananaboatbot/bot"
	"github.com/bananaboatbot/bananaboatbot/client"
	blog "github.com/bananaboatbot/bananaboatbot/log"
	"github.com/bananaboatbot/bananaboatbot/resources"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultIrcPort = 6667
)

func main() {
	// Set up and parse commandline flags
	downloadResources := flag.Bool("download-resources", false, "Install latest resource files and exit")
	luaFile := flag.String("lua", "", "Path to Lua script")
	logCommands := flag.Bool("log-commands", false, "Log commands received from servers")
	maxReconnect := flag.Int("max-reconnect", 3600, "Maximum reconnect interval in seconds")
	packageDir := flag.String("package-path", "", "Path to add to Lua package.path")
	ringSize := flag.Int("ring-size", 100, "Number of entries in log ringbuffer")
	version := flag.Bool("version", false, "Print version number & exit")
	webAddr := flag.String("addr", "localhost:9781", "Listening address for WebUI")
	flag.Parse()

	if *downloadResources {
		resources.GetResources()
		return
	}

	if *version {
		printVersion()
		return
	}

	// Set up custom logger for maintaining log in ringbuffer
	logger := blog.NewLogger(&blog.LoggerConfig{
		RingSize: *ringSize,
	})
	log.SetOutput(logger)

	// Create BananaBoatBot
	ctx, cancel := context.WithCancel(context.Background())
	b := bot.NewBananaBoatBot(ctx,
		&bot.BananaBoatBotConfig{
			DefaultIrcPort: defaultIrcPort,
			LogCommands:    *logCommands,
			LuaFile:        *luaFile,
			PackageDir:     *packageDir,
			MaxReconnect:   *maxReconnect,
			NewIrcServer:   client.NewIrcServer,
		},
	)

	// Register metrics
	b.Metrics.HandlersDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "bananaboatbot_handlers_duration_seconds",
		Help: "Summary of handler processing durations",
	})
	b.Metrics.WorkersDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "bananaboatbot_workers_duration_seconds",
		Help: "Summary of worker processing durations",
	})

	prometheus.MustRegister(b.Metrics.HandlersDuration)
	prometheus.MustRegister(b.Metrics.WorkersDuration)

	defer func() {
		cancel()
		b.Close(ctx)
	}()

	// Setup handlers for webserver
	// reload reloads Lua
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		err := b.ReloadLua(ctx)
		if err != nil {
			log.Printf("Lua error: %s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	// unrequire unloads Lua libraries
	http.HandleFunc("/unrequire", func(w http.ResponseWriter, r *http.Request) {
		b.Unrequire(ctx)
	})
	// log displays the log ringbuffer
	http.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = []string{"text/plain; charset=utf-8"}
		w.Write(logger.ShowRing())
	})
	// quit exits the process
	http.HandleFunc("/quit", func(w http.ResponseWriter, r *http.Request) {
		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			log.Printf("Error: %s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = p.Signal(os.Interrupt)
		if err != nil {
			log.Printf("Error: %s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	// metrics displays performance metrics using prometheus
	http.Handle("/metrics", promhttp.Handler())
	// rpc/foo calls functions defined in Lua
	http.Handle("/rpc/", b)
	// Start webserver
	go http.ListenAndServe(*webAddr, nil)

	// Catch interrupt signal and exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan
}
