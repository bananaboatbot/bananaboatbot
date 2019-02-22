package bot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fatalbanana/bananaboatbot/client"
	"github.com/yuin/gopher-lua"
	"golang.org/x/net/html"
	irc "gopkg.in/sorcix/irc.v2"
)

// BananaBoatBot contains config & state of the bot
type BananaBoatBot struct {
	Config        *BananaBoatBotConfig
	curNet        string
	curMessage    *irc.Message
	handlers      map[string]*lua.LFunction
	handlersMutex sync.RWMutex
	httpClient    http.Client
	luaMutex      sync.Mutex
	luaPool       sync.Pool
	luaState      *lua.LState
	nick          string
	realname      string
	username      string
	servers       map[string]*client.IrcServer
	serversMutex  sync.RWMutex
	serverErrors  chan client.IrcServerError
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

func (b *BananaBoatBot) handleLuaReturnValues(ctx context.Context, parentCtx context.Context, svrName string, luaState *lua.LState) {
	// Ignore nil
	lv := luaState.Get(-1)
	if lv.Type() == lua.LTNil {
		return
	}
	// Get table result
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
			// Create irc.Message
			ircMessage := &irc.Message{
				Command: command,
				Params:  params,
			}
			// Send it to the server
			svr, ok := b.servers[net]
			if ok {
				svr.SendMessage(ctx, parentCtx, ircMessage)
			} else {
				log.Printf("Lua eror: Invalid server: %s", net)
			}
		}
	})
}

// handleHandlers invokes any registered Lua handlers for a command
func (b *BananaBoatBot) handleHandlers(ctx context.Context, parentCtx context.Context, svrName string, msg *irc.Message) {
	// Log message
	log.Printf("[%s] %s", svrName, msg)
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
		b.handleLuaReturnValues(ctx, parentCtx, svrName, b.luaState)
		// Clear stack
		b.luaState.SetTop(0)
	} else {
		// Release read mutex for handlers
		b.handlersMutex.RUnlock()
	}
}

// ReconnectServers reconnects servers on error
func (b *BananaBoatBot) handleErrors(ctx context.Context, parentCtx context.Context, svrName string, err error) {
	// Log the error
	log.Printf("[%s] Connection error: %s", svrName, err)
	// Wait for context to complete
	_ = <-ctx.Done()
	// If parent context is complete just return
	if parentCtx.Err() != nil {
		return
	}
	// Try reconnect to the server if still configured
	b.serversMutex.RLock()
	svr, ok := b.servers[svrName]
	b.serversMutex.RUnlock()
	if ok {
		go svr.Dial(parentCtx)
	}
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
	if lv.Type() != lua.LTTable {
		return fmt.Errorf("Lua reload error: unexpected return type: %s", lv.Type())
	}
	tbl := lv.(*lua.LTable)

	lv = tbl.RawGetString("nick")
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
		return fmt.Errorf("Lua reload error: unexpected handlers type: %s", lv.Type())
	}

	// Delete handlers still in map but no longer defined in Lua
	for k := range b.handlers {
		if _, ok := luaCommands[k]; !ok {
			delete(b.handlers, k)
		}
	}

	// Make map of server names collected from Lua
	luaServerNames := make(map[string]struct{}, 0)
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
				serverSettings := &client.IrcServerSettings{
					Host:          host,
					Port:          portInt,
					TLS:           tls,
					Nick:          nick,
					Realname:      realname,
					Username:      username,
					ErrorCallback: b.handleErrors,
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
					log.Printf("Creating new IRC server: %s", serverNameStr)
					// Create new IRC server
					svr := client.NewIrcServer(serverNameStr, &client.IrcServerSettings{
						Host:          host,
						Port:          portInt,
						TLS:           tls,
						Nick:          nick,
						Realname:      realname,
						Username:      username,
						ErrorCallback: b.handleErrors,
						InputCallback: b.handleHandlers,
					})
					// Set server to map
					b.serversMutex.Lock()
					oldSvr, ok := b.servers[serverNameStr]
					if ok {
						log.Printf("Destroying pre-existing IRC server: %s", serverNameStr)
						oldSvr.Close()
					}
					b.servers[serverNameStr] = svr
					b.serversMutex.Unlock()
					go svr.Dial(ctx)
				}
			}
		})
	}

	// Remove servers no longer defined in Lua
	b.serversMutex.Lock()
	for k := range b.servers {
		if _, ok := luaServerNames[k]; !ok {
			log.Printf("Destroying removed IRC server: %s", k)
			go b.servers[k].Close()
			delete(b.servers, k)
		}
	}
	b.serversMutex.Unlock()

	return nil
}

type OWMResponse struct {
	Conditions []OWMCondition `json:"weather"`
	Main       OWMMain        `json:"main"`
}

type OWMCondition struct {
	Description string `json:"description"`
}

type OWMMain struct {
	Temperature float64 `json:"temp"`
}

// luaLibOpenWeatherMap gets weather for a city
func (b *BananaBoatBot) luaLibOpenWeatherMap(luaState *lua.LState) int {
	apiKey := luaState.CheckString(1)
	location := luaState.CheckString(2)
	owmURL := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?units=metric&APPID=%s&q=%s", apiKey, location)
	resp, err := b.httpClient.Get(owmURL)
	if err != nil {
		log.Printf("HTTP client error: %s", err)
		return 0
	}
	if ct, ok := resp.Header["Content-Type"]; ok {
		if ct[0][:16] != "application/json" {
			log.Printf("OWM GET aborted: wrong content-type: %s", ct[0])
			return 0
		}
	} else {
		log.Print("OWM GET aborted: no content-type header")
		return 0
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("OWM GET returned non-OK status: %d", resp.StatusCode)
		return 0
	}
	dec := json.NewDecoder(resp.Body)
	owmResponse := &OWMResponse{}
	err = dec.Decode(&owmResponse)
	if err != nil {
		log.Printf("OWM response decode failed: %s", err)
		return 0
	}
	numConditions := len(owmResponse.Conditions) + 1
	conditions := make([]string, numConditions)
	conditions[0] = fmt.Sprintf("%.f°", owmResponse.Main.Temperature)
	i := 1
	for _, v := range owmResponse.Conditions {
		conditions[i] = v.Description
		i++
	}
	luaState.Push(lua.LString(strings.Join(conditions, ", ")))
	return 1
}

type LuisResponse struct {
	TopScoringIntent LuisTopScoringIntent `json:"topScoringIntent"`
	Entities         []LuisEntity         `json:"entities"`
}

type LuisTopScoringIntent struct {
	Intent string  `json:"intent"`
	Score  float64 `json:"score"`
}

type LuisEntity struct {
	Entity string  `json:"entity"`
	Type   string  `json:"type"`
	Score  float64 `json:"score"`
}

// luaLibLuisPredict predicts intention using luis.ai
func (b *BananaBoatBot) luaLibLuisPredict(luaState *lua.LState) int {
	region := luaState.CheckString(1)
	appID := luaState.CheckString(2)
	endpointKey := luaState.CheckString(3)
	utterance := luaState.CheckString(4)
	if len(utterance) > 500 {
		utterance = utterance[:500]
	}
	luisURL := fmt.Sprintf("https://%s.api.cognitive.microsoft.com/luis/v2.0/apps/%s?subscription-key=%s&verbose=false&q=%s", region, appID, endpointKey, url.QueryEscape(utterance))
	resp, err := b.httpClient.Get(luisURL)
	if err != nil {
		log.Printf("HTTP client error: %s", err)
		return 0
	}
	if ct, ok := resp.Header["Content-Type"]; ok {
		if ct[0][:16] != "application/json" {
			log.Printf("Luis GET aborted: wrong content-type: %s", ct[0])
			return 0
		}
	} else {
		log.Print("Luis GET aborted: no content-type header")
		return 0
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("Luis GET returned non-OK status: %d", resp.StatusCode)
		return 0
	}
	dec := json.NewDecoder(resp.Body)
	luisResponse := &LuisResponse{}
	err = dec.Decode(&luisResponse)
	if err != nil {
		log.Printf("Luis response decode failed: %s", err)
		return 0
	}
	if luisResponse.TopScoringIntent.Intent == "" {
		return 0
	}
	luaState.Push(lua.LString(luisResponse.TopScoringIntent.Intent))
	luaState.Push(lua.LNumber(luisResponse.TopScoringIntent.Score))
	entsTbl := luaState.CreateTable(0, 0)
	i := 1
	for _, e := range luisResponse.Entities {
		entTbl := luaState.CreateTable(0, 0)
		luaState.RawSet(entTbl, lua.LString("entity"), lua.LString(e.Entity))
		luaState.RawSet(entTbl, lua.LString("type"), lua.LString(e.Type))
		luaState.RawSet(entTbl, lua.LString("score"), lua.LNumber(e.Score))
		luaState.RawSetInt(entsTbl, i, entTbl)
		i++
	}
	luaState.Push(entsTbl)
	return 3
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
	go func(functionProto *lua.FunctionProto, curNet string, curMessage *irc.Message) {
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
		b.handleLuaReturnValues(nil, nil, curNet, newState)
	}(functionProto, b.curNet, b.curMessage)
	return 0
}

// luaLibGetTitle tries to get the HTML title of a URL
func (b *BananaBoatBot) luaLibGetTitle(luaState *lua.LState) int {
	u := luaState.CheckString(1)
	resp, err := b.httpClient.Get(u)
	if err != nil {
		luaState.Push(lua.LNil)
		log.Printf("HTTP client error: %s", err)
		return 1
	}
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
	limitedReader := &io.LimitedReader{R: resp.Body, N: 12288}
	tokenizer := html.NewTokenizer(limitedReader)
	var title []byte
	keepTrying := true
	for keepTrying {
		tokenType := tokenizer.Next()
		if tokenType == html.StartTagToken {
			token := tokenizer.Token()
			switch token.Data {
			case "title":
				keepTrying = false
				tokenType = tokenizer.Next()
				if tokenType != html.TextToken {
					luaState.Push(lua.LNil)
					log.Printf("GET %s: wrong title token type: %s", u, tokenType)
					return 1
				}
				title = tokenizer.Text()
			case "body":
				keepTrying = false
			}
		} else if tokenType == html.ErrorToken {
			luaState.Push(lua.LNil)
			log.Printf("GET %s: tokenizer error: %s", u, tokenizer.Err())
			return 1
		}
	}
	if len(title) == 0 {
		luaState.Push(lua.LNil)
		log.Printf("GET %s: no title found", u)
		return 1
	}
	re := regexp.MustCompile(`[\n\t]`)
	title = re.ReplaceAll(title, []byte{})
	strTitle := strings.TrimSpace(string(title))
	if len(strTitle) > 400 {
		strTitle = strTitle[:400]
	}
	luaState.Push(lua.LString(strTitle))
	return 1
}

// luaLibLoader returns a table containing our Lua library functions
func (b *BananaBoatBot) luaLibLoader(luaState *lua.LState) int {
	exports := map[string]lua.LGFunction{
		"get_title":    b.luaLibGetTitle,
		"luis_predict": b.luaLibLuisPredict,
		"owm":          b.luaLibOpenWeatherMap,
		"random":       b.luaLibRandom,
		"worker":       b.luaLibWorker,
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
func NewBananaBoatBot(ctx context.Context, config *BananaBoatBotConfig) *BananaBoatBot {

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
		servers:      make(map[string]*client.IrcServer),
		serverErrors: make(chan client.IrcServerError, 1),
		username:     "bananarama",
	}
	b.luaState = b.newLuaState()
	b.luaPool = sync.Pool{
		New: func() interface{} {
			return b.newLuaState()
		},
	}
	b.httpClient = http.Client{
		Timeout: time.Second * 60,
	}

	// Call Lua script and process result
	err := b.ReloadLua(ctx)
	if err != nil {
		log.Printf("Lua error: %s", err)
	}

	// Return BananaBoatBot
	return &b
}
