package config

import (
	"os"
	"time"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTP      HTTPConfig      `yaml:"http"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Redis     RedisConfig     `yaml:"redis"`
	JWT       JWTConfig       `yaml:"jwt"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Log       LogConfig       `yaml:"log"`
}

type HTTPConfig struct {
	Addr            string        `yaml:"addr" default:":8080"`
	ReadTimeout     time.Duration `yaml:"read_timeout" default:"10s"`
	WriteTimeout    time.Duration `yaml:"write_timeout" default:"10s"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" default:"60s"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" default:"10s"`
}

type MySQLConfig struct {
	Host         string        `yaml:"host" default:"localhost"`
	Port         int           `yaml:"port" default:"3306"`
	DBName       string        `yaml:"db" default:"mkk_luna"`
	User         string        `yaml:"user" default:"root"`
	Password     string        `yaml:"pass" default:"root"`
	MaxOpenConns int           `yaml:"max_open" default:"10"`
	MaxIdleConns int           `yaml:"max_idle" default:"5"`
	MaxLifetime  time.Duration `yaml:"max_lifetime" default:"1h"`
}

type RedisConfig struct {
	Addr     string        `yaml:"addr" default:"localhost:6379"`
	Password string        `yaml:"pass" default:""`
	DB       int           `yaml:"db" default:"0"`
	TTL      time.Duration `yaml:"ttl" default:"5m"`
}

type JWTConfig struct {
	AccessTTL  time.Duration `yaml:"access_ttl" default:"15m"`
	RefreshTTL time.Duration `yaml:"refresh_ttl" default:"720h"`
	Secret     string        `yaml:"secret" default:"change-me-please-change-me-please-32"`
	Issuer     string        `yaml:"issuer" default:"task-service"`
	ClockSkew  time.Duration `yaml:"clock_skew" default:"60s"`
}

type RateLimitConfig struct {
	PerMinute int `yaml:"per_minute" default:"100"`
}

type AuthConfig struct {
	BcryptCost  int `yaml:"bcrypt_cost" default:"12"`
	LoginPerMin int `yaml:"login_per_min" default:"5"`
	RefreshPerMin int `yaml:"refresh_per_min" default:"20"`
}

type LogConfig struct {
	LevelStr string `yaml:"level" default:"info"`
}

func LoadFromEnv() (Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "config/local.yaml"
	}
	return Load(path)
}

func New() (*Config, error) {
	cfg, err := LoadFromEnv()
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if err := defaults.Set(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
