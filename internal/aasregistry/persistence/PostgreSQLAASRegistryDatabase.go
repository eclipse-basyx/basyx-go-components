// Package aasregistrydatabase provides a PostgreSQL-backed persistence layer
// for the AAS Registry. It offers creation, retrieval, listing, replacement,
// and deletion of Asset Administration Shell (AAS) descriptors and their
// related entities (endpoints, specific asset IDs, extensions, and submodel
// descriptors). The package uses goqu to build SQL and database/sql for query
// execution, and applies cursor-based pagination where appropriate.
package aasregistrydatabase

import (
	"context"
	"database/sql"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// PostgreSQLAASRegistryDatabase is a PostgreSQL-backed implementation of the AAS
// registry database. It is safe for concurrent use by multiple goroutines.
type PostgreSQLAASRegistryDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

// NewPostgreSQLAASRegistryDatabase creates a new PostgreSQL-backed AAS registry
// database handle. It initializes the database using the provided DSN and
// schema path (or the default bundled schema when empty), and configures the
// connection pool according to the supplied limits. The returned instance can
// be used concurrently by multiple goroutines.
func NewPostgreSQLAASRegistryDatabase(
	dsn string,
	maxOpenConns, maxIdleConns int,
	connMaxLifetimeMinutes int,
	cacheEnabled bool,
	databaseSchema string,
) (*PostgreSQLAASRegistryDatabase, error) {

	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	if maxOpenConns > 0 {
		db.SetMaxOpenConns(maxOpenConns)
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	if connMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetimeMinutes) * time.Minute)
	}

	return &PostgreSQLAASRegistryDatabase{
		db:           db,
		cacheEnabled: cacheEnabled,
	}, nil
}

// InsertAdministrationShellDescriptor inserts the provided AAS descriptor
// and all related nested entities into the database.
func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptor(
	ctx context.Context,
	aasd model.AssetAdministrationShellDescriptor,
) error {
	return descriptors.InsertAdministrationShellDescriptor(ctx, p.db, aasd)
}

// GetAssetAdministrationShellDescriptorByID returns the AAS descriptor
// identified by the given AAS ID.
func (p *PostgreSQLAASRegistryDatabase) GetAssetAdministrationShellDescriptorByID(
	ctx context.Context,
	aasIdentifier string,
) (model.AssetAdministrationShellDescriptor, error) {
	return descriptors.GetAssetAdministrationShellDescriptorByID(ctx, p.db, aasIdentifier)
}

// DeleteAssetAdministrationShellDescriptorByID deletes the AAS descriptor
// identified by the given AAS ID.
func (p *PostgreSQLAASRegistryDatabase) DeleteAssetAdministrationShellDescriptorByID(
	ctx context.Context,
	aasIdentifier string,
) error {
	return descriptors.DeleteAssetAdministrationShellDescriptorByID(ctx, p.db, aasIdentifier)
}

// ReplaceAdministrationShellDescriptor replaces an existing AAS descriptor
// with the given value and reports whether it existed.
func (p *PostgreSQLAASRegistryDatabase) ReplaceAdministrationShellDescriptor(
	ctx context.Context,
	aasd model.AssetAdministrationShellDescriptor,
) (bool, error) {
	return descriptors.ReplaceAdministrationShellDescriptor(ctx, p.db, aasd)
}

// ListAssetAdministrationShellDescriptors lists AAS descriptors with optional
// pagination and asset filtering, returning a next-page cursor when present.
func (p *PostgreSQLAASRegistryDatabase) ListAssetAdministrationShellDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
) ([]model.AssetAdministrationShellDescriptor, string, error) {
	return descriptors.ListAssetAdministrationShellDescriptors(ctx, p.db, limit, cursor, assetKind, assetType)
}

// ListSubmodelDescriptorsForAAS lists submodel descriptors for a given AAS ID
// with optional pagination, returning a next-page cursor when present.
func (p *PostgreSQLAASRegistryDatabase) ListSubmodelDescriptorsForAAS(
	ctx context.Context,
	aasID string,
	limit int32,
	cursor string,
) ([]model.SubmodelDescriptor, string, error) {
	return descriptors.ListSubmodelDescriptorsForAAS(ctx, p.db, aasID, limit, cursor)
}

// InsertSubmodelDescriptorForAAS inserts a submodel descriptor and associates
// it with the specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) InsertSubmodelDescriptorForAAS(
	ctx context.Context,
	aasID string,
	submodel model.SubmodelDescriptor,
) error {
	return descriptors.InsertSubmodelDescriptorForAAS(ctx, p.db, aasID, submodel)
}

// ReplaceSubmodelDescriptorForAAS replaces a submodel descriptor for the given
// AAS ID and reports whether it existed.
func (p *PostgreSQLAASRegistryDatabase) ReplaceSubmodelDescriptorForAAS(
	ctx context.Context,
	aasID string,
	submodel model.SubmodelDescriptor,
) (bool, error) {
	return descriptors.ReplaceSubmodelDescriptorForAAS(ctx, p.db, aasID, submodel)
}

// GetSubmodelDescriptorForAASByID returns the submodel descriptor identified
// by the submodel ID for the specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) GetSubmodelDescriptorForAASByID(
	ctx context.Context,
	aasID string,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	return descriptors.GetSubmodelDescriptorForAASByID(ctx, p.db, aasID, submodelID)
}

// DeleteSubmodelDescriptorForAASByID deletes the submodel descriptor identified
// by submodel ID for the specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) DeleteSubmodelDescriptorForAASByID(
	ctx context.Context,
	aasID string,
	submodelID string,
) error {
	return descriptors.DeleteSubmodelDescriptorForAASByID(ctx, p.db, aasID, submodelID)
}

// ExistsAASByID reports whether an AAS with the given ID exists.
func (p *PostgreSQLAASRegistryDatabase) ExistsAASByID(
	ctx context.Context,
	aasID string,
) (bool, error) {
	return descriptors.ExistsAASByID(ctx, p.db, aasID)
}

// ExistsSubmodelForAAS reports whether the given submodel ID exists for the
// specified AAS ID.
func (p *PostgreSQLAASRegistryDatabase) ExistsSubmodelForAAS(
	ctx context.Context,
	aasID,
	submodelID string,
) (bool, error) {
	return descriptors.ExistsSubmodelForAAS(ctx, p.db, aasID, submodelID)
}
