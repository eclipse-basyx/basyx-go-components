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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

type catalogSource struct{ kind, table, column string }

var catalogSources = []catalogSource{
	{"aas", "aas", "aas_id"}, {"submodel", "submodel", "submodel_identifier"},
	{"aas_descriptor", "aas_descriptor", "id"}, {"submodel_descriptor", "submodel_descriptor", "id"},
	{"concept_description", "concept_description", "id"}, {"discovery", "aas_identifier", "aasid"}, {"package", "aasx_package", "package_id"},
}

// Bootstrap assigns authorization identities to existing resources without inferring grants.
func (r *Runtime) Bootstrap(ctx context.Context) error {
	tx, err := r.State.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("REBAC-CATALOG-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := r.State.Lock(ctx, tx, true)
	if err != nil {
		return err
	}
	if state.Desired != state.Applied {
		return ErrPending
	}
	for _, source := range catalogSources {
		if err = r.bootstrapSource(ctx, tx, source); err != nil {
			return err
		}
	}
	for _, kind := range []string{"aas", "submodel", "aas_descriptor", "submodel_descriptor", "concept_description", "discovery", "package"} {
		if _, _, err = r.State.EnsureResource(ctx, tx, "repository", kind, "", ""); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("REBAC-CATALOG-COMMIT: %w", err)
	}
	return nil
}

func (r *Runtime) bootstrapSource(ctx context.Context, tx *sql.Tx, source catalogSource) error {
	after := ""
	for {
		ids, err := catalogIdentifiers(ctx, tx, source, after)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			if err := r.bootstrapResource(ctx, tx, source.kind, id); err != nil {
				return err
			}
		}
		after = ids[len(ids)-1]
	}
}

func catalogIdentifiers(ctx context.Context, tx *sql.Tx, source catalogSource, after string) ([]string, error) {
	query := stateDialect.From(source.table).Select(source.column).Distinct().Where(goqu.C(source.column).Gt(after)).Order(goqu.C(source.column).Asc()).Limit(500)
	if source.kind == "submodel_descriptor" {
		query = query.Where(goqu.C("aas_descriptor_id").IsNull())
	}
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-CATALOG-QUERY: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-CATALOG-READ: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("REBAC-CATALOG-SCAN: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// SyncElements maintains identities and containment in the same Submodel transaction.
func (r *Runtime) SyncElements(ctx context.Context, tx *sql.Tx, submodel StoredResource, creator string) ([]Tuple, []Tuple, error) {
	elements, err := loadElementCatalog(ctx, tx, submodel.Identifier)
	if err != nil {
		return nil, nil, err
	}
	writes := []Tuple{}
	parents := map[int64]StoredResource{}
	live := map[string]bool{}
	for _, element := range elements {
		parent := submodel
		if element.parent.Valid {
			var ok bool
			parent, ok = parents[element.parent.Int64]
			if !ok {
				return nil, nil, fmt.Errorf("REBAC-CATALOG-PARENT missing containing element")
			}
		}
		id := ElementIdentifier(submodel.Identifier, element.path)
		resource, tuples, ensureErr := r.State.EnsureResource(ctx, tx, "element", id, parent.UUID, creator)
		if ensureErr != nil {
			return nil, nil, ensureErr
		}
		parentObject, objectErr := ResourceObject(r.State.Scope, parent.Kind, parent.UUID)
		if objectErr != nil {
			return nil, nil, objectErr
		}
		object, objectErr := ResourceObject(r.State.Scope, "element", resource.UUID)
		if objectErr != nil {
			return nil, nil, objectErr
		}
		writes = append(writes, tuples...)
		writes = append(writes, Tuple{User: parentObject, Relation: "parent", Object: object})
		parents[element.id] = resource
		live[id] = true
	}
	deletes, err := r.retireAbsentElements(ctx, tx, submodel.Identifier, live)
	return writes, deletes, err
}

type elementCatalogRow struct {
	id     int64
	parent sql.NullInt64
	path   string
}

func loadElementCatalog(ctx context.Context, tx *sql.Tx, identifier string) ([]elementCatalogRow, error) {
	statement, args, err := stateDialect.From(goqu.T("submodel_element").As("e")).Join(goqu.T("submodel").As("s"), goqu.On(goqu.I("e.submodel_id").Eq(goqu.I("s.id")))).Select(goqu.I("e.id"), goqu.I("e.parent_sme_id"), goqu.I("e.idshort_path")).Where(goqu.I("s.submodel_identifier").Eq(identifier)).Order(goqu.I("e.depth").Asc(), goqu.I("e.id").Asc()).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-CATALOG-ELEMENTQUERY: %w", err)
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-CATALOG-ELEMENTREAD: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := []elementCatalogRow{}
	for rows.Next() {
		var row elementCatalogRow
		if err = rows.Scan(&row.id, &row.parent, &row.path); err != nil {
			return nil, fmt.Errorf("REBAC-CATALOG-ELEMENTSCAN: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *Runtime) retireAbsentElements(ctx context.Context, tx *sql.Tx, identifier string, live map[string]bool) ([]Tuple, error) {
	deletes := []Tuple{}
	after := ""
	for {
		resources, err := r.State.Resources(ctx, tx, "element", after, 500)
		if err != nil {
			return nil, err
		}
		if len(resources) == 0 {
			return deletes, nil
		}
		retired, err := r.retireElementBatch(ctx, tx, identifier, live, resources)
		if err != nil {
			return nil, err
		}
		deletes = append(deletes, retired...)

		after = resources[len(resources)-1].UUID
	}
}

func (r *Runtime) bootstrapResource(ctx context.Context, tx *sql.Tx, kind, id string) error {
	resource, _, err := r.State.EnsureResource(ctx, tx, kind, id, "", "")
	if err != nil {
		return err
	}
	if kind == "aas_descriptor" {
		writes, deletes, err := r.SyncEmbeddedDescriptors(ctx, tx, resource, "", false)
		if err != nil {
			return err
		}
		_, err = r.State.QueueChanged(ctx, tx, writes, deletes)
		return err
	}
	if kind != "submodel" {
		return nil
	}
	writes, deletes, err := r.SyncElements(ctx, tx, resource, "")
	if err != nil {
		return err
	}
	_, err = r.State.QueueChanged(ctx, tx, writes, deletes)
	return err
}

func (r *Runtime) retireElementBatch(ctx context.Context, tx *sql.Tx, identifier string, live map[string]bool, resources []StoredResource) ([]Tuple, error) {
	var deletes []Tuple
	for _, resource := range resources {
		var identity []string
		if err := json.Unmarshal([]byte(resource.Identifier), &identity); err != nil || len(identity) != 2 {
			return nil, fmt.Errorf("REBAC-CATALOG-ELEMENTIDENTITY invalid identity")
		}
		if identity[0] != identifier || live[resource.Identifier] {
			continue
		}
		tuples, err := r.State.RetireResource(ctx, tx, resource)
		if err != nil {
			return nil, err
		}
		deletes = append(deletes, tuples...)
	}
	return deletes, nil
}
