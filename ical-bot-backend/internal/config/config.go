package config

import (
	"github.com/caarlos0/env/v11"
)

type Config struct {
	LogLevel string `env:"ICAL_BACKEND_LOG_LEVEL" envDefault:"INFO"`
	HTTPPort int    `env:"ICAL_BACKEND_HTTP_PORT" envDefault:"8080"`
	GRPCPort int    `env:"ICAL_BACKEND_GRPC_PORT" envDefault:"8081"`

	Database Database
}

type Database struct {
	URI          string `env:"ICAL_BACKEND_DATABASE_URI" envDefault:"postgresql://postgres:postgres@localhost:5432/postgres"`
	MaxOpenConns int    `env:"ICAL_BACKEND_DATABASE_MAX_OPEN_CONNS"`
}

func Get() (Config, error) {
	return env.ParseAs[Config]()
}
