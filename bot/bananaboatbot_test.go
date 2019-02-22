package bot_test

import (
	"context"
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatalbanana/bananaboatbot/bot"
	irc "gopkg.in/sorcix/irc.v2"
)

func TestTrivial(t *testing.T) {
	// Discard logs
	log.SetOutput(ioutil.Discard)
	// We want the bot to respond "hello" and "goodbye" (after reload)
	gotHello := false
	gotGoodbye := false

	// Start fake IRC server on ephermal port
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	index := strings.LastIndex(addr, ":")
	// Set our server port to port used by the server
	serverPort, err := strconv.Atoi(addr[index+1:])
	if err != nil {
		t.Fatal(err)
	}

	// Forward-declare BananaBoatBot
	var b *bot.BananaBoatBot
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	// Channel for indicating server is ready
	ready := make(chan struct{}, 0)

	go func() {
		// We are ready to start accepting connections
		<-ready
		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}
		// Create decoder and encoder for connection
		encoder := irc.NewEncoder(conn)
		decoder := irc.NewDecoder(conn)
		// Consume messages until registration is completed
		for {
			conn.SetReadDeadline(time.Now().Add(time.Second * 5))
			msg, err := decoder.Decode()
			if err != nil {
				t.Fatal(err)
			}
			// XXX: capabilities
			if msg.Command == irc.USER {
				break
			}
		}
		// Send welcome
		encoder.Encode(&irc.Message{
			Command: irc.RPL_WELCOME,
		})
		// Send fake PM
		fakePrefix := &irc.Prefix{
			Name: "bob",
			User: "ubob",
			Host: "hbob",
		}
		encoder.Encode(&irc.Message{
			Command: irc.PRIVMSG,
			Params:  []string{"testbot1", "HELLO"},
			Prefix:  fakePrefix,
		})
		// Send another fake PM with wrong trigger
		encoder.Encode(&irc.Message{
			Command: irc.PRIVMSG,
			Params:  []string{"testbot1", "asdf"},
			Prefix:  fakePrefix,
		})
		// Decode client input
		msg, err := decoder.Decode()
		if err != nil {
			t.Fatal(err)
		}
		// We want client to respond with HELLO
		if msg.Command == irc.PRIVMSG {
			if msg.Params[1] == "HELLO" {
				gotHello = true
			}
		}
		// Set new config file and reload Lua
		b.Config.LuaFile = "../test/trivial2.lua"
		b.ReloadLua(ctx)
		// Send another PM
		encoder.Encode(&irc.Message{
			Command: irc.PRIVMSG,
			Params:  []string{"testbot1", "HELLO"},
			Prefix:  fakePrefix,
		})
		// Decode response
		msg, err = decoder.Decode()
		if err != nil {
			t.Fatal(err)
		}
		// This time bot must say GOODBYE
		if msg.Command == irc.PRIVMSG {
			if msg.Params[1] == "GOODBYE" {
				gotGoodbye = true
			}
		}
		// We are done - cancel context to suppress reconnect (for consistent coverage)
		cancel()
		<-ctx.Done()
		// Ready to die
		<-ready
	}()

	b = bot.NewBananaBoatBot(ctx, &bot.BananaBoatBotConfig{
		DefaultIrcPort: serverPort,
		LuaFile:        "../test/trivial1.lua",
	})
	ready <- struct{}{}
	ready <- struct{}{}

	if !gotHello {
		t.Fatal("Bot didn't say hello")
	}
	if !gotGoodbye {
		t.Fatal("Bot didn't say goodbye")
	}
	b.Close()
}
