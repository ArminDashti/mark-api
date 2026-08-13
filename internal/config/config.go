package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv loads KEY=VALUE pairs from a .env file into the process environment
// when the key is not already set. Missing file is ignored.
func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

// Config holds runtime settings for the API.
type Config struct {
	Addr          string
	DatabaseURL   string
	JWTSecret     string
	MigrationsDir string
	DataDir       string
}

// Load reads configuration from environment variables.
func Load() Config {
	return Config{
		Addr:          envOr("ADDR", ":8130"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		JWTSecret:     envOr("JWT_SECRET", "dev-jwt-secret-change-me"),
		MigrationsDir: envOr("MIGRATIONS_DIR", "migrations"),
		DataDir:       envOr("DATA_DIR", "data"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
