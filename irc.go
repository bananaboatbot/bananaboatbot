package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"time"

	"golang.org/x/time/rate"
	irc "gopkg.in/sorcix/irc.v2"
)

// IrcServer contains everything related to a given IRC server
type IrcServer struct {
	Input       chan (irc.Message)
	LimitOutput *rate.Limiter
	Output      chan (irc.Message)
	addr        string
	conn        *irc.Conn
	name        string
	settings    *IrcServerSettings
	tlsConfig   *tls.Config
}

// IrcServerError is used to supplement errors with the friendly server name
type IrcServerError struct {
	Name  string
	Error error
}

// Dial tries to connect to the server and start processing
func (s *IrcServer) Dial(errChan chan IrcServerError) {
	var err error
	// Use irc.Dial or .DialTLS according to configuration
	if s.settings.TLS {
		s.conn, err = irc.DialTLS(s.addr, s.tlsConfig)
	} else {
		s.conn, err = irc.Dial(s.addr)
	}
	// Handle Dial error
	if err != nil {
		errChan <- IrcServerError{Name: s.name, Error: err}
		return
	}
	// Process input
	go func() {
		// Forever ...
		for {
			// Try decode message
			msg, err := s.conn.Decoder.Decode()
			// Handle error
			if err != nil {
				if err != io.EOF {
					errChan <- IrcServerError{Name: s.name, Error: err}
				}
				return
			}
			// Send message to input channel
			s.Input <- *msg
		}
	}()
	// Process output
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
			for !s.LimitOutput.Allow() {
				time.Sleep(time.Millisecond * 500)
			}
			// Send message to socket
			err := s.conn.Encoder.Encode(&msg)
			// Handle error
			if err != nil {
				errChan <- IrcServerError{Name: s.name, Error: err}
				return
			}
		}
	}()
	// Send password if configured
	if len(s.settings.Password) > 0 {
		err = s.conn.Encoder.Encode(&irc.Message{
			Command: irc.PASS,
			Params:  []string{s.settings.Password},
		})
		// Handle error
		if err != nil {
			errChan <- IrcServerError{Name: s.name, Error: err}
			return
		}
	}
	// Send NICK
	err = s.conn.Encoder.Encode(&irc.Message{
		Command: irc.NICK,
		Params:  []string{s.settings.Nick},
	})
	// Handle error
	if err != nil {
		errChan <- IrcServerError{Name: s.name, Error: err}
		return
	}
	// Send USER
	err = s.conn.Encoder.Encode(&irc.Message{
		Command: irc.USER,
		Params:  []string{s.settings.Username, "0", "*", s.settings.Realname},
	})
	// Handle error
	if err != nil {
		errChan <- IrcServerError{Name: s.name, Error: err}
	}
	return
}

// IrcServerSettings contains all configuration for an IRC server
type IrcServerSettings struct {
	Host     string
	Nick     string
	Password string
	Port     int
	Realname string
	TLS      bool
	Username string
}

// NewIrcServer creates an IRC server
func NewIrcServer(name string, settings *IrcServerSettings) *IrcServer {
	// Return new IrcServer
	return &IrcServer{
		Input:       make(chan irc.Message, 1),
		Output:      make(chan irc.Message, 1),
		LimitOutput: rate.NewLimiter(1, 10),
		addr:        fmt.Sprintf("%s:%d", settings.Host, settings.Port),
		name:        name,
		settings:    settings,
		// FIXME: should be configurable
		tlsConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
}
