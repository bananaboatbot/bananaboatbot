package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"time"

	"golang.org/x/time/rate"
	irc "gopkg.in/sorcix/irc.v2"
)

type IrcServer struct {
	Input             chan (irc.Message)
	LimitOutput       *rate.Limiter
	Output            chan (irc.Message)
	addr              string
	conn              *irc.Conn
	reconnectInterval int
	settings          *IrcServerSettings
	tlsConfig         *tls.Config
}

func (s *IrcServer) Dial() {
	var err error
	if s.settings.TLS {
		s.conn, err = irc.DialTLS(s.addr, s.tlsConfig)
	} else {
		s.conn, err = irc.Dial(s.addr)
	}
	if err != nil {
		log.Printf("dial error: %s", err)
		return
	}
	go func() {
		for {
			msg, err := s.conn.Decoder.Decode()
			if err != nil {
				if err != io.EOF {
					log.Printf("decode error: %s", err)
				}
				break
			}
			s.Input <- *msg
		}
	}()
	go func() {
		for {
			msg, more := <-s.Output
			if !more {
				break
			}
			for !s.LimitOutput.Allow() {
				time.Sleep(time.Millisecond * 500)
			}
			err := s.conn.Encoder.Encode(&msg)
			if err != nil {
				log.Print(err)
				break
			}
		}
	}()
	if len(s.settings.Password) > 0 {
		err = s.conn.Encoder.Encode(&irc.Message{
			Command: irc.PASS,
			Params:  []string{s.settings.Password},
		})
		if err != nil {
			log.Printf("write error: %s", err)
			return
		}
	}
	err = s.conn.Encoder.Encode(&irc.Message{
		Command: irc.NICK,
		Params:  []string{s.settings.Nick},
	})
	if err != nil {
		log.Printf("write error: %s", err)
		return
	}
	err = s.conn.Encoder.Encode(&irc.Message{
		Command: irc.USER,
		Params:  []string{s.settings.Username, "0", "*", s.settings.Realname},
	})
	if err != nil {
		log.Printf("write error: %s", err)
		return
	}
}

type IrcServerSettings struct {
	Host     string
	Nick     string
	Password string
	Port     int
	Realname string
	TLS      bool
	Username string
}

func NewIrcServer(settings *IrcServerSettings) *IrcServer {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	s := IrcServer{
		Input:       make(chan irc.Message, 1),
		Output:      make(chan irc.Message, 1),
		LimitOutput: rate.NewLimiter(1, 10),
		addr:        fmt.Sprintf("%s:%d", settings.Host, settings.Port),
		settings:    settings,
		tlsConfig:   tlsConfig,
	}
	return &s
}
