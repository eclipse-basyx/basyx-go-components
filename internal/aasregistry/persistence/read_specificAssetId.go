package persistence_postgresql

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func readSpecificAssetIdsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.SpecificAssetId, error) {
	d := goqu.Dialect(dialect)

	sai := goqu.T(tblSpecificAssetID).As("sai")

	sqlStr, args, err := d.
		From(sai).
		Select(
			sai.Col(colID),
			sai.Col(colName),
			sai.Col(colValue),
			sai.Col(colSemanticID),
			sai.Col(colExternalSubjectRef),
		).
		Where(sai.Col(colDescriptorID).Eq(descriptorID)).
		Order(sai.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rowData struct {
		id                      int64
		name, value             sql.NullString
		semanticRefID           sql.NullInt64
		externalSubjectRefRefID sql.NullInt64
	}

	var out []model.SpecificAssetId

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.id,
			&r.name,
			&r.value,
			&r.semanticRefID,
			&r.externalSubjectRefRefID,
		); err != nil {
			return nil, err
		}

		var semanticRef *model.Reference
		if r.semanticRefID.Valid {
			if ref, err := persistence_utils.GetReferenceByReferenceDBID(db, r.semanticRefID); err == nil {
				semanticRef = ref
			} else {
				return nil, err
			}
		}

		var externalSubjectRef *model.Reference
		if r.externalSubjectRefRefID.Valid {
			if ref, err := persistence_utils.GetReferenceByReferenceDBID(db, r.externalSubjectRefRefID); err == nil {
				externalSubjectRef = ref
			} else {
				return nil, err
			}
		}

		supplemental, err := readSpecificAssetIdSupplementalSemantic(ctx, db, r.id)
		if err != nil {
			return nil, err
		}

		out = append(out, model.SpecificAssetId{
			Name:                    r.name.String,
			Value:                   r.value.String,
			SemanticId:              semanticRef,
			ExternalSubjectId:       externalSubjectRef,
			SupplementalSemanticIds: supplemental,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func readSpecificAssetIdSupplementalSemantic(
	ctx context.Context,
	db *sql.DB,
	specificAssetID int64,
) ([]model.Reference, error) {
	d := goqu.Dialect(dialect)

	ss := goqu.T(tblSpecificAssetIDSuppSemantic).As("ss")

	sqlStr, args, err := d.
		From(ss).
		Select(ss.Col(colReferenceID)).
		Where(ss.Col(colSpecificAssetIDID).Eq(specificAssetID)).
		Order(ss.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Reference
	for rows.Next() {
		var refID sql.NullInt64
		if err := rows.Scan(&refID); err != nil {
			return nil, err
		}
		if refID.Valid {
			ref, err := persistence_utils.GetReferenceByReferenceDBID(db, refID)
			if err != nil {
				return nil, err
			}
			if ref != nil {
				out = append(out, *ref)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Bulk: descriptorIDs -> []SpecificAssetId
func readSpecificAssetIdsByDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	descriptorIDs []int64,
) (map[int64][]model.SpecificAssetId, error) {
	out := make(map[int64][]model.SpecificAssetId, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}
	uniqDesc := dedupeInt64(descriptorIDs)

	d := goqu.Dialect(dialect)
	sai := goqu.T(tblSpecificAssetID).As("sai")

	// Pull all SAI rows for the descriptors; include descriptor_id for grouping.
	sqlStr, args, err := d.
		From(sai).
		Select(
			sai.Col(colDescriptorID),       // 0
			sai.Col(colID),                 // 1
			sai.Col(colName),               // 2
			sai.Col(colValue),              // 3
			sai.Col(colSemanticID),         // 4 (nullable ref id)
			sai.Col(colExternalSubjectRef), // 5 (nullable ref id)
		).
		Where(sai.Col(colDescriptorID).In(uniqDesc)).
		Order(sai.Col(colDescriptorID).Asc(), sai.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	type rowData struct {
		descID               int64
		specificID           int64
		name, value          sql.NullString
		semanticRefID        sql.NullInt64
		externalSubjectRefID sql.NullInt64
	}
	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect per-descriptor rowData and IDs for batch lookups
	perDesc := make(map[int64][]rowData, len(uniqDesc))
	allSpecificIDs := make([]int64, 0, 256)
	semRefIDs := make([]int64, 0, 128)
	extRefIDs := make([]int64, 0, 128)

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.descID,
			&r.specificID,
			&r.name,
			&r.value,
			&r.semanticRefID,
			&r.externalSubjectRefID,
		); err != nil {
			return nil, err
		}
		perDesc[r.descID] = append(perDesc[r.descID], r)
		allSpecificIDs = append(allSpecificIDs, r.specificID)
		if r.semanticRefID.Valid {
			semRefIDs = append(semRefIDs, r.semanticRefID.Int64)
		}
		if r.externalSubjectRefID.Valid {
			extRefIDs = append(extRefIDs, r.externalSubjectRefID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allSpecificIDs) == 0 {
		// Ensure keys (optional)
		for _, id := range uniqDesc {
			if _, ok := out[id]; !ok {
				out[id] = nil
			}
		}
		return out, nil
	}

	uniqSem := dedupeInt64(semRefIDs)
	uniqExt := dedupeInt64(extRefIDs)

	// Batch supplemental semantics: specific_id -> []Reference
	suppBySpecific, err := readSpecificAssetIdSupplementalSemanticBySpecificIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	// Batch main references (semantic + external). We assume a batch helper:
	//   persistence_utils.GetReferenceByReferenceDBIDs(db, ids []int64) (map[int64]*model.Reference, error)
	allRefIDs := dedupeInt64(append(append([]int64{}, uniqSem...), uniqExt...))
	refByID := make(map[int64]*model.Reference)
	if len(allRefIDs) > 0 {
		// todo: get references

	}

	// Assemble in stable order per descriptor
	for descID, rowsForDesc := range perDesc {
		for _, r := range rowsForDesc {
			var semRef *model.Reference
			if r.semanticRefID.Valid {
				semRef = refByID[r.semanticRefID.Int64]
			}
			var extRef *model.Reference
			if r.externalSubjectRefID.Valid {
				extRef = refByID[r.externalSubjectRefID.Int64]
			}

			out[descID] = append(out[descID], model.SpecificAssetId{
				Name:                    r.name.String,
				Value:                   r.value.String,
				SemanticId:              semRef,
				ExternalSubjectId:       extRef,
				SupplementalSemanticIds: suppBySpecific[r.specificID],
			})
		}
	}

	// Ensure keys exist (optional)
	for _, id := range uniqDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}

// Bulk: specificAssetIDs -> []Reference (supplemental semantics)
func readSpecificAssetIdSupplementalSemanticBySpecificIDs(
	ctx context.Context,
	db *sql.DB,
	specificAssetIDs []int64,
) (map[int64][]model.Reference, error) {
	out := make(map[int64][]model.Reference, len(specificAssetIDs))
	if len(specificAssetIDs) == 0 {
		return out, nil
	}
	uniqSpecific := dedupeInt64(specificAssetIDs)

	d := goqu.Dialect(dialect)
	ss := goqu.T(tblSpecificAssetIDSuppSemantic).As("ss")

	// Fetch all (specific_id, reference_id)
	sqlStr, args, err := d.
		From(ss).
		Select(
			ss.Col(colSpecificAssetIDID), // 0
			ss.Col(colReferenceID),       // 1 (nullable ref id)
		).
		Where(ss.Col(colSpecificAssetIDID).In(uniqSpecific)).
		Order(ss.Col(colSpecificAssetIDID).Asc(), ss.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	type linkRow struct {
		specID int64
		refID  sql.NullInt64
	}
	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := make([]linkRow, 0, 256)
	allRefIDs := make([]int64, 0, 256)

	for rows.Next() {
		var lr linkRow
		if err := rows.Scan(&lr.specID, &lr.refID); err != nil {
			return nil, err
		}
		links = append(links, lr)
		if lr.refID.Valid {
			allRefIDs = append(allRefIDs, lr.refID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Batch fetch references once
	refByID := make(map[int64]*model.Reference)
	if len(allRefIDs) > 0 {

		// todo: get references

	}

	// Group per specific asset id
	for _, lr := range links {
		if lr.refID.Valid {
			if ref := refByID[lr.refID.Int64]; ref != nil {
				out[lr.specID] = append(out[lr.specID], *ref)
			}
		}
	}

	// Ensure keys exist (optional)
	for _, id := range uniqSpecific {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}
