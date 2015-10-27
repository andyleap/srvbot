package main

import (
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
        func(conn *irc.Conn, line *irc.Line) { quit <- true })
		
	i.Connect()
	
	<-quit
}

func Connect(c *irc.Conn, l *irc.Line) {
	for _, channel := range Config.Channels {
		c.Join(channel)
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
					if logConfig.Live {
						for _, channel := range logConfig.Channels {
							c.Privmsg(channel, line.Text)
						}
					}
				}
			}
			
		}(name, logConfig)
	}
}

func Message(c *irc.Conn, l *irc.Line) {
	if c.StateTracker().GetNick(l.Nick).Channels[l.Target()].Op {
		data := ParseLine(l.Text())
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
		}
	}
}