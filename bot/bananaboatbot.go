package bot

import (
	"crypto/rand"
	"log"
	"math/big"
	"sync"

	"github.com/yuin/gopher-lua"
	irc "gopkg.in/sorcix/irc.v2"
)

// BananaBoatBot contains config & state of the bot
type BananaBoatBot struct {
	Config        *BananaBoatBotConfig
	curNet        string
	curMessage    *irc.Message
	handlers      map[string]*lua.LFunction
	handlersMutex sync.RWMutex
	nick          string
	realname      string
	username      string
	luaState      *lua.LState
	luaMutex      sync.Mutex
	luaPool       sync.Pool
	servers       map[string]*IrcServer
	serversMutex  sync.RWMutex
	serverErrors  chan IrcServerError
}

// Close handles shutdown-related tasks
func (b *BananaBoatBot) Close() {
	log.Print("Shutting down")
	b.serversMutex.Lock()
	for _, svr := range b.servers {
		svr.Close()
	}
	b.serversMutex.Unlock()
	b.luaMutex.Lock()
	b.luaState.Close()
	b.luaMutex.Unlock()
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

func (b *BananaBoatBot) handleLuaReturnValues(svrName string, luaState *lua.LState) {
	// Get table result (XXX: also allow nil?)
	res := luaState.CheckTable(-1)
	// For each numeric index in the table result...
	res.ForEach(func(index lua.LValue, messageL lua.LValue) {
		var command string
		var params []string
		// Get the nested table..
		if message, ok := messageL.(*lua.LTable); ok {
			// Get 'command' string from table
			lv := message.RawGetString("command")
			command = lua.LVAsString(lv)
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
			// Create irc.Message
			ircMessage := &irc.Message{
				Command: command,
				Params:  params,
			}
			// Send it to the server
			b.servers[svrName].Output <- *ircMessage
		}
	})
}

// handleHandlers invokes any registered Lua handlers for a command
func (b *BananaBoatBot) handleHandlers(svrName string, msg *irc.Message) {
	// Log message
	log.Print(msg)
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
		b.handleLuaReturnValues(svrName, b.luaState)
		// Clear stack
		b.luaState.SetTop(0)
	} else {
		// Release read mutex for handlers
		b.handlersMutex.RUnlock()
	}
}

// ReconnectServers reconnects servers on error
func (b *BananaBoatBot) ReconnectServers() {
	for {
		select {
		// Process any errors we might have received
		case serverErr := <-b.serverErrors:
			// Log the error
			log.Print(serverErr.Error)
			// Try reconnect to the server
			go b.servers[serverErr.Name].Dial()
		}
	}
}

func (b *BananaBoatBot) loadLuaCommon() {

	if err := b.luaState.DoFile(b.Config.LuaFile); err != nil {
		log.Fatalf("Lua error: %s", err)
	}

	tbl := b.luaState.CheckTable(-1)

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

	lv = tbl.RawGetString("handlers")
	defer b.handlersMutex.Unlock()
	b.handlersMutex.Lock()
	luaCommands := make(map[string]struct{}, 0)
	if handlerTbl, ok := lv.(*lua.LTable); ok {
		handlerTbl.ForEach(func(commandName lua.LValue, handlerFuncL lua.LValue) {
			if handlerFunc, ok := handlerFuncL.(*lua.LFunction); ok {
				commandNameStr := lua.LVAsString(commandName)
				b.handlers[commandNameStr] = handlerFunc
				luaCommands[commandNameStr] = struct{}{}
			}
		})
	} else {
		// FIXME: return error?
	}
	// Delete handlers still in map but no longer defined in Lua
	for k := range b.handlers {
		if _, ok := luaCommands[k]; !ok {
			delete(b.handlers, k)
		}
	}

	// Make map of server names collected from Lua
	luaServerNames := make(map[string]struct{}, 0)
	// Remember which servers we created
	createdServerNames := make([]string, 0)
	// Get 'servers' from table
	lv = tbl.RawGetString("servers")
	// Get table value
	if serverTbl, ok := lv.(*lua.LTable); ok {
		// Iterate over nested tables...
		serverTbl.ForEach(func(serverName lua.LValue, serverSettingsLV lua.LValue) {
			// Get nested table
			if serverSettings, ok := serverSettingsLV.(*lua.LTable); ok {
				// Get 'server' string from table
				lv = serverSettings.RawGetString("server")
				host := lua.LVAsString(lv)
				// Get 'tls' bool from table
				var tls bool
				lv = serverSettings.RawGetString("tls")
				if lv, ok := lv.(lua.LBool); ok {
					tls = bool(lv)
				}
				// Get 'port' from table
				var portInt int
				lv = serverSettings.RawGetString("port")
				if port, ok := lv.(lua.LNumber); ok {
					portInt = int(port)
				} else {
					portInt = b.Config.DefaultIrcPort
				}
				// Get 'nick' from table - use default if unavailable
				var nick string
				lv = serverSettings.RawGetString("nick")
				if lv, ok := lv.(lua.LString); ok {
					nick = lua.LVAsString(lv)
				} else {
					nick = b.nick
				}
				// Get 'realname' from table - use default if unavailable
				var realname string
				lv = serverSettings.RawGetString("realname")
				if lv, ok := lv.(lua.LString); ok {
					realname = lua.LVAsString(lv)
				} else {
					realname = b.realname
				}
				// Get 'username' from table - use default if unavailable
				var username string
				lv = serverSettings.RawGetString("username")
				if lv, ok := lv.(lua.LString); ok {
					username = lua.LVAsString(lv)
				} else {
					username = b.username
				}

				// Remember we found this key
				serverNameStr := lua.LVAsString(serverName)
				luaServerNames[serverNameStr] = struct{}{}
				createServer := false
				serverSettings := &IrcServerSettings{
					Host:          host,
					Port:          portInt,
					TLS:           tls,
					Nick:          nick,
					Realname:      realname,
					Username:      username,
					ErrorChannel:  b.serverErrors,
					InputCallback: b.handleHandlers,
				}
				// Check if server already exists and/or if we need to (re)create it
				if oldSvr, ok := b.servers[serverNameStr]; ok {
					if !(oldSvr.Settings.Host == serverSettings.Host &&
						oldSvr.Settings.Port == serverSettings.Port &&
						oldSvr.Settings.TLS == serverSettings.TLS &&
						oldSvr.Settings.Nick == serverSettings.Nick &&
						oldSvr.Settings.Realname == serverSettings.Realname &&
						oldSvr.Settings.Username == serverSettings.Username) {
						createServer = true
					}
				} else {
					createServer = true
				}
				if createServer {
					// Create new IRC server
					svr := NewIrcServer(serverNameStr, &IrcServerSettings{
						Host:          host,
						Port:          portInt,
						TLS:           tls,
						Nick:          nick,
						Realname:      realname,
						Username:      username,
						ErrorChannel:  b.serverErrors,
						InputCallback: b.handleHandlers,
					})
					// Set server to map
					b.serversMutex.Lock()
					b.servers[serverNameStr] = svr
					b.serversMutex.Unlock()
					// Remember to start it shortly
					createdServerNames = append(createdServerNames, serverNameStr)
				}
			}
		})
	}
	// Remove servers no longer defined in Lua
	b.serversMutex.Lock()
	for k := range b.servers {
		if _, ok := luaServerNames[k]; !ok {
			b.servers[k].Close()
			delete(b.servers, k)
		}
	}
	b.serversMutex.Unlock()
	// Start any servers which need to be started
	for _, name := range createdServerNames {
		go b.servers[name].Dial()
	}
	// Restart any servers that need restarting
	go b.ReconnectServers()
}

// ReloadLua deals with reloading Lua parts
func (b *BananaBoatBot) ReloadLua() {
	b.luaMutex.Lock()
	b.loadLuaCommon()
	b.luaState.SetTop(0)
	b.luaMutex.Unlock()
}

// luaLibRandom provides access to cryptographic random numbers in Lua
func (b *BananaBoatBot) luaLibRandom(luaState *lua.LState) int {
	i := luaState.ToInt(1)
	var r *big.Int
	var err error
	if i != 1 {
		r, err = rand.Int(rand.Reader, big.NewInt(int64(i)))
	}
	if err != nil {
		panic(err)
	}
	res := r.Int64() + 1
	ln := lua.LNumber(res)
	luaState.Push(ln)
	return 1
}

// luaLibWorker runs a task in a goroutine
func (b *BananaBoatBot) luaLibWorker(luaState *lua.LState) int {
	functionProto := luaState.CheckFunction(1).Proto
	go func(functionProto *lua.FunctionProto, curNet string, curMessage *irc.Message) {
		// Avoid crashing if misused
		if curMessage == nil || curNet == "" {
			return
		}
		// Get luaState from pool
		newState := lua.NewState()
		defer func() {
			// Clear stack and return state to pool
			newState.SetTop(0)
			b.luaPool.Put(newState)
		}()
		// Create function from prototype
		luaFunction := newState.NewFunctionFromProto(functionProto)
		// Create parameters for function
		luaParams := luaParamsFromMessage(curNet, curMessage)
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
		b.handleLuaReturnValues(curNet, newState)
	}(functionProto, b.curNet, b.curMessage)
	return 0
}

// luaLibLoader returns a table containing our Lua library functions
func (b *BananaBoatBot) luaLibLoader(luaState *lua.LState) int {
	exports := map[string]lua.LGFunction{
		"random": b.luaLibRandom,
		"worker": b.luaLibWorker,
	}
	mod := luaState.SetFuncs(luaState.NewTable(), exports)
	luaState.Push(mod)
	return 1
}

type BananaBoatBotConfig struct {
	LuaFile        string
	DefaultIrcPort int
}

func (b *BananaBoatBot) newLuaState() *lua.LState {
	// Create new Lua state
	luaState := lua.NewState()
	// Provide access to our library functions in Lua
	luaState.PreloadModule("bananaboat", b.luaLibLoader)
	return luaState
}

// NewBananaBoatBot creates a new BananaBoatBot
func NewBananaBoatBot(config *BananaBoatBotConfig) *BananaBoatBot {

	// We require a path to some script to load
	if len(config.LuaFile) == 0 {
		log.Fatal("Please specify script using -lua flag")
	}

	// Create BananaBoatBot
	b := BananaBoatBot{
		Config:       config,
		handlers:     make(map[string]*lua.LFunction),
		nick:         "BananaBoatBot",
		realname:     "Banana Boat Bot",
		servers:      make(map[string]*IrcServer),
		serverErrors: make(chan IrcServerError, 1),
		username:     "bananarama",
	}
	b.luaState = b.newLuaState()
	b.luaPool = sync.Pool{
		New: func() interface{} {
			return b.newLuaState()
		},
	}

	// Get Lua state mutex
	b.luaMutex.Lock()
	// Call Lua script and process result
	b.loadLuaCommon()
	// Clear Lua stack
	b.luaState.SetTop(0)
	// Release Lua State mutex
	b.luaMutex.Unlock()

	// Return BananaBoatBot
	return &b
}