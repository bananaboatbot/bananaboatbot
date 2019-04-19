package gluamap

import (
	"sync"

	lua "github.com/yuin/gopher-lua"
)

type GluaMap struct {
	gluaMap map[lua.LValue]lua.LValue
	mutex   sync.RWMutex
}

func (g *GluaMap) index(L *lua.LState) int {
	key := L.Get(2)
	g.mutex.RLock()
	val, ok := g.gluaMap[key]
	g.mutex.RUnlock()
	if !ok {
		L.Push(lua.LNil)
	} else {
		L.Push(val)
	}
	return 1
}

func (g *GluaMap) newIndex(L *lua.LState) int {
	key := L.Get(2)
	if !isValidKey(key) {
		L.RaiseError("key is not acceptable")
		return 0
	}
	val := L.Get(3)
	if !isGoroutineSafe(val) {
		L.RaiseError("value is not goroutine safe")
		return 0
	}
	isNil := (val.Type() == lua.LTNil)
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if !isNil {
		g.gluaMap[key] = val
	} else {
		delete(g.gluaMap, key)
	}
	return 0
}

func (g *GluaMap) Loader(L *lua.LState) int {
	metaMethods := map[string]lua.LGFunction{
		"__index":    g.index,
		"__newindex": g.newIndex,
	}
	metaTable := L.SetFuncs(L.NewTable(), metaMethods)
	mod := L.CreateTable(0, 0)
	L.SetMetatable(mod, metaTable)
	L.Push(mod)
	return 1
}

func New() *GluaMap {
	return &GluaMap{
		gluaMap: make(map[lua.LValue]lua.LValue),
	}
}

// we will only accept strings and numbers as keys
func isValidKey(lv lua.LValue) bool {
	switch lv.(type) {
	case lua.LNumber, lua.LString:
		return true
	default:
		return false
	}
}

// from https://github.com/yuin/gopher-lua/blob/master/utils.go
func isGoroutineSafe(lv lua.LValue) bool {
	switch v := lv.(type) {
	case *lua.LFunction, *lua.LUserData, *lua.LState:
		return false
	case *lua.LTable:
		return v.Metatable == lua.LNil
	default:
		return true
	}
}
