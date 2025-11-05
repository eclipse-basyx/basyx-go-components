// Package common provides configuration management, database initialization,
// and HTTP endpoint utilities for BaSyx Go components. It includes support
// for YAML configuration files, environment variable overrides, CORS setup,
// health endpoints, and PostgreSQL database connections with connection pooling.
// nolint:all
package common

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/spf13/viper"
)

// PrintSplash displays the BaSyx Go API ASCII art logo to the console.
// This function is typically called during application startup to provide
// visual branding and confirm the service is starting.
func PrintSplash() {
	log.Printf(`
	██████╗  █████╗ ███████╗██╗   ██╗██╗  ██╗     ██████╗  ██████╗ 
	██╔══██╗██╔══██╗██╔════╝╚██╗ ██╔╝╚██╗██╔╝    ██╔════╝ ██╔═══██╗
	██████╔╝███████║███████╗ ╚████╔╝  ╚███╔╝     ██║  ███╗██║   ██║
	██╔══██╗██╔══██║╚════██║  ╚██╔╝   ██╔██╗     ██║   ██║██║   ██║
	██████╔╝██║  ██║███████║   ██║   ██╔╝ ██╗    ╚██████╔╝╚██████╔╝
	╚═════╝ ╚═╝  ╚═╝╚══════╝   ╚═╝   ╚═╝  ╚═╝     ╚═════╝  ╚═════╝ 
																
	█████╗ ██████╗ ██╗                                            
	██╔══██╗██╔══██╗██║                                            
	███████║██████╔╝██║                                            
	██╔══██║██╔═══╝ ██║                                            
	██║  ██║██║     ██║                                            
	╚═╝  ╚═╝╚═╝     ╚═╝                                            
	`)
}

// Config represents the complete configuration structure for BaSyx services.
// It combines server settings, database configuration, CORS policy,
// OIDC authentication, and ABAC authorization settings.
type Config struct {
	Server     ServerConfig   `yaml:"server"`   // HTTP server configuration
	Postgres   PostgresConfig `yaml:"postgres"` // PostgreSQL database settings
	CorsConfig CorsConfig     `yaml:"cors"`     // CORS policy configuration

	OIDC OIDCConfig `mapstructure:"oidc" json:"oidc"` // OpenID Connect authentication
	ABAC ABACConfig `mapstructure:"abac" json:"abac"` // Attribute-Based Access Control
}

// ServerConfig contains HTTP server configuration parameters.
type ServerConfig struct {
	Port         int    `yaml:"port"`         // HTTP server port (default: 5004)
	ContextPath  string `yaml:"contextPath"`  // Base path for all endpoints
	CacheEnabled bool   `yaml:"cacheEnabled"` // Enable/disable response caching
}

// PostgresConfig contains PostgreSQL database connection parameters.
// It includes connection pooling settings for optimal performance.
type PostgresConfig struct {
	Host                   string `yaml:"host"`                   // Database host address
	Port                   int    `yaml:"port"`                   // Database port (default: 5432)
	User                   string `yaml:"user"`                   // Database username
	Password               string `yaml:"password"`               // Database password
	DBName                 string `yaml:"dbname"`                 // Database name
	MaxOpenConnections     int    `yaml:"maxOpenConnections"`     // Maximum open connections
	MaxIdleConnections     int    `yaml:"maxIdleConnections"`     // Maximum idle connections
	ConnMaxLifetimeMinutes int    `yaml:"connMaxLifetimeMinutes"` // Connection lifetime in minutes
}

// CorsConfig contains Cross-Origin Resource Sharing (CORS) policy settings.
type CorsConfig struct {
	AllowedOrigins   []string `yaml:"allowedOrigins"`   // Allowed origin domains
	AllowedMethods   []string `yaml:"allowedMethods"`   // Allowed HTTP methods
	AllowedHeaders   []string `yaml:"allowedHeaders"`   // Allowed request headers
	AllowCredentials bool     `yaml:"allowCredentials"` // Allow credentials in requests
}

// OIDCConfig contains OpenID Connect authentication provider settings.
type OIDCConfig struct {
	Issuer   string `mapstructure:"issuer" json:"issuer"`     // OIDC issuer URL
	Audience string `mapstructure:"audience" json:"audience"` // Expected token audience
	JWKSURL  string `mapstructure:"jwksURL" json:"jwksURL"`   // JSON Web Key Set URL
}

// ABACConfig contains Attribute-Based Access Control authorization settings.
type ABACConfig struct {
	Enabled             bool   `mapstructure:"enabled" json:"enabled"`                         // Enable/disable ABAC
	ClientRolesAudience string `mapstructure:"clientRolesAudience" json:"clientRolesAudience"` // Client roles audience
	ModelPath           string `mapstructure:"modelPath" json:"modelPath"`                     // Path to access control model
}

// LoadConfig loads the configuration from YAML files and environment variables.
//
// The function supports multiple configuration sources with the following precedence:
// 1. Environment variables (highest priority)
// 2. Configuration file (if provided)
// 3. Default values (lowest priority)
//
// Environment variables should use underscore notation (e.g., SERVER_PORT for server.port).
//
// Parameters:
//   - configPath: Path to the YAML configuration file. If empty, only environment
//     variables and defaults will be used.
//
// Returns:
//   - *Config: Loaded configuration structure
//   - error: Error if configuration loading fails
//
// Example:
//
//	config, err := LoadConfig("config/app.yaml")
//	if err != nil {
//	    log.Fatal("Failed to load config:", err)
//	}
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	if configPath != "" {
		log.Printf("📁 Loading config from file: %s", configPath)
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
	} else {
		log.Println("📁 No config file provided — loading from environment variables only")
	}

	// Override config with environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	cfg := new(Config)
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	log.Println("✅ Configuration loaded successfully")
	PrintConfiguration(cfg)
	return cfg, nil
}

// setDefaults configures sensible default values for all configuration options.
//
// This function sets up defaults that allow the service to run in development
// environments without requiring extensive configuration. Production deployments
// should override these values through configuration files or environment variables.
//
// Parameters:
//   - v: Viper instance to configure with default values
//
// Default values include:
//   - Server: Port 5004, no context path, caching disabled
//   - Database: Local PostgreSQL on port 5432 with test credentials
//   - CORS: Permissive policy allowing all origins and common methods
//   - OIDC: Local Keycloak realm configuration
//   - ABAC: Disabled by default
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 5004)
	v.SetDefault("server.contextPath", "")
	v.SetDefault("server.cacheEnabled", false)

	// PostgreSQL defaults
	v.SetDefault("postgres.host", "db")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.user", "admin")
	v.SetDefault("postgres.password", "admin123")
	v.SetDefault("postgres.dbname", "basyxTestDB")
	v.SetDefault("postgres.maxOpenConnections", 50)
	v.SetDefault("postgres.maxIdleConnections", 50)
	v.SetDefault("postgres.connMaxLifetimeMinutes", 5)

	// CORS defaults
	v.SetDefault("cors.allowedOrigins", []string{"*"})
	v.SetDefault("cors.allowedMethods", []string{"GET", "POST", "DELETE", "OPTIONS"})
	v.SetDefault("cors.allowedHeaders", []string{"*"})
	v.SetDefault("cors.allowCredentials", true)

	v.SetDefault("oidc.issuer", "http://localhost:8080/realms/basyx")
	v.SetDefault("oidc.audience", "discovery-service")
	v.SetDefault("oidc.jwksURL", "")

	v.SetDefault("abac.enabled", false)
	v.SetDefault("abac.clientRolesAudience", "discovery-service")
	v.SetDefault("abac.modelPath", "config/access_rules/access-rules.json")

}

// PrintConfiguration prints the current configuration to the console with sensitive data redacted.
//
// This function is useful for debugging and verifying configuration during startup.
// Sensitive information such as database credentials is masked to prevent accidental
// exposure in logs.
//
// Parameters:
//   - cfg: Configuration structure to print
//
// The output is formatted as pretty-printed JSON with the following redactions:
//   - Database host, username, and password are replaced with "****"
//
// Example output:
//
//	{
//	  "server": {
//	    "port": 5004,
//	    "contextPath": "/api/v1"
//	  },
//	  "postgres": {
//	    "host": "****",
//	    "user": "****",
//	    "password": "****"
//	  }
//	}
func PrintConfiguration(cfg *Config) {
	// Create a copy of the config to avoid modifying the original
	cfgCopy := *cfg

	// Redact sensitive information if present in the Postgres configuration
	if cfg.Postgres.Host != "" {
		// Simple redaction that preserves the structure but hides credentials
		cfgCopy.Postgres.Host = "****"
		cfgCopy.Postgres.User = "****"
		cfgCopy.Postgres.Password = "****"
	}

	// Convert to JSON for pretty printing
	configJSON, err := json.MarshalIndent(cfgCopy, "", "  ")
	if err != nil {
		log.Printf("Unable to marshal configuration to JSON: %v", err)
		return
	}

	log.Printf("📜 Loaded configuration:\n%s", string(configJSON))
}

// AddCors configures Cross-Origin Resource Sharing (CORS) middleware for the router.
//
// This function sets up CORS policies based on the provided configuration,
// enabling web applications from different domains to make requests to the API.
//
// Parameters:
//   - r: Chi router to configure with CORS middleware
//   - config: Configuration containing CORS policy settings
//
// The CORS configuration includes:
//   - Allowed origins (domains that can make requests)
//   - Allowed methods (HTTP methods permitted)
//   - Allowed headers (request headers permitted)
//   - Credentials support (whether to include cookies/auth headers)
//
// Example:
//
//	router := chi.NewRouter()
//	AddCors(router, config)
//	// Router now accepts cross-origin requests according to config
func AddCors(r *chi.Mux, config *Config) {
	c := cors.New(cors.Options{
		AllowedOrigins:   config.CorsConfig.AllowedOrigins,
		AllowedMethods:   config.CorsConfig.AllowedMethods,
		AllowedHeaders:   config.CorsConfig.AllowedHeaders,
		AllowCredentials: config.CorsConfig.AllowCredentials,
	})
	r.Use(c.Handler)
}
