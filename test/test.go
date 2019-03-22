package test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bananaboatbot/bananaboatbot/client"
	irc "gopkg.in/sorcix/irc.v2"
)

// FakeServer returns a listener and its port
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

// WaitForRegistration reads messages until USER is reached
func WaitForRegistration(ctx context.Context, conn net.Conn, dec *irc.Decoder, errors chan error) {
	for {
		conn.SetReadDeadline(time.Now().Add(time.Millisecond * 50))
		msg, err := dec.Decode()
		if err != nil {
			errors <- err
		}
		if msg.Command == "USER" {
			break
		}
	}
}

// MockIrcServer is a mock of IrcServer
type MockIrcServer struct {
	Cancel       context.CancelFunc
	done         <-chan struct{}
	messages     chan irc.Message
	reconnectExp *uint64
	settings     *client.IrcServerSettings
}

// NewMockIrcServer creates a new MockIrcServer
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

// Done returns the MockIrcServer's context's done channel
func (m *MockIrcServer) Done() <-chan struct{} {
	return m.done
}

// Dial does nothing
func (m *MockIrcServer) Dial(parentCtx context.Context) {
}

// Close does nothing
func (m *MockIrcServer) Close(parentCtx context.Context) {
}

// ReconnectWait does nothing
func (m *MockIrcServer) ReconnectWait(parentCtx context.Context) {
}

// GetSettings returns the settings of the MockIrcServer
func (m *MockIrcServer) GetSettings() *client.IrcServerSettings {
	return m.settings
}

// GetMessages returns the MockIrcServer's messages channel (message output queue)
func (m *MockIrcServer) GetMessages() chan irc.Message {
	return m.messages
}
