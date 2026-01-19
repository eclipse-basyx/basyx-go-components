// Package helpers contains database helper functions for AAS persistence.
package helpers

import (
	"database/sql"
	"log"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

// ReadSpecificAssetIDsByAssetInfoIDs loads AssetInformation records for the given IDs.
func ReadSpecificAssetIDsByAssetInfoIDs(
	db *sql.DB,
	assetInfoIDs []int64,
) (map[int64][]model.SpecificAssetID, error) {
	out := make(map[int64][]model.SpecificAssetID)
	if len(assetInfoIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")
	arr := pq.Array(assetInfoIDs)

	ds := dialect.
		From("aas_specific_asset_id").
		Select(
			"id",
			"asset_information_id",
			"name",
			"value",
			"semantic_id",
		).
		Where(goqu.L("asset_information_id = ANY(?::bigint[])", arr))

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	// collect semantic reference IDs
	semanticRefSet := map[int64]struct{}{}

	type tmpRow struct {
		assetInfoID int64
		sid         model.SpecificAssetID
		semanticID  sql.NullInt64
	}

	var rowsTmp []tmpRow

	for rows.Next() {
		var (
			id          int64
			assetInfoID int64
			name        string
			value       string
			semanticID  sql.NullInt64
		)

		if err := rows.Scan(&id, &assetInfoID, &name, &value, &semanticID); err != nil {
			return nil, err
		}

		if semanticID.Valid {
			semanticRefSet[semanticID.Int64] = struct{}{}
		}

		rowsTmp = append(rowsTmp, tmpRow{
			assetInfoID: assetInfoID,
			sid: model.SpecificAssetID{
				Name:  name,
				Value: value,
			},
			semanticID: semanticID,
		})
	}

	// batch load references
	semanticRefIDs := make([]int64, 0, len(semanticRefSet))
	for id := range semanticRefSet {
		semanticRefIDs = append(semanticRefIDs, id)
	}

	semanticRefsByID, err := descriptors.GetReferencesByIDsBatch(db, semanticRefIDs)
	if err != nil {
		return nil, err
	}

	// attach references
	for _, r := range rowsTmp {
		if r.semanticID.Valid {
			if ref, ok := semanticRefsByID[r.semanticID.Int64]; ok {
				r.sid.SemanticID = ref
			}
		}
		out[r.assetInfoID] = append(out[r.assetInfoID], r.sid)
	}

	return out, nil
}
