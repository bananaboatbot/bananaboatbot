package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/fatalbanana/bananaboatbot/client"
	"github.com/fatalbanana/bananaboatbot/test"
	irc "gopkg.in/sorcix/irc.v2"
)

func TestError(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start fake IRC server on ephermal port
	l, serverPort := test.FakeServer(t)
	defer l.Close()

	done := make(chan struct{}, 1)
	errors := make(chan error, 2)

	go func() {
		conn, err := l.Accept()
		if err != nil {
			errors <- err
		}
		dec := irc.NewDecoder(conn)
		enc := irc.NewEncoder(conn)
		test.WaitForRegistration(ctx, conn, dec, errors)
		conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 50))
		enc.Encode(&irc.Message{
			Command: irc.ERROR,
			Params:  []string{"Bye"},
		})
		done <- struct{}{}
		<-ctx.Done()
	}()

	// Create server settings
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
		ErrorCallback: func(ctx context.Context, svrName string, err error) {
			done <- struct{}{}
		},
		InputCallback: func(ctx context.Context, svrName string, msg *irc.Message) {
			// We will fake it by calling handleHandlers directly
		},
	}
	// Create client
	svrI, svrCtx := client.NewIrcServer(ctx, "test", settings)
	svr := svrI.(client.IrcServerInterface)

	// Dial
	svr.Dial(svrCtx)
	// Send message
	svr.GetMessages() <- irc.Message{
		Command: "PRIVMSG",
		Params:  []string{"hello"},
	}
	// Wait for error
	select {
	case err := <-errors:
		t.Fatal(err)
	case <-done:
		break
	}
	// Destroy server
	svr.Close(ctx)
}

func TestSendAndQuit(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start fake IRC server on ephermal port
	l, serverPort := test.FakeServer(t)
	defer l.Close()

	ready := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	errors := make(chan error, 2)

	go func() {
		conn, err := l.Accept()
		if err != nil {
			errors <- err
		}
		dec := irc.NewDecoder(conn)
		for {
			conn.SetReadDeadline(time.Now().Add(time.Millisecond * 50))
			msg, err := dec.Decode()
			if err != nil {
				errors <- err
				return
			}
			if msg.Command == irc.USER {
				ready <- struct{}{}
			} else if msg.Command == irc.QUIT {
				done <- struct{}{}
				break
			}
		}
		<-ctx.Done()
	}()

	// Create server settings
	settings := &client.IrcServerSettings{
		Basic: client.BasicIrcServerSettings{
			Host:     "localhost",
			Port:     serverPort,
			TLS:      false,
			Nick:     "testbot1",
			Realname: "testbotr",
			Username: "testbotu",
		},
		ErrorCallback: func(ctx context.Context, svrName string, err error) {
			errors <- err
		},
		InputCallback: func(ctx context.Context, svrName string, msg *irc.Message) {
		},
	}

	// Create client
	svrI, svrCtx := client.NewIrcServer(ctx, "test", settings)
	svr := svrI.(client.IrcServerInterface)

	// Dial
	svr.Dial(svrCtx)
	// Wait for server to acknowledge USER
	select {
	case err := <-errors:
		t.Fatal(err)
	case <-ready:
		break
	}
	// Destroy server
	svr.Close(svrCtx)
	// Wait for fake server to acknowledge QUIT
	select {
	case err := <-errors:
		t.Fatal(err)
	case <-done:
		break
	}
}
