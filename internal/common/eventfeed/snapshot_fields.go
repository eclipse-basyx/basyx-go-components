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

package eventfeed

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

func aasFieldsFromSnapshot(ctx context.Context, tx *sql.Tx, snap map[string]any) (aasID, globalAssetID string, submodels []SubmodelRef, err error) {
	if snap == nil {
		return "", "", nil, nil
	}
	aasID = stringField(snap, "id")
	if info, ok := snap["assetInformation"].(map[string]any); ok {
		globalAssetID = stringField(info, "globalAssetId")
	}
	submodelIDs := submodelIDsFromAASSnapshot(snap)
	submodels, err = submodelRefsForSubmodelsTx(ctx, tx, submodelIDs)
	if err != nil {
		return "", "", nil, err
	}
	return aasID, globalAssetID, submodels, nil
}

func submodelFieldsFromSnapshot(snap map[string]any) (submodelID, semanticID string) {
	if snap == nil {
		return "", ""
	}
	submodelID = stringField(snap, "id")
	if ref, ok := snap["semanticId"].(map[string]any); ok {
		semanticID = lastReferenceKeyValue(ref)
	}
	return submodelID, semanticID
}

func submodelIDsFromAASSnapshot(snap map[string]any) []string {
	raw, ok := snap["submodels"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		ref, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id := lastReferenceKeyValue(ref); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func lastReferenceKeyValue(ref map[string]any) string {
	keys, ok := ref["keys"].([]any)
	if !ok || len(keys) == 0 {
		return ""
	}
	last, ok := keys[len(keys)-1].(map[string]any)
	if !ok {
		return ""
	}
	return stringField(last, "value")
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return v
	default:
		return ""
	}
}

// submodelRefsForSubmodelsTx returns one SubmodelRef per id in submodelIDs.
// A left join is used deliberately: a submodel without a recorded semantic
// id must still appear in the result (with an empty SemanticID) rather than
// being dropped, otherwise every referring AAS/asset event would render an
// empty "submodels" array whenever none of its submodels happen to carry a
// semantic id.
func submodelRefsForSubmodelsTx(ctx context.Context, tx *sql.Tx, submodelIDs []string) ([]SubmodelRef, error) {
	if tx == nil || len(submodelIDs) == 0 {
		return nil, nil
	}
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From(goqu.T("submodel").As("s")).
		Select(goqu.I("s.submodel_identifier"), goqu.I("k.value")).
		LeftJoin(goqu.T("submodel_semantic_id_reference").As("r"), goqu.On(goqu.I("r.id").Eq(goqu.I("s.id")))).
		LeftJoin(goqu.T("submodel_semantic_id_reference_key").As("k"), goqu.On(goqu.I("k.reference_id").Eq(goqu.I("r.id")))).
		Where(goqu.I("s.submodel_identifier").In(submodelIDs)).
		Order(goqu.I("s.submodel_identifier").Asc(), goqu.I("k.position").Desc()).
		ToSQL()
	if err != nil {
		return nil, fmt.Errorf("EVENTFEED-SEMANTICIDS-BUILDSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("EVENTFEED-SEMANTICIDS-QUERY: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]struct{}, len(submodelIDs))
	out := make([]SubmodelRef, 0, len(submodelIDs))
	for rows.Next() {
		var id string
		var value sql.NullString
		if err = rows.Scan(&id, &value); err != nil {
			return nil, fmt.Errorf("EVENTFEED-SEMANTICIDS-SCAN: %w", err)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, SubmodelRef{SubmodelID: id, SemanticID: value.String})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("EVENTFEED-SEMANTICIDS-ROWS: %w", err)
	}
	return out, nil
}

func globalAssetIDsForSubmodelTx(ctx context.Context, tx *sql.Tx, submodelID string) ([]string, error) {
	if tx == nil || submodelID == "" {
		return nil, nil
	}
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From(goqu.T("aas_submodel_reference_key").As("k")).
		Select(goqu.DISTINCT(goqu.I("ai.global_asset_id"))).
		Join(goqu.T("aas_submodel_reference").As("r"), goqu.On(goqu.I("r.id").Eq(goqu.I("k.reference_id")))).
		Join(goqu.T("asset_information").As("ai"), goqu.On(goqu.I("ai.asset_information_id").Eq(goqu.I("r.aas_id")))).
		Where(
			goqu.I("k.value").Eq(submodelID),
			goqu.I("ai.global_asset_id").IsNotNull(),
			goqu.I("ai.global_asset_id").Neq(""),
		).
		ToSQL()
	if err != nil {
		return nil, fmt.Errorf("EVENTFEED-GLOBALASSET-BUILDSQL: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("EVENTFEED-GLOBALASSET-QUERY: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]string, 0, 4)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("EVENTFEED-GLOBALASSET-SCAN: %w", err)
		}
		if id != "" {
			out = append(out, id)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("EVENTFEED-GLOBALASSET-ROWS: %w", err)
	}
	return out, nil
}

