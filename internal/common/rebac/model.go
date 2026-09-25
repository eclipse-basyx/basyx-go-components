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

package rebac

import (
	"encoding/json"
	"fmt"
	"reflect"
)

var permissionRelations = []string{"read", "update", "create_child", "execute", "delete", "manage"}

// AuthorizationModelJSON returns the pinned OpenFGA authorization-model body.
func AuthorizationModelJSON() ([]byte, error) {
	return json.Marshal(expectedAuthorizationModel())
}

// ValidateAuthorizationModel verifies that a fetched OpenFGA model exactly
// matches the BaSyx model. The OpenFGA-assigned id is intentionally ignored.
func ValidateAuthorizationModel(raw json.RawMessage) error {
	var actual map[string]any
	if err := json.Unmarshal(raw, &actual); err != nil {
		return fmt.Errorf("REBAC-MODEL-DECODE: decode authorization model: %w", err)
	}
	normalizeAuthorizationModel(actual)
	if !reflect.DeepEqual(actual, expectedAuthorizationModel()) {
		return fmt.Errorf("REBAC-MODEL-MISMATCH authorization model differs from the required BaSyx model")
	}
	return nil
}

func normalizeAuthorizationModel(value any) {
	switch current := value.(type) {
	case map[string]any:
		delete(current, "id")
		delete(current, "module")
		delete(current, "source_info")
		if current["metadata"] == nil {
			delete(current, "metadata")
		}
		if current["object"] == "" {
			delete(current, "object")
		}
		if current["condition"] == "" {
			delete(current, "condition")
		}
		for _, nested := range current {
			normalizeAuthorizationModel(nested)
		}
	case []any:
		for _, nested := range current {
			normalizeAuthorizationModel(nested)
		}
	}
}

func expectedAuthorizationModel() map[string]any {
	return map[string]any{
		"schema_version":   "1.1",
		"type_definitions": authorizationTypeDefinitions(),
		"conditions":       map[string]any{},
	}
}

func authorizationTypeDefinitions() []any {
	definitions := []any{typeDefinition("user", nil, nil), groupDefinition(), repositoryDefinition()}
	definitions = append(definitions,
		containedDefinition("aas", []string{"repository"}, nil),
		containedDefinition("submodel", []string{"repository"}, []string{"aas"}),
		containedDefinition("element", []string{"submodel", "element"}, nil),
		sourcedDefinition("aas_descriptor", []string{"aas"}),
		sourcedDefinition("submodel_descriptor", []string{"submodel"}),
		sourcedDefinition("discovery", []string{"aas_descriptor", "submodel_descriptor"}),
		containedDefinition("concept_description", []string{"repository"}, nil),
		containedDefinition("package", []string{"repository"}, nil),
	)
	return definitions
}

func groupDefinition() map[string]any {
	return typeDefinition("group", map[string]any{"member": this()}, map[string]any{"member": directRelationMetadata([]any{userType("user")})})
}

func repositoryDefinition() map[string]any {
	relations := directRoleRelations()
	relations["creator"] = this()
	relations["create"] = union(computed("creator"), computed("editor"), computed("owner"))
	for _, permission := range permissionRelations {
		relations[permission] = rolePermission(permission)
	}
	return typeDefinition("repository", relations, directRoleMetadata("creator"))
}

func containedDefinition(name string, parents, aasParents []string) map[string]any {
	relations := directRoleRelations()
	relations["parent"] = this()
	metadata := directRoleMetadata()
	metadata["parent"] = directRelationMetadata(userTypes(parents))
	if len(aasParents) > 0 {
		relations["aas_parent"] = this()
		metadata["aas_parent"] = directRelationMetadata(userTypes(aasParents))
	}
	for _, permission := range permissionRelations {
		relations[permission] = containedPermission(permission, len(aasParents) > 0)
	}
	return typeDefinition(name, relations, metadata)
}

func sourcedDefinition(name string, sources []string) map[string]any {
	relations := directRoleRelations()
	relations["source"] = this()
	metadata := directRoleMetadata()
	metadata["source"] = directRelationMetadata(userTypes(sources))
	relations["read"] = union(rolePermission("read"), from("source", "read"))
	for _, permission := range permissionRelations[1:] {
		relations[permission] = rolePermission(permission)
	}
	return typeDefinition(name, relations, metadata)
}

func directRoleRelations() map[string]any {
	return map[string]any{
		"viewer":   this(),
		"editor":   this(),
		"executor": this(),
		"owner":    this(),
	}
}

func directRoleMetadata(extra ...string) map[string]any {
	metadata := map[string]any{}
	for _, relation := range append([]string{"viewer", "editor", "executor", "owner"}, extra...) {
		metadata[relation] = directRelationMetadata([]any{userType("user"), usersetType("group", "member")})
	}
	return metadata
}

func directRelationMetadata(types []any) map[string]any {
	return map[string]any{"directly_related_user_types": types}
}

func rolePermission(permission string) map[string]any {
	roles := []any{computed("owner")}
	switch permission {
	case "read":
		roles = append(roles, computed("viewer"), computed("editor"))
	case "update", "create_child":
		roles = append(roles, computed("editor"))
	case "execute":
		roles = append(roles, computed("executor"))
	}
	if len(roles) == 1 {
		return roles[0].(map[string]any)
	}
	return union(roles...)
}

func containedPermission(permission string, hasAASParent bool) map[string]any {
	terms := []any{rolePermission(permission), from("parent", permission)}
	if hasAASParent && (permission == "read" || permission == "update" || permission == "create_child" || permission == "execute") {
		terms = append(terms, from("aas_parent", permission))
	}
	return union(terms...)
}

func typeDefinition(name string, relations, metadata map[string]any) map[string]any {
	definition := map[string]any{"type": name, "relations": map[string]any{}}
	if relations != nil {
		definition["relations"] = relations
	}
	if len(metadata) > 0 {
		definition["metadata"] = map[string]any{"relations": metadata}
	}
	return definition
}

func this() map[string]any { return map[string]any{"this": map[string]any{}} }

func computed(relation string) map[string]any {
	return map[string]any{"computedUserset": map[string]any{"relation": relation}}
}

func from(tupleset, relation string) map[string]any {
	return map[string]any{"tupleToUserset": map[string]any{
		"tupleset":        map[string]any{"relation": tupleset},
		"computedUserset": map[string]any{"relation": relation},
	}}
}

func union(children ...any) map[string]any {
	return map[string]any{"union": map[string]any{"child": children}}
}

func userType(name string) map[string]any { return map[string]any{"type": name} }

func usersetType(name, relation string) map[string]any {
	return map[string]any{"type": name, "relation": relation}
}

func userTypes(names []string) []any {
	types := make([]any, 0, len(names))
	for _, name := range names {
		types = append(types, userType(name))
	}
	return types
}
