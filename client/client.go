package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"net"
	"strings"
	"time"

	"golang.org/x/time/rate"
	irc "gopkg.in/sorcix/irc.v2"
)

type IrcServerInterface interface {
	SendMessage(ctx context.Context, msg *irc.Message) bool
	Dial(ctx context.Context)
	Close(ctx context.Context)
	GetSettings() *IrcServerSettings
}

// IrcServer contains everything related to a given IRC server
type IrcServer struct {
	Cancel       context.CancelFunc
	addr         string
	conn         net.Conn
	decoder      *irc.Decoder
	encoder      *irc.Encoder
	limitOutput  *rate.Limiter
	name         string
	reconnectExp float64
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
	// Close connection
	if s.conn != nil {
		s.conn.Close()
	}
}

// SendCommand tries to send a message to the server and returns true on success
func (s *IrcServer) SendMessage(ctx context.Context, msg *irc.Message) bool {
	if !s.limitOutput.Allow() {
		log.Printf("Message ratelimited: %s", msg)
		return false
	}
	// Require message to be sent in 30s
	s.conn.SetWriteDeadline(time.Now().Add(time.Second * 30))
	// Send message to socket
	err := s.encoder.Encode(msg)
	// Handle error
	if err != nil {
		// Close connection
		s.conn.Close()
		// Call error callback
		go s.Settings.ErrorCallback(ctx, s.name, err)
		return false
	}
	return true
}

// ReconnectWait waits / backs off
func (s *IrcServer) reconnectWait(ctx context.Context) {
	if s.reconnectExp == 0 {
		s.reconnectExp = 0.001
		return
	}
	p := s.Settings.MaxReconnect * math.Tanh(s.reconnectExp)
	log.Printf("Sleeping for %.2f seconds before reconnecting to %s", p, s.name)
	<-time.After(time.Duration(p) * time.Second)
}

// Dial tries to connect to the server and start processing
func (s *IrcServer) Dial(ctx context.Context) {

	s.reconnectWait(ctx)
	dialer := net.Dialer{Timeout: 30 * time.Second}
	var err error
	s.conn, err = dialer.DialContext(ctx, "tcp", s.addr)
	if s.Settings.TLS {
		// users must set either ServerName or InsecureSkipVerify in the config.
		s.conn = tls.Client(s.conn, s.tlsConfig)
	}
	// Handle Dial error
	if err != nil {
		s.reconnectExp = s.reconnectExp * 2
		go s.Settings.ErrorCallback(ctx, s.name, err)
		return
	}
	s.encoder = irc.NewEncoder(s.conn)
	s.decoder = irc.NewDecoder(s.conn)
	s.reconnectExp = 0
	// Read loop
	go func() {
		for {
			// Read input from server and invoke callback
			s.conn.SetReadDeadline(time.Now().Add(time.Second * 300))
			// Try decode message
			msg, err := s.decoder.Decode()
			// Handle error
			if err != nil || msg.Command == irc.ERROR {
				// Close connection
				s.conn.Close()
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
func NewIrcServer(name string, settings *IrcServerSettings) interface{} {
	// Return new IrcServer
	s := &IrcServer{
		limitOutput: rate.NewLimiter(1, 10),
		addr:        fmt.Sprintf("%s:%d", settings.Host, settings.Port),
		name:        name,
		Settings:    settings,
		// FIXME: should be configurable
		tlsConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return s
}
