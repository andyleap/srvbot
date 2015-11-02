package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func init() {
	AddMonitorDriver("mysql", func(options *json.RawMessage) Monitor {
		m := &MysqlMonitor{}
		json.Unmarshal(*options, &m)
		m.Start()
		return m
	})
}

type MysqlMonitor struct {
	Connection string
	db         *sql.DB
}

func (m *MysqlMonitor) Start() {
	var err error
	m.db, err = sql.Open("mysql", m.Connection)
	if err != nil {
		log.Printf("Error opening database: %s", err)
	}
	err = m.db.Ping()
	if err != nil {
		log.Printf("Error connecting to database: %s", err)
	}
}

func (m *MysqlMonitor) GetVariables() []string {
	data, err := m.db.Query("SHOW STATUS")
	defer data.Close()
	if err != nil {
		log.Printf("Error getting monitor variables: %s", err)
	}
	variables := []string{}
	for data.Next() {
		var name string
		var value interface{}
		data.Scan(&name, &value)
		variables = append(variables, name)
	}
	return variables
}

func (m *MysqlMonitor) GetValues(names []string) (values map[string]interface{}) {
	values = make(map[string]interface{})
	if len(names) == 0 {
		return
	}
	args := []interface{}{}
	for _, name := range names {
		args = append(args, name)
	}
	data, err := m.db.Query("SHOW STATUS WHERE `Variable_name` IN (?"+strings.Repeat(",?", len(names)-1)+")", args...)
	defer data.Close()
	if err != nil {
		log.Printf("Error getting monitor values: %s", err)
	}
	for data.Next() {
		var name string
		var value string
		data.Scan(&name, &value)
		intValue, _ := strconv.ParseInt(value, 10, 64)
		values[name] = intValue
	}
	return
}
