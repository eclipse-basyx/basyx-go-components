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

// Object and subject types of the BaSyx relationship model.
const (
	TypeUser               = "user"
	TypeGroup              = "group"
	TypeRepository         = "repository"
	TypeAAS                = "aas"
	TypeSubmodel           = "submodel"
	TypeElement            = "element"
	TypeConceptDescription = "concept_description"
	TypeAASDescriptor      = "aas_descriptor"
	TypeSubmodelDescriptor = "submodel_descriptor"
	TypeAssetLinks         = "asset_links"
	TypeAASXPackage        = "aasx_package"
)

// Relations that can be granted.
const (
	RelationOwner    = "owner"
	RelationEditor   = "editor"
	RelationViewer   = "viewer"
	RelationExecutor = "executor"
	RelationAdmin    = "admin"
	RelationCreator  = "creator"
)

// Permissions derived from granted relations.
const (
	PermissionRead    = "can_read"
	PermissionUpdate  = "can_update"
	PermissionDelete  = "can_delete"
	PermissionExecute = "can_execute"
	PermissionManage  = "can_manage"
)

// maxIdentifierLength bounds generated subject and object keys.
const maxIdentifierLength = 250

// Principal is the issuer-scoped identity of an authenticated caller.
type Principal struct {
	Issuer  string
	Subject string
	Groups  []string
}

// ClaimNames names the claims that identify a caller: Subject holds the
// stable user identifier (for example sub, or oid for Microsoft Entra ID)
// and Groups the normalized group names.
type ClaimNames struct {
	Subject string
	Groups  string
}

// PrincipalFromClaims extracts the caller identity from validated OIDC claims.
// Anonymous callers and tokens without issuer or subject are no ReBAC
// principals.
func PrincipalFromClaims(claims auth.Claims, names ClaimNames) (Principal, bool) {
	issuer, _ := claims.GetString("iss")
	subject, _ := claims.GetString(names.Subject)
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	if issuer == "" || subject == "" {
		return Principal{}, false
	}
	return Principal{Issuer: issuer, Subject: subject, Groups: claimStrings(claims[names.Groups])}, true
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

// UserKey returns the subject key of the principal.
func (p Principal) UserKey() string {
	return UserKey(p.Issuer, p.Subject)
}

// UserKey returns the issuer-scoped subject key of a user.
func UserKey(issuer string, subject string) string {
	return TypeUser + ":" + scopedIdentifier(issuer, subject)
}

// GroupKey returns the issuer-scoped key of a group.
func GroupKey(issuer string, name string) string {
	return TypeGroup + ":" + scopedIdentifier(issuer, name)
}

// SubjectKeys returns the stored subject keys that grant the principal
// access: the user itself and the groups of the current token. Group
// memberships are never stored, so they take effect with the next token.
func (p Principal) SubjectKeys() []string {
	keys := make([]string, 0, 1+len(p.Groups))
	keys = append(keys, p.UserKey())
	for _, group := range p.Groups {
		keys = append(keys, GroupKey(p.Issuer, group))
	}
	return keys
}

// scopedIdentifier encodes issuer and value without separators. Overlong identifiers use a deterministic digest form, which can
// never collide with the plain form because base64url contains no dot.
func scopedIdentifier(issuer string, value string) string {
	encoding := base64.RawURLEncoding
	plain := encoding.EncodeToString([]byte(issuer)) + "." + encoding.EncodeToString([]byte(value))
	if len(plain) <= maxIdentifierLength {
		return plain
	}
	digest := sha256.Sum256([]byte(issuer + "\x00" + value))
	return "h." + encoding.EncodeToString(digest[:])
}

// ResourceKey returns the object key of an identifiable.
func ResourceKey(objectType string, authUUID string) string {
	return objectType + ":" + authUUID
}

// RepositoryKey returns the object key of a repository family.
func RepositoryKey(objectType string) string {
	return TypeRepository + ":" + objectType
}

// ElementKey returns the object key of a SubmodelElement path.
func ElementKey(submodelAuthUUID string, idShortPath string) string {
	digest := sha256.Sum256([]byte(idShortPath))
	return TypeElement + ":" + submodelAuthUUID + "." + hex.EncodeToString(digest[:])[:32]
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
		if objectType == TypeSubmodel || objectType == TypeElement || objectType == TypeAAS {
			return nil
		}
	}
	return fmt.Errorf("REBAC-VALIDATERELATION-UNSUPPORTED relation %q cannot be granted on %s", relation, objectType)
}
