package main

import (
	"encoding/json"
	"time"
)

type Monitor interface {
	GetVariables() []string
	GetValues([]string) map[string]interface{}
}

var monitorDrivers = make(map[string]func(*json.RawMessage) Monitor)

func AddMonitorDriver(monitor string, constructor func(*json.RawMessage) Monitor) {
	monitorDrivers[monitor] = constructor
}

type MonitorTrack struct {
	Variables map[string]*MonitorTrackVariable
	Interval  int
	timer     *time.Timer
}

func newMonitorTrack() *MonitorTrack {
	return &MonitorTrack{
		Variables: make(map[string]*MonitorTrackVariable),
	}
}

type MonitorTrackVariable struct {
	History int
	Data    []interface{}
}

func (mt *MonitorTrack) SetTrack(variable string, history int) {
	track, ok := mt.Variables[variable]
	if !ok && history > 0 {
		track = &MonitorTrackVariable{}
		mt.Variables[variable] = track
	}
	if history == 0 && ok {
		delete(mt.Variables, variable)
		return
	}
	track.History = history
}

func (mt *MonitorTrack) Start(monitor Monitor) {
	go func() {
		mt.timer = time.NewTimer(time.Duration(1) * time.Second)
		for _ = range mt.timer.C {
			if mt.Interval > 0 {
				mt.timer.Reset(time.Second * time.Duration(mt.Interval))
			}
			variables := []string{}
			for variable := range mt.Variables {
				variables = append(variables, variable)
			}
			values := monitor.GetValues(variables)
			for variable, vt := range mt.Variables {
				vt.Data = append(vt.Data, values[variable])
				if len(vt.Data) > vt.History {
					vt.Data = vt.Data[len(vt.Data)-vt.History:]
				}
			}
		}
	}()
}
