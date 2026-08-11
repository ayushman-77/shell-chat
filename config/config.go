package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration.
type Config struct {
	// SSH server settings
	SSHHost     string
	SSHPort     string
	HostKeyPath string

	// Redis settings
	RedisAddr string

	// ScyllaDB settings
	ScyllaHosts    []string
	ScyllaKeyspace string

	// Snowflake settings
	SnowflakeNodeID int64

	// AI Bot settings (Google Gemini)
	GeminiAPIKey string
	GeminiModel  string

	// Logging
	LogLevel string
}

// Load reads configuration from environment variables with defaults.
// It automatically parses any local .env file if present.
func Load() *Config {
	loadDotEnv(".env", ".env.local")

	return &Config{
		SSHHost:         getEnv("SSH_HOST", "0.0.0.0"),
		SSHPort:         getEnv("SSH_PORT", "10000"),
		HostKeyPath:     getEnv("HOST_KEY_PATH", ".ssh/id_ed25519"),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
		ScyllaHosts:     getEnvSlice("SCYLLA_HOSTS", []string{"127.0.0.1"}),
		ScyllaKeyspace:  getEnv("SCYLLA_KEYSPACE", "shell_chat"),
		SnowflakeNodeID: getEnvInt64("SNOWFLAKE_NODE_ID", 1),
		GeminiAPIKey:    getEnv("GEMINI_API_KEY", ""),
		GeminiModel:     getEnv("GEMINI_MODEL", "gemini-1.5-flash"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}
}

// loadDotEnv parses a local .env file and sets environment variables if not already set.
func loadDotEnv(filenames ...string) {
	for _, filename := range filenames {
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"'`)
				if _, exists := os.LookupEnv(k); !exists {
					_ = os.Setenv(k, v)
				}
			}
		}
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvSlice(key string, fallback []string) []string {
	if val, ok := os.LookupEnv(key); ok {
		return strings.Split(val, ",")
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if val, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
