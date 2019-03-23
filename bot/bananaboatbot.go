package bot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/bananaboatbot/bananaboatbot/client"
	"github.com/bananaboatbot/bananaboatbot/glua/rate"
	"github.com/bananaboatbot/bananaboatbot/resources"
	"github.com/bananaboatbot/bananaboatbot/util"
	"github.com/bananaboatbot/gluahttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/yuin/gopher-lua"
	"golang.org/x/net/html"

	irc "gopkg.in/sorcix/irc.v2"
	luajson "layeh.com/gopher-json"
)

// BananaBoatBot contains config & state of the bot
type BananaBoatBot struct {
	// Config contains elements that are passed on initialization
	Config *BananaBoatBotConfig
	// Metrics contains performance metrics
	Metrics *BananaBoatBotMetrics
	// curNet is set to friendly name of network we're handling a message from
	curNet string
	// curMessage is set to the message being handled
	curMessage *irc.Message
	// handlers is a map of IRC command names to Lua functions
	handlers map[string]*lua.LFunction
	// handlersMutex protects the handlers map
	handlersMutex sync.RWMutex
	// httpClient is used for HTTP requests
	httpClient http.Client
	// luaDirectory contains path to Lua libraries for adding to package.path
	luaDirectory string
	// luaMutex protects shared Lua state
	luaMutex sync.Mutex
	// luaPool is a pool for when shared state is undesirable
	luaPool sync.Pool
	// luaState contains shared Lua state
	luaState *lua.LState
	// nick is the default nick of the bot
	nick string
	// realname is the default "real name" of the bot
	realname string
	// username is the default username of the bot
	username string
	// servers is a map of friendly names to IRC servers
	Servers sync.Map
	// mutex for handling of server reconnecting
	serverReconnectMutex sync.Mutex
	// Map of endpoint names to function prototypes
	web map[string]BananaBoatBotWebFunc
	// Mutex protecting web map
	webMutex sync.RWMutex
}

// BananaBoatBotMetrics contains performance metrics
type BananaBoatBotMetrics struct {
	HandlersDuration prometheus.Summary
	WorkersDuration  prometheus.Summary
}

// BananaBoatBotWebFunc contains RPC functions defined in Lua
type BananaBoatBotWebFunc struct {
	CollectHeaders   bool
	Function         *lua.LFunction
	FunctionProto    *lua.FunctionProto
	ParseJsonBody    bool
	ParseQueryString bool
	UseSharedState   bool
}

// Close handles shutdown-related tasks
func (b *BananaBoatBot) Close(ctx context.Context) {
	b.serverReconnectMutex.Lock()
	log.Print("Shutting down")
	b.Servers.Range(func(k, value interface{}) bool {
		b.Servers.Delete(k)
		value.(client.IrcServerInterface).Close(ctx)
		return true
	})
	b.luaMutex.Lock()
	b.luaState.Close()
	b.luaMutex.Unlock()
	b.serverReconnectMutex.Unlock()
}

func luaParamsFromMessage(svrName string, msg *irc.Message) []lua.LValue {
	// Make empty list of parameters to pass to Lua
	luaParams := make([]lua.LValue, len(msg.Params)+4)
	// First parameter is the name of the server
	luaParams[0] = lua.LString(svrName)
	// Following three parameters are nick/user/host if set
	if msg.Prefix != nil {
		luaParams[1] = lua.LString(msg.Prefix.Name)
		luaParams[2] = lua.LString(msg.Prefix.User)
		luaParams[3] = lua.LString(msg.Prefix.Host)
	}
	// Fifth parameter onwards is unpacked parameters of the irc.Message
	pi := 0
	for i := 4; i < len(luaParams); i++ {
		luaParams[i] = lua.LString(msg.Params[pi])
		pi++
	}
	return luaParams
}

func (b *BananaBoatBot) handleLuaReturnNames(ctx context.Context, svrName string, message *lua.LTable) error {
	var params []string
	// Get 'command' string from table
	lv := message.RawGetString("command")
	command := lua.LVAsString(lv)
	// Get 'log' string from table
	lv = message.RawGetString("log")
	logMessage := lua.LVAsString(lv)
	if len(command) == 0 {
		if len(logMessage) == 0 {
			// If 'command' is missing and so is 'log', consider it an error
			return errors.New("lua error: no command found in associative table")
		} else {
			// Or if we have just 'log' then log the message & return
			log.Print(logMessage)
			return nil
		}
	} else if len(logMessage) > 0 {
		// Handle logging and continue
		log.Print(logMessage)
	}
	// Get 'net' from table
	lv = message.RawGetString("net")
	net := lua.LVAsString(lv)
	// If missing use the current server
	if len(net) == 0 {
		net = svrName
	}
	// Get 'params' table from table
	lv = message.RawGetString("params")
	if paramsT, ok := lv.(*lua.LTable); ok {
		// Make a list of parameters
		params = make([]string, paramsT.MaxN())
		// Copy parameters from Lua
		paramsIndex := 0
		paramsT.ForEach(func(index lua.LValue, paramL lua.LValue) {
			params[paramsIndex] = lua.LVAsString(paramL)
			paramsIndex++
		})
	} else {
		// No parameters, make an empty array
		params = make([]string, 0)
	}
	err := b.sendMessage(net, &irc.Message{
		Command: command,
		Params:  params,
	})
	return err
}

func (b *BananaBoatBot) handleLuaReturnNumeric(ctx context.Context, svrName string, commandLV lua.LValue, message *lua.LTable) error {
	var params []string
	command := lua.LVAsString(commandLV)
	if len(command) == 0 {
		return errors.New("lua error: no command found in numeric table")
	}
	end := message.MaxN()
	if end < 2 {
		params = make([]string, 0)
	} else {
		paramsIndex := 0
		params = make([]string, end-1)
		for i := 2; i <= end; i++ {
			lv := message.RawGetInt(i)
			params[paramsIndex] = lua.LVAsString(lv)
			paramsIndex++
		}
	}
	err := b.sendMessage(svrName, &irc.Message{
		Command: command,
		Params:  params,
	})
	return err
}

func (b *BananaBoatBot) sendMessage(net string, ircMessage *irc.Message) error {
	svr, ok := b.Servers.Load(net)
	if ok {
		select {
		case svr.(client.IrcServerInterface).GetMessages() <- *ircMessage:
			break
		default:
			return fmt.Errorf("channel full, message to server dropped: %s", ircMessage)
		}
	} else {
		return fmt.Errorf("lua eror: Invalid server: %s", net)
	}
	return nil
}

func (b *BananaBoatBot) handleLuaReturnValues(ctx context.Context, svrName string, luaState *lua.LState) {
	// Ignore nil
	lv := luaState.Get(-1)
	if lv.Type() == lua.LTNil {
		return
	}
	// Get table result
	res := luaState.CheckTable(-1)
	// For each numeric index in the table result...
	res.ForEach(func(index lua.LValue, messageL lua.LValue) {
		// Get the nested table..
		if message, ok := messageL.(*lua.LTable); ok {
			var err error
			// Check if numeric index is present
			lv := message.RawGetInt(1)
			if lv.Type() == lua.LTNil {
				err = b.handleLuaReturnNames(ctx, svrName, message)
			} else {
				err = b.handleLuaReturnNumeric(ctx, svrName, lv, message)
			}
			if err != nil {
				log.Print(err)
				return
			}
		}
	})
}

// ServeHTTP calls functions defined in Lua
func (b *BananaBoatBot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try find last element of path in map...
	name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	b.webMutex.RLock()
	webFunc, ok := b.web[name]
	b.webMutex.RUnlock()
	if !ok {
		http.Error(w, "does not exist", http.StatusNotFound)
		return
	}
	// Set up state and function as appropriate
	var state *lua.LState
	var luaFunction *lua.LFunction
	if webFunc.UseSharedState {
		state = b.luaState
		luaFunction = webFunc.Function
		b.luaMutex.Lock()
		defer b.luaMutex.Unlock()
	} else {
		state = b.luaPool.Get().(*lua.LState)
		defer func() {
			// Clear stack and return state to pool
			state.SetTop(0)
			b.luaPool.Put(state)
		}()
		luaFunction = state.NewFunctionFromProto(webFunc.FunctionProto)
	}

	// Create table to pass to function
	tbl := state.CreateTable(0, 0)

	// Add parsed query string or nil according to configuration
	if webFunc.ParseQueryString {
		queryTbl := state.CreateTable(0, 0)
		q := r.URL.Query()
		for label, values := range q {
			lValues := state.CreateTable(0, 0)
			for _, value := range values {
				lValues.Append(lua.LString(value))
			}
			state.RawSet(queryTbl, lua.LString(label), lValues)
		}
		state.RawSet(tbl, lua.LString("query"), queryTbl)
	}

	// Try parse JSON body according to configuration
	if webFunc.ParseJsonBody {
		dec := json.NewDecoder(r.Body)
		jsonMap := make(map[string]interface{})
		err := dec.Decode(&jsonMap)
		if err != nil {
			log.Printf("Error parsing JSON body for %s: %s", name, err)
		} else {
			jsonTbl := state.CreateTable(0, 0)
			for key, value := range jsonMap {
				jsonTbl.RawSetString(key, luajson.DecodeValue(state, value))
			}
			state.RawSet(tbl, lua.LString("json"), jsonTbl)
		}
	}

	// Add headers to request according to configuration
	if webFunc.CollectHeaders {
		headersTbl := state.CreateTable(0, 0)
		for k, l := range r.Header {
			hdrList := state.CreateTable(0, 0)
			for _, v := range l {
				hdrList.Append(lua.LString(v))
			}
			state.RawSet(headersTbl, lua.LString(k), hdrList)
		}
		state.RawSet(tbl, lua.LString("headers"), headersTbl)
	}

	// Call function
	err := state.CallByParam(lua.P{
		Fn:      luaFunction,
		NRet:    1,
		Protect: true,
	}, tbl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Handle return values
	b.handleLuaReturnValues(state.Context(), "", state)
}

// HandleHandlers invokes any registered Lua handlers for a command
func (b *BananaBoatBot) HandleHandlers(ctx context.Context, svrName string, msg *irc.Message) {
	// Set up instrumentation
	timer := prometheus.NewTimer(b.Metrics.HandlersDuration)
	defer timer.ObserveDuration()
	// Maybe log raw input
	if b.Config.LogCommands {
		// Log message
		log.Printf("[%s] %s", svrName, msg)
	}
	// Get read mutex for handlers map
	b.handlersMutex.RLock()
	// If we have a function corresponding to this command...
	if luaFunction, ok := b.handlers[msg.Command]; ok {
		// Release read mutex for handlers
		b.handlersMutex.RUnlock()
		// Deferred release of lua state mutex
		defer b.luaMutex.Unlock()
		// Make list of parameters to pass to Lua
		luaParams := luaParamsFromMessage(svrName, msg)
		// Get Lua mutex
		b.luaMutex.Lock()
		// Store some state information
		b.curMessage = msg
		b.curNet = svrName
		// Call function
		err := b.luaState.CallByParam(lua.P{
			Fn:      luaFunction,
			NRet:    1,
			Protect: true,
		}, luaParams...)
		// Abort on failure
		if err != nil {
			log.Printf("Handler for %s failed: %s", msg.Command, err)
			return
		}
		// Handle return values
		b.handleLuaReturnValues(ctx, svrName, b.luaState)
		// Clear stack
		b.luaState.SetTop(0)
	} else {
		// Release read mutex for handlers
		b.handlersMutex.RUnlock()
	}
}

// HandleErrors reconnects servers on error
func (b *BananaBoatBot) HandleErrors(ctx context.Context, svrName string, err error) {
	// Log the error
	log.Printf("[%s] Connection error: %s", svrName, err)

	// Try reconnect to the server if still configured
	svr, ok := b.Servers.Load(svrName)
	// Server is no longer configured, do nothing
	if !ok {
		return
	}
	s := svr.(client.IrcServerInterface)
	// Error doesn't belong to current incarnation, do nothing
	if ctx.Done() != s.Done() {
		return
	}
	b.serverReconnectMutex.Lock()
	s.Close(ctx)
	newSvr, svrCtx := b.Config.NewIrcServer(
		b.luaState.Context(),
		svrName,
		s.GetSettings())
	newSvr.SetReconnectExp(*(s.GetReconnectExp()))
	b.Servers.Store(svrName, newSvr)
	b.serverReconnectMutex.Unlock()
	newSvr.ReconnectWait(svrCtx)
	b.serverReconnectMutex.Lock()
	newSvr.Dial(svrCtx)
	b.serverReconnectMutex.Unlock()
}

// Set default values from table returned by Lua
func (b *BananaBoatBot) setDefaultsFromLua(ctx context.Context, tbl *lua.LTable) {
	lv := tbl.RawGetString("nick")
	nick := lua.LVAsString(lv)
	if len(nick) > 0 {
		b.nick = nick
	}

	lv = tbl.RawGetString("realname")
	realname := lua.LVAsString(lv)
	if len(realname) > 0 {
		b.realname = realname
	}

	lv = tbl.RawGetString("username")
	username := lua.LVAsString(lv)
	if len(username) > 0 {
		b.username = username
	}
}

// Set and mtaintain handler functions based on table returned by Lua
func (b *BananaBoatBot) setHandlersFromLua(ctx context.Context, tbl *lua.LTable) {
	luaCommands := make(map[string]struct{})
	lv := tbl.RawGetString("handlers")
	if handlerTbl, ok := lv.(*lua.LTable); ok {
		b.handlersMutex.Lock()
		defer b.handlersMutex.Unlock()
		handlerTbl.ForEach(func(commandName lua.LValue, handlerFuncL lua.LValue) {
			if handlerFunc, ok := handlerFuncL.(*lua.LFunction); ok {
				commandNameStr := lua.LVAsString(commandName)
				b.handlers[commandNameStr] = handlerFunc
				luaCommands[commandNameStr] = struct{}{}
			}
		})
		// Delete handlers still in map but no longer defined in Lua
		for k := range b.handlers {
			if _, ok := luaCommands[k]; !ok {
				delete(b.handlers, k)
			}
		}
	} else {
		log.Printf("lua reload error: unexpected handlers type: %s", lv.Type())
	}
}

// (Re)create servers if deemed necessary
func (b *BananaBoatBot) maybeCreateServer(ctx context.Context, serverNameStr string, serverSettings *client.IrcServerSettings) {
	var createServer bool
	// Check if server already exists and/or if we need to (re)create it
	if oldSvr, ok := b.Servers.Load(serverNameStr); ok {
		oldSettings := oldSvr.(client.IrcServerInterface).GetSettings()
		createServer = (oldSettings.Basic != serverSettings.Basic)
	} else {
		createServer = true
	}
	if !createServer {
		return
	}
	log.Printf("Creating new IRC server: %s", serverNameStr)
	// Create new IRC server
	svr, svrCtx := b.Config.NewIrcServer(ctx, serverNameStr, serverSettings)
	// Set server to map
	oldSvr, ok := b.Servers.Load(serverNameStr)
	b.serverReconnectMutex.Lock()
	if ok {
		log.Printf("Destroying pre-existing IRC server: %s", serverNameStr)
		b.Servers.Delete(serverNameStr)
		oldSvr.(client.IrcServerInterface).Close(ctx)
	}
	b.Servers.Store(serverNameStr, svr)
	svr.(client.IrcServerInterface).Dial(svrCtx)
	b.serverReconnectMutex.Unlock()
}

// Set and maintain servers based on table returned by Lua
func (b *BananaBoatBot) setWebFromLua(ctx context.Context, tbl *lua.LTable) {
	// Get 'web' from table, expect table result
	lv := tbl.RawGetString("web")
	if lv.Type() == lua.LTNil {
		return
	}
	webTbl, ok := lv.(*lua.LTable)
	if !ok {
		log.Printf("lua reload error: unexpected web type: %s", lv.Type())
		return
	}
	// Iterate table, set function prototypes to map
	luaWebNames := make(map[string]struct{})
	b.webMutex.Lock()
	defer b.webMutex.Unlock()
	// Iterate over nested tables...
	webTbl.ForEach(func(nameLV lua.LValue, webSettingsLV lua.LValue) {
		// Use name from key in the parent table
		name := lua.LVAsString(nameLV)
		// Create BananaBoatWebFunc from nested table
		ourWebFunc := b.createWebFuncFromTable(name, webSettingsLV)
		if ourWebFunc == nil {
			return
		}
		// If we got a result set it to the map...
		b.web[name] = *ourWebFunc
		// And remember that the name was found to assist cleanup
		luaWebNames[name] = struct{}{}
	})
	// Delete handlers no longer defined in Lua
	for k := range b.web {
		_, ok := luaWebNames[k]
		if !ok {
			delete(b.web, k)
		}
	}
}

func (b *BananaBoatBot) createWebFuncFromTable(name string, webSettingsLV lua.LValue) *BananaBoatBotWebFunc {

	// Get nested table
	webSettings, ok := webSettingsLV.(*lua.LTable)
	if !ok {
		log.Printf("found unexpected type inside web: %s", webSettingsLV.Type())
		return nil
	}

	// Get function from 'func' key
	lv := webSettings.RawGetString("func")
	webFunc, ok := lv.(*lua.LFunction)
	if !ok {
		log.Printf("lua reload error: unexpected type at web:%s:func: %s", name, lv.Type())
		return nil
	}

	useSharedState := util.BoolFromTable(webSettings, "use_shared_state", false)

	ourWebFunc := BananaBoatBotWebFunc{
		CollectHeaders:   util.BoolFromTable(webSettings, "collect_headers", true),
		ParseJsonBody:    util.BoolFromTable(webSettings, "parse_json_body", false),
		ParseQueryString: util.BoolFromTable(webSettings, "parse_query_string", true),
		UseSharedState:   useSharedState,
	}

	if useSharedState {
		ourWebFunc.Function = webFunc
	} else {
		ourWebFunc.FunctionProto = webFunc.Proto
	}

	return &ourWebFunc
}

// Set and maintain servers based on table returned by Lua
func (b *BananaBoatBot) setServersFromLua(ctx context.Context, tbl *lua.LTable) {
	// Make map of server names collected from Lua
	luaServerNames := make(map[string]struct{})
	// Get 'servers' from table
	lv := tbl.RawGetString("servers")
	// Get table value
	serverTbl, ok := lv.(*lua.LTable)
	if !ok {
		log.Printf("lua reload error: unexpected servers type: %s", lv.Type())
		return
	}
	// Iterate over nested tables...
	serverTbl.ForEach(func(serverName lua.LValue, serverSettingsLV lua.LValue) {

		// Get nested table
		serverSettings, ok := serverSettingsLV.(*lua.LTable)
		if !ok {
			log.Printf("found unexpected type inside servers: %s", lv.Type())
			return
		}

		// Defaults
		nick := b.nick
		portInt := b.Config.DefaultIrcPort
		realname := b.realname
		username := b.username

		// Get 'server' string from table
		lv = serverSettings.RawGetString("server")
		host := lua.LVAsString(lv)

		// Get 'tls' bool from table (default false)
		tls := util.BoolFromTable(serverSettings, "tls", false)

		// Get 'tls_verify' bool from table (default true)
		verifyTLS := util.BoolFromTable(serverSettings, "tls_verify", true)

		// Get 'port' from table (use default from so-called config)
		lv = serverSettings.RawGetString("port")
		if port, ok := lv.(lua.LNumber); ok {
			portInt = int(port)
		}

		// Get 'nick' from table - use default if unavailable
		lv = serverSettings.RawGetString("nick")
		if ls, ok := lv.(lua.LString); ok {
			nick = lua.LVAsString(ls)
		}

		// Get 'realname' from table - use default if unavailable
		lv = serverSettings.RawGetString("realname")
		if ls, ok := lv.(lua.LString); ok {
			realname = lua.LVAsString(ls)
		}

		// Get 'username' from table - use default if unavailable
		lv = serverSettings.RawGetString("username")
		if ls, ok := lv.(lua.LString); ok {
			username = lua.LVAsString(ls)
		}

		// Remember we found this key so we can delete unused servers later
		serverNameStr := lua.LVAsString(serverName)
		luaServerNames[serverNameStr] = struct{}{}
		ircServerSettings := &client.IrcServerSettings{
			Basic: client.BasicIrcServerSettings{
				Host:      host,
				Port:      portInt,
				TLS:       tls,
				VerifyTLS: verifyTLS,
				Nick:      nick,
				Realname:  realname,
				Username:  username,
			},
			MaxReconnect:  float64(b.Config.MaxReconnect),
			ErrorCallback: b.HandleErrors,
			InputCallback: b.HandleHandlers,
		}

		// (Re)create the server if necessary
		b.maybeCreateServer(ctx, serverName.String(), ircServerSettings)
	})

	// Remove servers no longer defined in Lua
	b.Servers.Range(func(k, value interface{}) bool {
		if _, ok := luaServerNames[k.(string)]; !ok {
			log.Printf("Destroying removed IRC server: %s", k)
			b.Servers.Delete(k)
			go value.(client.IrcServerInterface).Close(ctx)
		}
		return true
	})
}

// ReloadLua deals with reloading Lua parts
func (b *BananaBoatBot) ReloadLua(ctx context.Context) error {
	b.luaMutex.Lock()
	defer func() {
		// Clear stack and release Lua mutex
		b.luaState.SetTop(0)
		b.luaMutex.Unlock()
	}()

	if err := b.luaState.DoFile(b.Config.LuaFile); err != nil {
		return err
	}

	lv := b.luaState.Get(-1)
	tbl, ok := lv.(*lua.LTable)
	if !ok {
		return fmt.Errorf("lua reload error: unexpected return type: %s", lv.Type())
	}

	// Get and set default values like 'nick'
	b.setDefaultsFromLua(ctx, tbl)

	// Deal with 'handlers'
	b.setHandlersFromLua(ctx, tbl)

	// Deal with 'servers'
	b.setServersFromLua(ctx, tbl)

	// Deal with 'web'
	b.setWebFromLua(ctx, tbl)

	return nil
}

// Unrequire ensures that Lua libraries are reloaded
func (b *BananaBoatBot) Unrequire(ctx context.Context) {
	// Set package.loaded[foo] = nil for all packages in shared state
	b.luaMutex.Lock()
	t := b.luaState.GetGlobal("package").(*lua.LTable).RawGet(lua.LString("loaded")).(*lua.LTable)
	t.ForEach(func(index lua.LValue, paramL lua.LValue) {
		b.luaState.RawSet(t, index, lua.LNil)
	})
	b.luaMutex.Unlock()
	// Recreate lua Pool
	newPool := sync.Pool{
		New: func() interface{} {
			return b.newLuaState(ctx, b.Config.PackageDir)
		},
	}
	oldPoolPtr := unsafe.Pointer(&b.luaPool)
	newPoolPtr := unsafe.Pointer(&newPool)
	atomic.SwapPointer(&oldPoolPtr, newPoolPtr)
}

// luaLibRandom provides access to cryptographic random numbers in Lua
func (b *BananaBoatBot) luaLibRandom(luaState *lua.LState) int {
	// First argument should be int for upper bound (probably at least 1)
	i := luaState.ToInt(1)
	// Generate random integer given user supplied range
	r, err := rand.Int(rand.Reader, big.NewInt(int64(i)))
	// Add 1 to result
	res := r.Int64() + 1
	// Push result to stack
	ln := lua.LNumber(res)
	luaState.Push(ln)
	// Push error to stack
	if err == nil {
		// It might be nil in which case push nil
		luaState.Push(lua.LNil)
	} else {
		// Otherwise push the error text
		luaState.Push(lua.LString(err.Error()))
	}
	return 2
}

// luaLibSleep provides the ability to sleep a goroutine
func (b *BananaBoatBot) luaLibSleep(luaState *lua.LState) int {
	// First argument should be number of milliseconds to sleep
	time.Sleep(time.Millisecond * time.Duration(luaState.CheckNumber(1)))
	return 0
}

// luaLibWorker runs a task in a goroutine
func (b *BananaBoatBot) luaLibWorker(luaState *lua.LState) int {
	defer luaState.SetTop(0)
	// First parameter should be function
	functionProto := luaState.CheckFunction(1).Proto
	// Rest of parameters are parameters for that function
	numParams := luaState.GetTop()
	if numParams == 1 {
		numParams = 0
	} else {
		numParams--
	}
	luaParams := make([]lua.LValue, numParams)
	goIndex := 0
	for i := 2; goIndex < numParams; i++ {
		lv := luaState.Get(i)
		luaParams[goIndex] = lv
		goIndex++
	}
	// Run function in new goroutine
	go func(functionProto *lua.FunctionProto, curNet string, curMessage *irc.Message) {
		// Set up instrumentation
		timer := prometheus.NewTimer(b.Metrics.WorkersDuration)
		defer timer.ObserveDuration()
		// Get luaState from pool
		newState := b.luaPool.Get().(*lua.LState)
		defer func() {
			// Clear stack and return state to pool
			newState.SetTop(0)
			b.luaPool.Put(newState)
		}()
		// Create function from prototype
		luaFunction := newState.NewFunctionFromProto(functionProto)
		// Sanitise parameters
		for i, v := range luaParams {
			if v.Type() == lua.LTFunction {
				luaParams[i] = newState.NewFunctionFromProto(v.(*lua.LFunction).Proto)
			}
		}
		// Call function
		err := newState.CallByParam(lua.P{
			Fn:      luaFunction,
			NRet:    1,
			Protect: true,
		}, luaParams...)
		if err != nil {
			log.Printf("worker: error calling Lua: %s", err)
			return
		}
		// Handle return values
		b.handleLuaReturnValues(newState.Context(), curNet, newState)
	}(functionProto, b.curNet, b.curMessage)
	return 0
}

// luaLibGetTitle tries to get the HTML title of a URL
func (b *BananaBoatBot) luaLibGetTitle(luaState *lua.LState) int {
	// First argument should be some URL to try process
	u := luaState.CheckString(1)
	// Make request
	resp, err := b.httpClient.Get(u)
	// Handle HTTP request failure
	if err != nil {
		luaState.Push(lua.LNil)
		log.Printf("HTTP client error: %s", err)
		return 1
	}
	// Expect to see text/html content-type
	if ct, ok := resp.Header["Content-Type"]; ok {
		if ct[0][:9] != "text/html" {
			luaState.Push(lua.LNil)
			log.Printf("GET of %s aborted: wrong content-type: %s", u, ct[0])
			return 1
		}
	} else {
		luaState.Push(lua.LNil)
		log.Printf("GET of %s aborted: no content-type header", u)
		return 1
	}
	// Read up to 12288 bytes
	limitedReader := &io.LimitedReader{R: resp.Body, N: 12288}
	// Create new tokenizer
	tokenizer := html.NewTokenizer(limitedReader)
	var title []byte
	// Is it time to give up yet?
	keepTrying := true
	for keepTrying {
		// Get next token
		tokenType := tokenizer.Next()
		if tokenType == html.StartTagToken {
			token := tokenizer.Token()
			switch token.Data {
			// We found title tag, get title data
			case "title":
				keepTrying = false
				tokenType = tokenizer.Next()
				if tokenType != html.TextToken {
					luaState.Push(lua.LNil)
					log.Printf("GET %s: wrong title token type: %s", u, tokenType)
					return 1
				}
				title = tokenizer.Text()
			// We reached body tag, stop processing
			case "body":
				keepTrying = false
			}
			// Parser error
		} else if tokenType == html.ErrorToken {
			luaState.Push(lua.LNil)
			log.Printf("GET %s: tokenizer error: %s", u, tokenizer.Err())
			return 1
		}
	}
	// We didn't meet error nor manage to find title
	if len(title) == 0 {
		luaState.Push(lua.LNil)
		log.Printf("GET %s: no title found", u)
		return 1
	}
	// Strip newlines and tabs from the title
	re := regexp.MustCompile(`[\n\t]`)
	title = re.ReplaceAll(title, []byte{})
	// Trim whitespace around the title
	strTitle := strings.TrimSpace(string(title))
	// Return up to 400 characters
	if len(strTitle) > 400 {
		strTitle = strTitle[:400]
	}
	luaState.Push(lua.LString(strTitle))
	return 1
}

// luaLibLoader returns a table containing our Lua library functions
func (b *BananaBoatBot) luaLibLoader(luaState *lua.LState) int {
	// Create map of function names to functions
	exports := map[string]lua.LGFunction{
		"get_title": b.luaLibGetTitle,
		"random":    b.luaLibRandom,
		"sleep":     b.luaLibSleep,
		"worker":    b.luaLibWorker,
	}
	// Convert map to Lua table and push to stack
	mod := luaState.SetFuncs(luaState.NewTable(), exports)
	luaState.Push(mod)
	return 1
}

// BananaBoatBotConfig contains the primary configuration for the app
type BananaBoatBotConfig struct {
	// Default port for IRC
	DefaultIrcPort int
	// Path to script to be loaded
	LuaFile string
	// Shall we log each received command or not
	LogCommands bool
	// Maximum reconnect interval in seconds
	MaxReconnect int
	// NewIrcServer creates a new irc server
	NewIrcServer func(parentCtx context.Context, serverName string, settings *client.IrcServerSettings) (client.IrcServerInterface, context.Context)
	// PackageDir is a directory to add to Lua package.path
	PackageDir string
}

func (b *BananaBoatBot) newLuaState(ctx context.Context, packageDir string) *lua.LState {
	// Create new Lua state
	luaState := lua.NewState()
	luaState.SetContext(ctx)

	// Provide access to our library functions in Lua
	luaState.PreloadModule("bananaboat", b.luaLibLoader)
	// Provide some third-party libraries
	luaState.PreloadModule("http", gluahttp.NewHttpModule(&b.httpClient).Loader)
	luaState.PreloadModule("json", luajson.Loader)
	// Register ratelimiter in Lua
	rate.RegisterGlobals(luaState)

	// Tamper package.path according to configuration...
	// Get "package" global
	t := luaState.GetGlobal("package").(*lua.LTable)
	// Get "path" from package table
	lPath := lua.LString("path")
	s := luaState.RawGet(t, lPath).(lua.LString).String()
	// Create list of elements to join for new package path
	elems := []string{s}
	if len(packageDir) > 0 {
		elems = append(elems, packageDir)
	}
	if len(b.luaDirectory) > 0 {
		elems = append(elems, b.luaDirectory)
	}
	// Set new package.path
	luaState.RawSet(t, lPath, lua.LString(strings.Join(elems, ";")))
	// Clear stack
	luaState.SetTop(0)

	return luaState
}

// NewBananaBoatBot creates a new BananaBoatBot
func NewBananaBoatBot(ctx context.Context, config *BananaBoatBotConfig) *BananaBoatBot {
	// We require a path to some script to load
	if len(config.LuaFile) == 0 {
		log.Fatal("Please specify script using -lua flag")
	}

	// Create BananaBoatBot
	b := BananaBoatBot{
		Config:   config,
		Metrics:  new(BananaBoatBotMetrics),
		handlers: make(map[string]*lua.LFunction),
		nick:     "BananaBoatBot",
		realname: "Banana Boat Bot",
		username: "bananarama",
		web:      make(map[string]BananaBoatBotWebFunc),
	}

	resourcesDir, err := resources.GetResourcesDirectory()
	if err == nil {
		b.luaDirectory = path.Join(resourcesDir, "lua", "?.lua")
	} else {
		log.Printf("Failed to get resources directory: %s", err)
	}

	// Create new shared Lua state
	b.luaState = b.newLuaState(ctx, config.PackageDir)

	// Create new pool of Lua state
	b.luaPool = sync.Pool{
		New: func() interface{} {
			return b.newLuaState(ctx, config.PackageDir)
		},
	}

	// Create HTTP client
	b.httpClient = http.Client{
		Timeout: time.Second * 60,
	}

	// Call Lua script and process result
	err = b.ReloadLua(ctx)
	if err != nil {
		log.Printf("Lua error: %s", err)
	}

	// Return BananaBoatBot
	return &b
}
