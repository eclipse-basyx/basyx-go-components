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

package common

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultReBACTimeoutSeconds = 5
	maxReBACTimeoutSeconds     = 60
	maxReBACIdentifierLength   = 255
)

// ReBACConfig configures OpenFGA-backed Relationship-Based Access Control.
type ReBACConfig struct {
	Enabled            bool                           `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	URL                string                         `mapstructure:"url" yaml:"url" json:"url"`
	StoreID            string                         `mapstructure:"storeId" yaml:"storeId" json:"storeId"`
	ModelID            string                         `mapstructure:"modelId" yaml:"modelId" json:"modelId"`
	Scope              string                         `mapstructure:"scope" yaml:"scope" json:"scope"`
	Token              string                         `mapstructure:"token" yaml:"token" json:"-"`
	TimeoutSeconds     int                            `mapstructure:"timeoutSeconds" yaml:"timeoutSeconds" json:"timeoutSeconds"`
	Administrators     []ReBACAdministratorConfig     `mapstructure:"administrators" yaml:"administrators" json:"administrators"`
	GroupClaimMappings []ReBACGroupClaimMappingConfig `mapstructure:"groupClaimMappings" yaml:"groupClaimMappings" json:"groupClaimMappings"`
}

// ReBACAdministratorConfig identifies an issuer-scoped ReBAC administrator.
type ReBACAdministratorConfig struct {
	Type    string `mapstructure:"type" yaml:"type" json:"type"`
	Issuer  string `mapstructure:"issuer" yaml:"issuer" json:"issuer"`
	Subject string `mapstructure:"subject" yaml:"subject" json:"subject"`
}

// ReBACGroupClaimMappingConfig maps one issuer's group claim to OpenFGA groups.
type ReBACGroupClaimMappingConfig struct {
	Issuer string `mapstructure:"issuer" yaml:"issuer" json:"issuer"`
	Claim  string `mapstructure:"claim" yaml:"claim" json:"claim"`
}

func applyReBACEnvOverrides(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-REBAC-ENV-NIL configuration must not be nil")
	}
	applyBoolEnv("REBAC_ENABLED", func(value bool) { cfg.ReBAC.Enabled = value })
	applyReBACStringEnv("REBAC_URL", &cfg.ReBAC.URL)
	applyReBACStringEnv("REBAC_STORE_ID", &cfg.ReBAC.StoreID)
	applyReBACStringEnv("REBAC_MODEL_ID", &cfg.ReBAC.ModelID)
	applyReBACStringEnv("REBAC_SCOPE", &cfg.ReBAC.Scope)
	if err := applyReBACTokenEnv(&cfg.ReBAC); err != nil {
		return err
	}
	if err := applyReBACTimeoutEnv(&cfg.ReBAC); err != nil {
		return err
	}
	if err := applyReBACAdministratorsEnv(&cfg.ReBAC); err != nil {
		return err
	}
	return applyReBACGroupClaimMappingsEnv(&cfg.ReBAC)
}

func applyReBACTokenEnv(cfg *ReBACConfig) error {
	value, ok := os.LookupEnv("REBAC_TOKEN")
	if !ok {
		return nil
	}
	cfg.Token = strings.TrimSpace(value)
	if cfg.Token == "" {
		return fmt.Errorf("CONFIG-REBAC-ENV-TOKEN REBAC_TOKEN must not be empty or whitespace")
	}
	return nil
}

func applyReBACStringEnv(key string, destination *string) {
	if value, ok := lookupTrimmedEnv(key); ok {
		*destination = value
	}
}

func applyReBACTimeoutEnv(cfg *ReBACConfig) error {
	value, ok := lookupTrimmedEnv("REBAC_TIMEOUT_SECONDS")
	if !ok {
		return nil
	}
	timeout, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("CONFIG-REBAC-ENV-TIMEOUT REBAC_TIMEOUT_SECONDS must be an integer: %w", err)
	}
	cfg.TimeoutSeconds = timeout
	return nil
}

func applyReBACAdministratorsEnv(cfg *ReBACConfig) error {
	value, ok := lookupTrimmedEnv("REBAC_ADMINISTRATORS")
	if !ok {
		return nil
	}
	if err := json.Unmarshal([]byte(value), &cfg.Administrators); err != nil {
		return fmt.Errorf("CONFIG-REBAC-ENV-ADMINISTRATORS REBAC_ADMINISTRATORS must be a JSON array: %w", err)
	}
	return nil
}

func applyReBACGroupClaimMappingsEnv(cfg *ReBACConfig) error {
	value, ok := lookupTrimmedEnv("REBAC_GROUP_CLAIM_MAPPINGS")
	if !ok {
		return nil
	}
	if err := json.Unmarshal([]byte(value), &cfg.GroupClaimMappings); err != nil {
		return fmt.Errorf("CONFIG-REBAC-ENV-GROUPCLAIMS REBAC_GROUP_CLAIM_MAPPINGS must be a JSON array: %w", err)
	}
	return nil
}

func validateReBACConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-REBAC-NIL configuration must not be nil")
	}
	if !cfg.ReBAC.Enabled {
		return nil
	}
	if !cfg.ABAC.Enabled {
		return fmt.Errorf("CONFIG-REBAC-ABAC rebac.enabled requires abac.enabled")
	}
	if err := normalizeReBACConnection(&cfg.ReBAC); err != nil {
		return err
	}
	if err := validateReBACAdministrators(cfg.ReBAC.Administrators); err != nil {
		return err
	}
	return validateReBACGroupClaimMappings(cfg.ReBAC.GroupClaimMappings)
}

func normalizeReBACConnection(cfg *ReBACConfig) error {
	normalizedURL, err := normalizeReBACURL(cfg.URL)
	if err != nil {
		return err
	}
	cfg.URL = normalizedURL
	for _, field := range []struct {
		name        string
		destination *string
	}{
		{name: "storeId", destination: &cfg.StoreID},
		{name: "modelId", destination: &cfg.ModelID},
		{name: "scope", destination: &cfg.Scope},
	} {
		value, normalizeErr := normalizeReBACIdentifier(*field.destination, field.name)
		if normalizeErr != nil {
			return normalizeErr
		}
		*field.destination = value
	}
	if cfg.TimeoutSeconds < 1 || cfg.TimeoutSeconds > maxReBACTimeoutSeconds {
		return fmt.Errorf("CONFIG-REBAC-TIMEOUT rebac.timeoutSeconds must be between 1 and %d", maxReBACTimeoutSeconds)
	}
	if strings.TrimSpace(cfg.Token) != cfg.Token {
		return fmt.Errorf("CONFIG-REBAC-TOKEN rebac.token must not start or end with whitespace")
	}
	return nil
}

func normalizeReBACURL(rawURL string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if value == "" {
		return "", fmt.Errorf("CONFIG-REBAC-URL rebac.url is required when rebac.enabled")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("CONFIG-REBAC-URL rebac.url must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return value, nil
}

func normalizeReBACIdentifier(rawValue, field string) (string, error) {
	value := strings.TrimSpace(rawValue)
	code := strings.ToUpper(strings.ReplaceAll(field, ".", "-"))
	if value == "" {
		return "", fmt.Errorf("CONFIG-REBAC-%s rebac.%s is required when rebac.enabled", code, field)
	}
	if len(value) > maxReBACIdentifierLength || strings.ContainsAny(value, "\r\n\t") {
		return "", fmt.Errorf("CONFIG-REBAC-%s rebac.%s must contain at most %d printable characters", code, field, maxReBACIdentifierLength)
	}
	return value, nil
}

func validateReBACAdministrators(administrators []ReBACAdministratorConfig) error {
	if len(administrators) == 0 {
		return fmt.Errorf("CONFIG-REBAC-ADMINISTRATORS rebac.administrators requires at least one issuer-scoped administrator when rebac.enabled")
	}
	seen := make(map[string]struct{}, len(administrators))
	for index := range administrators {
		administrator, err := normalizeReBACAdministrator(administrators[index])
		if err != nil {
			return fmt.Errorf("CONFIG-REBAC-ADMINISTRATORS administrator %d: %w", index, err)
		}
		administrators[index] = administrator
		key := administrator.Type + "\x00" + administrator.Issuer + "\x00" + administrator.Subject
		if _, exists := seen[key]; exists {
			return fmt.Errorf("CONFIG-REBAC-ADMINISTRATORS duplicate administrator")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func normalizeReBACAdministrator(administrator ReBACAdministratorConfig) (ReBACAdministratorConfig, error) {
	administrator.Type = strings.ToLower(strings.TrimSpace(administrator.Type))
	if administrator.Type != "user" && administrator.Type != "group" {
		return ReBACAdministratorConfig{}, fmt.Errorf("CONFIG-REBAC-ADMIN-TYPE type must be user or group")
	}
	issuer, err := normalizeReBACIdentifier(administrator.Issuer, "administrators.issuer")
	if err != nil {
		return ReBACAdministratorConfig{}, err
	}
	subject, err := normalizeReBACIdentifier(administrator.Subject, "administrators.subject")
	if err != nil {
		return ReBACAdministratorConfig{}, err
	}
	administrator.Issuer, administrator.Subject = issuer, subject
	return administrator, nil
}

func validateReBACGroupClaimMappings(mappings []ReBACGroupClaimMappingConfig) error {
	seenIssuers := make(map[string]struct{}, len(mappings))
	for index := range mappings {
		issuer, err := normalizeReBACIdentifier(mappings[index].Issuer, "groupClaimMappings.issuer")
		if err != nil {
			return fmt.Errorf("CONFIG-REBAC-GROUPCLAIMS mapping %d: %w", index, err)
		}
		claim, err := normalizeReBACIdentifier(mappings[index].Claim, "groupClaimMappings.claim")
		if err != nil {
			return fmt.Errorf("CONFIG-REBAC-GROUPCLAIMS mapping %d: %w", index, err)
		}
		if _, exists := seenIssuers[issuer]; exists {
			return fmt.Errorf("CONFIG-REBAC-GROUPCLAIMS duplicate issuer mapping")
		}
		mappings[index].Issuer, mappings[index].Claim = issuer, claim
		seenIssuers[issuer] = struct{}{}
	}
	return nil
}
