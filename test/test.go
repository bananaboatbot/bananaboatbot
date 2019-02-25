package test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/fatalbanana/bananaboatbot/client"
	irc "gopkg.in/sorcix/irc.v2"
)

func FakeServer(t *testing.T) (net.Listener, int) {
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

type MockIrcServer struct {
	Cancel       context.CancelFunc
	done         <-chan struct{}
	messages     chan irc.Message
	reconnectExp *uint64
	settings     *client.IrcServerSettings
}

func NewMockIrcServer(parentCtx context.Context, name string, settings *client.IrcServerSettings) (client.IrcServerInterface, context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	messageOutput := make(chan irc.Message, 10)
	m := &MockIrcServer{
		Cancel:   cancel,
		done:     ctx.Done(),
		messages: messageOutput,
		settings: settings,
	}
	return m, ctx
}

// GetReconnectExp returns current reconnectExp
func (m *MockIrcServer) GetReconnectExp() *uint64 {
	return m.reconnectExp
}

// SetReconnectExp sets current reconnectExp
func (m *MockIrcServer) SetReconnectExp(val uint64) {
	m.reconnectExp = &val
}

func (m *MockIrcServer) Done() <-chan struct{} {
	return m.done
}

func (m *MockIrcServer) Dial(parentCtx context.Context) {
}

func (m *MockIrcServer) Close(parentCtx context.Context) {
}

func (m *MockIrcServer) ReconnectWait(parentCtx context.Context) {
}

func (m *MockIrcServer) GetSettings() *client.IrcServerSettings {
	return m.settings
}

func (m *MockIrcServer) GetMessages() chan irc.Message {
	return m.messages
}
