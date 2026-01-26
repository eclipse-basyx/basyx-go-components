// Package common provides configuration management, database initialization,
// and HTTP endpoint utilities for BaSyx Go components. It includes support
// for YAML configuration files, environment variable overrides, CORS setup,
// health endpoints, and PostgreSQL database connections with connection pooling.
// nolint:all
package common

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/spf13/viper"
)

// DefaultConfig holds all default values for configuration options.
// THESE VALUES ARE NOT USED! THEY VALIDATE IF CONFIGURATION IS DEFAULT IN THE PRINT STATEMENT
var DefaultConfig = struct {
	ServerPort         int
	ServerContextPath  string
	ServerCacheEnabled bool
	PgPort             int
	PgDBName           string
	PgMaxOpen          int
	PgMaxIdle          int
	PgConnLifetime     int
	AllowedOrigins     []string
	AllowedMethods     []string
	AllowedHeaders     []string
	AllowCredentials   bool
	OIDCTrustlistPath  string
	OIDCJWKSURL        string
	ABACEnabled        bool
	ABACModelPath      string
}{
	ServerPort:         5004,
	ServerContextPath:  "",
	ServerCacheEnabled: false,
	PgPort:             5432,
	PgDBName:           "basyxTestDB",
	PgMaxOpen:          50,
	PgMaxIdle:          50,
	PgConnLifetime:     5,
	AllowedOrigins:     []string{},
	AllowedMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
	AllowedHeaders:     []string{},
	AllowCredentials:   false,
	OIDCTrustlistPath:  "config/trustlist.json",
	OIDCJWKSURL:        "",
	ABACEnabled:        false,
	ABACModelPath:      "config/access_rules/access-rules.json",
}

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
	Server     ServerConfig   `mapstructure:"server" yaml:"server"`     // HTTP server configuration
	Postgres   PostgresConfig `mapstructure:"postgres" yaml:"postgres"` // PostgreSQL database settings
	CorsConfig CorsConfig     `mapstructure:"cors" yaml:"cors"`         // CORS policy configuration

	OIDC    OIDCConfig    `mapstructure:"oidc" yaml:"oidc"`       // OpenID Connect authentication
	ABAC    ABACConfig    `mapstructure:"abac" yaml:"abac"`       // Attribute-Based Access Control
	JWS     JWSConfig     `mapstructure:"jws" yaml:"jws"`         // JWS signing configuration
	Swagger SwaggerConfig `mapstructure:"swagger" yaml:"swagger"` // Swagger UI configuration
}

// JWSConfig contains JSON Web Signature configuration parameters.
type JWSConfig struct {
	PrivateKeyPath string `mapstructure:"privateKeyPath" yaml:"privateKeyPath"` // Path to the RSA private key for signing
}

// SwaggerConfig contains Swagger UI configuration parameters.
type SwaggerConfig struct {
	ContactName  string `mapstructure:"contactName" yaml:"contactName"`   // Contact name for OpenAPI spec
	ContactEmail string `mapstructure:"contactEmail" yaml:"contactEmail"` // Contact email for OpenAPI spec
	ContactURL   string `mapstructure:"contactUrl" yaml:"contactUrl"`     // Contact URL for OpenAPI spec
}

// ServerConfig contains HTTP server configuration parameters.
type ServerConfig struct {
	Host         string `mapstructure:"host" yaml:"host"`                 // HTTP server host (default: 0.0.0.0)
	Port         int    `mapstructure:"port" yaml:"port"`                 // HTTP server port (default: 5004)
	ContextPath  string `mapstructure:"contextPath" yaml:"contextPath"`   // Base path for all endpoints
	CacheEnabled bool   `mapstructure:"cacheEnabled" yaml:"cacheEnabled"` // Enable/disable response caching
}

// PostgresConfig contains PostgreSQL database connection parameters.
// It includes connection pooling settings for optimal performance.
type PostgresConfig struct {
	Host                   string `mapstructure:"host" yaml:"host"`                                     // Database host address
	Port                   int    `mapstructure:"port" yaml:"port"`                                     // Database port (default: 5432)
	User                   string `mapstructure:"user" yaml:"user"`                                     // Database username
	Password               string `mapstructure:"password" yaml:"password"`                             // Database password
	DBName                 string `mapstructure:"dbname" yaml:"dbname"`                                 // Database name
	MaxOpenConnections     int32  `mapstructure:"maxOpenConnections" yaml:"maxOpenConnections"`         // Maximum open connections
	MaxIdleConnections     int    `mapstructure:"maxIdleConnections" yaml:"maxIdleConnections"`         // Maximum idle connections
	ConnMaxLifetimeMinutes int    `mapstructure:"connMaxLifetimeMinutes" yaml:"connMaxLifetimeMinutes"` // Connection lifetime in minutes
}

// CorsConfig contains Cross-Origin Resource Sharing (CORS) policy settings.
type CorsConfig struct {
	AllowedOrigins   []string `mapstructure:"allowedOrigins" yaml:"allowedOrigins"`     // Allowed origin domains
	AllowedMethods   []string `mapstructure:"allowedMethods" yaml:"allowedMethods"`     // Allowed HTTP methods
	AllowedHeaders   []string `mapstructure:"allowedHeaders" yaml:"allowedHeaders"`     // Allowed request headers
	AllowCredentials bool     `mapstructure:"allowCredentials" yaml:"allowCredentials"` // Allow credentials in requests
}

// OIDCProviderConfig contains OpenID Connect authentication provider settings.
type OIDCProviderConfig struct {
	Issuer   string   `mapstructure:"issuer" yaml:"issuer" json:"issuer"`       // OIDC issuer URL
	Audience string   `mapstructure:"audience" yaml:"audience" json:"audience"` // Expected token audience
	Scopes   []string `mapstructure:"scopes" yaml:"scopes" json:"scopes"`       // Required scopes
}

// OIDCConfig contains OpenID Connect authentication provider settings.
type OIDCConfig struct {
	TrustlistPath string `mapstructure:"trustlistPath" yaml:"trustlistPath" json:"trustlistPath"` // Path to trustlist JSON
}

// ABACConfig contains Attribute-Based Access Control authorization settings.
type ABACConfig struct {
	Enabled   bool   `mapstructure:"enabled" json:"enabled"`     // Enable/disable ABAC
	ModelPath string `mapstructure:"modelPath" json:"modelPath"` // Path to access control model
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
	v.SetDefault("cors.allowedOrigins", []string{})
	v.SetDefault("cors.allowedMethods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	v.SetDefault("cors.allowedHeaders", []string{})
	v.SetDefault("cors.allowCredentials", false)

	v.SetDefault("oidc.trustlistPath", "config/trustlist.json")

	v.SetDefault("abac.enabled", false)
	v.SetDefault("abac.enableDebugErrorResponses", false)
	v.SetDefault("abac.modelPath", "config/access_rules/access-rules.json")

	// JWS defaults
	v.SetDefault("jws.privateKeyPath", "")

	// Swagger defaults
	v.SetDefault("swagger.contactName", "Eclipse BaSyx")
	v.SetDefault("swagger.contactEmail", "basyx-dev@eclipse.org")
	v.SetDefault("swagger.contactUrl", "https://basyx.org")

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
	divider := "---------------------"
	var lines []string

	add := func(label string, value any, def any) {
		suffix := ""
		if reflect.DeepEqual(value, def) {
			suffix = " (default)"
		}
		lines = append(lines, fmt.Sprintf("  %s: %v%s", label, value, suffix))
	}

	// Header
	lines = append(lines, "📜 Loaded configuration:")
	lines = append(lines, divider)

	// Server
	lines = append(lines, "🔹 Server:")
	add("Port", cfg.Server.Port, DefaultConfig.ServerPort)
	add("Context Path", cfg.Server.ContextPath, DefaultConfig.ServerContextPath)
	add("Cache Enabled", cfg.Server.CacheEnabled, DefaultConfig.ServerCacheEnabled)

	lines = append(lines, divider)

	// Postgres
	lines = append(lines, "🔹 Postgres:")
	add("Port", cfg.Postgres.Port, DefaultConfig.PgPort)
	add("DB Name", cfg.Postgres.DBName, DefaultConfig.PgDBName)
	add("Max Open Connections", cfg.Postgres.MaxOpenConnections, DefaultConfig.PgMaxOpen)
	add("Max Idle Connections", cfg.Postgres.MaxIdleConnections, DefaultConfig.PgMaxIdle)
	add("Conn Max Lifetime (min)", cfg.Postgres.ConnMaxLifetimeMinutes, DefaultConfig.PgConnLifetime)

	lines = append(lines, divider)

	// CORS
	lines = append(lines, "🔹 CORS:")
	add("Allowed Origins", cfg.CorsConfig.AllowedOrigins, DefaultConfig.AllowedOrigins)
	add("Allowed Methods", cfg.CorsConfig.AllowedMethods, DefaultConfig.AllowedMethods)
	add("Allowed Headers", cfg.CorsConfig.AllowedHeaders, DefaultConfig.AllowedHeaders)
	add("Allow Credentials", cfg.CorsConfig.AllowCredentials, DefaultConfig.AllowCredentials)

	lines = append(lines, divider)

	// ABAC
	lines = append(lines, "🔹 ABAC:")
	add("Enabled", cfg.ABAC.Enabled, DefaultConfig.ABACEnabled)
	if cfg.ABAC.Enabled {
		add("Model Path", cfg.ABAC.ModelPath, DefaultConfig.ABACModelPath)

		lines = append(lines, "🔹 OIDC:")
		add("Trustlist Path", cfg.OIDC.TrustlistPath, DefaultConfig.OIDCTrustlistPath)
	}

	lines = append(lines, divider)

	// JWS
	lines = append(lines, "🔹 JWS:")
	if cfg.JWS.PrivateKeyPath != "" {
		lines = append(lines, fmt.Sprintf("  Private Key Path: %s", cfg.JWS.PrivateKeyPath))
		// Check if file exists
		if _, err := os.Stat(cfg.JWS.PrivateKeyPath); err == nil {
			lines = append(lines, "  Private Key Mounted: true ✅")
		} else {
			lines = append(lines, "  Private Key Mounted: false ❌")
		}
	} else {
		lines = append(lines, "  Private Key Path: (not configured)")
		lines = append(lines, "  Private Key Mounted: false")
	}

	lines = append(lines, divider)

	// Find max width
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
