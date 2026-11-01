package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	NodeName string `yaml:"nodeName"`
	NodeHost string `yaml:"nodeHost"`
	Log      Log    `yaml:"log"`
	Mysql    Mysql  `yaml:"mysql"`
	API      API    `yaml:"apiGateway"`
}

type Log struct {
	LogPath string `yaml:"logPath"`
	LogName string `yaml:"logName"`
}

type Mysql struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Address  string `yaml:"address"`
	Name     string `yaml:"name"`
}

type API struct {
	HTTPPort int `yaml:"httpPort"`
}

func Load(path string) (Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Mysql.Address == "" || cfg.Mysql.Name == "" || cfg.API.HTTPPort == 0 {
		return Config{}, fmt.Errorf("mysql.address, mysql.name and apiGateway.httpPort are required")
	}
	return cfg, nil
}
