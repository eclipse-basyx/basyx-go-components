package common

import "database/sql"

// ExecuteInTransaction starts a transaction, executes fn, and commits on success.
func ExecuteInTransaction(db *sql.DB, startErrorCode string, commitErrorCode string, fn func(tx *sql.Tx) error) (err error) {
	if db == nil {
		return NewErrBadRequest("COMMON-EXECINTX-NILDB database handle must not be nil")
	}
	if fn == nil {
		return NewErrBadRequest("COMMON-EXECINTX-NILFN transaction callback must not be nil")
	}

	tx, cleanup, err := StartTransaction(db)
	if err != nil {
		if startErrorCode == "" {
			return NewInternalServerError("COMMON-EXECINTX-STARTTX " + err.Error())
		}
		return NewInternalServerError(startErrorCode + " " + err.Error())
	}
	defer cleanup(&err)

	err = fn(tx)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		if commitErrorCode == "" {
			return NewInternalServerError("COMMON-EXECINTX-COMMIT " + err.Error())
		}
		return NewInternalServerError(commitErrorCode + " " + err.Error())
	}

	return nil
}
