package bot

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

// Close closes the connection to the server
func (s *IrcServer) Close() {
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
	// Cancel context
	s.Cancel()
	// Close connection
	if s.conn != nil {
		s.conn.Close()
	}
}

// SendCommand tries to send a message to the server and returns true on success
func (s *IrcServer) SendMessage(ctx context.Context, parentCtx context.Context, msg *irc.Message) bool {
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
		if ctx != nil && parentCtx != nil {
			go s.Settings.ErrorCallback(ctx, parentCtx, s.name, err)
		} else {
			log.Printf("[%s] Error writing message (%s) - reconnect not handled", s.name, msg)
		}
		return false
	}
	return true
}

// ReconnectWait waits / backs off
func (s *IrcServer) ReconnectWait(ctx context.Context) bool {
	done := ctx.Done()
	if s.reconnectExp != 0 {
		p := 3600 * math.Tanh(s.reconnectExp)
		log.Printf("Sleeping for %.2f seconds before reconnecting to %s", p, s.name)
		select {
		case <-done:
			return false
		case <-time.After(time.Duration(p) * time.Second):
			return true
		}
	}
	return true
}

// Dial tries to connect to the server and start processing
func (s *IrcServer) Dial(parentCtx context.Context) {

	var ctx context.Context
	ctx, s.Cancel = context.WithCancel(parentCtx)

	if !s.ReconnectWait(ctx) {
		log.Printf("Stop trying to connect to %s", s.name)
		return
	}

	dialer := net.Dialer{Timeout: 30 * time.Second}
	var err error
	s.conn, err = dialer.DialContext(ctx, "tcp", s.addr)
	if s.Settings.TLS {
		// users must set either ServerName or InsecureSkipVerify in the config.
		s.conn = tls.Client(s.conn, s.tlsConfig)
	}
	// Handle Dial error
	if err != nil {
		if s.reconnectExp == 0 {
			s.reconnectExp = 0.001
		} else {
			s.reconnectExp = s.reconnectExp * 2
		}
		go s.Settings.ErrorCallback(ctx, parentCtx, s.name, err)
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
				go s.Settings.ErrorCallback(ctx, parentCtx, s.name, err)
				return
			}
			// Invoke callback to handle input
			s.Settings.InputCallback(ctx, parentCtx, s.name, msg)
		}
	}()
	// Send password if configured
	if len(s.Settings.Password) > 0 {
		err = s.encoder.Encode(&irc.Message{
			Command: irc.PASS,
			Params:  []string{s.Settings.Password},
		})
		// Handle error
		if err != nil {
			go s.Settings.ErrorCallback(ctx, parentCtx, s.name, err)
			return
		}
	}
	// Send NICK
	err = s.encoder.Encode(&irc.Message{
		Command: irc.NICK,
		Params:  []string{s.Settings.Nick},
	})
	// Handle error
	if err != nil {
		go s.Settings.ErrorCallback(ctx, parentCtx, s.name, err)
		return
	}
	// Send USER
	err = s.encoder.Encode(&irc.Message{
		Command: irc.USER,
		Params:  []string{s.Settings.Username, "0", "*", s.Settings.Realname},
	})
	// Handle error
	if err != nil {
		go s.Settings.ErrorCallback(ctx, parentCtx, s.name, err)
	}
	s.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
	return
}

// IrcServerSettings contains all configuration for an IRC server
type IrcServerSettings struct {
	Host          string
	Nick          string
	Password      string
	Port          int
	Realname      string
	TLS           bool
	Username      string
	ErrorCallback func(ctx context.Context, parentCtx context.Context, svrName string, err error)
	InputCallback func(ctx context.Context, parentCtx context.Context, svrName string, msg *irc.Message)
}

// NewIrcServer creates an IRC server
func NewIrcServer(name string, settings *IrcServerSettings) *IrcServer {
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
