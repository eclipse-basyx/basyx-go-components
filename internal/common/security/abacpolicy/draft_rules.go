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

package abacpolicy

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ClonePolicyVersion copies an existing policy version into a staged draft.
//
// Cloning is the normal workflow for changing authorization without mutating
// the active policy in place. The clone receives a new version_id and can be
// edited, validated, and activated independently.
func (r *Repository) ClonePolicyVersion(ctx context.Context, versionID int64) (*PolicyVersion, error) {
	actor := actorFromContext(ctx, "ClonePolicyVersion", managementBasePath)
	var cloned PolicyVersion
	err := common.ExecuteInTransaction(r.db, "ABACPOLICY-CLONE-BEGINTX", "ABACPOLICY-CLONE-COMMIT", func(tx *sql.Tx) error {
		source, loadErr := r.loadPolicyVersion(ctx, tx, versionID)
		if loadErr != nil {
			return loadErr
		}
		materialized, materializeErr := auth.MaterializeABACPolicy(source.ConfiguredPolicyJSON, r.apiRouter, r.basePath)
		if materializeErr != nil {
			return common.NewErrBadRequest("ABACPOLICY-CLONE-MATERIALIZE " + materializeErr.Error())
		}
		created, createErr := r.createPolicyVersionTx(ctx, tx, materialized, StatusStaged, SourceTypeAPI, "clone:"+source.PolicyID, actor)
		cloned = created
		return createErr
	})
	if err != nil {
		return nil, err
	}
	return &cloned, nil
}

// CreateRule inserts a configured rule into a staged policy version.
//
// Position is 1-based. A zero or out-of-range insert position appends the rule.
// The full draft is materialized after insertion so rule order, hashes, and
// matched_rule_id values are immediately inspectable.
func (r *Repository) CreateRule(ctx context.Context, versionID int64, request RuleMutationRequest) (*PolicyVersion, error) {
	rule, err := decodeConfiguredRule(request.Rule)
	if err != nil {
		return nil, err
	}
	return r.updateDraftRules(ctx, versionID, "CreateRule", func(rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		position := normalizeInsertPosition(request.Position, len(rules))
		return insertRuleAt(rules, position, rule), nil
	})
}

// ReplaceRule replaces one configured rule in a staged policy version.
//
// The rule index is 1-based and must refer to an existing configured rule. The
// replacement is validated by materializing the complete draft policy before it
// is persisted.
func (r *Repository) ReplaceRule(ctx context.Context, versionID int64, ruleIndex int, raw json.RawMessage) (*PolicyVersion, error) {
	rule, err := decodeConfiguredRule(raw)
	if err != nil {
		return nil, err
	}
	return r.updateDraftRules(ctx, versionID, "ReplaceRule", func(rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		if err := validateRuleIndex(ruleIndex, len(rules)); err != nil {
			return nil, err
		}
		rules[ruleIndex-1] = rule
		return rules, nil
	})
}

// PatchRule applies JSON object merge semantics to one configured draft rule.
//
// This is not RFC 6902 JSON Patch. Object fields in the patch replace existing
// values recursively, and null removes a field, matching the repository's
// management API semantics.
func (r *Repository) PatchRule(ctx context.Context, versionID int64, ruleIndex int, patch json.RawMessage) (*PolicyVersion, error) {
	return r.updateDraftRules(ctx, versionID, "PatchRule", func(rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		if err := validateRuleIndex(ruleIndex, len(rules)); err != nil {
			return nil, err
		}
		baseMap, err := configuredRuleMap(rules[ruleIndex-1])
		if err != nil {
			return nil, err
		}
		var patchMap map[string]any
		if err = common.DecodeJSONPreservingNumbers(patch, &patchMap); err != nil {
			return nil, common.NewErrBadRequest("ABACPOLICY-PATCHRULE-DECODE " + err.Error())
		}
		merged, err := common.CanonicalJSON(mergeJSONObjects(baseMap, patchMap))
		if err != nil {
			return nil, common.NewInternalServerError("ABACPOLICY-PATCHRULE-CANONICAL " + err.Error())
		}
		rules[ruleIndex-1], err = decodeConfiguredRule(merged)
		return rules, err
	})
}

// DeleteRule removes one configured rule from a staged policy version.
//
// Deleting the last rule is rejected so a policy cannot be activated with an
// empty rule set. Rule indexes are recomputed during materialization.
func (r *Repository) DeleteRule(ctx context.Context, versionID int64, ruleIndex int) (*PolicyVersion, error) {
	return r.updateDraftRules(ctx, versionID, "DeleteRule", func(rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		if err := validateRuleIndex(ruleIndex, len(rules)); err != nil {
			return nil, err
		}
		if len(rules) == 1 {
			return nil, common.NewErrBadRequest("ABACPOLICY-DELETERULE-EMPTY policy must contain at least one rule")
		}
		return append(rules[:ruleIndex-1], rules[ruleIndex:]...), nil
	})
}

// DuplicateRule copies one configured rule to a new staged-policy position.
//
// The source rule index is 1-based. Position zero inserts the copy directly
// after the source rule; otherwise the provided 1-based position is used.
func (r *Repository) DuplicateRule(ctx context.Context, versionID int64, ruleIndex int, position int) (*PolicyVersion, error) {
	return r.updateDraftRules(ctx, versionID, "DuplicateRule", func(rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		if err := validateRuleIndex(ruleIndex, len(rules)); err != nil {
			return nil, err
		}
		insertPosition := normalizeInsertPosition(position, len(rules))
		if position == 0 {
			insertPosition = ruleIndex + 1
		}
		return insertRuleAt(rules, insertPosition, rules[ruleIndex-1]), nil
	})
}

// MoveRule moves one configured rule to an explicit 1-based position.
//
// Rule ordering is security relevant because the evaluator preserves configured
// order and matched_rule_id contains the rule index. Moving a rule rematerializes
// the draft and recomputes rule_index values.
func (r *Repository) MoveRule(ctx context.Context, versionID int64, ruleIndex int, position int) (*PolicyVersion, error) {
	return r.updateDraftRules(ctx, versionID, "MoveRule", func(rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		if err := validateRuleIndex(ruleIndex, len(rules)); err != nil {
			return nil, err
		}
		if position < 1 || position > len(rules) {
			return nil, common.NewErrBadRequest("ABACPOLICY-MOVERULE-POSITION target position is out of range")
		}
		rule := rules[ruleIndex-1]
		rules = append(rules[:ruleIndex-1], rules[ruleIndex:]...)
		return insertRuleAt(rules, position, rule), nil
	})
}

// SetRuleEnabled toggles one rule by setting ACL.ACCESS to ALLOW or DISABLED.
//
// Rules that refer to shared ACL definitions through USEACL are converted to an
// inline rule-local ACL before toggling. This prevents enabling or disabling one
// draft rule from changing other rules that share the same definition.
func (r *Repository) SetRuleEnabled(ctx context.Context, versionID int64, ruleIndex int, enabled bool) (*PolicyVersion, error) {
	return r.updateDraftRulesWithVersion(ctx, versionID, "SetRuleEnabled", func(version PolicyVersion, rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		if err := validateRuleIndex(ruleIndex, len(rules)); err != nil {
			return nil, err
		}
		materialized, err := auth.MaterializeABACPolicy(version.ConfiguredPolicyJSON, r.apiRouter, r.basePath)
		if err != nil {
			return nil, err
		}
		var acl grammar.ACL
		if err = json.Unmarshal(materialized.Rules[ruleIndex-1].ACLJSON, &acl); err != nil {
			return nil, common.NewInternalServerError("ABACPOLICY-ENABLE-DECODEACL " + err.Error())
		}
		if enabled {
			acl.ACCESS = grammar.ACLACCESSALLOW
		} else {
			acl.ACCESS = grammar.ACLACCESSDISABLED
		}
		rules[ruleIndex-1].ACL = &acl
		rules[ruleIndex-1].USEACL = nil
		return rules, nil
	})
}

func (r *Repository) updateDraftRules(
	ctx context.Context,
	versionID int64,
	operation string,
	mutate func([]grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error),
) (*PolicyVersion, error) {
	return r.updateDraftRulesWithVersion(ctx, versionID, operation, func(_ PolicyVersion, rules []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error) {
		return mutate(rules)
	})
}

func (r *Repository) updateDraftRulesWithVersion(
	ctx context.Context,
	versionID int64,
	operation string,
	mutate func(PolicyVersion, []grammar.AccessPermissionRule) ([]grammar.AccessPermissionRule, error),
) (*PolicyVersion, error) {
	actor := actorFromContext(ctx, operation, managementBasePath)
	var updated PolicyVersion
	err := common.ExecuteInTransaction(r.db, "ABACPOLICY-DRAFT-BEGINTX", "ABACPOLICY-DRAFT-COMMIT", func(tx *sql.Tx) error {
		version, loadErr := r.loadPolicyVersion(ctx, tx, versionID)
		if loadErr != nil {
			return loadErr
		}
		if version.Status != StatusStaged {
			return common.NewErrConflict("ABACPOLICY-DRAFT-IMMUTABLE only staged policy versions are editable")
		}
		policy, decodeErr := decodeConfiguredPolicy(version.ConfiguredPolicyJSON)
		if decodeErr != nil {
			return decodeErr
		}
		rules := append([]grammar.AccessPermissionRule(nil), policy.AllAccessPermissionRules.Rules...)
		nextRules, mutateErr := mutate(version, rules)
		if mutateErr != nil {
			return mutateErr
		}
		policy.AllAccessPermissionRules.Rules = nextRules
		raw, canonicalErr := common.CanonicalJSON(policy)
		if canonicalErr != nil {
			return common.NewInternalServerError("ABACPOLICY-DRAFT-CANONICAL " + canonicalErr.Error())
		}
		materialized, materializeErr := auth.MaterializeABACPolicy(raw, r.apiRouter, r.basePath)
		if materializeErr != nil {
			return common.NewErrBadRequest("ABACPOLICY-DRAFT-MATERIALIZE " + materializeErr.Error())
		}
		var replaceErr error
		updated, replaceErr = r.replaceMaterializationTx(ctx, tx, versionID, materialized, actor)
		return replaceErr
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func decodeConfiguredPolicy(raw json.RawMessage) (grammar.AccessRuleModelSchemaJSON, error) {
	var policy grammar.AccessRuleModelSchemaJSON
	if err := json.Unmarshal(raw, &policy); err != nil {
		return grammar.AccessRuleModelSchemaJSON{}, common.NewInternalServerError("ABACPOLICY-DRAFT-DECODEPOLICY " + err.Error())
	}
	return policy, nil
}

func decodeConfiguredRule(raw json.RawMessage) (grammar.AccessPermissionRule, error) {
	var rule grammar.AccessPermissionRule
	if len(raw) == 0 {
		return rule, common.NewErrBadRequest("ABACPOLICY-RULE-DECODE rule body is required")
	}
	if err := json.Unmarshal(raw, &rule); err != nil {
		return rule, common.NewErrBadRequest("ABACPOLICY-RULE-DECODE " + err.Error())
	}
	return rule, nil
}

func configuredRuleMap(rule grammar.AccessPermissionRule) (map[string]any, error) {
	raw, err := common.CanonicalJSON(rule)
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-RULE-MAPCANONICAL " + err.Error())
	}
	var out map[string]any
	if err = common.DecodeJSONPreservingNumbers(raw, &out); err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-RULE-MAPDECODE " + err.Error())
	}
	return out, nil
}

func validateRuleIndex(ruleIndex int, ruleCount int) error {
	if ruleIndex < 1 || ruleIndex > ruleCount {
		return common.NewErrBadRequest("ABACPOLICY-RULE-INDEX rule index is out of range")
	}
	return nil
}

func normalizeInsertPosition(position int, ruleCount int) int {
	if position < 1 {
		return ruleCount + 1
	}
	if position > ruleCount+1 {
		return ruleCount + 1
	}
	return position
}

func insertRuleAt(rules []grammar.AccessPermissionRule, position int, rule grammar.AccessPermissionRule) []grammar.AccessPermissionRule {
	index := position - 1
	rules = append(rules, grammar.AccessPermissionRule{})
	copy(rules[index+1:], rules[index:])
	rules[index] = rule
	return rules
}

func mergeJSONObjects(base map[string]any, patch map[string]any) map[string]any {
	merged := make(map[string]any, len(base))
	for key, value := range base {
		merged[key] = value
	}
	for key, patchValue := range patch {
		if patchValue == nil {
			delete(merged, key)
			continue
		}
		baseValue, baseExists := merged[key]
		baseMap, baseIsMap := baseValue.(map[string]any)
		patchMap, patchIsMap := patchValue.(map[string]any)
		if baseExists && baseIsMap && patchIsMap {
			merged[key] = mergeJSONObjects(baseMap, patchMap)
			continue
		}
		merged[key] = patchValue
	}
	return merged
}
