package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server" json:"server"`
	Postgres PostgresConfig `mapstructure:"postgres" json:"postgres"`
	OIDC     OIDCConfig     `mapstructure:"oidc" json:"oidc"`
	ABAC     ABACConfig     `mapstructure:"abac" json:"abac"`
}

type ServerConfig struct {
	Port        int    `mapstructure:"port" json:"port"`
	ContextPath string `mapstructure:"contextPath" json:"contextPath"`
}

type PostgresConfig struct {
	Host                   string `mapstructure:"host" json:"host"`
	Port                   int    `mapstructure:"port" json:"port"`
	User                   string `mapstructure:"user" json:"user"`
	Password               string `mapstructure:"password" json:"password"`
	DBName                 string `mapstructure:"dbname" json:"dbname"`
	MaxOpenConnections     int    `mapstructure:"maxOpenConnections" json:"maxOpenConnections"`
	MaxIdleConnections     int    `mapstructure:"maxIdleConnections" json:"maxIdleConnections"`
	ConnMaxLifetimeMinutes int    `mapstructure:"connMaxLifetimeMinutes" json:"connMaxLifetimeMinutes"`
}

type OIDCConfig struct {
	Issuer   string `mapstructure:"issuer" json:"issuer"`
	Audience string `mapstructure:"audience" json:"audience"`
	JWKSURL  string `mapstructure:"jwksURL" json:"jwksURL"`
}

type ABACConfig struct {
	Enabled             bool   `mapstructure:"enabled" json:"enabled"`
	TenantClaim         string `mapstructure:"tenantClaim" json:"tenantClaim"`
	EditorRole          string `mapstructure:"editorRole" json:"editorRole"`
	ClientRolesAudience string `mapstructure:"clientRolesAudience" json:"clientRolesAudience"`
	RealmAdminRole      string `mapstructure:"realmAdminRole" json:"realmAdminRole"`
	ModelPath           string `mapstructure:"modelPath" json:"modelPath"`
	SchemaPath          string `mapstructure:"schemaPath" json:"schemaPath"`
	Validate            bool   `mapstructure:"validate" json:"validate"`
}

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
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

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 5004)
	v.SetDefault("server.contextPath", "")
	v.SetDefault("postgres.host", "localhost")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.user", "admin")
	v.SetDefault("postgres.password", "admin123")
	v.SetDefault("postgres.dbname", "basyxTestDB")
	v.SetDefault("postgres.maxOpenConnections", 50)
	v.SetDefault("postgres.maxIdleConnections", 50)
	v.SetDefault("postgres.connMaxLifetimeMinutes", 5)
	v.SetDefault("oidc.issuer", "http://localhost:8081/realms/basyx")
	v.SetDefault("oidc.audience", "discovery-service")
	v.SetDefault("oidc.jwksURL", "")
	v.SetDefault("abac.enabled", true)
	v.SetDefault("abac.tenantClaim", "tenant")
	v.SetDefault("abac.editorRole", "aas_editor")
	v.SetDefault("abac.clientRolesAudience", "discovery-service")
	v.SetDefault("abac.realmAdminRole", "admin")
	v.SetDefault("abac.modelPath", "access-rules.json")
	v.SetDefault("abac.schemaPath", "AccessRuleModel.schema.json")
	v.SetDefault("abac.validate", false)
}

func PrintConfiguration(cfg *Config) {
	copy := *cfg
	if copy.Postgres.Password != "" {
		copy.Postgres.Password = "****"
	}
	b, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		log.Printf("config marshal: %v", err)
		return
	}
	log.Printf("📜 Loaded configuration:\n%s", string(b))
}
