package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	IMAP   IMAPConfig   `yaml:"imap"`
	Safety SafetyConfig `yaml:"safety"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	MCPPath string `yaml:"mcp_path"`
}

type IMAPConfig struct {
	Host           string `yaml:"-"`
	Port           int    `yaml:"-"`
	Secure         bool   `yaml:"-"`
	User           string `yaml:"-"`
	Pass           string `yaml:"-"`
	DefaultMailbox string `yaml:"default_mailbox"`
	MaxResults     int    `yaml:"max_results"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type SafetyConfig struct {
	ReadOnly    bool `yaml:"read_only"`
	AllowDelete bool `yaml:"allow_delete"`
	AllowMove   bool `yaml:"allow_move"`
	AllowSend   bool `yaml:"allow_send"`
}

func Load(configPath string) (*Config, error) {
	_ = godotenv.Load()

	cfg := defaultConfig()
	if b, err := os.ReadFile(configPath); err == nil {
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("read %s: %w", configPath, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", configPath, err)
	}

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8095,
			MCPPath: "/mcp",
		},
		IMAP: IMAPConfig{
			Host:           "127.0.0.1",
			Port:           143,
			Secure:         false,
			DefaultMailbox: "AllMail",
			MaxResults:     50,
			TimeoutSeconds: 30,
		},
		Safety: SafetyConfig{
			ReadOnly: true,
		},
	}
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("IMAP_HOST"); v != "" {
		cfg.IMAP.Host = v
	}
	if v := os.Getenv("IMAP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("IMAP_PORT must be a number: %w", err)
		}
		cfg.IMAP.Port = port
	}
	if v := os.Getenv("IMAP_SECURE"); v != "" {
		secure, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("IMAP_SECURE must be true or false: %w", err)
		}
		cfg.IMAP.Secure = secure
	}
	cfg.IMAP.User = os.Getenv("IMAP_USER")
	cfg.IMAP.Pass = os.Getenv("IMAP_PASS")
	return nil
}

func (c *Config) Validate() error {
	if c.Server.Host == "" {
		return fmt.Errorf("server.host is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if c.Server.MCPPath == "" || c.Server.MCPPath[0] != '/' {
		return fmt.Errorf("server.mcp_path must start with /")
	}
	if c.IMAP.Host == "" {
		return fmt.Errorf("IMAP_HOST is required")
	}
	if c.IMAP.Port <= 0 || c.IMAP.Port > 65535 {
		return fmt.Errorf("IMAP_PORT must be between 1 and 65535")
	}
	if c.IMAP.User == "" {
		return fmt.Errorf("IMAP_USER is required")
	}
	if c.IMAP.Pass == "" {
		return fmt.Errorf("IMAP_PASS is required")
	}
	if c.IMAP.DefaultMailbox == "" {
		return fmt.Errorf("imap.default_mailbox is required")
	}
	if c.IMAP.MaxResults <= 0 {
		return fmt.Errorf("imap.max_results must be positive")
	}
	if c.IMAP.TimeoutSeconds <= 0 {
		return fmt.Errorf("imap.timeout_seconds must be positive")
	}
	if !c.Safety.ReadOnly {
		return fmt.Errorf("safety.read_only must be true in this version")
	}
	if c.Safety.AllowDelete || c.Safety.AllowMove || c.Safety.AllowSend {
		return fmt.Errorf("write safety flags must remain false in this version")
	}
	return nil
}

func (c *Config) HTTPAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) IMAPAddr() string {
	return fmt.Sprintf("%s:%d", c.IMAP.Host, c.IMAP.Port)
}

func (c *Config) IMAPTimeout() time.Duration {
	return time.Duration(c.IMAP.TimeoutSeconds) * time.Second
}
