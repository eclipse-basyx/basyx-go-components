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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Principal identifies a subject or group within a trusted issuer.
type Principal struct {
	Kind   string `json:"kind"`
	Issuer string `json:"issuer"`
	ID     string `json:"id"`
}

// Object encodes an issuer-scoped principal without ambiguous delimiters.
func (p Principal) Object() (string, error) {
	if p.Kind != "user" && p.Kind != "group" {
		return "", fmt.Errorf("REBAC-IDENTITY-KIND invalid principal kind")
	}
	if strings.TrimSpace(p.Issuer) == "" || strings.TrimSpace(p.ID) == "" {
		return "", fmt.Errorf("REBAC-IDENTITY-EMPTY issuer and identifier are required")
	}
	return p.Kind + ":" + IdentityDigest(p.Issuer, p.ID), nil
}

// IdentityDigest uses length-safe encoding to avoid namespace collisions.
func IdentityDigest(parts ...string) string {
	data, _ := json.Marshal(parts)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ResourceObject encodes a resource generation within its deployment scope.
func ResourceObject(scope, kind, id string) (string, error) {
	if scope == "" || id == "" {
		return "", fmt.Errorf("REBAC-IDENTITY-RESOURCE scope and UUID are required")
	}
	if kind == "embedded_submodel_descriptor" {
		kind = "submodel_descriptor"
	}
	if !validResourceKind(kind) {
		return "", fmt.Errorf("REBAC-IDENTITY-RESOURCEKIND unsupported resource kind")
	}
	return kind + ":" + IdentityDigest(scope, id), nil
}

func validResourceKind(kind string) bool {
	switch kind {
	case "repository", "aas", "submodel", "element", "aas_descriptor", "submodel_descriptor", "discovery", "concept_description", "package":
		return true
	default:
		return false
	}
}

// MembershipTuples creates deduplicated contextual membership from verified claims.
func MembershipTuples(user Principal, groups []string) ([]Tuple, error) {
	object, err := user.Object()
	if err != nil {
		return nil, err
	}
	if user.Kind != "user" {
		return nil, fmt.Errorf("REBAC-IDENTITY-MEMBER user principal required")
	}
	tuples := make([]Tuple, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, group := range groups {
		groupObject, err := (Principal{Kind: "group", Issuer: user.Issuer, ID: group}).Object()
		if err != nil {
			return nil, err
		}
		if seen[groupObject] {
			continue
		}
		seen[groupObject] = true
		tuples = append(tuples, Tuple{User: object, Relation: "member", Object: groupObject})
	}
	return tuples, nil
}
