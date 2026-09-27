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
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// Queryer is satisfied by *sql.DB and *sql.Tx.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var dialect = goqu.Dialect(common.Dialect)

// ResourceKind maps a BaSyx resource to its rows and object type. The
// authorization UUID lives in AuthTable; identifiers may live in a joined
// table, for example for descriptors.
type ResourceKind struct {
	Semantic   auth.SemanticResourceKind
	ObjectType string
	AuthTable  string
	// Prefix and Param address one resource in the HTTP API.
	Prefix string
	Param  string
	// rows selects auth_uuid and identifier of every resource of the kind.
	rows func() *goqu.SelectDataset
}

// Resource kinds covered by ReBAC.
var (
	KindAAS = ResourceKind{
		Semantic: auth.SemanticResourceAAS, ObjectType: TypeAAS, AuthTable: "aas",
		Prefix: "/shells", Param: paramAAS, rows: plainRows("aas", "aas_id"),
	}
	KindSubmodel = ResourceKind{
		Semantic: auth.SemanticResourceSM, ObjectType: TypeSubmodel, AuthTable: "submodel",
		Prefix: "/submodels", Param: paramSubmodel, rows: plainRows("submodel", "submodel_identifier"),
	}
	KindConceptDescription = ResourceKind{
		Semantic: auth.SemanticResourceCD, ObjectType: TypeConceptDescription, AuthTable: "concept_description",
		Prefix: "/concept-descriptions", Param: paramCD, rows: plainRows("concept_description", "id"),
	}
	KindAASDescriptor = ResourceKind{
		Semantic: auth.SemanticResourceAASDesc, ObjectType: TypeAASDescriptor, AuthTable: "descriptor",
		Prefix: "/shell-descriptors", Param: paramAAS, rows: descriptorRows("aas_descriptor", nil),
	}
	KindSubmodelDescriptor = ResourceKind{
		Semantic: auth.SemanticResourceSMDesc, ObjectType: TypeSubmodelDescriptor, AuthTable: "descriptor",
		Prefix: "/submodel-descriptors", Param: paramSubmodel,
		rows: descriptorRows("submodel_descriptor", goqu.I("object_row.aas_descriptor_id").IsNull()),
	}
	KindAssetLinks = ResourceKind{
		Semantic: auth.SemanticResourceBD, ObjectType: TypeAssetLinks, AuthTable: "aas_identifier",
		Prefix: "/lookup/shells", Param: paramAAS, rows: plainRows("aas_identifier", "aasid"),
	}
	KindAASXPackage = ResourceKind{
		Semantic: auth.SemanticResourceAASXPackage, ObjectType: TypeAASXPackage, AuthTable: "aasx_package",
		Prefix: "/packages", Param: paramPackage, rows: plainRows("aasx_package", "package_id"),
	}
)

// AllKinds lists every covered resource kind.
var AllKinds = []ResourceKind{
	KindAAS, KindSubmodel, KindConceptDescription, KindAASDescriptor, KindSubmodelDescriptor, KindAssetLinks, KindAASXPackage,
}

// Rows selects object_uuid and identifier of every resource of the kind.
func (k ResourceKind) Rows() *goqu.SelectDataset {
	return k.rows()
}

func plainRows(table string, identifierColumn string) func() *goqu.SelectDataset {
	return func() *goqu.SelectDataset {
		return dialect.From(goqu.T(table).As("object_row")).
			Select(goqu.I("object_row.auth_uuid").As("object_uuid"), goqu.I("object_row."+identifierColumn).As("identifier"))
	}
}

// descriptorRows selects descriptors of one type, whose authorization UUID
// lives in the shared descriptor table.
func descriptorRows(table string, condition exp.Expression) func() *goqu.SelectDataset {
	return func() *goqu.SelectDataset {
		ds := dialect.From(goqu.T(table).As("object_row")).
			InnerJoin(goqu.T("descriptor").As("object_auth"), goqu.On(goqu.I("object_auth.id").Eq(goqu.I("object_row.descriptor_id")))).
			Select(goqu.I("object_auth.auth_uuid").As("object_uuid"), goqu.I("object_row.id").As("identifier"))
		if condition != nil {
			ds = ds.Where(condition)
		}
		return ds
	}
}

// KindForSemantic returns the covered kind of a semantic resource.
func KindForSemantic(resource auth.SemanticResourceKind) (ResourceKind, bool) {
	for _, kind := range AllKinds {
		if kind.Semantic == resource {
			return kind, true
		}
	}
	return ResourceKind{}, false
}

// KindForObjectType returns the covered kind of an object type.
func KindForObjectType(objectType string) (ResourceKind, bool) {
	for _, kind := range AllKinds {
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

// SameRelationship reports whether two grants describe the same relationship.
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
	ds := dialect.From(kind.Rows().As("resource")).
		Select(goqu.L("resource.object_uuid::text")).
		Where(goqu.I("resource.identifier").Eq(identifier)).
		Limit(1).Prepared(true)
	var authUUID string
	found, err := queryRowDataset(ctx, q, "REBAC-LOOKUPAUTHUUID", ds, &authUUID)
	return authUUID, found, err
}

// IdentifierByAuthUUID resolves the public identifier of a resource.
func IdentifierByAuthUUID(ctx context.Context, q Queryer, kind ResourceKind, authUUID string) (string, bool, error) {
	ds := dialect.From(kind.Rows().As("resource")).
		Select(goqu.I("resource.identifier")).
		Where(goqu.I("resource.object_uuid").Eq(goqu.L("?::uuid", authUUID))).
		Limit(1).Prepared(true)
	var identifier string
	found, err := queryRowDataset(ctx, q, "REBAC-IDENTIFIERBYAUTHUUID", ds, &identifier)
	return identifier, found, err
}
