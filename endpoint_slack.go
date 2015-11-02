package main

import (
	"encoding/json"
	"fmt"

	irc "github.com/fluffle/goirc/client"
	"github.com/nlopes/slack"
)

func init() {
	AddEndpointDriver("slack", func(options *json.RawMessage) Endpoint {
		return newEndpointSlack(options)
	})
}

type EndpointSlack struct {
	Config   EndpointSlackConfig
	conn     *irc.Conn
	slack    *slack.Client
	channels []*slack.Channel
	users    map[string]string
	ims      map[string]string
	rtm      *slack.RTM
	msgid    int
	handler  func(text string, source User, channel string, response MessageTarget)
}

type SlackChannel struct {
	Members []string
}

type EndpointSlackConfig struct {
	Token    string
	Channels []string
}

type UserSlack struct {
	es   *EndpointSlack
	nick string
	id   string
}

type MessageTargetSlack struct {
	es     *EndpointSlack
	target string
	public bool
}

func newEndpointSlack(options *json.RawMessage) *EndpointSlack {
	e := &EndpointSlack{
		users: make(map[string]string),
		ims:   make(map[string]string),
	}
	json.Unmarshal(*options, &e.Config)
	e.slack = slack.New(e.Config.Token)
	return e
}

func (es *EndpointSlack) Run() {
	for _, channel := range es.Config.Channels {
		schannel, _ := es.slack.JoinChannel(channel)
		es.channels = append(es.channels, schannel)
	}

	ims, _ := es.slack.GetIMChannels()
	for _, im := range ims {
		es.ims[im.User] = im.ID
	}

	es.rtm = es.slack.NewRTM()
	go es.rtm.ManageConnection()
	go func() {
		for event := range es.rtm.IncomingEvents {
			switch ev := event.Data.(type) {
			case *slack.MessageEvent: //func(text string, source User, channel string, response MessageTarget)
				var username string
				var ok bool
				if username, ok = es.users[ev.User]; !ok {
					user, _ := es.slack.GetUserInfo(ev.User)
					username = user.Name
					es.users[ev.User] = username
				}
				u := &UserSlack{
					es:   es,
					id:   ev.User,
					nick: username,
				}
				public := true
				if ev.Channel[0] == 'D' {
					public = false
				}
				mt := &MessageTargetSlack{
					es:     es,
					target: ev.Channel,
					public: public,
				}
				es.handler(ev.Text, u, ev.Channel, mt)

			}

		}
	}()
}

func (es *EndpointSlack) GetUser(nick string) User {
	users, _ := es.slack.GetUsers()

	for _, user := range users {
		if user.Name == nick {
			return &UserSlack{
				es:   es,
				id:   user.ID,
				nick: nick,
			}
		}
	}
	return nil
}

func (es *EndpointSlack) GetChannel(channel string) MessageTarget {
	for _, schannel := range es.channels {
		if schannel.Name == channel {
			return &MessageTargetSlack{
				es:     es,
				target: schannel.ID,
				public: true,
			}
		}
	}
	return nil
}

func (es *EndpointSlack) HandleMessage(handler func(text string, source User, channel string, response MessageTarget)) {
	es.handler = handler
}

func (u *UserSlack) SendMessage(format string, args ...interface{}) {
	msg := &slack.OutgoingMessage{}
	var dm string
	var ok bool
	if dm, ok = u.es.ims[u.id]; !ok {
		_, _, dm, _ = u.es.slack.OpenIMChannel(u.id)
		u.es.ims[u.id] = dm
	}
	msg.Channel = dm
	msg.Text = fmt.Sprintf(format, args...)
	msg.Type = slack.TYPE_MESSAGE
	msg.ID = u.es.msgid
	u.es.msgid += 1
	u.es.rtm.SendMessage(msg)
}

func (u *UserSlack) HasRights() bool {
	return true
}

func (u *UserSlack) IsPublic() bool {
	return false
}

func (mt *MessageTargetSlack) SendMessage(format string, args ...interface{}) {
	msg := &slack.OutgoingMessage{}
	msg.Channel = mt.target
	msg.Text = fmt.Sprintf(format, args...)
	msg.Type = slack.TYPE_MESSAGE
	msg.ID = mt.es.msgid
	mt.es.msgid += 1
	mt.es.rtm.SendMessage(msg)
}

func (mt *MessageTargetSlack) IsPublic() bool {
	return mt.public
}
