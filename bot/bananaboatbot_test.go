package bot_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatalbanana/bananaboatbot/bot"
	"github.com/fatalbanana/bananaboatbot/client"
	irc "gopkg.in/sorcix/irc.v2"
)

type mockIrcServer struct {
	Cancel       context.CancelFunc
	done         <-chan struct{}
	messages     chan irc.Message
	reconnectExp *uint64
	settings     *client.IrcServerSettings
}

func newMockIrcServer(parentCtx context.Context, name string, settings *client.IrcServerSettings) (client.IrcServerInterface, context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	messageOutput := make(chan irc.Message, 10)
	m := &mockIrcServer{
		Cancel:   cancel,
		done:     ctx.Done(),
		messages: messageOutput,
		settings: settings,
	}
	return m, parentCtx
}

// GetReconnectExp returns current reconnectExp
func (m *mockIrcServer) GetReconnectExp() *uint64 {
	return m.reconnectExp
}

// SetReconnectExp sets current reconnectExp
func (m *mockIrcServer) SetReconnectExp(val uint64) {
	m.reconnectExp = &val
}

func (m *mockIrcServer) Done() <-chan struct{} {
	return m.done
}

func (m *mockIrcServer) Dial(parentCtx context.Context) {
}

func (m *mockIrcServer) Close(parentCtx context.Context) {
}

func (m *mockIrcServer) ReconnectWait(parentCtx context.Context) {
}

func (m *mockIrcServer) GetSettings() *client.IrcServerSettings {
	return m.settings
}

func (m *mockIrcServer) GetMessages() chan irc.Message {
	return m.messages
}

func TestReload(t *testing.T) {
	ctx := context.TODO()
	// Create BananaBoatBot
	b := bot.NewBananaBoatBot(ctx, &bot.BananaBoatBotConfig{
		LogCommands:  true,
		LuaFile:      "../test/trivial1.lua",
		MaxReconnect: 1,
		NewIrcServer: newMockIrcServer,
	})
	// Say hello
	b.HandleHandlers(ctx, "test", &irc.Message{
		Command: irc.PRIVMSG,
		Params:  []string{"testbot1", "HELLO"},
	})
	// Read response
	svrI, _ := b.Servers.Load("test")
	messages := svrI.(client.IrcServerInterface).GetMessages()
	msg := <-messages
	if msg.Command != irc.PRIVMSG {
		t.Fatalf("Got wrong message type in response1: %s", msg.Command)
	}
	if msg.Params[1] != "HELLO" {
		t.Fatalf("Got wrong parameters in response1: %s", strings.Join(msg.Params, ","))
	}
	// Set new config file and reload Lua
	b.Config.LuaFile = "../test/trivial2.lua"
	b.ReloadLua(ctx)
	// Send another PM
	b.HandleHandlers(ctx, "test", &irc.Message{
		Command: irc.PRIVMSG,
		Params:  []string{"testbot1", "HELLO"},
	})
	// Get response
	msg = <-messages
	// This time bot must say GOODBYE
	if msg.Command != irc.PRIVMSG {
		t.Fatalf("Got wrong message type in response2: %s", msg.Command)
	}
	if msg.Params[1] != "GOODBYE" {
		t.Fatalf("Got wrong parameters in response2: %s", strings.Join(msg.Params, ","))
	}
}

func TestLuis(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(&bot.LuisResponse{
			Entities: []bot.LuisEntity{
				bot.LuisEntity{
					Entity: "WORLD",
					Score:  0.5,
					Type:   "Thing",
				},
			},
			TopScoringIntent: bot.LuisTopScoringIntent{
				Intent: "Hello",
				Score:  0.5,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-type", "application/json")
		w.Write(b)
	}))
	defer ts.Close()
	ctx := context.TODO()
	b := bot.NewBananaBoatBot(ctx, &bot.BananaBoatBotConfig{
		LogCommands:     true,
		LuaFile:         "../test/luis.lua",
		LuisURLTemplate: fmt.Sprintf("%s?region=%%s&appid=%%s&key=%%s&utterance=%%s", ts.URL),
		MaxReconnect:    1,
		NewIrcServer:    newMockIrcServer,
	})
	// Say hello
	b.HandleHandlers(ctx, "test", &irc.Message{
		Command: irc.PRIVMSG,
		Params:  []string{"testbot1", "HELLO"},
	})
	svrI, _ := b.Servers.Load("test")
	messages := svrI.(client.IrcServerInterface).GetMessages()
	msg := <-messages
	if msg.Command != irc.PRIVMSG {
		t.Fatalf("Got wrong message type in response: %s", msg.Command)
	}
	if msg.Params[1] != "howdy WORLD" &&
		msg.Params[1] != "hey WORLD" &&
		msg.Params[1] != "hi WORLD" {
		t.Fatalf("Got wrong parameters in response: %s", strings.Join(msg.Params, ","))
	}
}

func TestOwm(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(&bot.OWMResponse{
			Conditions: []bot.OWMCondition{
				bot.OWMCondition{
					Description: "clear sky",
				},
			},
			Main: bot.OWMMain{
				Temperature: 21.3,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-type", "application/json")
		w.Write(b)
	}))
	defer ts.Close()
	ctx := context.TODO()
	b := bot.NewBananaBoatBot(ctx, &bot.BananaBoatBotConfig{
		LogCommands:    true,
		LuaFile:        "../test/owm.lua",
		OwmURLTemplate: fmt.Sprintf("%s?appid=%%s&query=%%s", ts.URL),
		MaxReconnect:   1,
		NewIrcServer:   newMockIrcServer,
	})
	// Say weather
	b.HandleHandlers(ctx, "test", &irc.Message{
		Command: irc.PRIVMSG,
		Params:  []string{"testbot1", "weather"},
	})
	svrI, _ := b.Servers.Load("test")
	messages := svrI.(client.IrcServerInterface).GetMessages()
	msg := <-messages
	if msg.Command != irc.PRIVMSG {
		t.Fatalf("Got wrong message type in response: %s", msg.Command)
	}
	if msg.Params[1] != "21°, clear sky" {
		t.Fatalf("Got wrong parameters in response: %s", strings.Join(msg.Params, ","))
	}
}

func TestTitleScrape(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-type", "text/html")
		w.Write([]byte(`<html><head><title>asdf</title></head></html>`))
	}))
	defer ts.Close()
	ctx := context.TODO()
	b := bot.NewBananaBoatBot(ctx, &bot.BananaBoatBotConfig{
		LogCommands:  true,
		LuaFile:      "../test/get_title.lua",
		MaxReconnect: 1,
		NewIrcServer: newMockIrcServer,
	})
	// Say URL
	b.HandleHandlers(ctx, "test", &irc.Message{
		Command: irc.PRIVMSG,
		Params:  []string{"testbot1", ts.URL},
	})
	svrI, _ := b.Servers.Load("test")
	messages := svrI.(client.IrcServerInterface).GetMessages()
	msg := <-messages
	if msg.Command != irc.PRIVMSG {
		t.Fatalf("Got wrong message type in response: %s", msg.Command)
	}
	if msg.Params[1] != "asdf" {
		t.Fatalf("Got wrong parameters in response: %s", strings.Join(msg.Params, ","))
	}
}
