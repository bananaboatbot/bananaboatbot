package gluamap_test

import (
	"testing"

	"github.com/yuin/gopher-lua"

	gluamap "github.com/bananaboatbot/bananaboatbot/glua/map"
)

func TestGluaMap(t *testing.T) {
	L := lua.NewState()

	gm := gluamap.New()

	L.PreloadModule("gluamap", gm.Loader)

	err := L.DoString(`
	  local gluamap = require "gluamap"
	  gluamap["foo"] = "bar"
	  gluamap["hello"] = "goodbye"
	  gluamap["hello"] = nil
	  gluamap[1] = "ack"
	  gluamap[2] = "ack"
	  gluamap[1] = "bar"
	  gluamap["ok"] = {}
	  gluamap.ok.whatever = "fine"
	`)
	if err != nil {
		t.Fatal(err)
	}
	L.Close()

	L = lua.NewState()
	L.PreloadModule("gluamap", gm.Loader)
	err = L.DoString(`
	  local gluamap = require "gluamap"
	  assert(gluamap["foo"] == "bar")
	  assert(gluamap["donkey"] == nil)
	  assert(gluamap["hello"] == nil)
	  assert(gluamap.ok.whatever == "fine")
	  assert(gluamap[2] == "ack")
	  assert(gluamap[1] == "bar")
	`)
	if err != nil {
		t.Fatal(err)
	}
	L.Close()

}
