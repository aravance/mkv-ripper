package main

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const DEFAULT_DATA_DIR = "."
const DEFAULT_LOG_DIR = "."
const DEFAULT_RIP_DIR = "."
const DEFAULT_PORT = 8080

type OmdbConfig struct {
	Apikey string
}

type TargetConfig struct {
	Scheme string
	Host   string
	Path   string
}

type Config struct {
	Data        string
	Log         string
	Rip         string
	Port        int
	Omdb        *OmdbConfig
	Targets     []TargetConfig
	UseMovieDir *bool
}

func ParseConfigFile(file string) Config {
	var config Config
	if b, err := os.ReadFile(file); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", file, err)
		config.Targets = make([]TargetConfig, 0)
	} else {
		parseConfigBytes(&config, b)
	}
	return config
}

func parseConfigBytes(config *Config, b []byte) {
	toml.Unmarshal(b, config)
	if config.Data == "" {
		config.Data = DEFAULT_DATA_DIR
	}
	if config.Log == "" {
		config.Log = DEFAULT_LOG_DIR
	}
	if config.Rip == "" {
		config.Rip = DEFAULT_RIP_DIR
	}
	if config.Port == 0 {
		config.Port = DEFAULT_PORT
	}
	if config.Targets == nil {
		config.Targets = make([]TargetConfig, 0)
	}
	if config.UseMovieDir == nil {
		def := false
		config.UseMovieDir = &def
	}
}
