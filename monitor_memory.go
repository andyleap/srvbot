package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
)

func init() {
	AddMonitorDriver("memory", func(options *json.RawMessage) Monitor {
		m := &MemoryMonitor{}
		json.Unmarshal(*options, &m)
		if m.File == "" {
			m.File = "/proc/meminfo"
		}
		m.Start()
		return m
	})
}

type MemoryMonitor struct {
	File string
}

func (m *MemoryMonitor) Start() {

}

func (m *MemoryMonitor) GetVariables() []string {
	data, err := ioutil.ReadFile(m.File)
	if err != nil {
		log.Printf("Error getting memory variables: %s", err)
		return []string{}
	}
	memstring := string(data)
	lines := strings.Split(memstring, "\n")
	variables := []string{}
	for _, line := range lines {
		name := strings.Split(line, ":")[0]
		variables = append(variables, name)
	}
	return variables
}

func (m *MemoryMonitor) GetValues(names []string) (values map[string]interface{}) {
	values = make(map[string]interface{})
	data, err := ioutil.ReadFile(m.File)
	if err != nil {
		log.Printf("Error getting memory variables: %s", err)
		return
	}
	memstring := string(data)
	lines := strings.Split(memstring, "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		found := false
		for _, name := range names {
			if name == parts[0] {
				found = true
			}
		}
		if !found {
			continue
		}
		value := strings.Split(strings.Trim(parts[1], " "), " ")[0]
		values[parts[0]], _ = strconv.ParseUint(value, 10, 64)
	}
	return
}
