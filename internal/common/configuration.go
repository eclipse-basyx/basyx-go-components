// Package common provides configuration management, database initialization,
// and HTTP endpoint utilities for BaSyx Go components. It includes support
// for YAML configuration files, environment variable overrides, CORS setup,
// health endpoints, and PostgreSQL database connections with connection pooling.
// nolint:all
package common

import (
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
	PrintSplash()
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
	// Redact sensitive information if present in the Postgres configuration
	pgHost := cfg.Postgres.Host
	pgUser := cfg.Postgres.User
	pgPassword := cfg.Postgres.Password
	if pgHost != "" {
		pgHost = "****"
		pgUser = "****"
		pgPassword = "****"
	}

	divider := "---------------------"
	var lines []string
	lines = append(lines, "📜 Loaded configuration:")
	lines = append(lines, divider)
	lines = append(lines, "🔹 Server:")
	lines = append(lines, fmt.Sprintf("  Port: %d", cfg.Server.Port))
	lines = append(lines, fmt.Sprintf("  Context Path: %s", cfg.Server.ContextPath))
	lines = append(lines, fmt.Sprintf("  Cache Enabled: %v", cfg.Server.CacheEnabled))
	lines = append(lines, divider)
	lines = append(lines, "🔹 Postgres:")
	lines = append(lines, fmt.Sprintf("  Host: %s", pgHost))
	lines = append(lines, fmt.Sprintf("  Port: %d", cfg.Postgres.Port))
	lines = append(lines, fmt.Sprintf("  User: %s", pgUser))
	lines = append(lines, fmt.Sprintf("  Password: %s", pgPassword))
	lines = append(lines, fmt.Sprintf("  DB Name: %s", cfg.Postgres.DBName))
	lines = append(lines, fmt.Sprintf("  Max Open Connections: %d", cfg.Postgres.MaxOpenConnections))
	lines = append(lines, fmt.Sprintf("  Max Idle Connections: %d", cfg.Postgres.MaxIdleConnections))
	lines = append(lines, fmt.Sprintf("  Conn Max Lifetime (min): %d", cfg.Postgres.ConnMaxLifetimeMinutes))
	lines = append(lines, divider)
	lines = append(lines, "🔹 CORS:")
	lines = append(lines, fmt.Sprintf("  Allowed Origins: %v", cfg.CorsConfig.AllowedOrigins))
	lines = append(lines, fmt.Sprintf("  Allowed Methods: %v", cfg.CorsConfig.AllowedMethods))
	lines = append(lines, fmt.Sprintf("  Allowed Headers: %v", cfg.CorsConfig.AllowedHeaders))
	lines = append(lines, fmt.Sprintf("  Allow Credentials: %v", cfg.CorsConfig.AllowCredentials))
	lines = append(lines, divider)
	lines = append(lines, "🔹 OIDC:")
	lines = append(lines, fmt.Sprintf("  Issuer: %s", cfg.OIDC.Issuer))
	lines = append(lines, fmt.Sprintf("  Audience: %s", cfg.OIDC.Audience))
	lines = append(lines, fmt.Sprintf("  JWKS URL: %s", cfg.OIDC.JWKSURL))
	lines = append(lines, divider)
	lines = append(lines, "🔹 ABAC:")
	lines = append(lines, fmt.Sprintf("  Enabled: %v", cfg.ABAC.Enabled))
	lines = append(lines, fmt.Sprintf("  Client Roles Audience: %s", cfg.ABAC.ClientRolesAudience))
	modelPath := cfg.ABAC.ModelPath
	lines = append(lines, fmt.Sprintf("  Model Path: %s", modelPath))
	lines = append(lines, divider)

	// Find max line length for box width
	maxLen := 0
	for _, l := range lines {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	boxTop := "╔" + strings.Repeat("═", maxLen+2) + "╗"
	boxBottom := "╚" + strings.Repeat("═", maxLen+2) + "╝"

	log.Print(boxTop)
	for _, l := range lines {
		// Remove leading spaces for consistent alignment
		trimmed := strings.TrimLeft(l, " ")
		log.Print("║  " + trimmed + strings.Repeat(" ", maxLen-len(trimmed)) + " ║")
	}
	log.Print(boxBottom)
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
