package main

import (
	"math"
	"strconv"
	"time"
	"os"
	"regexp"
	"fmt"
	"os/exec"
	"strings"
	"log"
	"io/ioutil"
	"flag"
	"encoding/json"
	
	"github.com/hpcloud/tail"
	"github.com/joliv/spark"
)

var (
	ConfigFile = flag.String("config", "srvbot.json", "Config file to load")
)

type ConfigData struct {
	Name     string
	Endpoints []*EndpointConfig
	Groups   []string
	Commands map[string]*Command
	Logs map[string]*Log
	Monitors map[string]*MonitorConfig
}

type EndpointConfig struct {
	Driver string
	Options *json.RawMessage
	e Endpoint
}

type Command struct {
	Command string
	Output bool
}

type Log struct {
	File string
	Regex string
//	Live bool
	Keep int
//	Channels []string
	lines []*tail.Line
}

type MonitorConfig struct {
	Driver string
	Options *json.RawMessage
	monitor Monitor
	track *MonitorTrack
}

var Config ConfigData

func main() {
	ConfigBytes, err := ioutil.ReadFile(*ConfigFile)
	if err != nil {
		log.Fatalf("Error reading config file %s\n", err)		
	}
	err = json.Unmarshal(ConfigBytes, &Config)
	if err != nil {
		log.Fatalf("Error parsing config file %s\n", err)		
	}
	
	for _, endpointConfig := range Config.Endpoints {
		log.Printf("Starting up %s handler", endpointConfig.Driver)
		endpointConfig.e = endpointDrivers[endpointConfig.Driver](endpointConfig.Options)
		endpointConfig.e.HandleMessage(Message)
		endpointConfig.e.Run()
	}
	
	for name, logConfig := range Config.Logs {
		go func(name string, logConfig *Log){
			logfile, err := tail.TailFile(logConfig.File, tail.Config{Location: &tail.SeekInfo{Whence: os.SEEK_END}, Follow: true, ReOpen: true})
			
			if err != nil {
				log.Printf("Error tailing file: %s", err)
			}
			var filter *regexp.Regexp
			if logConfig.Regex != "" {
				var err error
				filter, err = regexp.Compile(logConfig.Regex)
				if err != nil {
					log.Printf("Error compiling regex: %s", err)
				}
			}
			for line := range logfile.Lines {
				if line.Err != nil {
					log.Printf("Error tailing file: %s", line.Err)
				}
			    if filter == nil || filter.MatchString(line.Text) {
					logConfig.lines = append(logConfig.lines, line)
					if len(logConfig.lines) > logConfig.Keep {
						logConfig.lines = logConfig.lines[len(logConfig.lines) - logConfig.Keep:]
					}
					/*if logConfig.Live {
						for _, channel := range logConfig.Channels {
							c.Privmsg(channel, line.Text)
						}
					}*/
				}
			}
			
		}(name, logConfig)
	}
	for _, monitorConfig := range Config.Monitors {
		monitorConfig.monitor = monitorDrivers[monitorConfig.Driver](monitorConfig.Options)
		monitorConfig.track = newMonitorTrack()
		monitorConfig.track.Start(monitorConfig.monitor)
	}
	quit := make(chan bool)
	<-quit
}

func Message(text string, source User, channel string, response MessageTarget) {
	if source.HasRights() {
		data := ParseLine(text)
		if !response.IsPublic() {
			data = append([]string{Config.Name}, data...)
		}
		forMe := false
		if data[0] == Config.Name {
			forMe = true
		}
		for _, group := range Config.Groups {
			if group == data[0] {
				forMe = true
				break
			}
		}
		if !forMe {
			return
		}
		if len(data) < 2 {
			return
		}
		if cmd, ok := Config.Commands[data[1]]; ok {
			args := cmd.Command
			for pos, param := range data {
				args = strings.Replace(args, fmt.Sprintf("$%d", pos), param, -1)
			}
			cmdexec := exec.Command("bash", "-c", args)
			output, _ := cmdexec.CombinedOutput()
			if cmd.Output {
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					response.SendMessage(line)
				}
			}
		} else if log, ok := Config.Logs[data[1]]; ok {
			for _, line := range log.lines {
				response.SendMessage("%s", line.Text)
			}
		} else if data[1] == "monitor" {
			if len(data) < 3 {
				if response.IsPublic() {
					response.SendMessage("Responding in PM")
				}
				source.SendMessage("List of available monitors")
				for name, _ := range Config.Monitors {
					source.SendMessage(name)
				}
				return
			}
			if monitor, ok := Config.Monitors[data[2]]; ok {
				if len(data) < 4 {
					if response.IsPublic() {
						response.SendMessage("Responding in PM")
					}
					source.SendMessage("Available monitor commands: variables, get")
					return
				}
				switch data[3] {
				case "variables":
					if response.IsPublic() {
						response.SendMessage("Responding in PM")
					}
					variables := monitor.monitor.GetVariables()
					if len(data) > 4 {
						regex, err := regexp.Compile("(?i)" + data[4])
						if err != nil {
							source.SendMessage("Error compiling regex: %s", err)
							return
						}
						newvars := []string{}
						for _, name := range variables {
							if regex.MatchString(name) {
								newvars = append(newvars, name)
							}
						}
						variables = newvars
					}
					
					if len(variables) > 10 {
						if len(data) > 4 {
							source.SendMessage("There are over %d variables in monitor %s matching %s, filter using `monitor %s variables <regex>`", len(variables), data[2], data[4], data[2])
						} else {	
							source.SendMessage("There are over %d variables in monitor %s, filter using `monitor %s variables <regex>`", len(variables), data[2], data[2])
						}
					} else {
						if len(data) > 4 {
							source.SendMessage("List of %d variables in monitor %s matching %s", len(variables), data[2], data[4])
						} else {	
							source.SendMessage("List of %d variables in monitor %s", len(variables), data[2])
						}
						for _, name := range variables {
							source.SendMessage(name)
						}
					}
				case "get":
					if len(data) < 5 {
						response.SendMessage("Please specify a variable or variables to retrieve")
						return
					}
					variables := data[4:]
					values := monitor.monitor.GetValues(variables)
					for _, variable := range variables {
						if value, ok := values[variable]; ok {
							response.SendMessage("%s = %v", variable, value)
						}
					}
				case "track":
					if len(data) < 5 {
						if response.IsPublic() {
							response.SendMessage("Responding in PM")
						}
						source.SendMessage("History tracking for %d variables", len(monitor.track.Variables))
						for variable, vt := range monitor.track.Variables {
							source.SendMessage("%s = %d items", variable, vt.History)
						}
						return
					}
					if len(data) < 6 {
						if vt, ok := monitor.track.Variables[data[3]]; ok {
							response.SendMessage("Not tracking history for variable %s of monitor %s", data[4], data[3])
						} else {
							response.SendMessage("History tracking for variable %s of monitor %s set to %v items", data[4], data[3], vt.History)
						}
						return
					}
					h, err := strconv.ParseInt(data[5], 10, 32)
					if err != nil {
						response.SendMessage("Error parsing %s: %s", data[5], err)
						return
					}
					monitor.track.SetTrack(data[4], int(h))
					response.SendMessage("History tracking for variable %s of monitor %s set to %v items", data[4], data[3], h)
				case "interval":
					if len(data) < 5 {
						response.SendMessage("Interval for monitor %s set to %v", data[3], monitor.track.Interval)
						return
					}
					interval, err := strconv.ParseInt(data[4], 10, 32)
					if err != nil {
						response.SendMessage("Error parsing %s: %s", data[4], err)
						return
					}
					monitor.track.Interval = int(interval)
					monitor.track.timer.Reset(time.Second * time.Duration(interval))
					response.SendMessage("Interval for monitor %s set to %v", data[3], interval)
				case "spark":
					if len(data) < 5 {
						response.SendMessage("Please specify a variable to display")
						return
					}
					vt, ok := monitor.track.Variables[data[4]]
					if !ok {
						response.SendMessage("Not tracking that variable")
						return
					}
					values := make([]float64, len(vt.Data))
					high := -math.MaxFloat64
					low := math.MaxFloat64
					for i, val := range vt.Data {
						switch tt := val.(type){
						case float64:
							values[i] = tt
						case float32:
							values[i] = float64(tt)
						case uint32:
							values[i] = float64(tt)
						case uint64:
							values[i] = float64(tt)
						case int32:
							values[i] = float64(tt)
						case int64:
							values[i] = float64(tt)
						default:
							response.SendMessage("Variable is of type %t, cannot spark", tt)
						}
						if values[i] > high {
							high = values[i]
						}
						if values[i] < low {
							low = values[i]
						}
					}
					response.SendMessage("%s: %s High: %v Low: %v", data[4], spark.Line(values), high, low)
				default:
					response.SendMessage("Monitor command `%s` not recognized", data[3])
				}
				
			}
		}
	}
}
