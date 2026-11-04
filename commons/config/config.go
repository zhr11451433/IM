package config

import (
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

type Config struct {
	NodeName string         `yaml:"nodeName"`
	NodeHost string         `yaml:"nodeHost"`
	Log      Log            `yaml:"log"`
	Mysql    Mysql          `yaml:"mysql"`
	API      API            `yaml:"apiGateway"`
	Nav      Navigator      `yaml:"navGateway"`
	Connect  ConnectManager `yaml:"connectManager"`
}

//后面三个，它们决定了连接建立时允许等待多久，
//以及连接建立后如何判断客户端是否还活着

type ConnectManager struct {
	//WebSocket 服务监听哪个端口
	WSPort int `yaml:"wsPort"`
	//客户端建立 WebSocket 连接后，必须在多长时间内完成身份认证
	ConnectTimeoutSeconds int `yaml:"connectTimeoutSeconds"`
	//客户端和服务端定期互相发送的：
	//你还活着吗？
	//我还活着！
	HeartbeatIntervalSeconds int `yaml:"heartbeatIntervalSeconds"`
	//如果客户端很久没有发送心跳，服务端应该等多久才认定它掉线
	HeartbeatTimeoutSeconds int `yaml:"heartbeatTimeoutSeconds"`
}
type Navigator struct {
	HTTPPort int      `yaml:"httpPort"`
	Servers  []string `yaml:"servers"`
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
	if cfg.Nav.HTTPPort < 1 || cfg.Nav.HTTPPort > 65535 {
		return Config{}, fmt.Errorf("navGateway.httpPort must be between 1 and 65535")
	}
	if cfg.Nav.HTTPPort == cfg.API.HTTPPort {
		return Config{}, fmt.Errorf("navigator and apiGateway must use different ports")
	}
	if len(cfg.Nav.Servers) == 0 {
		return Config{}, fmt.Errorf("navGateway.servers is required")
	}
	if slices.Contains(cfg.Nav.Servers, "") {
		return Config{}, fmt.Errorf("navGateway.servers must not contain empty addresses")
	}
	if cfg.Connect.WSPort < 1 || cfg.Connect.WSPort > 65535 {
		return Config{}, fmt.Errorf(
			"connectManager.wsPort must be between 1 and 65535",
		)
	}
	if cfg.Connect.WSPort == cfg.API.HTTPPort ||
		cfg.Connect.WSPort == cfg.Nav.HTTPPort {
		return Config{}, fmt.Errorf(
			"connectManager must use a different port",
		)
	}
	if cfg.Connect.ConnectTimeoutSeconds < 1 ||
		cfg.Connect.ConnectTimeoutSeconds > 300 {
		return Config{}, fmt.Errorf(
			"connectManager.connectTimeoutSeconds must be between 1 and 300",
		)
	}
	if cfg.Connect.HeartbeatIntervalSeconds < 1 ||
		cfg.Connect.HeartbeatIntervalSeconds >= cfg.Connect.HeartbeatTimeoutSeconds ||
		cfg.Connect.HeartbeatTimeoutSeconds > 3600 {
		return Config{}, fmt.Errorf(
			"heartbeat must satisfy 1 <= interval < timeout <= 3600",
		)
	}
	return cfg, nil
}
