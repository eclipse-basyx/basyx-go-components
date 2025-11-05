package aasregistrydatabase

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
)

func readExtensionsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.Extension, error) {
	v, err := readExtensionsByDescriptorIDs(ctx, db, []int64{descriptorID})
	return v[descriptorID], err
}

func readExtensionsByDescriptorIDs(
    ctx context.Context,
    db *sql.DB,
    descriptorIDs []int64,
) (map[int64][]model.Extension, error) {
    start := time.Now()
    out := make(map[int64][]model.Extension, len(descriptorIDs))
    if len(descriptorIDs) == 0 {
        return out, nil
    }
    uniqDesc := descriptorIDs

    d := goqu.Dialect(dialect)

    // Build a single grouped query that aggregates extensions per descriptor_id
    ds := queries.GetExtensionsGroupedByEntityIDs(
        d,
        goqu.T(tblDescriptorExtension),
        "extension_id",
        "descriptor_id",
        uniqDesc,
    )

	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("building SQL failed: %w", err)
	}
    rows, err := db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("querying extension information failed: %w", err)
    }

	defer func() {
		_ = rows.Close()
	}()

	type row struct {
		ID         int64
		Extensions json.RawMessage
	}

	for rows.Next() {

		var r row
		if err := rows.Scan(&r.ID, &r.Extensions); err != nil {
			return nil, fmt.Errorf("scanning extension row failed: %w", err)
		}

        // Extensions
        if common.IsArrayNotEmpty(r.Extensions) {
            builder := builders.NewExtensionsBuilder()
            extensionRows, err := builders.ParseExtensionRows(r.Extensions)
            if err != nil {
                return nil, err
            }
            for _, extensionRow := range extensionRows {
                _, err = builder.AddExtension(extensionRow.DbID, extensionRow.Name, extensionRow.ValueType, extensionRow.Value, extensionRow.Position)
                if err != nil {
                    return nil, err
                }

                _, err = builder.AddSemanticID(extensionRow.DbID, extensionRow.SemanticID, extensionRow.SemanticIDReferredReferences)
                if err != nil {
                    return nil, err
                }

                _, err = builder.AddRefersTo(extensionRow.DbID, extensionRow.RefersTo, extensionRow.RefersToReferredReferences)
                if err != nil {
                    return nil, err
                }

                _, err = builder.AddSupplementalSemanticIDs(extensionRow.DbID, extensionRow.SupplementalSemanticIDs, extensionRow.SupplementalSemanticIDsReferredReferences)
                if err != nil {
                    return nil, err
                }
            }
            out[r.ID] = builder.Build()
        }
    }

    duration := time.Since(start)
    fmt.Printf("extension block took %v to complete\n", duration)
    return out, nil
}
