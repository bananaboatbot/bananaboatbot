package bot_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatalbanana/bananaboatbot/bot"
	"github.com/fatalbanana/bananaboatbot/client"
	"github.com/fatalbanana/bananaboatbot/test"
	irc "gopkg.in/sorcix/irc.v2"
)

var (
	stdConfig = &bot.BananaBoatBotConfig{
		LogCommands:  true,
		LuaFile:      "../test/trivial1.lua",
		MaxReconnect: 0,
		NewIrcServer: test.NewMockIrcServer,
	}
)

func init() {
	pDir, err := filepath.Abs("../test")
	if err != nil {
		panic(err)
	}
	stdConfig.PackageDir = fmt.Sprintf("%s%c?.lua", pDir, os.PathSeparator)
}

func TestReload(t *testing.T) {
	ctx := context.TODO()
	// Create BananaBoatBot
	b := bot.NewBananaBoatBot(ctx, stdConfig)
	defer b.Close(ctx)
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
				{
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
	cfg := stdConfig
	cfg.LuaFile = "../test/luis.lua"
	cfg.LuisURLTemplate = fmt.Sprintf("%s?region=%%s&appid=%%s&key=%%s&utterance=%%s", ts.URL)
	b := bot.NewBananaBoatBot(ctx, cfg)
	defer b.Close(ctx)
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
				{
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
	cfg := stdConfig
	cfg.LuaFile = "../test/owm.lua"
	cfg.OwmURLTemplate = fmt.Sprintf("%s?appid=%%s&query=%%s", ts.URL)

	b := bot.NewBananaBoatBot(ctx, cfg)
	defer b.Close(ctx)
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
	cfg := stdConfig
	cfg.LuaFile = "../test/get_title.lua"
	b := bot.NewBananaBoatBot(ctx, cfg)
	defer b.Close(ctx)
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

func makeErrorHandler(b *bot.BananaBoatBot, done chan struct{}) func(context.Context, string, error) {
	return func(ctx context.Context, svrName string, err error) {
		b.HandleErrors(ctx, svrName, err)
		done <- struct{}{}
	}
}

// Test error handling
func TestHandleServerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, serverPort := test.FakeServer(t)

	done := make(chan struct{}, 1)

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				fmt.Println(err)
				return
			}
			conn.Close()
			done <- struct{}{}
		}
	}()

	// Create BananaBoatBot
	b := bot.NewBananaBoatBot(ctx, stdConfig)

	// Naive approach to faking error won't work properly (but here for coverage)
	b.HandleErrors(ctx, "test", errors.New("something went wrong"))

	handleErrors := makeErrorHandler(b, done)

	// Create settings for superfluous client
	settings := &client.IrcServerSettings{
		Host:          "localhost",
		Port:          serverPort,
		TLS:           false,
		Nick:          "testbot1",
		Realname:      "testbotr",
		Username:      "testbotu",
		Password:      "yodel",
		ErrorCallback: handleErrors,
		InputCallback: func(ctx context.Context, svrName string, msg *irc.Message) {
			// Not relevant
		},
	}
	// Create client
	svrI, svrCtx := client.NewIrcServer(ctx, "test", settings)
	// Replace existing client with our one
	b.Servers.Store("test", svrI)
	// Dial server
	svrI.(client.IrcServerInterface).Dial(svrCtx)
	// Wait for dropped connection
	<-done
	// Wait for error handling
	<-done
}
