package config

import "os"

// Config holds runtime configuration, sourced from the environment.
type Config struct {
	Port       string
	DBURL      string
	RedisURL   string
	BankSimURL string
	DevSeed    bool // when true, insert a known dev merchant + API keys on boot (local only)
}

// Load reads configuration from the environment, applying local-dev defaults so
// the gateway is runnable out of the box via docker-compose.
func Load() Config {
	return Config{
		Port:       env("PORT", "8080"),
		DBURL:      env("DB_URL", "postgres://nempay:nempay@localhost:5432/nempay?sslmode=disable"),
		RedisURL:   env("REDIS_URL", "redis://localhost:6379/0"),
		BankSimURL: env("BANKSIM_URL", "http://localhost:9090"),
		DevSeed:    env("NEMPAY_DEV_SEED", "") != "",
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
