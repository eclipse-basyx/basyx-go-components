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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/google/uuid"
)

// Queryer is satisfied by *sql.DB and *sql.Tx.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var dialect = goqu.Dialect(common.Dialect)

// ResourceKind maps a BaSyx identifiable to its table and OpenFGA type.
type ResourceKind struct {
	Semantic         auth.SemanticResourceKind
	ObjectType       string
	Table            string
	IdentifierColumn string
}

// Resource kinds covered by ReBAC.
var (
	KindAAS = ResourceKind{
		Semantic: auth.SemanticResourceAAS, ObjectType: TypeAAS, Table: "aas", IdentifierColumn: "aas_id",
	}
	KindSubmodel = ResourceKind{
		Semantic: auth.SemanticResourceSM, ObjectType: TypeSubmodel, Table: "submodel", IdentifierColumn: "submodel_identifier",
	}
	KindConceptDescription = ResourceKind{
		Semantic: auth.SemanticResourceCD, ObjectType: TypeConceptDescription, Table: "concept_description", IdentifierColumn: "id",
	}
)

// KindForSemantic returns the covered kind of a semantic resource.
func KindForSemantic(resource auth.SemanticResourceKind) (ResourceKind, bool) {
	for _, kind := range []ResourceKind{KindAAS, KindSubmodel, KindConceptDescription} {
		if kind.Semantic == resource {
			return kind, true
		}
	}
	return ResourceKind{}, false
}

// KindForObjectType returns the covered kind of an OpenFGA object type.
func KindForObjectType(objectType string) (ResourceKind, bool) {
	for _, kind := range []ResourceKind{KindAAS, KindSubmodel, KindConceptDescription} {
		if kind.ObjectType == objectType {
			return kind, true
		}
	}
	return ResourceKind{}, false
}

// Grant is one desired relationship stored in rebac_grant.
type Grant struct {
	ObjectKey     string    `json:"-"`
	ObjectType    string    `json:"-"`
	ObjectUUID    string    `json:"-"`
	ElementPath   string    `json:"-"`
	Relation      string    `json:"relation"`
	SubjectType   string    `json:"subjectType"`
	SubjectKey    string    `json:"-"`
	SubjectIssuer string    `json:"issuer"`
	SubjectName   string    `json:"subject"`
	CreatedBy     string    `json:"createdBy,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Tuple returns the OpenFGA tuple projected from the grant.
func (g Grant) Tuple() Tuple {
	return Tuple{User: g.SubjectKey, Relation: g.Relation, Object: g.ObjectKey}
}

// SameRelationship reports whether two grants describe the same tuple.
func (g Grant) SameRelationship(other Grant) bool {
	return g.ObjectKey == other.ObjectKey && g.Relation == other.Relation && g.SubjectKey == other.SubjectKey
}

func execDataset(ctx context.Context, q Queryer, code string, ds interface {
	ToSQL() (string, []any, error)
}) (sql.Result, error) {
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("%s-BUILDQ: %w", code, err)
	}
	result, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s-EXECQ: %w", code, err)
	}
	return result, nil
}

func queryDataset(ctx context.Context, q Queryer, code string, ds *goqu.SelectDataset) (*sql.Rows, error) {
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("%s-BUILDQ: %w", code, err)
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s-EXECQ: %w", code, err)
	}
	return rows, nil
}

func queryRowDataset(ctx context.Context, q Queryer, code string, ds interface {
	ToSQL() (string, []any, error)
}, dest ...any) (bool, error) {
	query, args, err := ds.ToSQL()
	if err != nil {
		return false, fmt.Errorf("%s-BUILDQ: %w", code, err)
	}
	err = q.QueryRowContext(ctx, query, args...).Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s-EXECQ: %w", code, err)
	}
	return true, nil
}

// LookupAuthUUID resolves the authorization UUID of an identifiable. found is
// false for unknown identifiers so callers keep today's 404/403 behavior.
func LookupAuthUUID(ctx context.Context, q Queryer, kind ResourceKind, identifier string) (string, bool, error) {
	ds := dialect.From(goqu.T(kind.Table)).
		Select(goqu.L("auth_uuid::text")).
		Where(goqu.C(kind.IdentifierColumn).Eq(identifier)).
		Limit(1).Prepared(true)
	var authUUID string
	found, err := queryRowDataset(ctx, q, "REBAC-LOOKUPAUTHUUID", ds, &authUUID)
	return authUUID, found, err
}

// ExistingAuthUUIDs returns the subset of authUUIDs that still exist for kind.
func ExistingAuthUUIDs(ctx context.Context, q Queryer, kind ResourceKind, authUUIDs []string) (map[string]struct{}, error) {
	existing := make(map[string]struct{}, len(authUUIDs))
	if len(authUUIDs) == 0 {
		return existing, nil
	}
	literal, err := uuidArrayLiteral(authUUIDs)
	if err != nil {
		return nil, err
	}
	ds := dialect.From(goqu.T(kind.Table)).
		Select(goqu.L("auth_uuid::text")).
		Where(goqu.L("auth_uuid = ANY(?::uuid[])", literal))
	rows, err := queryDataset(ctx, q, "REBAC-EXISTINGAUTHUUIDS", ds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var authUUID string
		if err = rows.Scan(&authUUID); err != nil {
			return nil, fmt.Errorf("REBAC-EXISTINGAUTHUUIDS-SCAN: %w", err)
		}
		existing[authUUID] = struct{}{}
	}
	return existing, rows.Err()
}

// uuidArrayLiteral renders validated UUIDs as a PostgreSQL array parameter.
func uuidArrayLiteral(values []string) (string, error) {
	normalized := make([]string, len(values))
	for index, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return "", fmt.Errorf("REBAC-UUIDARRAY-PARSE: %w", err)
		}
		normalized[index] = parsed.String()
	}
	return "{" + strings.Join(normalized, ",") + "}", nil
}
