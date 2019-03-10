package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/fatalbanana/bananaboatbot/bot"
	"github.com/fatalbanana/bananaboatbot/client"
	blog "github.com/fatalbanana/bananaboatbot/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultIrcPort = 6667
)

func main() {
	// Set up and parse commandline flags
	luaFile := flag.String("lua", "", "Path to Lua script")
	logCommands := flag.Bool("log-commands", false, "Log commands received from servers")
	maxReconnect := flag.Int("max-reconnect", 3600, "Maximum reconnect interval in seconds")
	packageDir := flag.String("package-path", "", "Path to add to Lua package.path")
	ringSize := flag.Int("ring-size", 100, "Number of entries in log ringbuffer")
	version := flag.Bool("version", false, "Print version number & exit")
	webAddr := flag.String("addr", "localhost:9781", "Listening address for WebUI")
	flag.Parse()

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
	defer func() {
		cancel()
		b.Close(ctx)
	}()

	// Setup handlers for webserver
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		err := b.ReloadLua(ctx)
		if err != nil {
			log.Printf("Lua error: %s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	http.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		w.Write(logger.ShowRing())
	})
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
	http.Handle("/metrics", promhttp.Handler())
	// Start webserver
	go http.ListenAndServe(*webAddr, nil)

	// Catch interrupt signal and exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan
}
