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

// Package abacpolicy provides a PostgreSQL-backed ABAC policy repository and
// management API for BaSyx services.
package abacpolicy

import (
	"database/sql"
	"encoding/json"
	"time"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const (
	// StatusStaged marks a validated or editable policy version that is not active.
	StatusStaged = "staged"
	// StatusActive marks the only policy version used by the evaluator for a service scope.
	StatusActive = "active"
	// StatusSuperseded marks a version that was active before a newer activation.
	StatusSuperseded = "superseded"
	// StatusRejected marks a staged version that must not be activated.
	StatusRejected = "rejected"

	// SourceTypeFile records policy versions imported from abac.modelPath.
	SourceTypeFile = "file"
	// SourceTypeAPI records policy versions created through the management API.
	SourceTypeAPI = "api"

	tablePolicyVersions = "abac_policy_versions"
	tablePolicyRules    = "abac_policy_rules"
	tablePolicyEvents   = "abac_policy_events"

	managementBasePath = "/security/abac/policy-versions"
)

// PolicyVersion describes one durable ABAC policy version.
//
// The configured and materialized JSON fields are canonical documents stored in
// PostgreSQL. Active and superseded versions are immutable; staged versions are
// the only versions that draft rule operations may change.
type PolicyVersion struct {
	VersionID              int64           `json:"version_id"`
	ServiceScope           string          `json:"service_scope"`
	PolicyID               string          `json:"policy_id"`
	Status                 string          `json:"status"`
	SourceType             string          `json:"source_type"`
	SourceRef              string          `json:"source_ref,omitempty"`
	ConfiguredPolicyJSON   json.RawMessage `json:"configured_policy_json,omitempty"`
	ConfiguredPolicyHash   string          `json:"configured_policy_hash"`
	RawPolicyHash          string          `json:"raw_policy_hash,omitempty"`
	MaterializedPolicyJSON json.RawMessage `json:"materialized_policy_json,omitempty"`
	MaterializedPolicyHash string          `json:"materialized_policy_hash"`
	CreatedAt              time.Time       `json:"created_at"`
	CreatedBySubject       string          `json:"created_by_subject,omitempty"`
	CreatedByIssuer        string          `json:"created_by_issuer,omitempty"`
	CreatedByClientID      string          `json:"created_by_client_id,omitempty"`
	UpdatedAt              *time.Time      `json:"updated_at,omitempty"`
	UpdatedBySubject       string          `json:"updated_by_subject,omitempty"`
	UpdatedByIssuer        string          `json:"updated_by_issuer,omitempty"`
	UpdatedByClientID      string          `json:"updated_by_client_id,omitempty"`
	ActivatedAt            *time.Time      `json:"activated_at,omitempty"`
	ActivatedBySubject     string          `json:"activated_by_subject,omitempty"`
	ActivatedByIssuer      string          `json:"activated_by_issuer,omitempty"`
	ActivatedByClientID    string          `json:"activated_by_client_id,omitempty"`
	SupersededAt           *time.Time      `json:"superseded_at,omitempty"`
	ArtifactRef            json.RawMessage `json:"artifact_ref,omitempty"`
}

// PolicyRule describes one ordered, materialized ABAC rule row.
//
// RuleIndex is the stable 1-based order used by the evaluator. MatchedRuleID is
// deterministic for the configured rule content and is copied into mutation
// history when this rule authorizes a request.
type PolicyRule struct {
	RuleID               int64           `json:"rule_id"`
	VersionID            int64           `json:"version_id"`
	PolicyID             string          `json:"policy_id"`
	ServiceScope         string          `json:"service_scope"`
	RuleIndex            int             `json:"rule_index"`
	MatchedRuleID        string          `json:"matched_rule_id"`
	ConfiguredRuleJSON   json.RawMessage `json:"configured_rule_json"`
	MaterializedRuleJSON json.RawMessage `json:"materialized_rule_json"`
	ACLJSON              json.RawMessage `json:"acl_json,omitempty"`
	AttributesJSON       json.RawMessage `json:"attributes_json,omitempty"`
	ObjectsJSON          json.RawMessage `json:"objects_json,omitempty"`
	FormulaJSON          json.RawMessage `json:"formula_json,omitempty"`
	FiltersJSON          json.RawMessage `json:"filters_json,omitempty"`
	Access               string          `json:"access"`
	Rights               []string        `json:"rights"`
	RuleHash             string          `json:"rule_hash"`
	MaterializedRuleHash string          `json:"materialized_rule_hash"`
	CreatedAt            time.Time       `json:"created_at"`
	CreatedBySubject     string          `json:"created_by_subject,omitempty"`
	CreatedByIssuer      string          `json:"created_by_issuer,omitempty"`
	CreatedByClientID    string          `json:"created_by_client_id,omitempty"`
}

// PolicyImportRequest imports configured ABAC policy JSON through the API.
//
// Policy must contain the same access-rule model accepted by abac.modelPath.
// Activate optionally promotes the newly created staged version after import.
type PolicyImportRequest struct {
	SourceRef string          `json:"source_ref,omitempty"`
	Policy    json.RawMessage `json:"policy"`
	Activate  bool            `json:"activate,omitempty"`
}

// RuleMutationRequest creates or replaces one configured draft rule.
//
// Position is optional and 1-based for create operations. Rule contains either
// a configured access-rule object or, for API convenience, is filled from a
// direct rule request body by the HTTP decoder.
type RuleMutationRequest struct {
	Position int             `json:"position,omitempty"`
	Rule     json.RawMessage `json:"rule"`
}

// MoveRuleRequest changes the stable order of one staged rule.
//
// Position is a required 1-based target index inside the staged rule list.
type MoveRuleRequest struct {
	Position int `json:"position"`
}

// SetRuleEnabledRequest toggles one staged rule.
//
// Disabling maps to ACL.ACCESS=DISABLED and enabling maps to ACL.ACCESS=ALLOW.
type SetRuleEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// ValidationResult reports whether a staged policy can be materialized.
//
// Valid=false is returned for user-correctable policy errors and is still
// audited. A non-nil Go error indicates an infrastructure or immutable-state
// problem that prevented validation from completing normally.
type ValidationResult struct {
	Valid                  bool   `json:"valid"`
	PolicyID               string `json:"policy_id,omitempty"`
	MaterializedPolicyHash string `json:"materialized_policy_hash,omitempty"`
	Error                  string `json:"error,omitempty"`
}

type activePolicy struct {
	version PolicyVersion
	rules   []PolicyRule
	model   *auth.AccessModel
}

type auditActor struct {
	Subject       string `json:"actor_subject,omitempty"`
	Issuer        string `json:"actor_issuer,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Operation     string `json:"operation,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
}

type nullableStrings struct {
	sourceRef           sql.NullString
	rawPolicyHash       sql.NullString
	createdBySubject    sql.NullString
	createdByIssuer     sql.NullString
	createdByClientID   sql.NullString
	updatedBySubject    sql.NullString
	updatedByIssuer     sql.NullString
	updatedByClientID   sql.NullString
	activatedBySubject  sql.NullString
	activatedByIssuer   sql.NullString
	activatedByClientID sql.NullString
	artifactRef         sql.NullString
}
