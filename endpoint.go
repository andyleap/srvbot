package main

import (
	"encoding/json"
)

var endpointDrivers = make(map[string] func(*json.RawMessage) Endpoint)

func AddEndpointDriver(endpoint string, constructor func(*json.RawMessage) Endpoint) {
	endpointDrivers[endpoint] = constructor
}

type Endpoint interface {
	HandleMessage(func(text string, source User, channel string, response MessageTarget))
	GetUser(string) User
	GetChannel(string) MessageTarget
	Run()
}

type User interface {
	IsPublic() bool
	HasRights() bool
	SendMessage(format string, args ...interface{})
}

type MessageTarget interface {
	IsPublic() bool
	SendMessage(format string, args ...interface{})
}