package persistence_postgresql

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func createSpecificAssetId(tx *sql.Tx, descriptorId int64, specificAssetIds []model.SpecificAssetID) error {
	if specificAssetIds == nil {
		return nil
	}
	if len(specificAssetIds) > 0 {
		d := goqu.Dialect(dialect)
		for _, val := range specificAssetIds {
			var a sql.NullInt64

			externalSubjectReferenceId, err := persistence_utils.CreateReference(tx, val.ExternalSubjectID, a, a)
			if err != nil {
				return err
			}
			semanticId, err := persistence_utils.CreateReference(tx, val.SemanticID, a, a)
			if err != nil {
				return err
			}

			sqlStr, args, err := d.
				Insert(tblSpecificAssetID).
				Rows(goqu.Record{
					colDescriptorID:       descriptorId,
					colSemanticID:         semanticId,
					colName:               val.Name,
					colValue:              val.Value,
					colExternalSubjectRef: externalSubjectReferenceId,
				}).
				Returning(tSpecificAssetID.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&id); err != nil {
				return err
			}

			if err = createSpecificAssetIdSupplementalSemantic(tx, id, val.SupplementalSemanticIds); err != nil {
				return err
			}
		}
	}
	return nil
}

func createSpecificAssetIdSupplementalSemantic(tx *sql.Tx, specificAssetId int64, references []model.Reference) error {
	if len(references) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(references))
	for i := range references {
		var a sql.NullInt64
		referenceID, err := persistence_utils.CreateReference(tx, &references[i], a, a)
		if err != nil {
			return err
		}
		rows = append(rows, goqu.Record{
			colSpecificAssetIDID: specificAssetId,
			colReferenceID:       referenceID,
		})
	}
	sqlStr, args, err := d.Insert(tblSpecificAssetIDSuppSemantic).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}
