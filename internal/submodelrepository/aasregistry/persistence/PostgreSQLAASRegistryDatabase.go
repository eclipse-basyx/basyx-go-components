package persistence_postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLAASRegistryDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

func NewPostgreSQLAASRegistryDatabase(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool) (*PostgreSQLAASRegistryDatabase, error) {
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

	queryString, fileError := os.ReadFile(dir + "/resources/sql/aasregistryschema.sql")

	if fileError != nil {
		return nil, fileError
	}

	_, dbError := db.Exec(string(queryString))

	if dbError != nil {
		return nil, dbError
	}

	return &PostgreSQLAASRegistryDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptor(ctx context.Context, aasd model.AssetAdministrationShellDescriptor) error {

	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {

		if err != nil {
			tx.Rollback()
		}
	}()

	desc := aasd.Description

	fmt.Println(desc)

	var referenceID, displayNameId, descriptionId, administrationId sql.NullInt64
	displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, aasd.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Handle possibly nil Description
	descriptionId, err = persistence_utils.CreateLangStringTextTypes(tx, aasd.Description)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationId, err = persistence_utils.CreateAdministrativeInformation(tx, &aasd.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	fmt.Println(referenceID)
	fmt.Println(displayNameId)
	fmt.Println(descriptionId)
	fmt.Println(administrationId)

	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}
