package topologyexporter

// Config holds configuration for the topology exporter.
type Config struct {
	// PostgresDSN is the connection string for the Postgres topology store.
	PostgresDSN string `mapstructure:"postgres_dsn"`
}

func defaultConfig() *Config {
	return &Config{
		PostgresDSN: "postgres://agentpulse:agentpulse@localhost:5432/agentpulse?sslmode=disable",
	}
}
