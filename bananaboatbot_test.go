package main

import (
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"strings"
	"testing"

	irc "gopkg.in/sorcix/irc.v2"
)

func TestTrivial(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	done := false
	gotHello := false
	gotGoodbye := false

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	index := strings.LastIndex(addr, ":")
	serverPort, err := strconv.Atoi(addr[index+1:])
	if err != nil {
		t.Fatal(err)
	}

	var b *BananaBoatBot

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				t.Fatal(err)
			}
			encoder := irc.NewEncoder(conn)
			decoder := irc.NewDecoder(conn)
			for {
				msg, err := decoder.Decode()
				if err != nil {
					t.Fatal(err)
				}
				// XXX: capabilities
				if msg.Command == irc.USER {
					break
				}
			}
			encoder.Encode(&irc.Message{
				Command: irc.RPL_WELCOME,
			})
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
			encoder.Encode(&irc.Message{
				Command: irc.PRIVMSG,
				Params:  []string{"testbot1", "asdf"},
				Prefix:  fakePrefix,
			})
			msg, err := decoder.Decode()
			if err != nil {
				t.Fatal(err)
			}
			if msg.Command == irc.PRIVMSG {
				if msg.Params[1] == "HELLO" {
					gotHello = true
				}
			}
			b.config.luaFile = "test/trivial2.lua"
			b.ReloadLua()
			encoder.Encode(&irc.Message{
				Command: irc.PRIVMSG,
				Params:  []string{"testbot1", "HELLO"},
				Prefix:  fakePrefix,
			})
			msg, err = decoder.Decode()
			if err != nil {
				t.Fatal(err)
			}
			if msg.Command == irc.PRIVMSG {
				if msg.Params[1] == "GOODBYE" {
					gotGoodbye = true
				}
			}
			conn.Close()
			done = true
			break
		}
	}()

	b = NewBananaBoatBot(&BananaBoatBotConfig{
		defaultIrcPort: serverPort,
		luaFile:        "test/trivial1.lua",
	})

	for done == false {
		b.LoopOnce()
	}

	if !gotHello {
		t.Fatal("Bot didn't say hello")
	}
	if !gotGoodbye {
		t.Fatal("Bot didn't say goodbye")
	}

	b.Close()
}
