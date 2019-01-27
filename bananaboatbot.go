package main

import (
	"log"
	"sync"

	"github.com/yuin/gopher-lua"
	irc "gopkg.in/sorcix/irc.v2"
)

type BananaBoatBot struct {
	handlers      map[string]*lua.LFunction
	handlersMutex sync.RWMutex
	nick          string
	realname      string
	username      string
	luaFile       string
	luaState      *lua.LState
	luaMutex      sync.Mutex
	servers       map[string]*IrcServer
}

// XXX: probably unused
func (b *BananaBoatBot) Close() {
	log.Print("Shutting down")
	b.luaState.Close()
}

func (b *BananaBoatBot) Loop() {
	for svrName, svr := range b.servers {
		go func() {
			for {
				func() {

					msg := <-svr.Input
					log.Print(msg)

					b.handlersMutex.RLock()
					if luaFunction, ok := b.handlers[msg.Command]; ok {

						b.handlersMutex.RUnlock()
						defer b.luaMutex.Unlock()

						luaParams := make([]lua.LValue, len(msg.Params)+5)
						luaParams[0] = lua.LString(svrName)
						if msg.Prefix != nil {
							luaParams[1] = lua.LString(msg.Prefix.Name)
							luaParams[2] = lua.LString(msg.Prefix.User)
							luaParams[3] = lua.LString(msg.Prefix.Host)
						}
						luaParams[4] = lua.LString(msg.Command)
						pi := 0
						for i := 5; i < len(luaParams); i++ {
							luaParams[i] = lua.LString(msg.Params[pi])
							pi++
						}

						b.luaMutex.Lock()

						err := b.luaState.CallByParam(lua.P{
							Fn:      luaFunction,
							NRet:    1,
							Protect: true},
							luaParams...)
						if err != nil {
							log.Printf("Handler for %s failed: %s", msg.Command, err)
						}
						res := b.luaState.CheckTable(-1)
						res.ForEach(func(index lua.LValue, messageL lua.LValue) {
							var command string
							var params []string
							if message, ok := messageL.(*lua.LTable); ok {
								lv := message.RawGetString("command")
								command = lua.LVAsString(lv)
								lv = message.RawGetString("params")
								if paramsT, ok := lv.(*lua.LTable); ok {
									params = make([]string, paramsT.MaxN())
									paramsIndex := 0
									paramsT.ForEach(func(index lua.LValue, paramL lua.LValue) {
										params[paramsIndex] = lua.LVAsString(paramL)
										paramsIndex++
									})
								} else {
									params = make([]string, 0)
								}
								ircMessage := &irc.Message{
									Command: command,
									Params:  params,
								}
								svr.Output <- *ircMessage
							}
						})

						b.luaState.SetTop(0)
					} else {
						b.handlersMutex.RUnlock()
					}
				}()
			}
		}()
	}
	select {}
}

func (b *BananaBoatBot) loadLuaCommon() *lua.LTable {

	if err := b.luaState.DoFile(b.luaFile); err != nil {
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

	// FIXME: remove deleted handlers on reload
	lv = tbl.RawGetString("handlers")
	defer b.handlersMutex.Unlock()
	b.handlersMutex.Lock()
	if handlerTbl, ok := lv.(*lua.LTable); ok {
		handlerTbl.ForEach(func(commandName lua.LValue, handlerFuncL lua.LValue) {
			if handlerFunc, ok := handlerFuncL.(*lua.LFunction); ok {
				b.handlers[lua.LVAsString(commandName)] = handlerFunc
			}
		})
	}
	// FIXME: deal with servers, remove this function
	return tbl
}

func (b *BananaBoatBot) ReloadLua() {
	b.luaMutex.Lock()
	b.loadLuaCommon()
	// FIXME: handle reconfiguring servers
	b.luaState.SetTop(0)
	b.luaMutex.Unlock()
}

func NewBananaBoatBot(luaFile string) *BananaBoatBot {

	b := BananaBoatBot{
		handlers: make(map[string]*lua.LFunction),
		luaFile:  luaFile,
		luaState: lua.NewState(),
		nick:     "BananaBoatBot",
		realname: "Banana Boat Bot",
		servers:  make(map[string]*IrcServer),
		username: "bananarama",
	}

	if len(luaFile) == 0 {
		log.Fatal("Please specify script using -lua flag")
	}

	tbl := b.loadLuaCommon()

	lv := tbl.RawGetString("servers")
	if serverTbl, ok := lv.(*lua.LTable); ok {
		serverTbl.ForEach(func(serverName lua.LValue, serverSettingsLV lua.LValue) {

			if serverSettings, ok := serverSettingsLV.(*lua.LTable); ok {

				lv = serverSettings.RawGetString("server")
				host := lua.LVAsString(lv)

				var tls bool
				lv = serverSettings.RawGetString("tls")
				if lv, ok := lv.(lua.LBool); ok {
					tls = bool(lv)
				}

				var portInt int
				lv = serverSettings.RawGetString("port")
				if port, ok := lv.(lua.LNumber); ok {
					portInt = int(port)
				}

				svr := NewIrcServer(&IrcServerSettings{
					Host:     host,
					Port:     portInt,
					TLS:      tls,
					Nick:     b.nick,
					Realname: b.realname,
					Username: b.username,
				})

				b.servers[lua.LVAsString(serverName)] = svr
			}
		})
	}

	b.luaState.SetTop(0)

	for _, server := range b.servers {
		server.Dial()
	}

	return &b
}
