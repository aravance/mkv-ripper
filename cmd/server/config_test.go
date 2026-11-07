package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseConfigBytesEmpty(t *testing.T) {
	var config Config
	parseConfigBytes(&config, []byte{})
	expected := Config{
		Data:      DEFAULT_DATA_DIR,
		Log:       DEFAULT_LOG_DIR,
		Rip:       DEFAULT_RIP_DIR,
		Port:      DEFAULT_PORT,
		Omdb:      nil,
		Targets:   []TargetConfig{},
		Libraries: []LibraryConfig{},
	}
	if !cmp.Equal(config, expected) {
		t.Fatalf("parseConfigBytes(&config, []byte{}) = %v, expected: %v", config, expected)
	}
}

const tomlstr = `
rip="/var/rip"
port=1337

[omdb]
apikey="foobar"

[[targets]]
path="/home"
shafile="checksums.sha256"
usemoviedir=true

[[targets]]
scheme="ssh"
host="localhost"
path="/var"

[[libraries]]
type="jellyfin"
url="https://jellyfin.example.com"
token="secret"
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
			{Path: "/home", Shafile: "checksums.sha256", UseMovieDir: true},
			{Scheme: "ssh", Host: "localhost", Path: "/var", Shafile: DEFAULT_SHAFILE},
		},
		Libraries: []LibraryConfig{
			{Type: "jellyfin", Url: "https://jellyfin.example.com", Token: "secret"},
		},
	}

	if !cmp.Equal(config, expected) {
		t.Fatalf("parseConfigBytes(&config, []byte{}) = %v, expected: %v", config, expected)
	}
}
