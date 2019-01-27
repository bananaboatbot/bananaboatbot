package main

import (
	"flag"
	"net/http"
)

func main() {
	luaFile := flag.String("lua", "", "Path to Lua script")
	webAddr := flag.String("addr", "localhost:9781", "Listening address for WebUI")
	flag.Parse()

	b := NewBananaBoatBot(*luaFile)
	http.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		b.ReloadLua()
	})
	go http.ListenAndServe(*webAddr, nil)
	defer b.Close()

	b.Loop()
}
