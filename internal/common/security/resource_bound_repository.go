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

package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
)

type boundQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
type boundSQL interface {
	ToSQL() (string, []interface{}, error)
}

type resourceBoundRepository struct {
	compiled      atomic.Pointer[boundCompiledSnapshot]
	db            *sql.DB
	scope         string
	router        *chi.Mux
	basePath      string
	fallback      AccessModelProvider
	implicitCasts bool
}

type boundAccess struct {
	ID       int64                    `json:"-"`
	Revision int64                    `json:"revision"`
	Resource grammar.ObjectItem       `json:"resource"`
	Policy   *ResourceBoundPolicy     `json:"localPolicy"`
	Owners   []common.AccessPrincipal `json:"owners"`
	Managers []common.AccessPrincipal `json:"managers"`
	Grants   []boundGrant             `json:"grants"`
}
type boundGrant struct {
	ID        string                 `json:"id"`
	Principal common.AccessPrincipal `json:"principal"`
	Rights    []grammar.RightsEnum   `json:"rights"`
	Rule      json.RawMessage        `json:"-"`
}

func boundExec(ctx context.Context, db boundQueryer, statement boundSQL) error {
	query, args, err := statement.ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-SQL-BUILD %w", err)
	}
	if _, err = db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("REBAC-SQL-EXEC %w", err)
	}
	return nil
}

func (repo *resourceBoundRepository) initialize(ctx context.Context, cfg common.ReBACConfig) (err error) {
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-INIT-BEGIN %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	statement := goqu.Dialect("postgres").Insert("rebac_scope").Rows(goqu.Record{"scope": repo.scope}).OnConflict(goqu.DoNothing()).Returning("scope").Prepared(true)
	query, args, err := statement.ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-INIT-BUILD %w", err)
	}
	var inserted string
	err = tx.QueryRowContext(ctx, query, args...).Scan(&inserted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("REBAC-INIT-SCOPE %w", err)
	}
	if _, err = repo.lock(ctx, tx, true); err != nil {
		return err
	}
	if err = repo.adoptResources(ctx, tx, cfg.BootstrapOwner); err != nil {
		return err
	}
	if cfg.ModelPath != "" {
		if err = repo.importInitial(ctx, tx, cfg); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("REBAC-INIT-COMMIT %w", err)
	}
	return nil
}

func (repo *resourceBoundRepository) adoptResources(ctx context.Context, tx *sql.Tx, owner common.AccessPrincipal) error {
	for _, collection := range []string{"/shells", "/submodels", "/shell-descriptors", "/submodel-descriptors", "/concept-descriptions", "/lookup/shells"} {
		if err := repo.seedCollection(ctx, tx, collection, owner); err != nil {
			return err
		}
	}
	for _, table := range []string{"aas", "submodel", "submodel_element", "aas_descriptor", "submodel_descriptor", "concept_description", "aas_identifier"} {
		if err := seedBoundResources(ctx, tx, repo.scope, table, nil, owner); err != nil {
			return err
		}
	}
	return nil
}

func (repo *resourceBoundRepository) lock(ctx context.Context, tx *sql.Tx, write bool) (int64, error) {
	ds := goqu.Dialect("postgres").From("rebac_scope").Select("revision").Where(goqu.Ex{"scope": repo.scope})
	if write {
		ds = ds.ForUpdate(goqu.Wait)
	} else {
		ds = ds.ForShare(goqu.Wait)
	}
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("REBAC-LOCK-BUILD %w", err)
	}
	var revision int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&revision); err != nil {
		return 0, fmt.Errorf("REBAC-LOCK-READ %w", err)
	}
	return revision, nil
}

func (repo *resourceBoundRepository) readRevision(ctx context.Context, db boundQueryer) (int64, error) {
	ds := goqu.Dialect("postgres").From("rebac_scope").Select("revision").Where(goqu.Ex{"scope": repo.scope})
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return 0, fmt.Errorf("REBAC-REVISION-BUILD %w", err)
	}
	var revision int64
	if err = db.QueryRowContext(ctx, query, args...).Scan(&revision); err != nil {
		return 0, fmt.Errorf("REBAC-REVISION-READ %w", err)
	}
	return revision, nil
}

func (repo *resourceBoundRepository) seedCollection(ctx context.Context, tx *sql.Tx, collection string, owner common.AccessPrincipal) error {
	ds := goqu.Dialect("postgres").Insert("rebac_access").Rows(goqu.Record{"scope": repo.scope, "collection": collection}).OnConflict(goqu.DoNothing()).Returning("id").Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-SEED-BUILD %w", err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, query, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("REBAC-SEED-INSERT %w", err)
	}
	return insertBoundOwners(ctx, tx, []int64{id}, owner)
}

func boundForeignKey(table string) string {
	switch table {
	case "submodel_element":
		return "sme_id"
	case "concept_description":
		return "concept_description_id"
	case "aas_identifier":
		return "discovery_aas_id"
	case "aas_descriptor", "submodel_descriptor":
		return table + "_id"
	}
	return table + "_id"
}

func boundSourceID(table string) string {
	if table == "aas_descriptor" || table == "submodel_descriptor" {
		return "descriptor_id"
	}
	return "id"
}

func seedBoundResources(ctx context.Context, tx *sql.Tx, scope, table string, submodelID *int64, owner common.AccessPrincipal) error {
	for {
		ids, err := insertBoundResourceBatch(ctx, tx, scope, table, submodelID)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err = authorizeBoundCreatedBindings(ctx, tx, table, ids); err != nil {
			return err
		}
		if err = insertBoundOwners(ctx, tx, ids, owner); err != nil {
			return err
		}
	}
}

func insertBoundResourceBatch(ctx context.Context, tx *sql.Tx, scope, table string, submodelID *int64) ([]int64, error) {
	dialect := goqu.Dialect("postgres")
	column := boundForeignKey(table)
	sourceID := boundSourceID(table)
	existing := dialect.From("rebac_access").Select(goqu.L("1")).Where(goqu.Ex{"scope": scope}, goqu.C(column).Eq(goqu.I("source."+sourceID)))
	source := dialect.From(goqu.T(table).As("source")).Select(goqu.V(scope), goqu.I("source."+sourceID)).Where(goqu.L("NOT EXISTS ?", existing)).Limit(1000)
	if submodelID != nil {
		field := "source.id"
		if table == "submodel_element" {
			field = "source.submodel_id"
		}
		source = source.Where(goqu.I(field).Eq(*submodelID))
	}
	ds := dialect.Insert("rebac_access").Cols("scope", column).FromQuery(source).OnConflict(goqu.DoNothing()).Returning("id").Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-SEED-BUILD %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-SEED-QUERY %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, 1000)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("REBAC-SEED-SCAN %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("REBAC-SEED-ROWS %w", err)
	}
	return ids, nil
}

func insertBoundOwners(ctx context.Context, tx *sql.Tx, ids []int64, owner common.AccessPrincipal) error {
	rows := make([]goqu.Record, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, goqu.Record{"access_id": id, "issuer": owner.Issuer, "subject": owner.Subject, "relation": "owner"})
	}
	return boundExec(ctx, tx, goqu.Dialect("postgres").Insert("rebac_principal").Rows(rows).Prepared(true))
}

func (repo *resourceBoundRepository) importInitial(ctx context.Context, tx *sql.Tx, cfg common.ReBACConfig) error {
	data, err := os.ReadFile(cfg.ModelPath)
	if err != nil {
		return fmt.Errorf("REBAC-IMPORT-READ %w", err)
	}
	models, err := ParseResourceBoundDocument(data)
	if err != nil {
		return err
	}
	for _, model := range models {
		target, err := boundTargetFromObject(model.Resource)
		if err != nil {
			return err
		}
		access, err := repo.load(ctx, tx, target)
		if err != nil {
			return err
		}
		claimed, err := claimInitialPolicyImport(ctx, tx, repo.scope, access.ID)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		access.Policy = &model
		if err = repo.save(ctx, tx, access, cfg.BootstrapOwner); err != nil {
			return err
		}
	}
	return nil
}

func claimInitialPolicyImport(ctx context.Context, tx *sql.Tx, scope string, accessID int64) (bool, error) {
	ds := goqu.Dialect("postgres").Insert("rebac_policy_import").Rows(goqu.Record{"scope": scope, "access_id": accessID}).OnConflict(goqu.DoNothing()).Returning("access_id").Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return false, fmt.Errorf("REBAC-IMPORT-BUILD %w", err)
	}
	var claimed int64
	err = tx.QueryRowContext(ctx, query, args...).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("REBAC-IMPORT-CLAIM %w", err)
	}
	return true, nil
}

func (repo *resourceBoundRepository) load(ctx context.Context, db boundQueryer, target boundTarget) (*boundAccess, error) {
	if err := validateBoundContext(ctx, db, target); err != nil {
		return nil, err
	}

	condition, err := target.condition()
	if err != nil {
		return nil, err
	}
	ds := goqu.Dialect("postgres").From("rebac_access").Select("id", "revision", "policy").Where(goqu.Ex{"scope": repo.scope}, condition).Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-LOAD-BUILD %w", err)
	}
	access := &boundAccess{Resource: target.object(), Owners: []common.AccessPrincipal{}, Managers: []common.AccessPrincipal{}, Grants: []boundGrant{}}
	var raw []byte
	if err = db.QueryRowContext(ctx, query, args...).Scan(&access.ID, &access.Revision, &raw); err != nil {
		return nil, fmt.Errorf("REBAC-LOAD-QUERY %w", err)
	}
	access.Policy, err = loadBoundPolicy(raw, access.Resource)
	if err != nil {
		return nil, err
	}
	if err = loadBoundPrincipals(ctx, db, access); err != nil {
		return nil, err
	}
	if err = loadBoundGrants(ctx, db, access); err != nil {
		return nil, err
	}
	return access, nil
}

func loadBoundPolicy(raw []byte, resource grammar.ObjectItem) (*ResourceBoundPolicy, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	policy, err := decodeBoundPolicy(raw)
	if err != nil {
		return nil, err
	}
	if !boundObjectsEqual(policy.Resource, resource) {
		policy.Resource = resource
	}
	return policy, nil
}

func loadBoundPrincipals(ctx context.Context, db boundQueryer, access *boundAccess) error {
	query, args, err := goqu.Dialect("postgres").From("rebac_principal").Select("issuer", "subject", "relation").Where(goqu.Ex{"access_id": access.ID}).Order(goqu.C("issuer").Asc(), goqu.C("subject").Asc()).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-PRINCIPALS-BUILD %w", err)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("REBAC-PRINCIPALS-QUERY %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var p common.AccessPrincipal
		var relation string
		if err = rows.Scan(&p.Issuer, &p.Subject, &relation); err != nil {
			return fmt.Errorf("REBAC-PRINCIPALS-SCAN %w", err)
		}
		if relation == "owner" {
			access.Owners = append(access.Owners, p)
		} else {
			access.Managers = append(access.Managers, p)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("REBAC-PRINCIPALS-ROWS %w", err)
	}
	return nil
}

func loadBoundGrants(ctx context.Context, db boundQueryer, access *boundAccess) error {
	query, args, err := goqu.Dialect("postgres").From("rebac_grant").Select("id", "issuer", "subject", "rights", "rule").Where(goqu.Ex{"access_id": access.ID}).Order(goqu.C("id").Asc()).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-GRANTS-BUILD %w", err)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("REBAC-GRANTS-QUERY %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var grant boundGrant
		var rights []byte
		if err = rows.Scan(&grant.ID, &grant.Principal.Issuer, &grant.Principal.Subject, &rights, &grant.Rule); err != nil {
			return fmt.Errorf("REBAC-GRANTS-SCAN %w", err)
		}
		if err = json.Unmarshal(rights, &grant.Rights); err != nil {
			return fmt.Errorf("REBAC-GRANTS-DECODE %w", err)
		}
		access.Grants = append(access.Grants, grant)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("REBAC-GRANTS-ROWS %w", err)
	}
	return nil
}

func (repo *resourceBoundRepository) save(ctx context.Context, tx *sql.Tx, access *boundAccess, actor common.AccessPrincipal) error {
	var policy any
	if access.Policy != nil {
		raw, err := json.Marshal(access.Policy)
		if err != nil {
			return fmt.Errorf("REBAC-SAVE-ENCODE %w", err)
		}
		policy = string(raw)
	}
	dialect := goqu.Dialect("postgres")
	if err := boundExec(ctx, tx, dialect.Update("rebac_access").Set(goqu.Record{"policy": policy, "revision": goqu.L("? + 1", goqu.C("revision"))}).Where(goqu.Ex{"id": access.ID}).Prepared(true)); err != nil {
		return err
	}
	if err := boundExec(ctx, tx, dialect.Update("rebac_scope").Set(goqu.Record{"revision": goqu.L("? + 1", goqu.C("revision"))}).Where(goqu.Ex{"scope": repo.scope}).Prepared(true)); err != nil {
		return err
	}
	return boundExec(ctx, tx, dialect.Insert("rebac_policy_version").Rows(goqu.Record{"scope": repo.scope, "access_id": access.ID, "revision": access.Revision + 1, "policy": policy, "actor_issuer": actor.Issuer, "actor_subject": actor.Subject}).Prepared(true))
}
