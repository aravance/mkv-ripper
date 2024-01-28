package main

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const DEFAULT_DATA_DIR = "."
const DEFAULT_LOG_DIR = "."
const DEFAULT_RIP_DIR = "."

type OmdbConfig struct {
	Apikey string
}

type TargetConfig struct {
	Scheme string
	Host   string
	Path   string
}

type Config struct {
	Data    string
	Log     string
	Rip     string
	Omdb    OmdbConfig
	Targets []TargetConfig
}

func parseConfig() Config {
	var config Config
	if b, err := os.ReadFile("mkv-ripper.toml"); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing mkv-ripper.toml: %v", err)
		config.Targets = make([]TargetConfig, 0)
	} else {
		toml.Unmarshal(b, &config)
	}
	if config.Data == "" {
		config.Data = DEFAULT_DATA_DIR
	}
	if config.Log == "" {
		config.Log = DEFAULT_LOG_DIR
	}
	if config.Rip == "" {
		config.Rip = DEFAULT_RIP_DIR
	}
	return config
}
