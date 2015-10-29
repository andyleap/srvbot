package main

import (
	"log"
	"encoding/json"
	"time"
	
	irc "github.com/fluffle/goirc/client"
)

func init() {
	AddEndpointDriver("irc", func(options *json.RawMessage) Endpoint{
		return newEndpointIRC(options)
	})
}

type EndpointIRC struct {
	Config EndpointIRCConfig
	conn *irc.Conn
	handler func(text string, source User, channel string, response MessageTarget)
}

type EndpointIRCConfig struct {
	Nick     string
	Server   string
	Channels []string
}

type UserIRC struct {
	ei *EndpointIRC
	nick string
}

type MessageTargetIRC struct {
	ei   *EndpointIRC
	target string
	public bool
}

func newEndpointIRC(options *json.RawMessage) *EndpointIRC {
	e := &EndpointIRC{}
	json.Unmarshal(*options, &e.Config)
	c := irc.NewConfig(e.Config.Nick)
	c.Server = e.Config.Server
	e.conn = irc.Client(c)
	e.conn.EnableStateTracking()
	e.conn.HandleFunc(irc.CONNECTED, e.connect)
	e.conn.HandleFunc(irc.PRIVMSG, e.message)
    e.conn.HandleFunc(irc.DISCONNECTED,
        func(conn *irc.Conn, line *irc.Line) { 
			time.AfterFunc(time.Second*30, func(){
				e.conn.Connect()
			})
		})
	return e
}

func (ei *EndpointIRC) Run() {
	err := ei.conn.Connect()
	if err != nil {
		log.Printf("Error while connecting IRC endpoint: %s", err)
	}
}

func (ei *EndpointIRC) connect(c *irc.Conn, l *irc.Line) {
	for _, channel := range ei.Config.Channels {
		c.Join(channel)
	}
}

func (ei *EndpointIRC) message(c *irc.Conn, l *irc.Line) {
	log.Println(l.Raw)
	var messageTarget MessageTarget
	if l.Public() {
		messageTarget = ei.GetChannel(l.Target())
	} else {
		messageTarget = ei.GetUser(l.Target())
	}
	ei.handler(l.Text(), ei.GetUser(l.Nick), l.Target(), messageTarget)
}

func (ei *EndpointIRC) GetUser(nick string) User {
	return &UserIRC{
		ei: ei,
		nick: nick,
	}
}

func (ei *EndpointIRC) GetChannel(channel string) MessageTarget {
	return &MessageTargetIRC{
		ei: ei,
		target: channel,
		public: true,
	}
}

func (ei *EndpointIRC) HandleMessage(handler func(text string, source User, channel string, response MessageTarget)) {
	ei.handler = handler
}

func (u *UserIRC) SendMessage(format string, args ...interface{}) {
	u.ei.conn.Privmsgf(u.nick, format, args...)
}

func (u *UserIRC) HasRights() bool {
	user := u.ei.conn.StateTracker().GetNick(u.nick)
	for channel, privs := range user.Channels {
		validChannel := false
		for _, mychannel := range u.ei.Config.Channels {
			if mychannel == channel {
				validChannel = true
				break
			}
		}
		if !validChannel {
			continue
		}
		if privs.Op {
			return true
		}
	}
	return false
}
	
func (u *UserIRC) IsPublic() bool {
	return false
}

func (mt *MessageTargetIRC) SendMessage(format string, args ...interface{}) {
	mt.ei.conn.Privmsgf(mt.target, format, args...)
}

func (mt *MessageTargetIRC) IsPublic() bool {
	return mt.public
}
