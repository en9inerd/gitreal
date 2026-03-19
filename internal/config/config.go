package config

import "strconv"

type Config struct {
	Port        string
	GitHubToken string
	Verbose     bool
	APIEnabled  bool
}

func ParseConfig(getenv func(string) string) (*Config, error) {
	return &Config{
		Port:        envStr(getenv, "GITREAL_PORT", "8080"),
		GitHubToken: envStr(getenv, "GITREAL_GITHUB_TOKEN", ""),
		Verbose:     envBool(getenv, "GITREAL_VERBOSE", false),
		APIEnabled:  envBool(getenv, "GITREAL_API", false),
	}, nil
}

func envStr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(getenv func(string) string, key string, fallback bool) bool {
	if v := getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
