package common

import (
	"database/sql"
	"os"
	"time"
)

func InitializeDatabase(dsn string, schemaFileName string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	//Set Max Connection
	db.SetMaxOpenConns(500)
	db.SetMaxIdleConns(500)
	db.SetConnMaxLifetime(time.Minute * 5)

	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	dir, osErr := os.Getwd()

	if osErr != nil {
		return nil, osErr
	}

	queryString, fileError := os.ReadFile(dir + "/resources/sql/" + schemaFileName)

	if fileError != nil {
		return nil, fileError
	}

	_, dbError := db.Exec(string(queryString))

	if dbError != nil {
		return nil, dbError
	}
	return db, nil
}
