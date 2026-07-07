/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
// Author: Jannik Fried ( Fraunhofer IESE ), Martin Stemmer ( Fraunhofer IESE )

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

	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/spf13/viper"
)

const defaultServerStrictVerification = string(commonmodel.VerificationModePermissive)

// DefaultConfig holds all default values for configuration options.
// These values are also used to mark default values in the printed configuration.
var DefaultConfig = struct {
	ServerHost                          string
	ServerPort                          int
	ServerContextPath                   string
	ServerCacheEnabled                  bool
	ServerStrictVerification            string
	ServerVerificationEndpointAvailable bool
	ServerReadHeaderTimeoutSeconds      int
	ServerReadTimeoutSeconds            int
	ServerWriteTimeoutSeconds           int
	ServerIdleTimeoutSeconds            int
	ServerShutdownTimeoutSeconds        int
	PgPort                              int
	PgDBName                            string
	PgSSLMode                           string
	PgMaxOpen                           int
	PgMaxIdle                           int
	PgConnLifetime                      int
	AllowedOrigins                      []string
	AllowedMethods                      []string
	AllowedHeaders                      []string
	AllowCredentials                    bool
	OIDCTrustlistPath                   string
	OIDCJWKSURL                         string
	ABACEnabled                         bool
	ABACModelPath                       string
	ABACPolicyFileImport                string
	ABACPolicyScope                     string
	ABACManagementAPIEnabled            bool
	GeneralImplicitCasts                bool
	GeneralDescriptorDebug              bool
	GeneralDiscoveryIntegration         bool
	GeneralSupportsSingularSSID         bool
	GeneralEnableCustomHeaderMW         bool
	GeneralTrustProxyHeaders            bool
	GeneralTrustedProxyCIDRs            []string
	GeneralAASPreconfigPaths            []string
	GeneralBulkBatchLimit               int
	HistoryConfigMode                   string
	HistoryConfigRetentionDays          int
	HistoryConfigFullSnapshotInterval   int
	HistoryConfigImmutability           string
	HistoryConfigAuditIdentityMode      string
	HistoryEvidenceEnabled              bool
	HistoryEvidenceProvider             string
	HistoryEvidenceBucket               string
	HistoryEvidencePrefix               string
	HistoryEvidenceRegion               string
	HistoryEvidenceEndpoint             string
	HistoryEvidenceAccessKeyID          string
	HistoryEvidenceSecretAccessKey      string
	HistoryEvidenceUsePathStyle         bool
	HistoryEvidenceRetentionMode        string
	HistoryEvidenceRetentionDays        int
	HistoryEvidenceWriteTimeoutSeconds  int
	HistoryEvidenceSigningPrivateKey    string
	HistoryEvidenceSigningPublicKey     string
	HistoryEvidenceSigningRequired      bool
	HistoryIntegrityAnchorProvider      string
	EventingEnabled                     bool
	EventingFormat                      string
	EventingSinks                       []string
	EventingOutboxEnabled               bool
	EventingTopicPrefix                 string
	SwaggerEnabled                      bool
}{
	ServerHost:                          "0.0.0.0",
	ServerPort:                          5004,
	ServerContextPath:                   "",
	ServerCacheEnabled:                  false,
	ServerStrictVerification:            defaultServerStrictVerification,
	ServerVerificationEndpointAvailable: true,
	ServerReadHeaderTimeoutSeconds:      15,
	ServerReadTimeoutSeconds:            300,
	ServerWriteTimeoutSeconds:           300,
	ServerIdleTimeoutSeconds:            60,
	ServerShutdownTimeoutSeconds:        10,
	PgPort:                              5432,
	PgDBName:                            "basyxTestDB",
	PgSSLMode:                           "disable",
	PgMaxOpen:                           50,
	PgMaxIdle:                           50,
	PgConnLifetime:                      5,
	AllowedOrigins:                      []string{},
	AllowedMethods:                      []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
	AllowedHeaders:                      []string{},
	AllowCredentials:                    false,
	OIDCTrustlistPath:                   "config/trustlist.json",
	OIDCJWKSURL:                         "",
	ABACEnabled:                         false,
	ABACModelPath:                       "config/access_rules/access-rules.json",
	ABACPolicyFileImport:                "",
	ABACPolicyScope:                     "",
	ABACManagementAPIEnabled:            false,
	GeneralImplicitCasts:                true,
	GeneralDescriptorDebug:              false,
	GeneralDiscoveryIntegration:         false,
	GeneralSupportsSingularSSID:         false,
	GeneralEnableCustomHeaderMW:         false,
	GeneralTrustProxyHeaders:            false,
	GeneralTrustedProxyCIDRs:            []string{},
	GeneralAASPreconfigPaths:            []string{},
	GeneralBulkBatchLimit:               1000,
	HistoryConfigMode:                   "off",
	HistoryConfigRetentionDays:          0,
	HistoryConfigFullSnapshotInterval:   1,
	HistoryConfigImmutability:           "none",
	HistoryConfigAuditIdentityMode:      "none",
	HistoryEvidenceEnabled:              false,
	HistoryEvidenceProvider:             "none",
	HistoryEvidenceBucket:               "",
	HistoryEvidencePrefix:               "basyx-history-evidence",
	HistoryEvidenceRegion:               "us-east-1",
	HistoryEvidenceEndpoint:             "",
	HistoryEvidenceAccessKeyID:          "",
	HistoryEvidenceSecretAccessKey:      "",
	HistoryEvidenceUsePathStyle:         false,
	HistoryEvidenceRetentionMode:        "",
	HistoryEvidenceRetentionDays:        0,
	HistoryEvidenceWriteTimeoutSeconds:  10,
	HistoryEvidenceSigningPrivateKey:    "",
	HistoryEvidenceSigningPublicKey:     "",
	HistoryEvidenceSigningRequired:      false,
	HistoryIntegrityAnchorProvider:      "none",
	EventingEnabled:                     false,
	EventingFormat:                      "cloudevents",
	EventingSinks:                       []string{},
	EventingOutboxEnabled:               false,
	EventingTopicPrefix:                 "basyx",
	SwaggerEnabled:                      true,
}

const (
	// ABACPolicyFileImportAlways imports abac.modelPath on every service start.
	ABACPolicyFileImportAlways = "always"
	// ABACPolicyFileImportIfMissing imports abac.modelPath only when no active DB policy exists.
	ABACPolicyFileImportIfMissing = "if_missing"
	// ABACPolicyFileImportNever disables startup file import and requires an active DB policy.
	ABACPolicyFileImportNever = "never"

	maxABACPolicyScopeLength = 255
)

// PrintSplash displays the BaSyx Go API ASCII art logo to the console.
// This function is typically called during application startup to provide
// visual branding and confirm the service is starting.
func PrintSplash() {
	log.Printf(`

                                   ###########
                               ###################
                           (##########################
                        ##################################
                    #########################################.
                #################################################
            (########################################################
          #############################################################
          #############################################################
            #########################################################
                #################################################
                    ##########################################
                  /((/((##################################/((/(
              /(//((/(((((/###########################(((((((((((((
           (//((/((/(((((/((/((###################/((/(((((((/(((/((((
          ///((/(((((/((/((//(/((((###########(((((((((((((((((((((((((
           /((/((/((/((/((/((/(((((((((((((((((((((/((((((((/((((((/((
              ((/(((((//(/(((((((((((((((((((((((((((((((((((((((((
                  /((//((((((((((((((((((((((((((((((((((((((((.
                    (((((((((((((((((((((((((((((((((((((((((
                (((((((((((((((((((((((((((((((((((((((((((((((((
            /((((((((((((((((((((((((((((((((((((((((((((((((((((((((
          /((((((((((((((((((((((((((((((((((((((((((((((((((((((((((((
          (((((((((((((((((((((((((((((((((((((((((((((((((((((((((((((
            (((((((((((((((((((((((((((((((((((((((((((((((((((((((((
                (((((((((((((((((((((((((((((((((((((((((((((((((.
                    ((((((((((((((((((((((((((((((((((((((((((
                       (((((((((((((((((((((((((((((((((((
                           (((((((((((((((((((((((((((
                               (((((((((((((((((((
                                   (((((((((((
		██████╗  █████╗ ███████╗██╗   ██╗██╗  ██╗     ██████╗  ██████╗
		██╔══██╗██╔══██╗██╔════╝╚██╗ ██╔╝╚██╗██╔╝    ██╔════╝ ██╔═══██╗
		██████╔╝███████║███████╗ ╚████╔╝  ╚███╔╝     ██║  ███╗██║   ██║
		██╔══██╗██╔══██║╚════██║  ╚██╔╝   ██╔██╗     ██║   ██║██║   ██║
		██████╔╝██║  ██║███████║   ██║   ██╔╝ ██╗    ╚██████╔╝╚██████╔╝
		╚═════╝ ╚═╝  ╚═╝╚══════╝   ╚═╝   ╚═╝  ╚═╝     ╚═════╝  ╚═════╝
	`)
}

// Config represents the complete configuration structure for BaSyx services.
// It combines server settings, database configuration, CORS policy,
// OIDC authentication, and ABAC authorization settings.
type Config struct {
	Server     ServerConfig   `mapstructure:"server" yaml:"server"`     // HTTP server configuration
	Postgres   PostgresConfig `mapstructure:"postgres" yaml:"postgres"` // PostgreSQL database settings
	CorsConfig CorsConfig     `mapstructure:"cors" yaml:"cors"`         // CORS policy configuration

	General  GeneralConfig  `mapstructure:"general" yaml:"general"`   // General configuration
	OIDC     OIDCConfig     `mapstructure:"oidc" yaml:"oidc"`         // OpenID Connect authentication
	ABAC     ABACConfig     `mapstructure:"abac" yaml:"abac"`         // Attribute-Based Access Control
	JWS      JWSConfig      `mapstructure:"jws" yaml:"jws"`           // JWS signing configuration
	Swagger  SwaggerConfig  `mapstructure:"swagger" yaml:"swagger"`   // Swagger/OpenAPI documentation configuration
	History  HistoryConfig  `mapstructure:"history" yaml:"history"`   // History/audit behavior
	Eventing EventingConfig `mapstructure:"eventing" yaml:"eventing"` // Eventing placeholders
}

// JWSConfig contains JSON Web Signature configuration parameters.
type JWSConfig struct {
	PrivateKeyPath       string `mapstructure:"privateKeyPath" yaml:"privateKeyPath"`             // Path to the RSA private key for signing
	CertificateChainPath string `mapstructure:"certificateChainPath" yaml:"certificateChainPath"` // Path to PEM encoded X.509 certificates for x5c
}

// HistoryConfig contains history and audit configuration.
type HistoryConfig struct {
	Mode                 string                       `mapstructure:"mode" yaml:"mode" json:"mode"`                                                 // off|api|audit
	RetentionDays        int                          `mapstructure:"retentionDays" yaml:"retentionDays" json:"retentionDays"`                      // 0 = keep forever
	FullSnapshotInterval int                          `mapstructure:"fullSnapshotInterval" yaml:"fullSnapshotInterval" json:"fullSnapshotInterval"` // 1 = every history row is a full snapshot
	Immutability         string                       `mapstructure:"immutability" yaml:"immutability" json:"immutability"`                         // none|postgres_guarded|external_anchor
	AuditIdentityMode    string                       `mapstructure:"auditIdentityMode" yaml:"auditIdentityMode" json:"auditIdentityMode"`          // none|minimal|extended
	Evidence             HistoryEvidenceConfig        `mapstructure:"evidence" yaml:"evidence" json:"evidence"`
	IntegrityAnchor      HistoryIntegrityAnchorConfig `mapstructure:"integrityAnchor" yaml:"integrityAnchor" json:"integrityAnchor"`
}

// HistoryEvidenceConfig configures WORM-compatible evidence artifact storage.
type HistoryEvidenceConfig struct {
	Enabled         bool                         `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Provider        string                       `mapstructure:"provider" yaml:"provider" json:"provider"` // none|s3
	Bucket          string                       `mapstructure:"bucket" yaml:"bucket" json:"bucket"`
	Prefix          string                       `mapstructure:"prefix" yaml:"prefix" json:"prefix"`
	Region          string                       `mapstructure:"region" yaml:"region" json:"region"`
	Endpoint        string                       `mapstructure:"endpoint" yaml:"endpoint" json:"endpoint"`
	AccessKeyID     string                       `mapstructure:"accessKeyId" yaml:"accessKeyId" json:"accessKeyId"`
	SecretAccessKey string                       `mapstructure:"secretAccessKey" yaml:"secretAccessKey" json:"secretAccessKey"`
	UsePathStyle    bool                         `mapstructure:"pathStyle" yaml:"pathStyle" json:"pathStyle"`
	RetentionMode   string                       `mapstructure:"retentionMode" yaml:"retentionMode" json:"retentionMode"` // governance|compliance
	RetentionDays   int                          `mapstructure:"retentionDays" yaml:"retentionDays" json:"retentionDays"`
	WriteTimeoutSec int                          `mapstructure:"writeTimeoutSeconds" yaml:"writeTimeoutSeconds" json:"writeTimeoutSeconds"`
	Signing         HistoryEvidenceSigningConfig `mapstructure:"signing" yaml:"signing" json:"signing"`
}

// HistoryEvidenceSigningConfig configures optional manifest signing.
type HistoryEvidenceSigningConfig struct {
	PrivateKeyPath string `mapstructure:"privateKeyPath" yaml:"privateKeyPath" json:"privateKeyPath"`
	PublicKeyPath  string `mapstructure:"publicKeyPath" yaml:"publicKeyPath" json:"publicKeyPath"`
	Required       bool   `mapstructure:"required" yaml:"required" json:"required"`
}

// HistoryIntegrityAnchorConfig reserves future ledger/timestamping backends.
type HistoryIntegrityAnchorConfig struct {
	Provider string `mapstructure:"provider" yaml:"provider" json:"provider"` // none today; immudb/Rekor/Trillian later
}

// EventingConfig reserves future-compatible eventing configuration.
type EventingConfig struct {
	Enabled       bool     `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Format        string   `mapstructure:"format" yaml:"format" json:"format"`
	Sinks         []string `mapstructure:"sinks" yaml:"sinks" json:"sinks"`
	OutboxEnabled bool     `mapstructure:"outboxEnabled" yaml:"outboxEnabled" json:"outboxEnabled"`
	TopicPrefix   string   `mapstructure:"topicPrefix" yaml:"topicPrefix" json:"topicPrefix"`
}

// SwaggerConfig contains Swagger/OpenAPI documentation configuration parameters.
type SwaggerConfig struct {
	Enabled      bool   `mapstructure:"enabled" yaml:"enabled"`           // Enable/disable Swagger UI and OpenAPI spec endpoints
	ContactName  string `mapstructure:"contactName" yaml:"contactName"`   // Contact name for OpenAPI spec
	ContactEmail string `mapstructure:"contactEmail" yaml:"contactEmail"` // Contact email for OpenAPI spec
	ContactURL   string `mapstructure:"contactUrl" yaml:"contactUrl"`     // Contact URL for OpenAPI spec
}

// ServerConfig contains HTTP server configuration parameters.
type ServerConfig struct {
	Host                          string `mapstructure:"host" yaml:"host"`                                                                         // HTTP server host (default: 0.0.0.0)
	Port                          int    `mapstructure:"port" yaml:"port"`                                                                         // HTTP server port (default: 5004)
	ContextPath                   string `mapstructure:"contextPath" yaml:"contextPath"`                                                           // Base path for all endpoints
	CacheEnabled                  bool   `mapstructure:"cacheEnabled" yaml:"cacheEnabled"`                                                         // Enable/disable response caching
	StrictVerification            string `mapstructure:"strictVerification" yaml:"strictVerification"`                                             // Verification mode: off|permissive|strict (default: permissive)
	VerificationEndpointAvailable bool   `mapstructure:"verificationEndpointAvailable" yaml:"verificationEndpointAvailable"`                       // Enable/disable verification endpoint
	ReadHeaderTimeoutSeconds      int    `mapstructure:"readHeaderTimeoutSeconds" yaml:"readHeaderTimeoutSeconds" json:"readHeaderTimeoutSeconds"` // Maximum time to read request headers
	ReadTimeoutSeconds            int    `mapstructure:"readTimeoutSeconds" yaml:"readTimeoutSeconds" json:"readTimeoutSeconds"`                   // Maximum time to read an entire request
	WriteTimeoutSeconds           int    `mapstructure:"writeTimeoutSeconds" yaml:"writeTimeoutSeconds" json:"writeTimeoutSeconds"`                // Maximum time before timing out response writes
	IdleTimeoutSeconds            int    `mapstructure:"idleTimeoutSeconds" yaml:"idleTimeoutSeconds" json:"idleTimeoutSeconds"`                   // Maximum idle keep-alive connection time
	ShutdownTimeoutSeconds        int    `mapstructure:"shutdownTimeoutSeconds" yaml:"shutdownTimeoutSeconds" json:"shutdownTimeoutSeconds"`       // Maximum graceful shutdown wait time
}

// PostgresConfig contains PostgreSQL database connection parameters.
// It includes connection pooling settings for optimal performance.
type PostgresConfig struct {
	DSN                     string `mapstructure:"dsn" yaml:"dsn"`                                         // Complete PostgreSQL DSN; mutually exclusive with connection fields
	Host                    string `mapstructure:"host" yaml:"host"`                                       // Database host address
	Port                    int    `mapstructure:"port" yaml:"port"`                                       // Database port (default: 5432)
	User                    string `mapstructure:"user" yaml:"user"`                                       // Database username
	Password                string `mapstructure:"password" yaml:"password"`                               // Database password
	DBName                  string `mapstructure:"dbname" yaml:"dbname"`                                   // Database name
	SSLMode                 string `mapstructure:"sslmode" yaml:"sslmode"`                                 // SSL mode: disable|allow|prefer|require|verify-ca|verify-full
	SSLCert                 string `mapstructure:"sslcert" yaml:"sslcert"`                                 // Client certificate path
	SSLKey                  string `mapstructure:"sslkey" yaml:"sslkey"`                                   // Client private key path
	SSLRootCert             string `mapstructure:"sslrootcert" yaml:"sslrootcert"`                         // Root certificate path
	ConnectTimeoutSeconds   int    `mapstructure:"connectTimeoutSeconds" yaml:"connectTimeoutSeconds"`     // Connection timeout in seconds
	ApplicationName         string `mapstructure:"applicationName" yaml:"applicationName"`                 // PostgreSQL application_name
	FallbackApplicationName string `mapstructure:"fallbackApplicationName" yaml:"fallbackApplicationName"` // PostgreSQL fallback_application_name
	SearchPath              string `mapstructure:"searchPath" yaml:"searchPath"`                           // PostgreSQL search_path
	Options                 string `mapstructure:"options" yaml:"options"`                                 // PostgreSQL startup options
	TimeZone                string `mapstructure:"timezone" yaml:"timezone"`                               // PostgreSQL session timezone
	MaxOpenConnections      int    `mapstructure:"maxOpenConnections" yaml:"maxOpenConnections"`           // Maximum open connections
	MaxIdleConnections      int    `mapstructure:"maxIdleConnections" yaml:"maxIdleConnections"`           // Maximum idle connections
	ConnMaxLifetimeMinutes  int    `mapstructure:"connMaxLifetimeMinutes" yaml:"connMaxLifetimeMinutes"`   // Connection lifetime in minutes
}

// CorsConfig contains Cross-Origin Resource Sharing (CORS) policy settings.
type CorsConfig struct {
	AllowedOrigins   []string `mapstructure:"allowedOrigins" yaml:"allowedOrigins"`     // Allowed origin domains
	AllowedMethods   []string `mapstructure:"allowedMethods" yaml:"allowedMethods"`     // Allowed HTTP methods
	AllowedHeaders   []string `mapstructure:"allowedHeaders" yaml:"allowedHeaders"`     // Allowed request headers
	AllowCredentials bool     `mapstructure:"allowCredentials" yaml:"allowCredentials"` // Allow credentials in requests
}

// GeneralConfig contains non-domain-specific configuration.
type GeneralConfig struct {
	EnableImplicitCasts                    bool     `mapstructure:"enableImplicitCasts" yaml:"enableImplicitCasts" json:"enableImplicitCasts"`                                                          // Enable implicit casts during backend simplification
	EnableDescriptorDebug                  bool     `mapstructure:"enableDescriptorDebug" yaml:"enableDescriptorDebug" json:"enableDescriptorDebug"`                                                    // Enable descriptor query debug output
	DiscoveryIntegration                   bool     `mapstructure:"discoveryIntegration" yaml:"discoveryIntegration" json:"discoveryIntegration"`                                                       // Enable integration with discovery aas_identifier linking
	EnableCustomMiddlewareHeaderInjection  bool     `mapstructure:"enableCustomMiddlewareHeaderInjection" yaml:"enableCustomMiddlewareHeaderInjection" json:"enableCustomMiddlewareHeaderInjection"`    // Enable custom security middleware header injections
	SupportsSingularSupplementalSemanticId bool     `mapstructure:"supportsSingularSupplementalSemanticId" yaml:"supportsSingularSupplementalSemanticId" json:"supportsSingularSupplementalSemanticId"` // Use singular supplementalSemanticId for SubmodelDescriptor I/O
	AASRegistryIntegration                 bool     `mapstructure:"aasRegistryIntegration" yaml:"aasRegistryIntegration" json:"aasRegistryIntegration"`                                                 // Enable AAS repository -> registry descriptor synchronization
	SubmodelRegistryIntegration            bool     `mapstructure:"submodelRegistryIntegration" yaml:"submodelRegistryIntegration" json:"submodelRegistryIntegration"`                                  // Enable Submodel repository -> registry descriptor synchronization
	ExternalURL                            string   `mapstructure:"externalUrl" yaml:"externalUrl" json:"externalUrl"`                                                                                  // Public base URL(s) used for registry synchronization endpoint generation
	TrustProxyHeaders                      bool     `mapstructure:"trustProxyHeaders" yaml:"trustProxyHeaders" json:"trustProxyHeaders"`                                                                // Trust Forwarded/X-Forwarded-* headers when request source matches trustedProxyCIDRs
	TrustedProxyCIDRs                      []string `mapstructure:"trustedProxyCIDRs" yaml:"trustedProxyCIDRs" json:"trustedProxyCIDRs"`                                                                // CIDR allowlist for proxy source addresses eligible to provide forwarded headers
	UploadMaxSizeBytes                     int64    `mapstructure:"uploadMaxSizeBytes" yaml:"uploadMaxSizeBytes" json:"uploadMaxSizeBytes"`                                                             // Maximum allowed upload payload size in bytes
	AASPreconfigPaths                      []string `mapstructure:"aasPreconfigPaths" yaml:"aasPreconfigPaths" json:"aasPreconfigPaths"`                                                                // Files/directories loaded at startup for AAS preconfiguration
	BulkBatchLimit                         int      `mapstructure:"bulkBatchLimit" yaml:"bulkBatchLimit" json:"bulkBatchLimit"`                                                                         // Maximum row count per generated bulk SQL statement
}

// OIDCProviderConfig contains OpenID Connect authentication provider settings.
type OIDCProviderConfig struct {
	Issuer        string                   `mapstructure:"issuer" yaml:"issuer" json:"issuer"`                      // OIDC issuer URL
	Audience      string                   `mapstructure:"audience" yaml:"audience" json:"audience"`                // Optional token audience (skip audience validation if empty)
	Scopes        []string                 `mapstructure:"scopes" yaml:"scopes" json:"scopes"`                      // Required scopes
	DiscoveryURL  string                   `mapstructure:"discoveryUrl" yaml:"discoveryUrl" json:"discoveryUrl"`    // Optional non-standard OIDC discovery URL
	ScopeClaims   []string                 `mapstructure:"scopeClaims" yaml:"scopeClaims" json:"scopeClaims"`       // Optional JSON pointers to OAuth scope claims
	ClaimMappings []OIDCClaimMappingConfig `mapstructure:"claimMappings" yaml:"claimMappings" json:"claimMappings"` // Optional canonical BaSyx claim mappings
}

// OIDCClaimMappingConfig maps provider claims into the reserved basyx.* namespace.
type OIDCClaimMappingConfig struct {
	Target  string   `mapstructure:"target" yaml:"target" json:"target"`
	Mode    string   `mapstructure:"mode" yaml:"mode" json:"mode"`
	Sources []string `mapstructure:"sources" yaml:"sources" json:"sources"`
}

// OIDCConfig contains OpenID Connect authentication provider settings.
type OIDCConfig struct {
	TrustlistPath string `mapstructure:"trustlistPath" yaml:"trustlistPath" json:"trustlistPath"` // Path to trustlist JSON
}

// ABACConfig contains Attribute-Based Access Control authorization settings.
type ABACConfig struct {
	Enabled          bool                    `mapstructure:"enabled" yaml:"enabled" json:"enabled"`                             // Enable/disable ABAC
	ModelPath        string                  `mapstructure:"modelPath" yaml:"modelPath" json:"modelPath"`                       // Path to access control model
	PolicyFileImport string                  `mapstructure:"policyFileImport" yaml:"policyFileImport" json:"policyFileImport"`  // always|if_missing|never; empty uses the service default
	PolicyScope      string                  `mapstructure:"policyScope" yaml:"policyScope" json:"policyScope"`                 // Optional DB policy namespace; empty uses the service default
	ManagementAPI    ABACManagementAPIConfig `mapstructure:"managementApi" yaml:"managementApi" json:"managementApi,omitempty"` // Runtime ABAC policy management API
}

// ABACManagementAPIConfig controls the protected runtime ABAC management API.
type ABACManagementAPIConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
}

type ConfigMode int

const (
	QUIET ConfigMode = iota
	NORMAL
)

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
//   - configMode: QUIET = No Output, NORMAL = Normal Logging
//
// Returns:
//   - *Config: Loaded configuration structure
//   - error: Error if configuration loading fails
//
// Example:
//
//	config, err := LoadConfig("config/app.yaml", NORMAL)
//	if err != nil {
//	    log.Fatal("Failed to load config:", err)
//	}
func LoadConfig(configPath string, configMode ConfigMode) (*Config, error) {
	if configMode == NORMAL {
		PrintSplash()
	}
	v := viper.New()

	// Set default values
	setDefaults(v)

	if configPath != "" {
		if configMode == NORMAL {
			log.Printf("📁 Loading config from file: %s", configPath)
		}
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
	} else {
		if configMode == NORMAL {
			log.Println("📁 No config file provided — loading from environment variables only")
		}
	}

	// Override config with environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	cfg := new(Config)
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	verificationMode, err := commonmodel.ParseVerificationMode(cfg.Server.StrictVerification)
	if err != nil {
		return nil, fmt.Errorf("invalid server.strictVerification: %w", err)
	}
	cfg.Server.StrictVerification = string(verificationMode)
	applyAASPreconfigPathOverrides(cfg)
	applyServerEnvOverrides(cfg)
	applyGeneralEnvOverrides(cfg)
	if err = validatePostgresConfig(v, cfg.Postgres); err != nil {
		return nil, err
	}
	if err = validateServerConfig(cfg.Server); err != nil {
		return nil, err
	}
	applyABACEnvOverrides(cfg)
	if err = validateGeneralConfig(cfg); err != nil {
		return nil, err
	}
	if err = validateABACConfig(cfg); err != nil {
		return nil, err
	}
	applyHistoryEnvOverrides(cfg)
	applyEventingEnvOverrides(cfg)
	if err = validateHistoryAndEventingConfig(cfg); err != nil {
		return nil, err
	}
	if configMode == NORMAL {
		log.Println("✅ Configuration loaded successfully")
		PrintConfiguration(cfg)
	}
	return cfg, nil
}

func applyGeneralEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if value, ok := lookupFirstTrimmedEnv("GENERAL_BULK_BATCH_LIMIT", "BASYX_GENERAL_BULK_BATCH_LIMIT"); ok {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			cfg.General.BulkBatchLimit = parsed
		}
	}
}

func applyServerEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	applyFirstIntEnv(func(value int) { cfg.Server.ReadHeaderTimeoutSeconds = value },
		"SERVER_READ_HEADER_TIMEOUT_SECONDS",
		"BASYX_SERVER_READ_HEADER_TIMEOUT_SECONDS",
	)
	applyFirstIntEnv(func(value int) { cfg.Server.ReadTimeoutSeconds = value },
		"SERVER_READ_TIMEOUT_SECONDS",
		"BASYX_SERVER_READ_TIMEOUT_SECONDS",
	)
	applyFirstIntEnv(func(value int) { cfg.Server.WriteTimeoutSeconds = value },
		"SERVER_WRITE_TIMEOUT_SECONDS",
		"BASYX_SERVER_WRITE_TIMEOUT_SECONDS",
	)
	applyFirstIntEnv(func(value int) { cfg.Server.IdleTimeoutSeconds = value },
		"SERVER_IDLE_TIMEOUT_SECONDS",
		"BASYX_SERVER_IDLE_TIMEOUT_SECONDS",
	)
	applyFirstIntEnv(func(value int) { cfg.Server.ShutdownTimeoutSeconds = value },
		"SERVER_SHUTDOWN_TIMEOUT_SECONDS",
		"BASYX_SERVER_SHUTDOWN_TIMEOUT_SECONDS",
	)
}

func validateGeneralConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-GENERAL-NIL configuration must not be nil")
	}
	if cfg.General.BulkBatchLimit <= 0 {
		return fmt.Errorf("CONFIG-GENERAL-BULKBATCHLIMIT general.bulkBatchLimit must be greater than 0")
	}
	return nil
}

func validateServerConfig(cfg ServerConfig) error {
	timeouts := map[string]int{
		"server.readHeaderTimeoutSeconds": cfg.ReadHeaderTimeoutSeconds,
		"server.readTimeoutSeconds":       cfg.ReadTimeoutSeconds,
		"server.writeTimeoutSeconds":      cfg.WriteTimeoutSeconds,
		"server.idleTimeoutSeconds":       cfg.IdleTimeoutSeconds,
		"server.shutdownTimeoutSeconds":   cfg.ShutdownTimeoutSeconds,
	}
	for key, value := range timeouts {
		if value <= 0 {
			return fmt.Errorf("CONFIG-SERVER-TIMEOUT %s must be greater than 0", key)
		}
	}
	return nil
}

func validatePostgresConfig(v *viper.Viper, cfg PostgresConfig) error {
	if strings.TrimSpace(cfg.DSN) != "" {
		conflictingKeys := explicitlyConfiguredPostgresConnectionKeys(v)
		if len(conflictingKeys) > 0 {
			return fmt.Errorf("CONFIG-POSTGRES-DSN-CONFLICT postgres.dsn is mutually exclusive with individual postgres connection fields; remove postgres.dsn or remove these fields: %s", strings.Join(conflictingKeys, ", "))
		}
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(cfg.SSLMode)) {
	case "", "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("CONFIG-POSTGRES-SSLMODE unsupported postgres.sslmode %q", cfg.SSLMode)
	}
	if cfg.ConnectTimeoutSeconds < 0 {
		return fmt.Errorf("CONFIG-POSTGRES-CONNECTTIMEOUT postgres.connectTimeoutSeconds must not be negative")
	}
	return nil
}

func explicitlyConfiguredPostgresConnectionKeys(v *viper.Viper) []string {
	keys := []string{
		"host",
		"port",
		"user",
		"password",
		"dbname",
		"sslmode",
		"sslcert",
		"sslkey",
		"sslrootcert",
		"connectTimeoutSeconds",
		"applicationName",
		"fallbackApplicationName",
		"searchPath",
		"options",
		"timezone",
	}

	conflictingKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		configKey := "postgres." + key
		if v.InConfig(configKey) || postgresEnvConfigured(configKey) {
			conflictingKeys = append(conflictingKeys, configKey)
		}
	}
	return conflictingKeys
}

func postgresEnvConfigured(configKey string) bool {
	envKey := strings.ToUpper(strings.ReplaceAll(configKey, ".", "_"))
	_, ok := os.LookupEnv(envKey)
	return ok
}

func applyABACEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if value, ok := lookupFirstTrimmedEnv("ABAC_POLICY_FILE_IMPORT", "BASYX_ABAC_POLICY_FILE_IMPORT"); ok {
		cfg.ABAC.PolicyFileImport = value
	}
	if value, ok := lookupFirstTrimmedEnv("ABAC_POLICY_SCOPE", "BASYX_ABAC_POLICY_SCOPE"); ok {
		cfg.ABAC.PolicyScope = value
	}
	applyFirstBoolEnv(func(value bool) { cfg.ABAC.ManagementAPI.Enabled = value },
		"ABAC_MANAGEMENT_API_ENABLED",
		"ABAC_MANAGEMENTAPI_ENABLED",
		"BASYX_ABAC_MANAGEMENT_API_ENABLED",
	)
}

func validateABACConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-ABAC-NIL configuration must not be nil")
	}
	if strings.TrimSpace(cfg.ABAC.PolicyFileImport) != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.ABAC.PolicyFileImport)) {
		case ABACPolicyFileImportAlways, ABACPolicyFileImportIfMissing, ABACPolicyFileImportNever:
			cfg.ABAC.PolicyFileImport = strings.ToLower(strings.TrimSpace(cfg.ABAC.PolicyFileImport))
		default:
			return fmt.Errorf("CONFIG-ABAC-POLICYFILEIMPORT unsupported abac.policyFileImport %q", cfg.ABAC.PolicyFileImport)
		}
	}
	if strings.TrimSpace(cfg.ABAC.PolicyScope) == "" {
		cfg.ABAC.PolicyScope = ""
		return nil
	}
	scope, err := normalizeABACPolicyScope(cfg.ABAC.PolicyScope)
	if err != nil {
		return err
	}
	cfg.ABAC.PolicyScope = scope
	return nil
}

// ConfiguredPolicyScope resolves the DB-backed ABAC policy namespace for a service.
//
// When abac.policyScope is empty, the service's built-in scope is used so
// existing deployments keep their current policy isolation behavior.
func ConfiguredPolicyScope(cfg *Config, defaultServiceScope string) (string, error) {
	scope := defaultServiceScope
	if cfg != nil && strings.TrimSpace(cfg.ABAC.PolicyScope) != "" {
		scope = cfg.ABAC.PolicyScope
	}
	return normalizeABACPolicyScope(scope)
}

func normalizeABACPolicyScope(scope string) (string, error) {
	trimmed := strings.TrimSpace(scope)
	if trimmed == "" {
		return "", fmt.Errorf("CONFIG-ABAC-POLICYSCOPE abac.policyScope must not be empty")
	}
	if len(trimmed) > maxABACPolicyScopeLength {
		return "", fmt.Errorf("CONFIG-ABAC-POLICYSCOPE abac.policyScope must not exceed %d characters", maxABACPolicyScopeLength)
	}
	for _, char := range trimmed {
		if !isABACPolicyScopeChar(char) {
			return "", fmt.Errorf("CONFIG-ABAC-POLICYSCOPE abac.policyScope contains unsupported character %q", char)
		}
	}
	return trimmed, nil
}

func isABACPolicyScopeChar(char rune) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '_' ||
		char == '-' ||
		char == '.' ||
		char == ':'
}

func applyHistoryEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_MODE"); ok {
		cfg.History.Mode = value
	}
	applyIntEnv("BASYX_HISTORY_RETENTION_DAYS", func(value int) { cfg.History.RetentionDays = value })
	applyIntEnv("BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL", func(value int) { cfg.History.FullSnapshotInterval = value })
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_IMMUTABILITY"); ok {
		cfg.History.Immutability = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_AUDIT_IDENTITY_MODE"); ok {
		cfg.History.AuditIdentityMode = value
	}
	applyHistoryEvidenceEnvOverrides(cfg)
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_INTEGRITY_ANCHOR_PROVIDER"); ok {
		cfg.History.IntegrityAnchor.Provider = value
	}
}

func applyHistoryEvidenceEnvOverrides(cfg *Config) {
	applyBoolEnv("BASYX_HISTORY_EVIDENCE_ENABLED", func(value bool) { cfg.History.Evidence.Enabled = value })
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_PROVIDER"); ok {
		cfg.History.Evidence.Provider = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_BUCKET"); ok {
		cfg.History.Evidence.Bucket = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_PREFIX"); ok {
		cfg.History.Evidence.Prefix = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_REGION"); ok {
		cfg.History.Evidence.Region = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_ENDPOINT"); ok {
		cfg.History.Evidence.Endpoint = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_ACCESS_KEY_ID"); ok {
		cfg.History.Evidence.AccessKeyID = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_SECRET_ACCESS_KEY"); ok {
		cfg.History.Evidence.SecretAccessKey = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_SECRET_KEY"); ok {
		cfg.History.Evidence.SecretAccessKey = value
	}
	applyBoolEnv("BASYX_HISTORY_EVIDENCE_PATH_STYLE", func(value bool) { cfg.History.Evidence.UsePathStyle = value })
	applyBoolEnv("BASYX_HISTORY_EVIDENCE_USE_PATH_STYLE", func(value bool) { cfg.History.Evidence.UsePathStyle = value })
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_RETENTION_MODE"); ok {
		cfg.History.Evidence.RetentionMode = value
	}
	applyIntEnv("BASYX_HISTORY_EVIDENCE_RETENTION_DAYS", func(value int) { cfg.History.Evidence.RetentionDays = value })
	applyIntEnv("BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS", func(value int) { cfg.History.Evidence.WriteTimeoutSec = value })
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_SIGNING_PRIVATE_KEY_PATH"); ok {
		cfg.History.Evidence.Signing.PrivateKeyPath = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_HISTORY_EVIDENCE_SIGNING_PUBLIC_KEY_PATH"); ok {
		cfg.History.Evidence.Signing.PublicKeyPath = value
	}
	applyBoolEnv("BASYX_HISTORY_EVIDENCE_SIGNING_REQUIRED", func(value bool) { cfg.History.Evidence.Signing.Required = value })
}

func applyEventingEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if value, ok := lookupTrimmedEnv("BASYX_EVENTING_ENABLED"); ok {
		cfg.Eventing.Enabled = strings.EqualFold(value, "true")
	}
	if value, ok := lookupTrimmedEnv("BASYX_EVENTING_FORMAT"); ok {
		cfg.Eventing.Format = value
	}
	if value, ok := lookupTrimmedEnv("BASYX_EVENTING_SINKS"); ok {
		cfg.Eventing.Sinks = parseCommaSeparated(value)
	}
	if value, ok := lookupTrimmedEnv("BASYX_EVENTING_OUTBOX_ENABLED"); ok {
		cfg.Eventing.OutboxEnabled = strings.EqualFold(value, "true")
	}
	if value, ok := lookupTrimmedEnv("BASYX_EVENTING_TOPIC_PREFIX"); ok {
		cfg.Eventing.TopicPrefix = value
	}
}

func validateHistoryAndEventingConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-HISTORY-NIL configuration must not be nil")
	}

	if err := validateHistoryConfig(cfg); err != nil {
		return err
	}
	return validateEventingConfig(cfg.Eventing)
}

func validateHistoryConfig(cfg *Config) error {
	switch strings.ToLower(strings.TrimSpace(cfg.History.Mode)) {
	case "off", "api", "audit":
	default:
		return fmt.Errorf("CONFIG-HISTORY-MODE unsupported history.mode %q", cfg.History.Mode)
	}
	if cfg.History.RetentionDays != 0 {
		return fmt.Errorf("CONFIG-HISTORY-RETENTION history.retentionDays is not implemented yet; use 0")
	}
	if cfg.History.FullSnapshotInterval < 1 {
		return fmt.Errorf("CONFIG-HISTORY-SNAPSHOTINTERVAL history.fullSnapshotInterval must be at least 1")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.History.Immutability)) {
	case "none", "postgres_guarded":
	case "external_anchor":
		if normalizeProvider(cfg.History.IntegrityAnchor.Provider) == "none" {
			return fmt.Errorf("CONFIG-HISTORY-ANCHOR history.immutability external_anchor requires a configured history.integrityAnchor.provider")
		}
	default:
		return fmt.Errorf("CONFIG-HISTORY-IMMUTABILITY unsupported history.immutability %q", cfg.History.Immutability)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.History.AuditIdentityMode)) {
	case "none":
	case "minimal", "extended":
	default:
		return fmt.Errorf("CONFIG-HISTORY-AUDITIDENTITY unsupported history.auditIdentityMode %q", cfg.History.AuditIdentityMode)
	}
	if err := validateHistoryEvidenceConfig(cfg); err != nil {
		return err
	}
	return validateIntegrityAnchorConfig(cfg.History.IntegrityAnchor)
}

func validateHistoryEvidenceConfig(cfg *Config) error {
	evidence := cfg.History.Evidence
	provider := normalizeProvider(evidence.Provider)
	switch provider {
	case "none", "s3":
	default:
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-PROVIDER unsupported history.evidence.provider %q", evidence.Provider)
	}
	if !evidence.Enabled {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(cfg.History.Mode), "off") {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-MODE history.evidence.enabled requires history.mode api or audit")
	}
	if provider == "none" {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-PROVIDER history.evidence.enabled requires history.evidence.provider")
	}
	if provider != "s3" {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-PROVIDER history.evidence.provider %q is not implemented", evidence.Provider)
	}
	if strings.TrimSpace(evidence.Bucket) == "" {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-BUCKET history.evidence.bucket is required for S3 evidence")
	}
	if strings.TrimSpace(evidence.Region) == "" {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-REGION history.evidence.region is required for S3 evidence")
	}
	if (strings.TrimSpace(evidence.AccessKeyID) == "") != (strings.TrimSpace(evidence.SecretAccessKey) == "") {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-CREDENTIALS history.evidence accessKeyId and secretAccessKey must be configured together")
	}
	if evidence.RetentionDays < 0 {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-RETENTIONDAYS history.evidence.retentionDays must not be negative")
	}
	retentionMode := strings.ToLower(strings.TrimSpace(evidence.RetentionMode))
	if retentionMode == "" {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-RETENTIONMODE history.evidence.retentionMode is required when evidence is enabled")
	}
	switch retentionMode {
	case "governance", "compliance":
	default:
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-RETENTIONMODE unsupported history.evidence.retentionMode %q", evidence.RetentionMode)
	}
	if evidence.RetentionDays < 1 {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-RETENTION history.evidence.retentionDays must be at least 1 when evidence is enabled")
	}
	if evidence.WriteTimeoutSec < 1 {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-TIMEOUT history.evidence.writeTimeoutSeconds must be at least 1")
	}
	if evidence.Signing.Required && effectiveHistoryEvidenceSigningKeyPath(cfg) == "" && strings.TrimSpace(evidence.Signing.PublicKeyPath) == "" {
		return fmt.Errorf("CONFIG-HISTORY-EVIDENCE-SIGNING history.evidence.signing.required needs history.evidence.signing.privateKeyPath, jws.privateKeyPath, or history.evidence.signing.publicKeyPath")
	}
	return nil
}

func validateIntegrityAnchorConfig(cfg HistoryIntegrityAnchorConfig) error {
	switch normalizeProvider(cfg.Provider) {
	case "none":
		return nil
	default:
		return fmt.Errorf("CONFIG-HISTORY-INTEGRITYANCHOR-NOTIMPLEMENTED history.integrityAnchor.provider %q is reserved for a future backend", cfg.Provider)
	}
}

func validateEventingConfig(cfg EventingConfig) error {
	if cfg.Enabled || cfg.OutboxEnabled || len(cfg.Sinks) > 0 {
		return fmt.Errorf("CONFIG-EVENTING-NOTIMPLEMENTED eventing publishing and outbox processing are not implemented yet")
	}
	return nil
}

func normalizeProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return "none"
	}
	return normalized
}

func effectiveHistoryEvidenceSigningKeyPath(cfg *Config) string {
	if cfg == nil {
		return ""
	}
	keyPath := strings.TrimSpace(cfg.History.Evidence.Signing.PrivateKeyPath)
	if keyPath != "" {
		return keyPath
	}
	return strings.TrimSpace(cfg.JWS.PrivateKeyPath)
}

func lookupTrimmedEnv(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func lookupFirstTrimmedEnv(keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := lookupTrimmedEnv(key); ok {
			return value, true
		}
	}
	return "", false
}

func applyBoolEnv(key string, assign func(bool)) {
	value, ok := lookupTrimmedEnv(key)
	if !ok {
		return
	}
	assign(strings.EqualFold(value, "true"))
}

func applyFirstBoolEnv(assign func(bool), keys ...string) {
	value, ok := lookupFirstTrimmedEnv(keys...)
	if !ok {
		return
	}
	assign(strings.EqualFold(value, "true"))
}

func applyIntEnv(key string, assign func(int)) {
	value, ok := lookupTrimmedEnv(key)
	if !ok {
		return
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
		assign(parsed)
	}
}

func applyFirstIntEnv(assign func(int), keys ...string) {
	value, ok := lookupFirstTrimmedEnv(keys...)
	if !ok {
		return
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
		assign(parsed)
	}
}

func parseCommaSeparated(rawValue string) []string {
	parts := strings.Split(rawValue, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func applyAASPreconfigPathOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if envRawPaths, exists := os.LookupEnv("GENERAL_AAS_PRECONFIG_PATHS"); exists {
		cfg.General.AASPreconfigPaths = parseAASPreconfigPathList(envRawPaths)
		return
	}

	cfg.General.AASPreconfigPaths = normalizeAASPreconfigPaths(cfg.General.AASPreconfigPaths)
}

func parseAASPreconfigPathList(rawPaths string) []string {
	if strings.TrimSpace(rawPaths) == "" {
		return []string{}
	}

	parts := strings.Split(rawPaths, ",")
	return normalizeAASPreconfigPaths(parts)
}

func normalizeAASPreconfigPaths(rawPaths []string) []string {
	if len(rawPaths) == 0 {
		return []string{}
	}

	normalized := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		normalized = append(normalized, path)
	}

	return normalized
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
//   - Server: Port 5004, no context path, caching disabled, bounded HTTP timeouts
//   - Database: Local PostgreSQL on port 5432 with test credentials
//   - CORS: Permissive policy allowing all origins and common methods
//   - OIDC: Local Keycloak realm configuration
//   - ABAC: Disabled by default
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", DefaultConfig.ServerHost)
	v.SetDefault("server.port", DefaultConfig.ServerPort)
	v.SetDefault("server.contextPath", "")
	v.SetDefault("server.cacheEnabled", false)
	v.SetDefault("server.strictVerification", DefaultConfig.ServerStrictVerification)
	v.SetDefault("server.verificationEndpointAvailable", DefaultConfig.ServerVerificationEndpointAvailable)
	v.SetDefault("server.readHeaderTimeoutSeconds", DefaultConfig.ServerReadHeaderTimeoutSeconds)
	v.SetDefault("server.readTimeoutSeconds", DefaultConfig.ServerReadTimeoutSeconds)
	v.SetDefault("server.writeTimeoutSeconds", DefaultConfig.ServerWriteTimeoutSeconds)
	v.SetDefault("server.idleTimeoutSeconds", DefaultConfig.ServerIdleTimeoutSeconds)
	v.SetDefault("server.shutdownTimeoutSeconds", DefaultConfig.ServerShutdownTimeoutSeconds)

	// PostgreSQL defaults
	v.SetDefault("postgres.host", "db")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.user", "admin")
	v.SetDefault("postgres.password", "admin123")
	v.SetDefault("postgres.dbname", "basyxTestDB")
	v.SetDefault("postgres.dsn", "")
	v.SetDefault("postgres.sslmode", DefaultConfig.PgSSLMode)
	v.SetDefault("postgres.sslcert", "")
	v.SetDefault("postgres.sslkey", "")
	v.SetDefault("postgres.sslrootcert", "")
	v.SetDefault("postgres.connectTimeoutSeconds", 0)
	v.SetDefault("postgres.applicationName", "")
	v.SetDefault("postgres.fallbackApplicationName", "")
	v.SetDefault("postgres.searchPath", "")
	v.SetDefault("postgres.options", "")
	v.SetDefault("postgres.timezone", "")
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
	v.SetDefault("abac.policyFileImport", DefaultConfig.ABACPolicyFileImport)
	v.SetDefault("abac.policyScope", DefaultConfig.ABACPolicyScope)
	v.SetDefault("abac.managementApi.enabled", DefaultConfig.ABACManagementAPIEnabled)

	// JWS defaults
	v.SetDefault("jws.privateKeyPath", "")
	v.SetDefault("jws.certificateChainPath", "")

	// History/audit defaults
	v.SetDefault("history.mode", "off")
	v.SetDefault("history.retentionDays", 0)
	v.SetDefault("history.fullSnapshotInterval", DefaultConfig.HistoryConfigFullSnapshotInterval)
	v.SetDefault("history.immutability", "none")
	v.SetDefault("history.auditIdentityMode", "none")
	v.SetDefault("history.evidence.enabled", DefaultConfig.HistoryEvidenceEnabled)
	v.SetDefault("history.evidence.provider", DefaultConfig.HistoryEvidenceProvider)
	v.SetDefault("history.evidence.bucket", DefaultConfig.HistoryEvidenceBucket)
	v.SetDefault("history.evidence.prefix", DefaultConfig.HistoryEvidencePrefix)
	v.SetDefault("history.evidence.region", DefaultConfig.HistoryEvidenceRegion)
	v.SetDefault("history.evidence.endpoint", DefaultConfig.HistoryEvidenceEndpoint)
	v.SetDefault("history.evidence.accessKeyId", DefaultConfig.HistoryEvidenceAccessKeyID)
	v.SetDefault("history.evidence.secretAccessKey", DefaultConfig.HistoryEvidenceSecretAccessKey)
	v.SetDefault("history.evidence.pathStyle", DefaultConfig.HistoryEvidenceUsePathStyle)
	v.SetDefault("history.evidence.retentionMode", DefaultConfig.HistoryEvidenceRetentionMode)
	v.SetDefault("history.evidence.retentionDays", DefaultConfig.HistoryEvidenceRetentionDays)
	v.SetDefault("history.evidence.writeTimeoutSeconds", DefaultConfig.HistoryEvidenceWriteTimeoutSeconds)
	v.SetDefault("history.evidence.signing.privateKeyPath", DefaultConfig.HistoryEvidenceSigningPrivateKey)
	v.SetDefault("history.evidence.signing.publicKeyPath", DefaultConfig.HistoryEvidenceSigningPublicKey)
	v.SetDefault("history.evidence.signing.required", DefaultConfig.HistoryEvidenceSigningRequired)
	v.SetDefault("history.integrityAnchor.provider", DefaultConfig.HistoryIntegrityAnchorProvider)

	// Eventing placeholders
	v.SetDefault("eventing.enabled", false)
	v.SetDefault("eventing.format", "cloudevents")
	v.SetDefault("eventing.sinks", []string{})
	v.SetDefault("eventing.outboxEnabled", false)
	v.SetDefault("eventing.topicPrefix", "basyx")

	// Swagger defaults
	v.SetDefault("swagger.enabled", DefaultConfig.SwaggerEnabled)
	v.SetDefault("swagger.contactName", "Eclipse BaSyx")
	v.SetDefault("swagger.contactEmail", "basyx-dev@eclipse.org")
	v.SetDefault("swagger.contactUrl", "https://basyx.org")

	// General defaults
	v.SetDefault("general.enableImplicitCasts", true)
	v.SetDefault("general.enableDescriptorDebug", false)
	v.SetDefault("general.discoveryIntegration", false)
	v.SetDefault("general.enableCustomMiddlewareHeaderInjection", false)
	v.SetDefault("general.supportsSingularSupplementalSemanticId", false)
	v.SetDefault("general.aasRegistryIntegration", false)
	v.SetDefault("general.submodelRegistryIntegration", false)
	v.SetDefault("general.externalUrl", "")
	v.SetDefault("general.trustProxyHeaders", DefaultConfig.GeneralTrustProxyHeaders)
	v.SetDefault("general.trustedProxyCIDRs", DefaultConfig.GeneralTrustedProxyCIDRs)
	v.SetDefault("general.uploadMaxSizeBytes", int64(128<<20))
	v.SetDefault("general.aasPreconfigPaths", []string{})
	v.SetDefault("general.bulkBatchLimit", DefaultConfig.GeneralBulkBatchLimit)

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
	add("Host", cfg.Server.Host, DefaultConfig.ServerHost)
	add("Port", cfg.Server.Port, DefaultConfig.ServerPort)
	add("Context Path", cfg.Server.ContextPath, DefaultConfig.ServerContextPath)
	add("Cache Enabled", cfg.Server.CacheEnabled, DefaultConfig.ServerCacheEnabled)
	add("Verification Mode", cfg.Server.StrictVerification, DefaultConfig.ServerStrictVerification)
	add("Verification Endpoint Available", cfg.Server.VerificationEndpointAvailable, DefaultConfig.ServerVerificationEndpointAvailable)
	add("Read Header Timeout (s)", cfg.Server.ReadHeaderTimeoutSeconds, DefaultConfig.ServerReadHeaderTimeoutSeconds)
	add("Read Timeout (s)", cfg.Server.ReadTimeoutSeconds, DefaultConfig.ServerReadTimeoutSeconds)
	add("Write Timeout (s)", cfg.Server.WriteTimeoutSeconds, DefaultConfig.ServerWriteTimeoutSeconds)
	add("Idle Timeout (s)", cfg.Server.IdleTimeoutSeconds, DefaultConfig.ServerIdleTimeoutSeconds)
	add("Shutdown Timeout (s)", cfg.Server.ShutdownTimeoutSeconds, DefaultConfig.ServerShutdownTimeoutSeconds)

	lines = append(lines, divider)

	// Postgres
	lines = append(lines, "🔹 Postgres:")
	add("Port", cfg.Postgres.Port, DefaultConfig.PgPort)
	add("DB Name", cfg.Postgres.DBName, DefaultConfig.PgDBName)
	add("SSL Mode", cfg.Postgres.SSLMode, DefaultConfig.PgSSLMode)
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
		add("Policy File Import", cfg.ABAC.PolicyFileImport, DefaultConfig.ABACPolicyFileImport)
		add("Policy Scope", cfg.ABAC.PolicyScope, DefaultConfig.ABACPolicyScope)
		add("Management API Enabled", cfg.ABAC.ManagementAPI.Enabled, DefaultConfig.ABACManagementAPIEnabled)

		lines = append(lines, "🔹 OIDC:")
		add("Trustlist Path", cfg.OIDC.TrustlistPath, DefaultConfig.OIDCTrustlistPath)
	}

	lines = append(lines, divider)

	// General
	lines = append(lines, "General:")
	add("Bulk Batch Limit", cfg.General.BulkBatchLimit, DefaultConfig.GeneralBulkBatchLimit)

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
	if cfg.JWS.CertificateChainPath != "" {
		lines = append(lines, fmt.Sprintf("  Certificate Chain Path: %s", cfg.JWS.CertificateChainPath))
		if _, err := os.Stat(cfg.JWS.CertificateChainPath); err == nil {
			lines = append(lines, "  Certificate Chain Mounted: true ✅")
		} else {
			lines = append(lines, "  Certificate Chain Mounted: false ❌")
		}
	}

	lines = append(lines, divider)

	lines = append(lines, "🔹 Swagger:")
	add("Enabled", cfg.Swagger.Enabled, DefaultConfig.SwaggerEnabled)

	lines = append(lines, divider)

	// History
	lines = append(lines, "🔹 History/Audit:")
	add("Mode", cfg.History.Mode, DefaultConfig.HistoryConfigMode)
	add("Retention Days", cfg.History.RetentionDays, DefaultConfig.HistoryConfigRetentionDays)
	add("Full Snapshot Interval", cfg.History.FullSnapshotInterval, DefaultConfig.HistoryConfigFullSnapshotInterval)
	add("Immutability", cfg.History.Immutability, DefaultConfig.HistoryConfigImmutability)
	add("Audit Identity Mode", cfg.History.AuditIdentityMode, DefaultConfig.HistoryConfigAuditIdentityMode)
	add("Evidence Enabled", cfg.History.Evidence.Enabled, DefaultConfig.HistoryEvidenceEnabled)
	add("Evidence Provider", cfg.History.Evidence.Provider, DefaultConfig.HistoryEvidenceProvider)
	if cfg.History.Evidence.Enabled {
		add("Evidence Bucket", cfg.History.Evidence.Bucket, DefaultConfig.HistoryEvidenceBucket)
		add("Evidence Prefix", cfg.History.Evidence.Prefix, DefaultConfig.HistoryEvidencePrefix)
		add("Evidence Region", cfg.History.Evidence.Region, DefaultConfig.HistoryEvidenceRegion)
		add("Evidence Endpoint", cfg.History.Evidence.Endpoint, DefaultConfig.HistoryEvidenceEndpoint)
		add("Evidence Path Style", cfg.History.Evidence.UsePathStyle, DefaultConfig.HistoryEvidenceUsePathStyle)
		add("Evidence Retention Mode", cfg.History.Evidence.RetentionMode, DefaultConfig.HistoryEvidenceRetentionMode)
		add("Evidence Retention Days", cfg.History.Evidence.RetentionDays, DefaultConfig.HistoryEvidenceRetentionDays)
		add("Evidence Write Timeout Seconds", cfg.History.Evidence.WriteTimeoutSec, DefaultConfig.HistoryEvidenceWriteTimeoutSeconds)
		add("Evidence Signing Public Key Path", cfg.History.Evidence.Signing.PublicKeyPath, DefaultConfig.HistoryEvidenceSigningPublicKey)
		add("Evidence Signing Required", cfg.History.Evidence.Signing.Required, DefaultConfig.HistoryEvidenceSigningRequired)
	}
	add("Integrity Anchor Provider", cfg.History.IntegrityAnchor.Provider, DefaultConfig.HistoryIntegrityAnchorProvider)

	// Eventing
	lines = append(lines, "🔹 Eventing:")
	add("Enabled", cfg.Eventing.Enabled, DefaultConfig.EventingEnabled)
	if cfg.Eventing.Enabled {
		add("Format", cfg.Eventing.Format, DefaultConfig.EventingFormat)
		add("Sinks", cfg.Eventing.Sinks, DefaultConfig.EventingSinks)
		add("Outbox Enabled", cfg.Eventing.OutboxEnabled, DefaultConfig.EventingOutboxEnabled)
		add("Topic Prefix", cfg.Eventing.TopicPrefix, DefaultConfig.EventingTopicPrefix)
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
