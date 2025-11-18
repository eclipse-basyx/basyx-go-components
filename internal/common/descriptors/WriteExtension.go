package descriptors

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func createExtensions(tx *sql.Tx, descriptorID int64, extensions []model.Extension) error {
	if extensions == nil {
		return nil
	}
	if len(extensions) > 0 {

		for id, val := range extensions {

			a, err := persistence_utils.CreateExtension(tx, val, id)

			if err != nil {
				return err
			}

			if err = createDescriptorExtensionLink(tx, descriptorID, a.Int64); err != nil {
				return err
			}

		}
	}
	return nil
}

func createDescriptorExtensionLink(tx *sql.Tx, descriptorID int64, extensionID int64) error {
	d := goqu.Dialect(dialect)
	sqlStr, args, err := d.
		Insert(tblDescriptorExtension).
		Rows(goqu.Record{
			colDescriptorID: descriptorID,
			colExtensionID:  extensionID,
		}).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}
