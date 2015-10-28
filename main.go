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
	
	irc "github.com/fluffle/goirc/client"
	"github.com/fluffle/goirc/state"
	"github.com/hpcloud/tail"
	"github.com/joliv/spark"
)

var (
	ConfigFile = flag.String("config", "srvbot.json", "Config file to load")
)

type ConfigData struct {
	Nick     string
	Server   string
	Channels []string
	Groups   []string
	Admins   []string
	Commands map[string]*Command
	Logs map[string]*Log
	Monitors map[string]*MonitorConfig
}

type Command struct {
	Command string
	Output bool
}

type Log struct {
	File string
	Regex string
	Live bool
	Keep int
	Channels []string
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
	
	c := irc.NewConfig(Config.Nick)
	c.Server = Config.Server
	i := irc.Client(c)
	i.EnableStateTracking()
	i.HandleFunc(irc.CONNECTED, Connect)
	i.HandleFunc(irc.PRIVMSG, Message)
	quit := make(chan bool)
    i.HandleFunc(irc.DISCONNECTED,
        func(conn *irc.Conn, line *irc.Line) { 
			time.AfterFunc(time.Second*30, func(){
				i.Connect()
			})
		})
		
	i.Connect()
	
	<-quit
}

var reconnect bool

func Connect(c *irc.Conn, l *irc.Line) {
	for _, channel := range Config.Channels {
		c.Join(channel)
	}
	if reconnect {
		return
	}
	reconnect = true
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
					if logConfig.Live {
						for _, channel := range logConfig.Channels {
							c.Privmsg(channel, line.Text)
						}
					}
				}
			}
			
		}(name, logConfig)
	}
	for _, monitorConfig := range Config.Monitors {
		monitorConfig.monitor = monitorDrivers[monitorConfig.Driver](monitorConfig.Options)
		monitorConfig.track = newMonitorTrack()
		monitorConfig.track.Start(monitorConfig.monitor)
	}
}

func Message(c *irc.Conn, l *irc.Line) {
	if HasRights(c.StateTracker().GetNick(l.Nick)) {
		data := ParseLine(l.Text())
		if !l.Public() {
			data = append([]string{Config.Nick}, data...)
		}
		forMe := false
		if data[0] == Config.Nick {
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
					c.Privmsg(l.Target(), line)
				}
			}
		} else if log, ok := Config.Logs[data[1]]; ok {
			for _, line := range log.lines {
				c.Privmsgf(l.Target(), "%s", line.Text)
			}
		} else if data[1] == "monitor" {
			if len(data) < 3 {
				if l.Public() {
					c.Privmsg(l.Target(), "Responding in PM")
				}
				c.Privmsg(l.Nick, "List of available monitors")
				for name, _ := range Config.Monitors {
					c.Privmsg(l.Nick, name)
				}
				return
			}
			if monitor, ok := Config.Monitors[data[2]]; ok {
				if len(data) < 4 {
					if l.Public() {
						c.Privmsg(l.Target(), "Responding in PM")
					}
					c.Privmsg(l.Nick, "Available monitor commands: variables, get")
					return
				}
				switch data[3] {
				case "variables":
					if l.Public() {
						c.Privmsg(l.Target(), "Responding in PM")
					}
					variables := monitor.monitor.GetVariables()
					if len(data) > 4 {
						regex, err := regexp.Compile("(?i)" + data[4])
						if err != nil {
							c.Privmsgf(l.Nick, "Error compiling regex: %s", err)
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
							c.Privmsgf(l.Nick, "There are over %d variables in monitor %s matching %s, filter using `monitor %s variables <regex>`", len(variables), data[2], data[4], data[2])
						} else {	
							c.Privmsgf(l.Nick, "There are over %d variables in monitor %s, filter using `monitor %s variables <regex>`", len(variables), data[2], data[2])
						}
					} else {
						if len(data) > 4 {
							c.Privmsgf(l.Nick, "List of %d variables in monitor %s matching %s", len(variables), data[2], data[4])
						} else {	
							c.Privmsgf(l.Nick, "List of %d variables in monitor %s", len(variables), data[2])
						}
						for _, name := range variables {
							c.Privmsg(l.Nick, name)
						}
					}
				case "get":
					if len(data) < 5 {
						c.Privmsg(l.Target(), "Please specify a variable or variables to retrieve")
						return
					}
					variables := data[4:]
					values := monitor.monitor.GetValues(variables)
					for _, variable := range variables {
						if value, ok := values[variable]; ok {
							c.Privmsgf(l.Target(), "%s = %v", variable, value)
						}
					}
				case "track":
					if len(data) < 5 {
						if l.Public() {
							c.Privmsg(l.Target(), "Responding in PM")
						}
						c.Privmsgf(l.Nick, "History tracking for %d variables", len(monitor.track.Variables))
						for variable, vt := range monitor.track.Variables {
							c.Privmsgf(l.Nick, "%s = %d items", variable, vt.History)
						}
						return
					}
					if len(data) < 6 {
						if vt, ok := monitor.track.Variables[data[3]]; ok {
							c.Privmsgf(l.Target(), "Not tracking history for variable %s of monitor %s", data[4], data[3])
						} else {
							c.Privmsgf(l.Target(), "History tracking for variable %s of monitor %s set to %v items", data[4], data[3], vt.History)
						}
						return
					}
					h, err := strconv.ParseInt(data[5], 10, 32)
					if err != nil {
						c.Privmsgf(l.Target(), "Error parsing %s: %s", data[5], err)
						return
					}
					monitor.track.SetTrack(data[4], int(h))
					c.Privmsgf(l.Target(), "History tracking for variable %s of monitor %s set to %v items", data[4], data[3], h)
				case "interval":
					if len(data) < 5 {
						c.Privmsgf(l.Target(), "Interval for monitor %s set to %v", data[3], monitor.track.Interval)
						return
					}
					interval, err := strconv.ParseInt(data[4], 10, 32)
					if err != nil {
						c.Privmsgf(l.Target(), "Error parsing %s: %s", data[4], err)
						return
					}
					monitor.track.Interval = int(interval)
					monitor.track.timer.Reset(time.Second * time.Duration(interval))
					c.Privmsgf(l.Target(), "Interval for monitor %s set to %v", data[3], interval)
				case "spark":
					if len(data) < 5 {
						c.Privmsg(l.Target(), "Please specify a variable to display")
						return
					}
					vt, ok := monitor.track.Variables[data[4]]
					if !ok {
						c.Privmsg(l.Target(), "Not tracking that variable")
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
							c.Privmsgf(l.Target(), "Variable is of type %t, cannot spark", tt)
						}
						if values[i] > high {
							high = values[i]
						}
						if values[i] < low {
							low = values[i]
						}
					}
					c.Privmsgf(l.Target(), "%s: %s High: %v Low: %v", data[4], spark.Line(values), high, low)
				default:
					c.Privmsgf(l.Target(), "Monitor command `%s` not recognized", data[3])
				}
				
			}
		}
	}
}

func HasRights(user *state.Nick) bool {
	for channel, privs := range user.Channels {
		validChannel := false
		for _, mychannel := range Config.Channels {
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