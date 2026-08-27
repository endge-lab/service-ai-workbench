package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/buildinfo"
	kitconfig "github.com/endge-lab/service-kit-go/config"
)

type BaseConfig = kitconfig.ServiceConfig

type Config struct {
	*kitconfig.ServiceConfig
	Knowledge KnowledgeConfig
	Context   ContextConfig
	Debug     DebugConfig
}

type KnowledgeConfig struct {
	BundlePath string
	MaxResults int
}

type ContextConfig struct {
	DomainMaxResults int
	MessageLimit     int
	ModelMaxChars    int
}

type DebugConfig struct {
	Enabled    bool
	OutputPath string
}

type AppConfig = kitconfig.ServiceAppConfig
type HTTPConfig = kitconfig.ServiceHTTPConfig
type LoggerConfig = kitconfig.ServiceLoggerConfig
type MetricsConfig = kitconfig.ServiceMetricsConfig
type PostgresConfig = kitconfig.ServicePostgresConfig
type AuthConfig = kitconfig.ServiceAuthConfig
type TelemetryConfig = kitconfig.ServiceTelemetryConfig
type RedpandaConfig = kitconfig.ServiceRedpandaConfig
type TLSConfig = kitconfig.ServiceTLSConfig

func Load() (*Config, error) {
	base, err := kitconfig.LoadServiceConfig()
	if err != nil {
		return nil, err
	}
	base.App.Version = buildinfo.Resolve(base.App.Version)

	debugEnabled, err := boolFromEnv("AI_DEBUG", false)
	if err != nil {
		return nil, err
	}
	maxResults, err := intFromEnv("AI_KNOWLEDGE_MAX_RESULTS", 8)
	if err != nil {
		return nil, err
	}
	if maxResults < 1 || maxResults > 20 {
		return nil, fmt.Errorf("AI_KNOWLEDGE_MAX_RESULTS must be between 1 and 20")
	}
	domainMaxResults, err := intFromEnv("AI_DOMAIN_CONTEXT_MAX_RESULTS", 20)
	if err != nil {
		return nil, err
	}
	if domainMaxResults < 1 || domainMaxResults > 50 {
		return nil, fmt.Errorf("AI_DOMAIN_CONTEXT_MAX_RESULTS must be between 1 and 50")
	}
	messageLimit, err := intFromEnv("AI_CONVERSATION_CONTEXT_MESSAGE_LIMIT", 10)
	if err != nil {
		return nil, err
	}
	if messageLimit < 1 || messageLimit > 50 {
		return nil, fmt.Errorf("AI_CONVERSATION_CONTEXT_MESSAGE_LIMIT must be between 1 and 50")
	}
	modelMaxChars, err := intFromEnv("AI_MODEL_CONTEXT_MAX_CHARS", 24000)
	if err != nil {
		return nil, err
	}
	if modelMaxChars < 4000 || modelMaxChars > 500000 {
		return nil, fmt.Errorf("AI_MODEL_CONTEXT_MAX_CHARS must be between 4000 and 500000")
	}

	return &Config{
		ServiceConfig: base,
		Knowledge: KnowledgeConfig{
			BundlePath: strings.TrimSpace(os.Getenv("AI_KNOWLEDGE_BUNDLE_PATH")),
			MaxResults: maxResults,
		},
		Context: ContextConfig{
			DomainMaxResults: domainMaxResults,
			MessageLimit:     messageLimit,
			ModelMaxChars:    modelMaxChars,
		},
		Debug: DebugConfig{
			Enabled:    debugEnabled,
			OutputPath: envOrDefault("AI_DEBUG_OUTPUT_PATH", "tmp/debug"),
		},
	}, nil
}

func boolFromEnv(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func intFromEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
