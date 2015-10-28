package main

import (
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
							c.Privmsgf(l.Target(), "%s = %s", variable, value)
						}
					}
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