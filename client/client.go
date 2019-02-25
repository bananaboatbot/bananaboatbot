package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	irc "gopkg.in/sorcix/irc.v2"
)

type IrcServerInterface interface {
	Dial(ctx context.Context)
	Close(ctx context.Context)
	GetSettings() *IrcServerSettings
	GetMessages() chan irc.Message
	GetReconnectExp() *uint64
	SetReconnectExp(val uint64)
	ReconnectWait(ctx context.Context)
	Done() <-chan struct{}
}

// IrcServer contains everything related to a given IRC server
type IrcServer struct {
	Cancel       context.CancelFunc
	done         <-chan struct{}
	messages     chan irc.Message
	addr         string
	conn         net.Conn
	decoder      *irc.Decoder
	encoder      *irc.Encoder
	limitOutput  *rate.Limiter
	name         string
	reconnectExp *uint64
	Settings     *IrcServerSettings
	tlsConfig    *tls.Config
}

// IrcServerError is used to supplement errors with the friendly server name
type IrcServerError struct {
	Name  string
	Error error
}

// GetSettings returns pointer to IrcServerSettings
func (s *IrcServer) GetSettings() *IrcServerSettings {
	return s.Settings
}

// GetMessages returns pointer to IrcServerSettings
func (s *IrcServer) GetMessages() chan irc.Message {
	return s.messages
}

// GetReconnectExp returns current reconnectExp
func (s *IrcServer) GetReconnectExp() *uint64 {
	return s.reconnectExp
}

// SetReconnectExp sets current reconnectExp
func (s *IrcServer) SetReconnectExp(val uint64) {
	s.reconnectExp = &val
}

// Done returns Done channel for the server
func (s *IrcServer) Done() <-chan struct{} {
	return s.done
}

// Close closes the connection to the server
func (s *IrcServer) Close(ctx context.Context) {
	// Send QUIT
	if s.encoder != nil && s.conn != nil {
		s.conn.SetWriteDeadline(time.Now().Add(time.Second * 30))
		err := s.encoder.Encode(&irc.Message{
			Command: irc.QUIT,
			Params:  []string{"Leaving"},
		})
		if err != nil {
			log.Printf("Failed to send QUIT: %s", err)
		}
	}
	// Cancel server context
	s.Cancel()
}

// SendCommand tries to send a message to the server and returns true on success
func (s *IrcServer) sendMessages(ctx context.Context) {
	messagesToSend := s.GetMessages()
	for {
		msg, ok := <-messagesToSend
		if !ok {
			return
		}
		if !s.limitOutput.Allow() {
			log.Printf("Message ratelimited: %s", msg)
			return
		}
		// Require message to be sent in 30s
		s.conn.SetWriteDeadline(time.Now().Add(time.Second * 30))
		// Send message to socket
		err := s.encoder.Encode(&msg)
		// Handle error
		if err != nil {
			// Call error callback
			go s.Settings.ErrorCallback(ctx, s.name, err)
			return
		}
	}
}

// ReconnectWait waits / backs off
func (s *IrcServer) ReconnectWait(ctx context.Context) {
	atomic.AddUint64(s.reconnectExp, 1)
	p := s.Settings.MaxReconnect * math.Tanh(float64(*s.reconnectExp)/1000.0)
	log.Printf("Sleeping for %.2fseconds before attempting reconnect", p)
	<-time.After(time.Duration(p) * time.Second)
}

// Dial tries to connect to the server and start processing
func (s *IrcServer) Dial(ctx context.Context) {

	// Create dialer and dial
	dialer := net.Dialer{Timeout: 30 * time.Second}
	var err error
	s.conn, err = dialer.DialContext(ctx, "tcp", s.addr)
	if s.Settings.TLS {
		// users must set either ServerName or InsecureSkipVerify in the config.
		s.conn = tls.Client(s.conn, s.tlsConfig)
	}
	// Handle Dial error
	if err != nil {
		go s.Settings.ErrorCallback(ctx, s.name, err)
		return
	}
	atomic.StoreUint64(s.reconnectExp, 0)
	s.encoder = irc.NewEncoder(s.conn)
	s.decoder = irc.NewDecoder(s.conn)
	// Read loop
	go func() {
		for {
			// Read input from server and invoke callback
			s.conn.SetReadDeadline(time.Now().Add(time.Second * 300))
			// Try decode message
			msg, err := s.decoder.Decode()
			// Handle error
			if err != nil || msg.Command == irc.ERROR {
				// Set error if needed
				if err != nil && msg != nil && msg.Command == irc.ERROR {
					err = fmt.Errorf("[%s] server error: %s", s.name, strings.Join(msg.Params, ", "))
				}
				// Call error callback
				go s.Settings.ErrorCallback(ctx, s.name, err)
				return
			}
			// Invoke callback to handle input
			s.Settings.InputCallback(ctx, s.name, msg)
		}
	}()
	// Write loop
	go s.sendMessages(ctx)
	var connectCommands []*irc.Message
	index := 0
	// Send password if configured
	if len(s.Settings.Password) > 0 {
		connectCommands = make([]*irc.Message, 3)
		connectCommands[0] = &irc.Message{
			Command: irc.PASS,
			Params:  []string{s.Settings.Password},
		}
		index = 1
	} else {
		connectCommands = make([]*irc.Message, 2)
	}
	connectCommands[index] = &irc.Message{
		Command: irc.NICK,
		Params:  []string{s.Settings.Nick},
	}
	index += 1
	connectCommands[index] = &irc.Message{
		Command: irc.USER,
		Params:  []string{s.Settings.Username, "0", "*", s.Settings.Realname},
	}
	for _, cmd := range connectCommands {
		err := s.encoder.Encode(cmd)
		if err != nil {
			// Call error callback
			go s.Settings.ErrorCallback(ctx, s.name, err)
			return
		}
	}
}

// IrcServerSettings contains all configuration for an IRC server
type IrcServerSettings struct {
	Host          string
	Nick          string
	MaxReconnect  float64
	Password      string
	Port          int
	Realname      string
	TLS           bool
	Username      string
	ErrorCallback func(ctx context.Context, svrName string, err error)
	InputCallback func(ctx context.Context, svrName string, msg *irc.Message)
}

// NewIrcServer creates an IRC server
func NewIrcServer(parentCtx context.Context, name string, settings *IrcServerSettings) (IrcServerInterface, context.Context) {
	var reconnectExp uint64
	ctx, cancel := context.WithCancel(parentCtx)
	// Return new IrcServer
	s := &IrcServer{
		Cancel:       cancel,
		done:         ctx.Done(),
		limitOutput:  rate.NewLimiter(1, 10),
		addr:         fmt.Sprintf("%s:%d", settings.Host, settings.Port),
		messages:     make(chan irc.Message, 10),
		name:         name,
		reconnectExp: &reconnectExp,
		Settings:     settings,
		// FIXME: should be configurable
		tlsConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return s, ctx
}
