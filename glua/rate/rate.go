package rate

import (
	"github.com/yuin/gopher-lua"
	"golang.org/x/time/rate"
)

const (
	luaLimiterTypeName = "limiter"
)

// limiterAllow wraps Limiter.Allow
func limiterAllow(L *lua.LState) int {
	// Get parameters
	self := L.CheckUserData(1)
	// Get and return result
	limiter := self.Value.(*rate.Limiter)
	L.Push(lua.LBool(limiter.Allow()))
	return 1
}

// limiterWait wraps Limiter.Wait
func limiterWait(L *lua.LState) int {
	// Get parameters
	self := L.CheckUserData(1)
	// Wait
	limiter := self.Value.(*rate.Limiter)
	limiter.Wait(L.Context())
	return 0
}

// newLimiter creates a new rate.Limiter
func newLimiter(L *lua.LState) int {
	// Get parameters
	limit := L.CheckNumber(1)
	burst := L.ToInt(2)
	// Create and populate userdata
	ud := L.NewUserData()
	ud.Value = rate.NewLimiter(rate.Limit(limit), burst)
	L.SetMetatable(ud, L.GetTypeMetatable(luaLimiterTypeName))
	// Return the userdata
	L.Push(ud)
	return 1
}

// RegisterGlobals registers our types
func RegisterGlobals(L *lua.LState) {
	// Register limiter type
	mt := L.NewTypeMetatable(luaLimiterTypeName)
	L.SetGlobal(luaLimiterTypeName, mt)
	// Add constructor
	L.SetField(mt, "new", L.NewFunction(newLimiter))
	// Add methods
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"allow": limiterAllow,
		"wait":  limiterWait,
	}))
}
