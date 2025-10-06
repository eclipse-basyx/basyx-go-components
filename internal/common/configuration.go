package common

import (
	"encoding/json"
	"flag"
	"log"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/spf13/viper"
)

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

type Config struct {
	Server     ServerConfig   `yaml:"server"`
	Postgres   PostgresConfig `yaml:"postgres"`
	CorsConfig CorsConfig     `yaml:"cors"`
}

type ServerConfig struct {
	Port         int    `yaml:"port"`
	ContextPath  string `yaml:"contextPath"`
	CacheEnabled bool   `yaml:"cacheEnabled"`
}

type PostgresConfig struct {
	Host                   string `yaml:"host"`
	Port                   int    `yaml:"port"`
	User                   string `yaml:"user"`
	Password               string `yaml:"password"`
	DBName                 string `yaml:"dbname"`
	MaxOpenConnections     int    `yaml:"maxOpenConnections"`
	MaxIdleConnections     int    `yaml:"maxIdleConnections"`
	ConnMaxLifetimeMinutes int    `yaml:"connMaxLifetimeMinutes"`
}

type CorsConfig struct {
	AllowedOrigins   []string `yaml:"allowedOrigins"`
	AllowedMethods   []string `yaml:"allowedMethods"`
	AllowedHeaders   []string `yaml:"allowedHeaders"`
	AllowCredentials bool     `yaml:"allowCredentials"`
}

// LoadConfig loads the configuration from files and environment variables
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Read config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	// Override config with environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// setDefaults sets sensible defaults for configuration
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", "5004")
	v.SetDefault("server.contextPath", "")
	v.SetDefault("server.cacheEnabled", false)

	// MongoDB defaults
	v.SetDefault("postgres.host", "localhost")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.user", "admin")
	v.SetDefault("postgres.password", "admin123")
	v.SetDefault("postgres.dbname", "basyx")
	v.SetDefault("postgres.maxOpenConnections", 50)
	v.SetDefault("postgres.maxIdleConnections", 50)
	v.SetDefault("postgres.connMaxLifetimeMinutes", 5)

	// CORS defaults
	v.SetDefault("cors.allowedOrigins", []string{"*"})
	v.SetDefault("cors.allowedMethods", []string{"GET", "POST", "DELETE", "OPTIONS"})
	v.SetDefault("cors.allowedHeaders", []string{"*"})
	v.SetDefault("cors.allowCredentials", true)

}

// PrintConfiguration prints the current configuration with sensitive data redacted
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

	log.Printf("Configuration:\n%s", string(configJSON))
}

func ConfigureServer(configPath string) (*Config, *chi.Mux) {
	PrintSplash()

	if configPath == "" {
		cfgPathFlag := flag.String("config", "", "Path to config file")
		flag.Parse()
		configPath = *cfgPathFlag
	}

	// Load configuration
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
		return nil, nil
	}

	PrintConfiguration(cfg)

	// Create Chi router
	r := chi.NewRouter()
	return cfg, r
}

func AddCors(r *chi.Mux, config *Config) {
	c := cors.New(cors.Options{
		AllowedOrigins:   config.CorsConfig.AllowedOrigins,
		AllowedMethods:   config.CorsConfig.AllowedMethods,
		AllowedHeaders:   config.CorsConfig.AllowedHeaders,
		AllowCredentials: config.CorsConfig.AllowCredentials,
	})
	r.Use(c.Handler)
}
