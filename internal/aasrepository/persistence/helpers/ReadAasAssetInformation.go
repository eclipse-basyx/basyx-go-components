// Package helpers contains database helper functions for AAS persistence.
package helpers

import (
	"database/sql"
	"log"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

// ReadAasAssetInformationByIDs loads SpecificAssetIDs grouped by AssetInformation ID.
func ReadAasAssetInformationByIDs(
	db *sql.DB,
	assetInfoIDs []int64,
) (map[int64]*model.AssetInformation, error) {
	out := make(map[int64]*model.AssetInformation, len(assetInfoIDs))
	if len(assetInfoIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")
	arr := pq.Array(assetInfoIDs)

	ds := dialect.
		From("asset_information").
		Select(
			"id",
			"asset_kind",
			"global_asset_id",
			"asset_type",
		).
		Where(goqu.L("id = ANY(?::bigint[])", arr))

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

	for rows.Next() {
		var (
			id            int64
			assetKind     sql.NullString
			globalAssetID sql.NullString
			assetType     sql.NullString
		)

		if err := rows.Scan(&id, &assetKind, &globalAssetID, &assetType); err != nil {
			return nil, err
		}

		ai := &model.AssetInformation{
			GlobalAssetID: globalAssetID.String,
			AssetType:     assetType.String,
		}

		if assetKind.Valid {
			if k, err := model.NewAssetKindFromValue(assetKind.String); err == nil {
				ai.AssetKind = k
			}
		}

		out[id] = ai
	}

	return out, nil
}
