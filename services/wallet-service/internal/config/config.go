package config

import (
	base "github.com/aous0968/casino/pkg/config"
)

const (
	Name        = "wallet-service"
	DefaultPort = 8082
)

// Load reads configuration with this service's identity.
func Load() (*base.Config, error) {
	return base.Load(base.Options{
		ServiceName: Name,
		DefaultPort: DefaultPort,
	})
}