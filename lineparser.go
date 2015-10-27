package main

import (
	"strings"
	)

const (
	stateConsume = iota
	stateUnQuoted
	stateQuoted
)

func ParseLine(line string) []string {
	parts := []string{}
	curpart := ""
	state := stateConsume 
	reader := strings.NewReader(line)
	
	for rune, _, err := reader.ReadRune();err == nil;rune, _, err = reader.ReadRune() {
		switch state {
		case stateConsume:
			if rune == '"' {
				state = stateQuoted
			} else if rune != ' ' {
				state = stateUnQuoted
				curpart = string(rune)
			}
		case stateQuoted:
			if rune == '\\' {
				rune, _, err = reader.ReadRune()
				curpart = curpart + string(rune)
			} else if rune != '"' {
				curpart = curpart + string(rune)
			} else {
				parts = append(parts, curpart)
				curpart = ""
				state = stateConsume
			}
		case stateUnQuoted:
			if rune == '\\' {
				rune, _, err = reader.ReadRune()
				curpart = curpart + string(rune)
			} else if rune != ' ' {
				curpart = curpart + string(rune)
			} else {
				parts = append(parts, curpart)
				curpart = ""
				state = stateConsume
			}
		}
	}
	if curpart != "" {
		parts = append(parts, curpart)
	}
	return parts
}