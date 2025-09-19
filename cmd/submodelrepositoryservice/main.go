package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/spf13/viper"

	api "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

func runServer(ctx context.Context, configPath string) error {

	// Load configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
		return err
	}

	PrintConfiguration(config)

	// Create Chi router
	r := chi.NewRouter()

	// Enable CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})
	r.Use(c.Handler)

	// Add health endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{\"status\":\"UP\"}"))
	})

	// Instantiate generated services & controllers
	// ==== Discovery Service ====
	smDatabase, err := persistence_postgresql.NewPostgreSQLSubmodelBackend("postgres://" + config.Postgres.User + ":" + config.Postgres.Password + "@" + config.Postgres.Host + ":" + strconv.Itoa(config.Postgres.Port) + "/" + config.Postgres.DBName + "?sslmode=disable")
	if err != nil {
		log.Fatalf("Failed to initialize database connection: %v", err)
		return err
	}
	smSvc := api.NewSubmodelRepositoryAPIAPIService(*smDatabase)
	smCtrl := openapi.NewSubmodelRepositoryAPIAPIController(smSvc)
	for _, rt := range smCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// ==== Description Service ====
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	for _, rt := range descCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	// Add a demo-insert endpoint to bypass validation for POST
	r.Post("/demo-insert", func(w http.ResponseWriter, r *http.Request) {
		m := openapi.Submodel{
			Id:        "sm-99",
			IdShort:   "Demo",
			ModelType: "Submodel",
			Kind:      "Instance",
		}
		_, err := smDatabase.CreateSubmodel(m)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("inserted sm-99"))
	})

	// Start the server
	addr := "0.0.0.0:5004"
	log.Printf("▶️  Submodel Repository listening on %s\n", addr)
	// Start server in a goroutine
	go func() {
		if err := http.ListenAndServe(addr, r); err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server...")
	return nil
}

func main() {
	ctx := context.Background()
	if err := runServer(ctx, ""); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Postgres PostgresConfig `yaml:"postgres"`
}

type ServerConfig struct {
	Port        int    `yaml:"port"`
	ContextPath string `yaml:"contextPath"`
}

type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
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

	// MongoDB defaults
	v.SetDefault("postgres.host", "localhost")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.user", "admin")
	v.SetDefault("postgres.password", "admin123")
	v.SetDefault("postgres.dbname", "basyx")

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
