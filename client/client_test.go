package client_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatalbanana/bananaboatbot/client"
	irc "gopkg.in/sorcix/irc.v2"
)

func fakeServer(t *testing.T) (net.Listener, int) {
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

	return l, serverPort
}

func TestError(t *testing.T) {

	// Start fake IRC server on ephermal port
	l, serverPort := fakeServer(t)
	defer l.Close()

	done := make(chan struct{}, 1)

	go func() {
		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}
		dec := irc.NewDecoder(conn)
		enc := irc.NewEncoder(conn)
		for {
			conn.SetReadDeadline(time.Now().Add(time.Millisecond * 50))
			msg, err := dec.Decode()
			if err != nil {
				t.Fatal(err)
			}
			if msg.Command == "USER" {
				break
			}
		}
		enc.Encode(&irc.Message{
			Command: irc.ERROR,
			Params:  []string{"Bye"},
		})
	}()

	// Create server settings
	settings := &client.IrcServerSettings{
		Host:     "localhost",
		Port:     serverPort,
		TLS:      false,
		Nick:     "testbot1",
		Realname: "testbotr",
		Username: "testbotu",
		Password: "yodel",
		ErrorCallback: func(ctx context.Context, svrName string, err error) {
			done <- struct{}{}
		},
		InputCallback: func(ctx context.Context, svrName string, msg *irc.Message) {
		},
	}
	// Create client
	svrI := client.NewIrcServer("test", settings)
	svr := svrI.(client.IrcServerInterface)

	// Dial
	ctx := context.TODO()
	svr.Dial(ctx)
	// Wait for error
	<-done
	// Destroy server
	svr.Close(ctx)
}

func TestSendAndQuit(t *testing.T) {
	// Start fake IRC server on ephermal port
	l, serverPort := fakeServer(t)
	defer l.Close()

	ready := make(chan struct{})
	done := make(chan struct{})

	go func() {
		conn, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}
		dec := irc.NewDecoder(conn)
		for {
			conn.SetReadDeadline(time.Now().Add(time.Millisecond * 50))
			msg, err := dec.Decode()
			if err != nil {
				t.Fatal(err)
			}
			if msg.Command == irc.USER {
				ready <- struct{}{}
			} else if msg.Command == irc.QUIT {
				done <- struct{}{}
				break
			}
		}
		select {}
	}()

	// Create server settings
	settings := &client.IrcServerSettings{
		Host:     "localhost",
		Port:     serverPort,
		TLS:      false,
		Nick:     "testbot1",
		Realname: "testbotr",
		Username: "testbotu",
		ErrorCallback: func(ctx context.Context, svrName string, err error) {
		},
		InputCallback: func(ctx context.Context, svrName string, msg *irc.Message) {
		},
	}

	// Create client
	svrI := client.NewIrcServer("test", settings)
	svr := svrI.(client.IrcServerInterface)

	// Dial
	ctx := context.TODO()
	svr.Dial(ctx)
	// Wait for server to acknowledge USER
	<-ready
	// Send message
	svr.SendMessage(ctx, &irc.Message{
		Command: "PRIVMSG",
		Params:  []string{"hello"},
	})
	// Destroy server
	svr.Close(ctx)
	// Wait for fake server to acknowledge QUIT
	<-done
}
