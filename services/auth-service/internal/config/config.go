package config

import (
	base "github.com/aous0968/casino/pkg/config"
)

const (
	Name        = "auth-service"
	DefaultPort = 8081
)

// Load reads configuration with this service's identity.
func Load() (*base.Config, error) {
	return base.Load(base.Options{
		ServiceName: Name,
		DefaultPort: DefaultPort,
	})
}