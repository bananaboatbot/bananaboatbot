package bot_test

import (
	"bytes"
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
	"time"

	"github.com/fatalbanana/bananaboatbot/bot"
	"github.com/fatalbanana/bananaboatbot/client"
	"github.com/fatalbanana/bananaboatbot/test"
	irc "gopkg.in/sorcix/irc.v2"
)

var (
	stdConfig = bot.BananaBoatBotConfig{
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
	cloneCfg := stdConfig
	b := bot.NewBananaBoatBot(ctx, &cloneCfg)
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

func TestRPC(t *testing.T) {
	ctx := context.TODO()
	// Create BananaBoatBot
	cloneCfg := stdConfig
	cloneCfg.LuaFile = "../test/rpc.lua"
	b := bot.NewBananaBoatBot(ctx, &cloneCfg)
	defer b.Close(ctx)
	// Create test webserver
	ts := httptest.NewServer(b)
	defer ts.Close()
	endPoints := []string{"hello", "hello_shared", "hello_json"}
	for _, fragment := range []string{"hello", "hello_shared"} {
		// Say hello
		resp, err := http.Get(fmt.Sprintf("%s/rpc/%s?p1=HI%%20THERE", ts.URL, fragment))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatal(fmt.Errorf("got bad HTTP status [%d]", resp.StatusCode))
		}
	}
	for _, fragment := range []string{"hello_json"} {
		// Say hello
		resp, err := http.Post(fmt.Sprintf("%s/rpc/%s", ts.URL, fragment), "application/json", bytes.NewReader([]byte(`{"p1":["HI THERE"]}`)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatal(fmt.Errorf("got bad HTTP status [%d]", resp.StatusCode))
		}
		resp, _ = http.Post(fmt.Sprintf("%s/rpc/%s", ts.URL, fragment), "application/json", bytes.NewReader([]byte(`GARBAGE`)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	// Read response
	svrI, _ := b.Servers.Load("test")
	messages := svrI.(client.IrcServerInterface).GetMessages()
	for range endPoints {
		msg := <-messages
		if msg.Command != irc.PRIVMSG {
			t.Fatalf("Got wrong message type in response1: %s", msg.Command)
		}
		if msg.Params[1] != "HI THERE" {
			t.Fatalf("Got wrong parameters in response1: %s", strings.Join(msg.Params, ","))
		}
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
	b := bot.NewBananaBoatBot(ctx, &cfg)
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

	b := bot.NewBananaBoatBot(ctx, &cfg)
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
	b := bot.NewBananaBoatBot(ctx, &cfg)
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
	defer l.Close()

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
	b := bot.NewBananaBoatBot(ctx, &stdConfig)

	// Naive approach to faking error won't work properly (but here for coverage)
	b.HandleErrors(ctx, "test", errors.New("something went wrong"))

	handleErrors := makeErrorHandler(b, done)

	// Create settings for superfluous client
	settings := &client.IrcServerSettings{
		Basic: client.BasicIrcServerSettings{
			Host:     "localhost",
			Port:     serverPort,
			TLS:      false,
			Nick:     "testbot1",
			Realname: "testbotr",
			Username: "testbotu",
			Password: "yodel",
		},
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

// Test that the bot won't reconnect when it shouldn't and vice versa
func TestDisconnectSanity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errors := make(chan error, 1)
	done := make(chan struct{}, 1)

	l, serverPort := test.FakeServer(t)
	defer l.Close()

	// Create BananaBoatBot
	cfg := stdConfig
	cfg.DefaultIrcPort = serverPort
	cfg.NewIrcServer = client.NewIrcServer
	b := bot.NewBananaBoatBot(ctx, &cfg)
	defer b.Close(ctx)

	go func() {
		numConnections := 0
		for {
			conn, err := l.Accept()
			if err != nil {
				errors <- err
			}
			numConnections++
			if numConnections > 1 {
				done <- struct{}{}
				return
			}
			enc := irc.NewEncoder(conn)
			dec := irc.NewDecoder(conn)
			count := 0
			test.WaitForRegistration(ctx, conn, dec, errors)
			for {
				conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 50))
				err := enc.Encode(&irc.Message{
					Command: irc.PRIVMSG,
					Params:  []string{"testbot1", "HELLO"},
				})
				if err != nil {
					errors <- err
					return
				}
				conn.SetReadDeadline(time.Now().Add(time.Millisecond * 50))
				msg, err := dec.Decode()
				if err != nil {
					errors <- err
					return
				}
				if len(msg.Params) != 2 {
					errors <- fmt.Errorf("Unexpected number of parameters: %d (%s)", len(msg.Params), msg)
					return
				}
				if msg.Params[1] != "HELLO" {
					errors <- fmt.Errorf("%s != HELLO", msg.Params[1])
					return
				}
				count++
				if count >= 3 {
					done <- struct{}{}
					break
				} else {
					b.ReloadLua(ctx)
				}
			}
		}
	}()

	select {
	case <-done:
		break
	case err := <-errors:
		t.Fatal(err)
	}

	// Now test inverse case
	b.Config.LuaFile = "../test/newname.lua"
	b.ReloadLua(ctx)
	select {
	case <-done:
		break
	case err := <-errors:
		t.Fatal(err)
	}
}
