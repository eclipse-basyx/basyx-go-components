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

package rebac

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// OpenFGA object types of the BaSyx authorization model.
const (
	TypeUser               = "user"
	TypeGroup              = "group"
	TypeRepository         = "repository"
	TypeAAS                = "aas"
	TypeSubmodel           = "submodel"
	TypeElement            = "element"
	TypeConceptDescription = "concept_description"
)

// Relations of the BaSyx authorization model.
const (
	RelationOwner      = "owner"
	RelationEditor     = "editor"
	RelationViewer     = "viewer"
	RelationExecutor   = "executor"
	RelationMember     = "member"
	RelationAdmin      = "admin"
	RelationCreator    = "creator"
	RelationLinkedAAS  = "linked_aas"
	RelationSubmodel   = "submodel"
	RelationParent     = "parent"
	RelationCanRead    = "can_read"
	RelationCanUpdate  = "can_update"
	RelationCanDelete  = "can_delete"
	RelationCanExecute = "can_execute"
	RelationCanManage  = "can_manage"
)

// maxOpenFGAIdentifierLength keeps generated identifiers below the OpenFGA
// object identifier limit.
const maxOpenFGAIdentifierLength = 250

// Tuple is one OpenFGA relationship tuple.
type Tuple struct {
	User     string
	Relation string
	Object   string
}

// Principal is the issuer-scoped identity of an authenticated caller.
type Principal struct {
	Issuer  string
	Subject string
	Groups  []string
}

// PrincipalFromClaims extracts the caller identity from validated OIDC claims.
// groupClaim names the normalized claim carrying group names. Anonymous
// callers and tokens without issuer or subject are no ReBAC principals.
func PrincipalFromClaims(claims auth.Claims, groupClaim string) (Principal, bool) {
	issuer, _ := claims.GetString("iss")
	subject, _ := claims.GetString("sub")
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	if issuer == "" || subject == "" {
		return Principal{}, false
	}
	return Principal{Issuer: issuer, Subject: subject, Groups: claimStrings(claims[groupClaim])}, true
}

func claimStrings(raw any) []string {
	var values []string
	switch typed := raw.(type) {
	case string:
		values = []string{typed}
	case []string:
		values = typed
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	}
	groups := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			groups = append(groups, trimmed)
		}
	}
	slices.Sort(groups)
	return slices.Compact(groups)
}

// UserObject returns the OpenFGA user identifier of the principal.
func (p Principal) UserObject() string {
	return UserObject(p.Issuer, p.Subject)
}

// UserObject returns the issuer-scoped OpenFGA identifier of a user.
func UserObject(issuer string, subject string) string {
	return TypeUser + ":" + scopedIdentifier(issuer, subject)
}

// GroupObject returns the issuer-scoped OpenFGA identifier of a group.
func GroupObject(issuer string, name string) string {
	return TypeGroup + ":" + scopedIdentifier(issuer, name)
}

// GroupMembers returns the userset that grants a group's members access.
func GroupMembers(issuer string, name string) string {
	return GroupObject(issuer, name) + "#" + RelationMember
}

// GroupTuples returns the group memberships of the current token. They are
// sent as contextual tuples and never stored, so membership changes take
// effect with the next token.
func (p Principal) GroupTuples() []Tuple {
	tuples := make([]Tuple, 0, len(p.Groups))
	for _, group := range p.Groups {
		tuples = append(tuples, Tuple{User: p.UserObject(), Relation: RelationMember, Object: GroupObject(p.Issuer, group)})
	}
	return tuples
}

// SubjectKeys returns the stored subject keys that can grant the principal
// access: the user itself and the member usersets of its groups.
func (p Principal) SubjectKeys() []string {
	keys := make([]string, 0, 1+len(p.Groups))
	keys = append(keys, p.UserObject())
	for _, group := range p.Groups {
		keys = append(keys, GroupMembers(p.Issuer, group))
	}
	return keys
}

// scopedIdentifier encodes issuer and value without separators OpenFGA
// reserves. Overlong identifiers use a deterministic digest form, which can
// never collide with the plain form because base64url contains no dot.
func scopedIdentifier(issuer string, value string) string {
	encoding := base64.RawURLEncoding
	plain := encoding.EncodeToString([]byte(issuer)) + "." + encoding.EncodeToString([]byte(value))
	if len(plain) <= maxOpenFGAIdentifierLength {
		return plain
	}
	digest := sha256.Sum256([]byte(issuer + "\x00" + value))
	return "h." + encoding.EncodeToString(digest[:])
}

// ResourceObject returns the OpenFGA object of an identifiable.
func ResourceObject(objectType string, authUUID string) string {
	return objectType + ":" + authUUID
}

// RepositoryObject returns the OpenFGA object of a repository family.
func RepositoryObject(objectType string) string {
	return TypeRepository + ":" + objectType
}

// ElementObject returns the OpenFGA object of a SubmodelElement path.
func ElementObject(submodelAuthUUID string, idShortPath string) string {
	digest := sha256.Sum256([]byte(idShortPath))
	return TypeElement + ":" + submodelAuthUUID + "." + hex.EncodeToString(digest[:])[:32]
}

// ElementAncestry returns the contextual tuples that connect an element path
// to its ancestors and its Submodel. Elements exist as stored objects only
// when they carry direct grants, so the structure is always sent per check.
func ElementAncestry(submodelAuthUUID string, idShortPath string) []Tuple {
	submodel := ResourceObject(TypeSubmodel, submodelAuthUUID)
	paths := ElementPathChain(idShortPath)
	tuples := make([]Tuple, 0, len(paths))
	for index, path := range paths {
		object := ElementObject(submodelAuthUUID, path)
		if index == 0 {
			tuples = append(tuples, Tuple{User: submodel, Relation: RelationSubmodel, Object: object})
			continue
		}
		parent := ElementObject(submodelAuthUUID, paths[index-1])
		tuples = append(tuples, Tuple{User: parent, Relation: RelationParent, Object: object})
	}
	return tuples
}

// ElementPathChain returns every ancestor path of idShortPath from the
// top-level element down to the path itself, e.g. a, a.b, a.b[0].
func ElementPathChain(idShortPath string) []string {
	var chain []string
	depth := 0
	for index, char := range idShortPath {
		switch char {
		case '[':
			if depth == 0 && index > 0 {
				chain = append(chain, idShortPath[:index])
			}
			depth++
		case ']':
			depth--
		case '.':
			if depth == 0 {
				chain = append(chain, idShortPath[:index])
			}
		}
	}
	return append(chain, idShortPath)
}

// ParentElementPath returns the parent path of idShortPath or "" for
// top-level elements.
func ParentElementPath(idShortPath string) string {
	chain := ElementPathChain(idShortPath)
	if len(chain) < 2 {
		return ""
	}
	return chain[len(chain)-2]
}

// ValidateRelation rejects relations that cannot be granted directly.
func ValidateRelation(objectType string, relation string) error {
	if objectType == TypeRepository {
		if relation == RelationAdmin || relation == RelationCreator {
			return nil
		}
		return fmt.Errorf("REBAC-VALIDATERELATION-UNSUPPORTED relation %q cannot be granted on %s", relation, objectType)
	}
	switch relation {
	case RelationOwner, RelationEditor, RelationViewer:
		return nil
	case RelationExecutor:
		if objectType != TypeConceptDescription {
			return nil
		}
	}
	return fmt.Errorf("REBAC-VALIDATERELATION-UNSUPPORTED relation %q cannot be granted on %s", relation, objectType)
}
