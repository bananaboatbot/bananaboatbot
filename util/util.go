package util

import (
	"github.com/yuin/gopher-lua"
)

func BoolFromTable(table *lua.LTable, key string, defaultTrue bool) bool {
	lv := table.RawGetString(key)
	switch lType := lv.Type(); lType {
	case lua.LTBool:
		boolVal := lv.(lua.LBool)
		if boolVal == lua.LTrue {
			return true
		} else {
			return false
		}
	case lua.LTNil:
		return defaultTrue
	default:
		return true
	}
}
