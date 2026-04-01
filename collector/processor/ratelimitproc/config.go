package ratelimitproc

import "time"

// Config holds configuration for the ratelimitproc processor.
type Config struct {
	// RatePerSecond is the sustained span-batch rate allowed per project.
	// Tokens are refilled at this rate. Default: 100.
	RatePerSecond float64 `mapstructure:"rate_per_second"`

	// BurstSize is the maximum number of batches a project can send in a burst
	// before the rate limit kicks in. Default: 200.
	BurstSize int `mapstructure:"burst_size"`

	// StaleAfter controls how long an idle bucket persists before eviction.
	// Default: 5m.
	StaleAfter time.Duration `mapstructure:"stale_after"`
}

func defaultConfig() *Config {
	return &Config{
		RatePerSecond: 100,
		BurstSize:     200,
		StaleAfter:    5 * time.Minute,
	}
}
