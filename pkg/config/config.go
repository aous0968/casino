package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Env      string
	LogLevel string
	Service  ServiceConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig
}

type ServiceConfig struct {
	Name string
	Port int
}

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	MaxConns int32
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		p.User, p.Password, p.Host, p.Port, p.Database,
	)
}

type RedisConfig struct {
	Addr string
}

type RabbitMQConfig struct {
	URL string
}

func Load() (*Config, error) {
	cfg := &Config{
		Env:      getEnv("APP_ENV", "local"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
		Service: ServiceConfig{
			Name: getEnv("SERVICE_NAME", "unknown"),
			Port: getEnvInt("PORT", 8080),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnvInt("POSTGRES_PORT", 5432),
			User:     os.Getenv("POSTGRES_USER"),
			Password: os.Getenv("POSTGRES_PASSWORD"),
			Database: os.Getenv("POSTGRES_DB"),
			MaxConns: int32(getEnvInt("POSTGRES_MAX_CONNS", 10)),
		},
		Redis: RedisConfig{
			Addr: getEnv("REDIS_ADDR", "localhost:6379"),
		},
		RabbitMQ: RabbitMQConfig{
			URL: os.Getenv("RABBITMQ_URL"),
		},
	}

	var missing []string
	if cfg.Postgres.User == "" {
		missing = append(missing, "POSTGRES_USER")
	}
	if cfg.Postgres.Password == "" {
		missing = append(missing, "POSTGRES_PASSWORD")
	}
	if cfg.Postgres.Database == "" {
		missing = append(missing, "POSTGRES_DB")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("env var %s must be an integer, got %q", key, v))
	}
	return n
}
