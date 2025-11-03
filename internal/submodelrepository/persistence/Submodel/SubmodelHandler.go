/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package submodel_persistence

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	submodel_query "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/submodelQueries"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

func GetSubmodelByID(db *sql.DB, submodelIdFilter string) (*gen.Submodel, error) {
	submodels, _, err := getSubmodels(db, submodelIdFilter, 1, "", nil)
	if err != nil {
		return nil, err
	}
	if len(submodels) == 0 {
		return nil, common.NewErrNotFound(submodelIdFilter)
	}
	return submodels[0], nil
}

func GetAllSubmodels(db *sql.DB, limit int64, cursor string, query *grammar.QueryWrapper) ([]*gen.Submodel, string, error) {
	return getSubmodels(db, "", limit, cursor, query)
}

// getSubmodels retrieves submodels from the database with full nested structures.
//
// This function performs a complex query to fetch submodels along with all their related
// data including display names, descriptions, semantic IDs, supplemental semantic IDs,
// and embedded data specifications. It handles the reconstruction of nested reference
// structures and language strings from normalized database tables.
//
// Parameters:
//   - db: Database connection to execute the query against
//   - submodelIdFilter: Optional filter for a specific submodel ID. If empty, all submodels are retrieved.
//   - limit: Maximum number of results to return (0 means no limit)
//   - cursor: The submodel ID to start pagination from (empty string means start from beginning)
//   - query: Optional AAS QueryLanguage filtering
//
// Returns:
//   - []*gen.Submodel: Slice of fully populated Submodel objects with all nested structures
//   - string: Next cursor for pagination (empty string if no more pages)
//   - error: An error if database query fails, scanning fails, or data parsing fails
//
// The function:
//   - Executes an optimized SQL query with JSON aggregation for nested data
//   - Pre-sizes result slices based on total count for better performance
//   - Builds reference hierarchies using ReferenceBuilder instances
//   - Parses JSON-encoded language strings and references
//   - Measures and logs query execution time for performance monitoring
//   - Implements cursor-based pagination with peek ahead pattern
//
// Note: The function builds nested reference structures in two phases:
//  1. Initial parsing during row iteration
//  2. Final structure building after all rows are processed
func getSubmodels(db *sql.DB, submodelIdFilter string, limit int64, cursor string, query *grammar.QueryWrapper) ([]*gen.Submodel, string, error) {
	var result []*gen.Submodel
	referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	start := time.Now().Local().UnixMilli()
	rows, err := getSubmodelDataFromDbWithJSONQuery(db, submodelIdFilter, limit, cursor, query)
	end := time.Now().Local().UnixMilli()
	fmt.Printf("Total Query Only time: %d milliseconds\n", end-start)
	if err != nil {
		return nil, "", fmt.Errorf("error getting submodel data from DB: %w", err)
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		count++
		var row builders.SubmodelRow
		if err := rows.Scan(
			&row.ID, &row.IDShort, &row.Category, &row.Kind,
			&row.DisplayNames, &row.Descriptions,
			&row.SemanticID, &row.ReferredSemanticIDs,
			&row.SupplementalSemanticIDs, &row.SupplementalReferredSemIDs,
			&row.DataSpecReference, &row.DataSpecReferenceReferred,
			&row.DataSpecIEC61360, &row.Qualifiers, &row.Extensions, &row.Administration, &row.RootSubmodelElements, &row.ChildSubmodelElements, &row.TotalSubmodels,
		); err != nil {
			return nil, "", fmt.Errorf("error scanning row: %w", err)
		}

		if result == nil {
			result = make([]*gen.Submodel, 0, row.TotalSubmodels)
		}

		submodel := &gen.Submodel{
			ModelType: "Submodel",
			ID:        row.ID,
			IdShort:   row.IDShort,
			Category:  row.Category,
			Kind:      gen.ModellingKind(row.Kind),
		}
		if count > limit {
			result = append(result, submodel)
			break
		}
		var semanticID []*gen.Reference
		if isArrayNotEmpty(row.SemanticID) {
			semanticID, err = builders.ParseReferences(row.SemanticID, referenceBuilderRefs)
			if err != nil {
				return nil, "", err
			}
			if hasSemanticID(semanticID) {
				submodel.SemanticID = semanticID[0]
				builders.ParseReferredReferences(row.ReferredSemanticIDs, referenceBuilderRefs)
			}
		}

		if isArrayNotEmpty(row.SupplementalSemanticIDs) {
			supplementalSemanticIds, err := builders.ParseReferences(row.SupplementalSemanticIDs, referenceBuilderRefs)
			if err != nil {
				return nil, "", err
			}
			if moreThanZeroReferences(supplementalSemanticIds) {
				submodel.SupplementalSemanticIds = supplementalSemanticIds
				builders.ParseReferredReferences(row.SupplementalReferredSemIDs, referenceBuilderRefs)
			}
		}

		// DisplayNames
		err = addDisplayNames(row, submodel)
		if err != nil {
			return nil, "", err
		}

		// Descriptions
		err = addDescriptions(row, submodel)
		if err != nil {
			return nil, "", err
		}

		// Embedded Data Specifications
		if isArrayNotEmpty(row.DataSpecReference) {
			builder := builders.NewEmbeddedDataSpecificationsBuilder()
			err := builder.BuildReferences(row.DataSpecReference, row.DataSpecReferenceReferred)
			if err != nil {
				fmt.Println(err)
				return nil, "", err
			}
			err = builder.BuildContentsIec61360(row.DataSpecIEC61360)
			if err != nil {
				fmt.Println(err)
				return nil, "", err
			}
			submodel.EmbeddedDataSpecifications = builder.Build()
		}

		// Qualifiers
		if isArrayNotEmpty(row.Qualifiers) {
			builder := builders.NewQualifiersBuilder()
			qualifierRows, err := builders.ParseQualifiersRow(row.Qualifiers)
			if err != nil {
				return nil, "", err
			}
			for _, qualifierRow := range qualifierRows {
				builder.AddQualifier(qualifierRow.DbID, qualifierRow.Kind, qualifierRow.Type, qualifierRow.ValueType, qualifierRow.Value)

				builder.AddSemanticID(qualifierRow.DbID, qualifierRow.SemanticID, qualifierRow.SemanticIDReferredReferences)

				builder.AddValueID(qualifierRow.DbID, qualifierRow.ValueID, qualifierRow.ValueIDReferredReferences)

				builder.AddSupplementalSemanticIds(qualifierRow.DbID, qualifierRow.SupplementalSemanticIDs, qualifierRow.SupplementalSemanticIDsReferredReferences)
			}
			submodel.Qualifier = builder.Build()
		}

		// Extensions
		if isArrayNotEmpty(row.Extensions) {
			builder := builders.NewExtensionsBuilder()
			extensionRows, err := builders.ParseExtensionRows(row.Extensions)
			if err != nil {
				return nil, "", err
			}
			for _, extensionRow := range extensionRows {
				builder.AddExtension(extensionRow.DbID, extensionRow.Name, extensionRow.ValueType, extensionRow.Value)

				builder.AddSemanticID(extensionRow.DbID, extensionRow.SemanticID, extensionRow.SemanticIDReferredReferences)

				builder.AddRefersTo(extensionRow.DbID, extensionRow.RefersTo, extensionRow.RefersToReferredReferences)

				builder.AddSupplementalSemanticIds(extensionRow.DbID, extensionRow.SupplementalSemanticIDs, extensionRow.SupplementalSemanticIDsReferredReferences)
			}
			submodel.Extension = builder.Build()
		}

		// Administration
		if isArrayNotEmpty(row.Administration) {
			adminRow, err := builders.ParseAdministrationRow(row.Administration)
			if err != nil {
				fmt.Println(err)
				return nil, "", err
			}
			if adminRow != nil {

				admin, err := builders.BuildAdministration(*adminRow)
				if err != nil {
					fmt.Println(err)
					return nil, "", err
				}
				submodel.Administration = admin
			} else {
				fmt.Println("Administration row is nil")
			}
		}

		result = append(result, submodel)
	}

	for _, referenceBuilder := range referenceBuilderRefs {
		referenceBuilder.BuildNestedStructure()
	}

	// Handle pagination with peek ahead pattern
	var nextCursor string
	if limit > 0 && len(result) > int(limit) {
		// We have more results than requested, so there's a next page
		actualResults := result[:limit]
		nextCursor = result[limit].ID // Use the ID of the next result as cursor
		return actualResults, nextCursor, nil
	}

	// No more pages
	return result, "", nil
}

// addDisplayNames parses and adds display names to a submodel.
//
// This helper function extracts language-specific display names from the database
// row and adds them to the submodel object. It only processes the data if the
// display names array is not empty.
//
// Parameters:
//   - row: SubmodelRow containing JSON-encoded display names data
//   - submodel: Submodel object to add the display names to
//
// Returns:
//   - error: An error if parsing the language strings fails, nil otherwise
func addDisplayNames(row builders.SubmodelRow, submodel *gen.Submodel) error {
	if isArrayNotEmpty(row.DisplayNames) {
		displayNames, err := builders.ParseLangStringNameType(row.DisplayNames)
		if err != nil {
			return fmt.Errorf("error parsing display names: %w", err)
		}
		submodel.DisplayName = displayNames
	}
	return nil
}

// addDescriptions parses and adds descriptions to a submodel.
//
// This helper function extracts language-specific descriptions from the database
// row and adds them to the submodel object. It only processes the data if the
// descriptions array is not empty.
//
// Parameters:
//   - row: SubmodelRow containing JSON-encoded descriptions data
//   - submodel: Submodel object to add the descriptions to
//
// Returns:
//   - error: An error if parsing the language strings fails, nil otherwise
func addDescriptions(row builders.SubmodelRow, submodel *gen.Submodel) error {
	if isArrayNotEmpty(row.Descriptions) {
		descriptions, err := builders.ParseLangStringTextType(row.Descriptions)
		if err != nil {
			return fmt.Errorf("error parsing descriptions: %w", err)
		}
		submodel.Description = descriptions
	}
	return nil
}

// isArrayNotEmpty checks if a JSON array contains data.
//
// This utility function determines whether a JSON RawMessage contains an actual
// array with data, as opposed to being empty or containing a null value.
//
// Parameters:
//   - data: JSON RawMessage to check
//
// Returns:
//   - bool: true if the data is not empty and not "null", false otherwise
func isArrayNotEmpty(data json.RawMessage) bool {
	return len(data) > 0 && string(data) != "null"
}

// hasSemanticID validates that exactly one semantic ID reference exists.
//
// According to the AAS specification, a submodel should have exactly one semantic ID.
// This function checks that the parsed semantic ID data contains exactly one reference.
//
// Parameters:
//   - semanticIdData: Slice of Reference objects parsed from semantic ID data
//
// Returns:
//   - bool: true if exactly one semantic ID reference exists, false otherwise
func hasSemanticID(semanticIdData []*gen.Reference) bool {
	return len(semanticIdData) == 1
}

// moreThanZeroReferences checks if References exist.
//
// Parameters:
//   - referenceArray: Slice of Reference objects
//
// Returns:
//   - bool: true if at least one Reference exists, false otherwise
func moreThanZeroReferences(referenceArray []*gen.Reference) bool {
	return len(referenceArray) > 0
}

// getSubmodelDataFromDbWithJSONQuery executes the submodel query against the database.
//
// This function builds and executes a complex SQL query that retrieves submodel data
// with all nested structures aggregated as JSON. It serves as a bridge between the
// query building logic and the database execution.
//
// Parameters:
//   - db: Database connection to execute the query against
//   - submodelID: Optional filter for a specific submodel ID. Empty string retrieves all submodels.
//
// Returns:
//   - *sql.Rows: Result set containing submodel data with JSON-aggregated nested structures
//   - error: An error if query building or execution fails
func getSubmodelDataFromDbWithJSONQuery(db *sql.DB, submodelID string, limit int64, cursor string, query *grammar.QueryWrapper) (*sql.Rows, error) {
	q, err := submodel_query.GetQueryWithGoqu(submodelID, limit, cursor, query)
	if err != nil {
		fmt.Printf("Error building query: %v\n", err)
		return nil, err
	}
	rows, err := db.Query(q)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}
