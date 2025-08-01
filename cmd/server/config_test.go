package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseConfigBytesEmpty(t *testing.T) {
	var config Config
	parseConfigBytes(&config, []byte{})
	expected := Config{
		Data:        DEFAULT_DATA_DIR,
		Log:         DEFAULT_LOG_DIR,
		Rip:         DEFAULT_RIP_DIR,
		Port:        DEFAULT_PORT,
		Omdb:        nil,
		Targets:     []TargetConfig{},
		UseMovieDir: false,
	}
	if !cmp.Equal(config, expected) {
		t.Fatalf("parseConfigBytes(&config, []byte{}) = %v, expected: %v", config, expected)
	}
}

const tomlstr = `
rip="/var/rip"
port=1337
usemoviedir=true

[omdb]
apikey="foobar"

[[targets]]
path="/home"

[[targets]]
scheme="ssh"
host="localhost"
path="/var"
`

func TestParseConfigBytesPartial(t *testing.T) {
	var config Config
	parseConfigBytes(&config, []byte(tomlstr))
	expected := Config{
		Data: DEFAULT_DATA_DIR,
		Log:  DEFAULT_LOG_DIR,
		Rip:  "/var/rip",
		Port: 1337,
		Omdb: &OmdbConfig{"foobar"},
		Targets: []TargetConfig{
			{Path: "/home"},
			{Scheme: "ssh", Host: "localhost", Path: "/var"},
		},
		UseMovieDir: true,
	}

	if !cmp.Equal(config, expected) {
		t.Fatalf("parseConfigBytes(&config, []byte{}) = %v, expected: %v", config, expected)
	}
}
