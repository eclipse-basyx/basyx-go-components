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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package common

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

// ReBAC OpenFGA credential methods.
const (
	ReBACCredentialsNone              = "none"
	ReBACCredentialsAPIToken          = "apiToken"
	ReBACCredentialsClientCredentials = "clientCredentials"
)

// ReBAC OpenFGA consistency preferences.
const (
	ReBACConsistencyHigher          = "HIGHER_CONSISTENCY"
	ReBACConsistencyMinimizeLatency = "MINIMIZE_LATENCY"
)

const (
	defaultReBACScope                 = "default"
	defaultReBACTimeoutMillis         = 2000
	defaultReBACBatchCheckMaxItems    = 50
	maxReBACBatchCheckItems           = 50
	defaultReBACListObjectsMaxResults = 1000
	defaultReBACMaxScanCandidates     = 5000
	defaultReBACGroupClaim            = "groups"
	maxReBACScopeLength               = 64
)

// ReBACConfig configures the experimental relationship-based access control.
//
// ReBAC extends ABAC as a strict union: a request is allowed when ABAC or
// ReBAC allows it. The section covers authorization only; evidence storage and
// telemetry keep their existing configuration.
type ReBACConfig struct {
	Enabled               bool               `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Scope                 string             `mapstructure:"scope" yaml:"scope" json:"scope"`
	OpenFGA               ReBACOpenFGAConfig `mapstructure:"openfga" yaml:"openfga" json:"openfga"`
	ListObjectsMaxResults int                `mapstructure:"listObjectsMaxResults" yaml:"listObjectsMaxResults" json:"listObjectsMaxResults"`
	MaxScanCandidates     int                `mapstructure:"maxScanCandidates" yaml:"maxScanCandidates" json:"maxScanCandidates"`
	GroupClaim            string             `mapstructure:"groupClaim" yaml:"groupClaim" json:"groupClaim"`
	Administrators        []string           `mapstructure:"administrators" yaml:"administrators" json:"administrators"`
	ProvisionModel        bool               `mapstructure:"provisionModel" yaml:"provisionModel" json:"provisionModel"`
}

// ReBACOpenFGAConfig configures the OpenFGA endpoint BaSyx projects grants to.
//
// StoreID and AuthorizationModelID may be omitted; the service then uses the
// store and model recorded for the scope in the database. When configured they
// must match that record. The model is always pinned, never "latest".
type ReBACOpenFGAConfig struct {
	APIURL               string                        `mapstructure:"apiUrl" yaml:"apiUrl" json:"apiUrl"`
	StoreID              string                        `mapstructure:"storeId" yaml:"storeId" json:"storeId"`
	AuthorizationModelID string                        `mapstructure:"authorizationModelId" yaml:"authorizationModelId" json:"authorizationModelId"`
	Credentials          ReBACOpenFGACredentialsConfig `mapstructure:"credentials" yaml:"credentials" json:"credentials"`
	TimeoutMillis        int                           `mapstructure:"timeoutMillis" yaml:"timeoutMillis" json:"timeoutMillis"`
	Consistency          string                        `mapstructure:"consistency" yaml:"consistency" json:"consistency"`
	BatchCheckMaxItems   int                           `mapstructure:"batchCheckMaxItems" yaml:"batchCheckMaxItems" json:"batchCheckMaxItems"`
}

// ReBACOpenFGACredentialsConfig selects the OpenFGA SDK credential method.
type ReBACOpenFGACredentialsConfig struct {
	Method         string `mapstructure:"method" yaml:"method" json:"method"`
	APIToken       string `mapstructure:"apiToken" yaml:"apiToken" json:"-"`
	ClientID       string `mapstructure:"clientId" yaml:"clientId" json:"clientId"`
	ClientSecret   string `mapstructure:"clientSecret" yaml:"clientSecret" json:"-"`
	APITokenIssuer string `mapstructure:"apiTokenIssuer" yaml:"apiTokenIssuer" json:"apiTokenIssuer"`
	APIAudience    string `mapstructure:"apiAudience" yaml:"apiAudience" json:"apiAudience"`
}

// Required reports whether the OpenFGA connection settings are needed, either
// to authorize requests or to provision the model.
func (c ReBACConfig) Required() bool {
	return c.Enabled || c.ProvisionModel
}

func setReBACDefaults(v *viper.Viper) {
	v.SetDefault("rebac.enabled", false)
	v.SetDefault("rebac.scope", defaultReBACScope)
	v.SetDefault("rebac.openfga.apiUrl", "")
	v.SetDefault("rebac.openfga.storeId", "")
	v.SetDefault("rebac.openfga.authorizationModelId", "")
	v.SetDefault("rebac.openfga.credentials.method", ReBACCredentialsNone)
	v.SetDefault("rebac.openfga.credentials.apiToken", "")
	v.SetDefault("rebac.openfga.credentials.clientId", "")
	v.SetDefault("rebac.openfga.credentials.clientSecret", "")
	v.SetDefault("rebac.openfga.credentials.apiTokenIssuer", "")
	v.SetDefault("rebac.openfga.credentials.apiAudience", "")
	v.SetDefault("rebac.openfga.timeoutMillis", defaultReBACTimeoutMillis)
	v.SetDefault("rebac.openfga.consistency", ReBACConsistencyHigher)
	v.SetDefault("rebac.openfga.batchCheckMaxItems", defaultReBACBatchCheckMaxItems)
	v.SetDefault("rebac.listObjectsMaxResults", defaultReBACListObjectsMaxResults)
	v.SetDefault("rebac.maxScanCandidates", defaultReBACMaxScanCandidates)
	v.SetDefault("rebac.groupClaim", defaultReBACGroupClaim)
	v.SetDefault("rebac.administrators", []string{})
	v.SetDefault("rebac.provisionModel", false)
}

func applyReBACEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	rebac := &cfg.ReBAC
	applyFirstStringEnv(&rebac.OpenFGA.APIURL, "REBAC_OPENFGA_API_URL")
	applyFirstStringEnv(&rebac.OpenFGA.StoreID, "REBAC_OPENFGA_STORE_ID")
	applyFirstStringEnv(&rebac.OpenFGA.AuthorizationModelID, "REBAC_OPENFGA_AUTHORIZATION_MODEL_ID")
	applyFirstStringEnv(&rebac.OpenFGA.Credentials.Method, "REBAC_OPENFGA_CREDENTIALS_METHOD")
	applyFirstStringEnv(&rebac.OpenFGA.Credentials.APIToken, "REBAC_OPENFGA_CREDENTIALS_API_TOKEN")
	applyFirstStringEnv(&rebac.OpenFGA.Credentials.ClientID, "REBAC_OPENFGA_CREDENTIALS_CLIENT_ID")
	applyFirstStringEnv(&rebac.OpenFGA.Credentials.ClientSecret, "REBAC_OPENFGA_CREDENTIALS_CLIENT_SECRET")
	applyFirstStringEnv(&rebac.OpenFGA.Credentials.APITokenIssuer, "REBAC_OPENFGA_CREDENTIALS_API_TOKEN_ISSUER")
	applyFirstStringEnv(&rebac.OpenFGA.Credentials.APIAudience, "REBAC_OPENFGA_CREDENTIALS_API_AUDIENCE")
	applyFirstIntEnv(func(value int) { rebac.OpenFGA.TimeoutMillis = value }, "REBAC_OPENFGA_TIMEOUT_MILLIS")
	applyFirstIntEnv(func(value int) { rebac.OpenFGA.BatchCheckMaxItems = value }, "REBAC_OPENFGA_BATCH_CHECK_MAX_ITEMS")
	applyFirstIntEnv(func(value int) { rebac.ListObjectsMaxResults = value }, "REBAC_LIST_OBJECTS_MAX_RESULTS")
	applyFirstIntEnv(func(value int) { rebac.MaxScanCandidates = value }, "REBAC_MAX_SCAN_CANDIDATES")
	applyFirstStringEnv(&rebac.GroupClaim, "REBAC_GROUP_CLAIM")
	applyFirstBoolEnv(func(value bool) { rebac.ProvisionModel = value }, "REBAC_PROVISION_MODEL")
	if value, ok := lookupFirstTrimmedEnv("REBAC_ADMINISTRATORS"); ok {
		rebac.Administrators = parseCommaSeparated(value)
	}
}

func applyFirstStringEnv(target *string, keys ...string) {
	if value, ok := lookupFirstTrimmedEnv(keys...); ok {
		*target = value
	}
}

// validateReBACConfig normalizes the rebac section. Cross-feature
// requirements such as ABAC and OIDC are checked when a service enables
// ReBAC enforcement, because the configuration service only provisions.
func validateReBACConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-REBAC-NIL configuration must not be nil")
	}
	rebac := &cfg.ReBAC
	if !rebac.Required() {
		return nil
	}
	scope, err := normalizeReBACScope(rebac.Scope)
	if err != nil {
		return err
	}
	rebac.Scope = scope
	if err = validateReBACOpenFGA(&rebac.OpenFGA); err != nil {
		return err
	}
	if err = validateReBACLimits(rebac); err != nil {
		return err
	}
	rebac.GroupClaim = strings.TrimSpace(rebac.GroupClaim)
	if rebac.GroupClaim == "" {
		return fmt.Errorf("CONFIG-REBAC-GROUPCLAIM rebac.groupClaim must not be empty")
	}
	administrators, err := normalizeReBACAdministrators(rebac.Administrators)
	if err != nil {
		return err
	}
	rebac.Administrators = administrators
	return nil
}

func normalizeReBACScope(scope string) (string, error) {
	trimmed := strings.TrimSpace(scope)
	if trimmed == "" {
		return "", fmt.Errorf("CONFIG-REBAC-SCOPE rebac.scope must not be empty")
	}
	if len(trimmed) > maxReBACScopeLength {
		return "", fmt.Errorf("CONFIG-REBAC-SCOPE rebac.scope must not exceed %d characters", maxReBACScopeLength)
	}
	for _, char := range trimmed {
		if !isABACPolicyScopeChar(char) {
			return "", fmt.Errorf("CONFIG-REBAC-SCOPE rebac.scope contains unsupported character %q", char)
		}
	}
	return trimmed, nil
}

func validateReBACOpenFGA(openfga *ReBACOpenFGAConfig) error {
	openfga.APIURL = strings.TrimRight(strings.TrimSpace(openfga.APIURL), "/")
	parsed, err := url.Parse(openfga.APIURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("CONFIG-REBAC-APIURL rebac.openfga.apiUrl must be an absolute http(s) URL")
	}
	openfga.StoreID = strings.TrimSpace(openfga.StoreID)
	openfga.AuthorizationModelID = strings.TrimSpace(openfga.AuthorizationModelID)
	if strings.EqualFold(openfga.AuthorizationModelID, "latest") {
		return fmt.Errorf("CONFIG-REBAC-MODELID rebac.openfga.authorizationModelId must pin a model and must not be \"latest\"")
	}
	if openfga.TimeoutMillis <= 0 {
		return fmt.Errorf("CONFIG-REBAC-TIMEOUT rebac.openfga.timeoutMillis must be positive")
	}
	if openfga.BatchCheckMaxItems <= 0 || openfga.BatchCheckMaxItems > maxReBACBatchCheckItems {
		return fmt.Errorf("CONFIG-REBAC-BATCHCHECK rebac.openfga.batchCheckMaxItems must be between 1 and %d", maxReBACBatchCheckItems)
	}
	switch strings.ToUpper(strings.TrimSpace(openfga.Consistency)) {
	case ReBACConsistencyHigher, ReBACConsistencyMinimizeLatency:
		openfga.Consistency = strings.ToUpper(strings.TrimSpace(openfga.Consistency))
	default:
		return fmt.Errorf("CONFIG-REBAC-CONSISTENCY unsupported rebac.openfga.consistency %q", openfga.Consistency)
	}
	return validateReBACCredentials(&openfga.Credentials)
}

func validateReBACCredentials(credentials *ReBACOpenFGACredentialsConfig) error {
	credentials.Method = strings.TrimSpace(credentials.Method)
	switch credentials.Method {
	case "", ReBACCredentialsNone:
		credentials.Method = ReBACCredentialsNone
		return nil
	case ReBACCredentialsAPIToken:
		if strings.TrimSpace(credentials.APIToken) == "" {
			return fmt.Errorf("CONFIG-REBAC-CREDENTIALS rebac.openfga.credentials.apiToken is required for method apiToken")
		}
		return nil
	case ReBACCredentialsClientCredentials:
		if strings.TrimSpace(credentials.ClientID) == "" || strings.TrimSpace(credentials.ClientSecret) == "" ||
			strings.TrimSpace(credentials.APITokenIssuer) == "" {
			return fmt.Errorf("CONFIG-REBAC-CREDENTIALS clientId, clientSecret and apiTokenIssuer are required for method clientCredentials")
		}
		return nil
	default:
		return fmt.Errorf("CONFIG-REBAC-CREDENTIALS unsupported rebac.openfga.credentials.method %q", credentials.Method)
	}
}

func validateReBACLimits(rebac *ReBACConfig) error {
	if rebac.ListObjectsMaxResults <= 0 {
		return fmt.Errorf("CONFIG-REBAC-LISTOBJECTS rebac.listObjectsMaxResults must be positive")
	}
	if rebac.MaxScanCandidates <= 0 {
		return fmt.Errorf("CONFIG-REBAC-MAXSCAN rebac.maxScanCandidates must be positive")
	}
	return nil
}

// ReBACAdministrator is one configured bootstrap and recovery principal.
type ReBACAdministrator struct {
	Issuer  string
	Subject string
	Group   string
}

// ParseReBACAdministrator parses "issuer|subject" or "issuer|group:<name>".
func ParseReBACAdministrator(entry string) (ReBACAdministrator, error) {
	issuer, principal, found := strings.Cut(strings.TrimSpace(entry), "|")
	issuer = strings.TrimSpace(issuer)
	principal = strings.TrimSpace(principal)
	if !found || issuer == "" || principal == "" {
		return ReBACAdministrator{}, fmt.Errorf("CONFIG-REBAC-ADMINISTRATORS entry %q must be issuer|subject or issuer|group:<name>", entry)
	}
	if group, isGroup := strings.CutPrefix(principal, "group:"); isGroup {
		group = strings.TrimSpace(group)
		if group == "" {
			return ReBACAdministrator{}, fmt.Errorf("CONFIG-REBAC-ADMINISTRATORS entry %q has an empty group name", entry)
		}
		return ReBACAdministrator{Issuer: issuer, Group: group}, nil
	}
	return ReBACAdministrator{Issuer: issuer, Subject: principal}, nil
}

func normalizeReBACAdministrators(entries []string) ([]string, error) {
	normalized := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		admin, err := ParseReBACAdministrator(entry)
		if err != nil {
			return nil, err
		}
		if admin.Group != "" {
			normalized = append(normalized, admin.Issuer+"|group:"+admin.Group)
			continue
		}
		normalized = append(normalized, admin.Issuer+"|"+admin.Subject)
	}
	return normalized, nil
}
