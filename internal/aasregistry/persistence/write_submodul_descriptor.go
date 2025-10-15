package persistence_postgresql

import (
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func createSubModelDescriptors(tx *sql.Tx, aasDescriptorId int64, submodelDescriptors []model.SubmodelDescriptor) error {
	if submodelDescriptors == nil {
		return nil
	}
	if len(submodelDescriptors) > 0 {
		d := goqu.Dialect(dialect)
		for _, val := range submodelDescriptors {
			var (
				semanticId       sql.NullInt64
				displayNameId    sql.NullInt64
				descriptionId    sql.NullInt64
				administrationId sql.NullInt64
				err              error
			)

			displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, val.DisplayName)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
			}

			descriptionId, err = persistence_utils.CreateLangStringTextTypes(tx, val.Description)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
			}

			administrationId, err = CreateAdministrativeInformation(tx, &val.Administration)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
			}

			semanticId, err = persistence_utils.CreateReference(tx, val.SemanticId)
			if err != nil {
				return err
			}

			sqlStr, args, err := d.
				Insert(tblDescriptor).
				Returning(tDescriptor.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var submodelDescriptorId int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&submodelDescriptorId); err != nil {
				return err
			}

			sqlStr, args, err = d.
				Insert(tblSubmodelDescriptor).
				Rows(goqu.Record{
					colDescriptorID:    submodelDescriptorId,
					colAASDescriptorID: aasDescriptorId,
					colDescriptionID:   descriptionId,
					colDisplayNameID:   displayNameId,
					colAdminInfoID:     administrationId,
					colIdShort:         val.IdShort,
					colAASID:           val.Id,
					colSemanticID:      semanticId,
				}).
				ToSQL()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(sqlStr, args...); err != nil {
				return err
			}

			if err = createsubModelDescriptorSupplementalSemantic(tx, submodelDescriptorId, val.SupplementalSemanticId); err != nil {
				return err
			}

			if err = createExtensions(tx, submodelDescriptorId, val.Extensions); err != nil {
				return err
			}

			if len(val.Endpoints) <= 0 {
				return common.NewErrBadRequest("Submodel Descriptor needs at least 1 Endpoint.")
			}
			if err = createEndpoints(tx, submodelDescriptorId, val.Endpoints); err != nil {
				return err
			}
		}
	}
	return nil
}

func createsubModelDescriptorSupplementalSemantic(tx *sql.Tx, subModelDescriptorId int64, references []model.Reference) error {
	if len(references) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(references))
	for i := range references {
		referenceID, err := persistence_utils.CreateReference(tx, &references[i])
		if err != nil {
			return err
		}
		rows = append(rows, goqu.Record{
			colDescriptorID: subModelDescriptorId,
			colReferenceID:  referenceID,
		})
	}
	sqlStr, args, err := d.Insert(tblSubmodelDescriptorSuppSemantic).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}
