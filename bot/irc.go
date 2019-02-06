package bot

import (
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"time"

	"golang.org/x/time/rate"
	irc "gopkg.in/sorcix/irc.v2"
)

// IrcServer contains everything related to a given IRC server
type IrcServer struct {
	Output       chan (irc.Message)
	addr         string
	conn         *irc.Conn
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
	// XXX: QUIT
	s.conn.Close()
}

// Dial tries to connect to the server and start processing
func (s *IrcServer) Dial() {

	if s.reconnectExp != 0 {
		p := 3600 * math.Tanh(s.reconnectExp)
		log.Printf("Sleeping for %.2f seconds before reconnecting to %s", p, s.name)
		time.Sleep(time.Duration(p) * time.Second)
	}
	var err error
	// Use irc.Dial or .DialTLS according to configuration
	if s.Settings.TLS {
		s.conn, err = irc.DialTLS(s.addr, s.tlsConfig)
	} else {
		s.conn, err = irc.Dial(s.addr)
	}
	// Handle Dial error
	if err != nil {
		if s.reconnectExp == 0 {
			s.reconnectExp = 0.001
		} else {
			s.reconnectExp = s.reconnectExp * 2
		}
		s.Settings.ErrorChannel <- IrcServerError{Name: s.name, Error: err}
		return
	}
	s.reconnectExp = 0
	// Read input from server and invoke callback
	go func() {
		// Forever ...
		for {
			// Try decode message
			msg, err := s.conn.Decoder.Decode()
			// Handle error
			if err != nil {
				s.Settings.ErrorChannel <- IrcServerError{Name: s.name, Error: err}
				return
			}
			// Invoke callback
			go s.Settings.InputCallback(s.name, msg)
		}
	}()
	// Read messages from Output channel and send them to the server
	go func() {
		// Forever ...
		for {
			// Get output
			msg, more := <-s.Output
			if !more {
				// Abort if channel is closed
				return
			}
			// If ratelimit doesn't allow sending, wait
			for !s.limitOutput.Allow() {
				time.Sleep(time.Millisecond * 500)
			}
			// Send message to socket
			err := s.conn.Encoder.Encode(&msg)
			// Handle error
			if err != nil {
				s.Settings.ErrorChannel <- IrcServerError{Name: s.name, Error: err}
				return
			}
		}
	}()
	// Send password if configured
	if len(s.Settings.Password) > 0 {
		err = s.conn.Encoder.Encode(&irc.Message{
			Command: irc.PASS,
			Params:  []string{s.Settings.Password},
		})
		// Handle error
		if err != nil {
			s.Settings.ErrorChannel <- IrcServerError{Name: s.name, Error: err}
			return
		}
	}
	// Send NICK
	err = s.conn.Encoder.Encode(&irc.Message{
		Command: irc.NICK,
		Params:  []string{s.Settings.Nick},
	})
	// Handle error
	if err != nil {
		s.Settings.ErrorChannel <- IrcServerError{Name: s.name, Error: err}
		return
	}
	// Send USER
	err = s.conn.Encoder.Encode(&irc.Message{
		Command: irc.USER,
		Params:  []string{s.Settings.Username, "0", "*", s.Settings.Realname},
	})
	// Handle error
	if err != nil {
		s.Settings.ErrorChannel <- IrcServerError{Name: s.name, Error: err}
	}
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
	ErrorChannel  chan IrcServerError
	InputCallback func(svrName string, msg *irc.Message)
}

// NewIrcServer creates an IRC server
func NewIrcServer(name string, settings *IrcServerSettings) *IrcServer {
	// Return new IrcServer
	return &IrcServer{
		Output:      make(chan irc.Message, 1),
		limitOutput: rate.NewLimiter(1, 10),
		addr:        fmt.Sprintf("%s:%d", settings.Host, settings.Port),
		name:        name,
		Settings:    settings,
		// FIXME: should be configurable
		tlsConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
}
