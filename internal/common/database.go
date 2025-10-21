package common

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

func InitializeDatabase(dsn string, schemaFilePath string) (*sql.DB, error) {
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
	if schemaFilePath == "" {
		fmt.Println("No SQL Schema passed - skipping schema loading.")
		return db, nil
	}
	queryString, fileError := os.ReadFile(schemaFilePath)

	if fileError != nil {
		return nil, fileError
	}

	_, dbError := db.Exec(string(queryString))

	if dbError != nil {
		return nil, dbError
	}
	return db, nil
}
