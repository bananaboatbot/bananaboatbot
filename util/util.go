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

func SetBoolToMapFromTable(bmap map[string]bool, tbl *lua.LTable, key string) bool {
	lv := tbl.RawGetString(key)
	if lv.Type() != lua.LTBool {
		return false
	}
	b := lv.(lua.LBool)
	if b == lua.LTrue {
		bmap[key] = true
	} else {
		bmap[key] = false
	}
	return true
}

func SetStringToMapFromTable(smap map[string]string, tbl *lua.LTable, key string) bool {
	lv := tbl.RawGetString(key)
	s := lua.LVAsString(lv)
	if len(s) > 0 {
		smap[key] = s
		return true
	}
	return false
}
