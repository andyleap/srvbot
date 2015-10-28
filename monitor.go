package main

import (
	"encoding/json"
)

type Monitor interface {
	GetVariables() []string
	GetValues([]string) map[string]interface{}
}

var monitorDrivers = make(map[string] func(*json.RawMessage) Monitor)

func AddMonitorDriver(monitor string, constructor func(*json.RawMessage) Monitor) {
	monitorDrivers[monitor] = constructor
}