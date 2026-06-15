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
	"encoding/json"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type definitionKind string

const (
	definitionKindAttributes definitionKind = "attributes"
	definitionKindACLs       definitionKind = "acls"
	definitionKindObjects    definitionKind = "objects"
	definitionKindFormulas   definitionKind = "formulas"
)

// ListDefinitions loads the reusable definition sections of one policy version.
//
// Definitions are read from the configured policy JSON so administrators can
// inspect the exact draft, active, superseded, or rejected policy source.
func (r *Repository) ListDefinitions(ctx context.Context, versionID int64) (PolicyDefinitions, error) {
	version, err := r.loadPolicyVersion(ctx, r.db, versionID)
	if err != nil {
		return PolicyDefinitions{}, err
	}
	policy, err := decodeConfiguredPolicy(version.ConfiguredPolicyJSON)
	if err != nil {
		return PolicyDefinitions{}, err
	}
	return definitionsFromPolicy(policy), nil
}

// ListDefinitionsByKind loads one reusable definition section of a policy.
//
// Kind accepts attributes, acls, objects, or formulas. The returned value is
// one of the generated grammar slices and can be encoded directly as JSON.
func (r *Repository) ListDefinitionsByKind(ctx context.Context, versionID int64, kind string) (any, error) {
	normalizedKind, err := normalizeDefinitionKind(kind)
	if err != nil {
		return nil, err
	}
	definitions, err := r.ListDefinitions(ctx, versionID)
	if err != nil {
		return nil, err
	}
	return definitionsByKind(definitions, normalizedKind), nil
}

// GetDefinition loads one named reusable definition from a policy version.
//
// Definition names are compared after trimming whitespace. Missing definitions
// return NotFound so callers can safely use it for management UI lookups.
func (r *Repository) GetDefinition(ctx context.Context, versionID int64, kind string, name string) (any, error) {
	normalizedKind, normalizedName, err := normalizeDefinitionPath(kind, name)
	if err != nil {
		return nil, err
	}
	definitions, err := r.ListDefinitions(ctx, versionID)
	if err != nil {
		return nil, err
	}
	return definitionByName(definitions, normalizedKind, normalizedName)
}

// CreateDefinition appends one reusable definition to a staged policy version.
//
// The complete staged policy is canonicalized and materialized after the edit,
// which rejects duplicate names, malformed definitions, and broken references
// before the change is committed.
func (r *Repository) CreateDefinition(ctx context.Context, versionID int64, kind string, raw json.RawMessage) (*PolicyVersion, error) {
	normalizedKind, err := normalizeDefinitionKind(kind)
	if err != nil {
		return nil, err
	}
	return r.updateDraftPolicy(ctx, versionID, "CreateDefinition", func(_ PolicyVersion, policy *grammar.AccessRuleModelSchemaJSON) error {
		return createDefinitionInPolicy(policy, normalizedKind, raw)
	})
}

// ReplaceDefinition replaces one reusable definition on a staged policy version.
//
// The definition name in the body must match the path name. Renaming is modeled
// as delete plus create so accidental reference rewrites cannot happen silently.
func (r *Repository) ReplaceDefinition(ctx context.Context, versionID int64, kind string, name string, raw json.RawMessage) (*PolicyVersion, error) {
	normalizedKind, normalizedName, err := normalizeDefinitionPath(kind, name)
	if err != nil {
		return nil, err
	}
	return r.updateDraftPolicy(ctx, versionID, "ReplaceDefinition", func(_ PolicyVersion, policy *grammar.AccessRuleModelSchemaJSON) error {
		return replaceDefinitionInPolicy(policy, normalizedKind, normalizedName, raw)
	})
}

// PatchDefinition merge-patches one reusable definition on a staged policy.
//
// This uses the same JSON object merge semantics as PatchRule: null removes a
// field, object values merge recursively, and the result must still validate as
// the selected definition kind.
func (r *Repository) PatchDefinition(ctx context.Context, versionID int64, kind string, name string, patch json.RawMessage) (*PolicyVersion, error) {
	normalizedKind, normalizedName, err := normalizeDefinitionPath(kind, name)
	if err != nil {
		return nil, err
	}
	return r.updateDraftPolicy(ctx, versionID, "PatchDefinition", func(_ PolicyVersion, policy *grammar.AccessRuleModelSchemaJSON) error {
		return patchDefinitionInPolicy(policy, normalizedKind, normalizedName, patch)
	})
}

// DeleteDefinition removes one reusable definition from a staged policy version.
//
// The delete is committed only when the full policy can still be materialized,
// so definitions that are still referenced by rules or other definitions are
// rejected as bad requests.
func (r *Repository) DeleteDefinition(ctx context.Context, versionID int64, kind string, name string) (*PolicyVersion, error) {
	normalizedKind, normalizedName, err := normalizeDefinitionPath(kind, name)
	if err != nil {
		return nil, err
	}
	return r.updateDraftPolicy(ctx, versionID, "DeleteDefinition", func(_ PolicyVersion, policy *grammar.AccessRuleModelSchemaJSON) error {
		return deleteDefinitionFromPolicy(policy, normalizedKind, normalizedName)
	})
}

func definitionsFromPolicy(policy grammar.AccessRuleModelSchemaJSON) PolicyDefinitions {
	all := policy.AllAccessPermissionRules
	return PolicyDefinitions{
		Attributes: append([]grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem(nil), all.DEFATTRIBUTES...),
		ACLs:       append([]grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem(nil), all.DEFACLS...),
		Objects:    append([]grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem(nil), all.DEFOBJECTS...),
		Formulas:   append([]grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem(nil), all.DEFFORMULAS...),
	}
}

func definitionsByKind(definitions PolicyDefinitions, kind definitionKind) any {
	switch kind {
	case definitionKindAttributes:
		return definitions.Attributes
	case definitionKindACLs:
		return definitions.ACLs
	case definitionKindObjects:
		return definitions.Objects
	case definitionKindFormulas:
		return definitions.Formulas
	default:
		return nil
	}
}

func definitionByName(definitions PolicyDefinitions, kind definitionKind, name string) (any, error) {
	switch kind {
	case definitionKindAttributes:
		if definition, ok := findNamedDefinition(definitions.Attributes, func(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem) string {
			return definition.Name
		}, name); ok {
			return definition, nil
		}
	case definitionKindACLs:
		if definition, ok := findNamedDefinition(definitions.ACLs, func(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem) string {
			return definition.Name
		}, name); ok {
			return definition, nil
		}
	case definitionKindObjects:
		if definition, ok := findNamedDefinition(definitions.Objects, func(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem) string {
			return definition.Name
		}, name); ok {
			return definition, nil
		}
	case definitionKindFormulas:
		if definition, ok := findNamedDefinition(definitions.Formulas, func(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem) string {
			return definition.Name
		}, name); ok {
			return definition, nil
		}
	}
	return nil, common.NewErrNotFound("ABACPOLICY-DEFINITION-NOTFOUND definition not found")
}

func createDefinitionInPolicy(policy *grammar.AccessRuleModelSchemaJSON, kind definitionKind, raw json.RawMessage) error {
	switch kind {
	case definitionKindAttributes:
		definition, name, err := decodeAttributeDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFATTRIBUTES, err = appendNamedDefinition(policy.AllAccessPermissionRules.DEFATTRIBUTES, attributeDefinitionName, definition, name)
		return err
	case definitionKindACLs:
		definition, name, err := decodeACLDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFACLS, err = appendNamedDefinition(policy.AllAccessPermissionRules.DEFACLS, aclDefinitionName, definition, name)
		return err
	case definitionKindObjects:
		definition, name, err := decodeObjectDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFOBJECTS, err = appendNamedDefinition(policy.AllAccessPermissionRules.DEFOBJECTS, objectDefinitionName, definition, name)
		return err
	case definitionKindFormulas:
		definition, name, err := decodeFormulaDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFFORMULAS, err = appendNamedDefinition(policy.AllAccessPermissionRules.DEFFORMULAS, formulaDefinitionName, definition, name)
		return err
	default:
		return unsupportedDefinitionKindError(string(kind))
	}
}

func replaceDefinitionInPolicy(policy *grammar.AccessRuleModelSchemaJSON, kind definitionKind, name string, raw json.RawMessage) error {
	switch kind {
	case definitionKindAttributes:
		definition, bodyName, err := decodeAttributeDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFATTRIBUTES, err = replaceNamedDefinition(policy.AllAccessPermissionRules.DEFATTRIBUTES, attributeDefinitionName, name, bodyName, definition)
		return err
	case definitionKindACLs:
		definition, bodyName, err := decodeACLDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFACLS, err = replaceNamedDefinition(policy.AllAccessPermissionRules.DEFACLS, aclDefinitionName, name, bodyName, definition)
		return err
	case definitionKindObjects:
		definition, bodyName, err := decodeObjectDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFOBJECTS, err = replaceNamedDefinition(policy.AllAccessPermissionRules.DEFOBJECTS, objectDefinitionName, name, bodyName, definition)
		return err
	case definitionKindFormulas:
		definition, bodyName, err := decodeFormulaDefinition(raw)
		if err != nil {
			return err
		}
		policy.AllAccessPermissionRules.DEFFORMULAS, err = replaceNamedDefinition(policy.AllAccessPermissionRules.DEFFORMULAS, formulaDefinitionName, name, bodyName, definition)
		return err
	default:
		return unsupportedDefinitionKindError(string(kind))
	}
}

func patchDefinitionInPolicy(policy *grammar.AccessRuleModelSchemaJSON, kind definitionKind, name string, patch json.RawMessage) error {
	current, err := definitionByName(definitionsFromPolicy(*policy), kind, name)
	if err != nil {
		return err
	}
	merged, err := mergeDefinitionPatch(current, patch)
	if err != nil {
		return err
	}
	return replaceDefinitionInPolicy(policy, kind, name, merged)
}

func deleteDefinitionFromPolicy(policy *grammar.AccessRuleModelSchemaJSON, kind definitionKind, name string) error {
	var err error
	switch kind {
	case definitionKindAttributes:
		policy.AllAccessPermissionRules.DEFATTRIBUTES, err = deleteNamedDefinition(policy.AllAccessPermissionRules.DEFATTRIBUTES, attributeDefinitionName, name)
	case definitionKindACLs:
		policy.AllAccessPermissionRules.DEFACLS, err = deleteNamedDefinition(policy.AllAccessPermissionRules.DEFACLS, aclDefinitionName, name)
	case definitionKindObjects:
		policy.AllAccessPermissionRules.DEFOBJECTS, err = deleteNamedDefinition(policy.AllAccessPermissionRules.DEFOBJECTS, objectDefinitionName, name)
	case definitionKindFormulas:
		policy.AllAccessPermissionRules.DEFFORMULAS, err = deleteNamedDefinition(policy.AllAccessPermissionRules.DEFFORMULAS, formulaDefinitionName, name)
	default:
		return unsupportedDefinitionKindError(string(kind))
	}
	return err
}

func decodeAttributeDefinition(raw json.RawMessage) (grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem, string, error) {
	var definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem
	if err := decodeDefinition(raw, &definition); err != nil {
		return definition, "", err
	}
	name, err := normalizeDefinitionName(definition.Name)
	return definition, name, err
}

func decodeACLDefinition(raw json.RawMessage) (grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem, string, error) {
	var definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem
	if err := decodeDefinition(raw, &definition); err != nil {
		return definition, "", err
	}
	name, err := normalizeDefinitionName(definition.Name)
	return definition, name, err
}

func decodeObjectDefinition(raw json.RawMessage) (grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem, string, error) {
	var definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem
	if err := decodeDefinition(raw, &definition); err != nil {
		return definition, "", err
	}
	name, err := normalizeDefinitionName(definition.Name)
	return definition, name, err
}

func decodeFormulaDefinition(raw json.RawMessage) (grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem, string, error) {
	var definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem
	if err := decodeDefinition(raw, &definition); err != nil {
		return definition, "", err
	}
	name, err := normalizeDefinitionName(definition.Name)
	return definition, name, err
}

func decodeDefinition(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return common.NewErrBadRequest("ABACPOLICY-DEFINITION-DECODE definition body is required")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return common.NewErrBadRequest("ABACPOLICY-DEFINITION-DECODE " + err.Error())
	}
	return nil
}

func mergeDefinitionPatch(current any, patch json.RawMessage) (json.RawMessage, error) {
	baseMap, err := definitionMap(current)
	if err != nil {
		return nil, err
	}
	var patchMap map[string]any
	if err = common.DecodeJSONPreservingNumbers(patch, &patchMap); err != nil {
		return nil, common.NewErrBadRequest("ABACPOLICY-DEFINITION-PATCH-DECODE " + err.Error())
	}
	merged, err := common.CanonicalJSON(mergeJSONObjects(baseMap, patchMap))
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-DEFINITION-PATCH-CANONICAL " + err.Error())
	}
	return merged, nil
}

func definitionMap(value any) (map[string]any, error) {
	raw, err := common.CanonicalJSON(value)
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-DEFINITION-MAPCANONICAL " + err.Error())
	}
	var out map[string]any
	if err = common.DecodeJSONPreservingNumbers(raw, &out); err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-DEFINITION-MAPDECODE " + err.Error())
	}
	return out, nil
}

func attributeDefinitionName(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem) string {
	return definition.Name
}

func aclDefinitionName(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem) string {
	return definition.Name
}

func objectDefinitionName(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem) string {
	return definition.Name
}

func formulaDefinitionName(definition grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem) string {
	return definition.Name
}

func appendNamedDefinition[T any](definitions []T, nameOf func(T) string, definition T, name string) ([]T, error) {
	if _, ok := findNamedDefinition(definitions, nameOf, name); ok {
		return definitions, duplicateDefinitionError(name)
	}
	return append(definitions, definition), nil
}

func replaceNamedDefinition[T any](definitions []T, nameOf func(T) string, name string, bodyName string, definition T) ([]T, error) {
	if err := requireMatchingDefinitionName(name, bodyName); err != nil {
		return definitions, err
	}
	index := namedDefinitionIndex(definitions, nameOf, name)
	if index < 0 {
		return definitions, missingDefinitionError()
	}
	definitions[index] = definition
	return definitions, nil
}

func deleteNamedDefinition[T any](definitions []T, nameOf func(T) string, name string) ([]T, error) {
	index := namedDefinitionIndex(definitions, nameOf, name)
	if index < 0 {
		return definitions, missingDefinitionError()
	}
	return append(definitions[:index], definitions[index+1:]...), nil
}

func findNamedDefinition[T any](definitions []T, nameOf func(T) string, name string) (T, bool) {
	index := namedDefinitionIndex(definitions, nameOf, name)
	if index < 0 {
		var zero T
		return zero, false
	}
	return definitions[index], true
}

func namedDefinitionIndex[T any](definitions []T, nameOf func(T) string, name string) int {
	for i, definition := range definitions {
		if strings.TrimSpace(nameOf(definition)) == name {
			return i
		}
	}
	return -1
}

func normalizeDefinitionPath(kind string, name string) (definitionKind, string, error) {
	normalizedKind, err := normalizeDefinitionKind(kind)
	if err != nil {
		return "", "", err
	}
	normalizedName, err := normalizeDefinitionName(name)
	if err != nil {
		return "", "", err
	}
	return normalizedKind, normalizedName, nil
}

func normalizeDefinitionKind(kind string) (definitionKind, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "attributes", "attribute", "defattributes":
		return definitionKindAttributes, nil
	case "acls", "acl", "defacls":
		return definitionKindACLs, nil
	case "objects", "object", "defobjects":
		return definitionKindObjects, nil
	case "formulas", "formula", "defformulas":
		return definitionKindFormulas, nil
	default:
		return "", unsupportedDefinitionKindError(kind)
	}
}

func normalizeDefinitionName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", common.NewErrBadRequest("ABACPOLICY-DEFINITION-NAME definition name is required")
	}
	return normalized, nil
}

func requireMatchingDefinitionName(pathName string, bodyName string) error {
	if pathName != bodyName {
		return common.NewErrBadRequest("ABACPOLICY-DEFINITION-NAME definition body name must match path name")
	}
	return nil
}

func duplicateDefinitionError(name string) error {
	return common.NewErrConflict("ABACPOLICY-DEFINITION-DUPLICATE definition already exists: " + name)
}

func missingDefinitionError() error {
	return common.NewErrNotFound("ABACPOLICY-DEFINITION-NOTFOUND definition not found")
}

func unsupportedDefinitionKindError(kind string) error {
	return common.NewErrBadRequest("ABACPOLICY-DEFINITION-KIND unsupported definition kind " + strings.TrimSpace(kind))
}
