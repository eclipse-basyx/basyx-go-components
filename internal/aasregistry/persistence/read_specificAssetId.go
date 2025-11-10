package aasregistrydatabase

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

func readSpecificAssetIDsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.SpecificAssetID, error) {

	v, err := readSpecificAssetIDsByDescriptorIDs(ctx, db, []int64{descriptorID})
	return v[descriptorID], err
}

// Bulk: descriptorIDs -> []SpecificAssetId
func readSpecificAssetIDsByDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	descriptorIDs []int64,
) (map[int64][]model.SpecificAssetID, error) {
	start := time.Now()
	out := make(map[int64][]model.SpecificAssetID, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}
	uniqDesc := descriptorIDs

    d := goqu.Dialect(dialect)
    sai := goqu.T(tblSpecificAssetID).As("sai")

    arr := pq.Array(uniqDesc)
    sqlStr, args, err := d.
        From(sai).
        Select(
            sai.Col(colDescriptorID),
            sai.Col(colID),
            sai.Col(colName),
            sai.Col(colValue),
            sai.Col(colSemanticID),
            sai.Col(colExternalSubjectRef),
        ).
        Where(goqu.L("sai.descriptor_id = ANY(?::bigint[])", arr)).
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
	defer func() {
		_ = rows.Close()
	}()

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
		for _, id := range uniqDesc {
			if _, ok := out[id]; !ok {
				out[id] = nil
			}
		}
		return out, nil
	}

	uniqSem := semRefIDs
	uniqExt := extRefIDs

	suppBySpecific, err := readSpecificAssetIDSupplementalSemanticBySpecificIDs(ctx, db, allSpecificIDs)
	if err != nil {
		return nil, err
	}

	allRefIDs := append(append([]int64{}, uniqSem...), uniqExt...)
	refByID := make(map[int64]*model.Reference)
	if len(allRefIDs) > 0 {
		refByID, err = GetReferencesByIDsBatch(db, allRefIDs)
		if err != nil {
			return nil, err
		}
	}

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

			out[descID] = append(out[descID], model.SpecificAssetID{
				Name:                    nvl(r.name),
				Value:                   nvl(r.value),
				SemanticID:              semRef,
				ExternalSubjectID:       extRef,
				SupplementalSemanticIds: suppBySpecific[r.specificID],
			})
		}
	}

	for _, id := range uniqDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}

	duration := time.Since(start)
	fmt.Printf("specific assetId block took %v to complete\n", duration)
	return out, nil
}

func readSpecificAssetIDSupplementalSemanticBySpecificIDs(
	ctx context.Context,
	db *sql.DB,
	specificAssetIDs []int64,
) (map[int64][]model.Reference, error) {
	out := make(map[int64][]model.Reference, len(specificAssetIDs))
	if len(specificAssetIDs) == 0 {
		return out, nil
	}
	uniqSpecific := specificAssetIDs

	m, err := readEntityReferences1ToMany(
		ctx,
		db,
		specificAssetIDs,
		tblSpecificAssetIDSuppSemantic,
		colSpecificAssetIDID,
		colReferenceID,
	)
	if err != nil {
		return nil, err
	}

	for _, id := range uniqSpecific {
		out[id] = m[id]
	}
	return out, nil
}

func nvl(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
