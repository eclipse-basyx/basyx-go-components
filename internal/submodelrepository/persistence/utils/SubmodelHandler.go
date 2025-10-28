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
package persistence_utils

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodel_query "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils/SubmodelQuery"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

func GetSubmodelById(db *sql.DB, submodelIdFilter string) (*gen.Submodel, error) {
	submodels, err := getSubmodels(db, submodelIdFilter)
	if err != nil || len(submodels) == 0 {
		return nil, err
	}
	return submodels[0], nil
}

func GetAllSubmodels(db *sql.DB) ([]*gen.Submodel, error) {
	return getSubmodels(db, "")
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
//
// Returns:
//   - []*gen.Submodel: Slice of fully populated Submodel objects with all nested structures
//   - error: An error if database query fails, scanning fails, or data parsing fails
//
// The function:
//   - Executes an optimized SQL query with JSON aggregation for nested data
//   - Pre-sizes result slices based on total count for better performance
//   - Builds reference hierarchies using ReferenceBuilder instances
//   - Parses JSON-encoded language strings and references
//   - Measures and logs query execution time for performance monitoring
//
// Note: The function builds nested reference structures in two phases:
//  1. Initial parsing during row iteration
//  2. Final structure building after all rows are processed
func getSubmodels(db *sql.DB, submodelIdFilter string) ([]*gen.Submodel, error) {
	var result []*gen.Submodel
	referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	start := time.Now().Local().UnixMilli()
	rows, err := getSubmodelDataFromDbWithJSONQuery(db, submodelIdFilter)
	end := time.Now().Local().UnixMilli()
	fmt.Printf("Total Query Only time: %d milliseconds\n", end-start)
	if err != nil {
		return nil, fmt.Errorf("error getting submodel data from DB: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row builders.SubmodelRow
		if err := rows.Scan(
			&row.Id, &row.IdShort, &row.Category, &row.Kind,
			&row.DisplayNames, &row.Descriptions,
			&row.SemanticId, &row.ReferredSemanticIds,
			&row.SupplementalSemanticIds, &row.SupplementalReferredSemIds,
			&row.DataSpecReference, &row.DataSpecReferenceReferred,
			&row.DataSpecIEC61360, &row.Qualifiers, &row.Extensions, &row.Administration, &row.RootSubmodelElements, &row.ChildSubmodelElements, &row.TotalSubmodels,
		); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		if result == nil {
			result = make([]*gen.Submodel, 0, row.TotalSubmodels)
		}

		submodel := &gen.Submodel{
			ModelType: "Submodel",
			Id:        row.Id,
			IdShort:   row.IdShort,
			Category:  row.Category,
			Kind:      gen.ModellingKind(row.Kind),
		}

		var semanticId []*gen.Reference
		if isArrayNotEmpty(row.SemanticId) {
			semanticId, err = builders.ParseReferences(row.SemanticId, referenceBuilderRefs)
			if err != nil {
				return nil, err
			}
			if hasSemanticId(semanticId) {
				submodel.SemanticId = semanticId[0]
				builders.ParseReferredReferences(row.ReferredSemanticIds, referenceBuilderRefs)
			}
		}

		if isArrayNotEmpty(row.SupplementalSemanticIds) {
			supplementalSemanticIds, err := builders.ParseReferences(row.SupplementalSemanticIds, referenceBuilderRefs)
			if err != nil {
				return nil, err
			}
			if moreThanZeroReferences(supplementalSemanticIds) {
				submodel.SupplementalSemanticIds = supplementalSemanticIds
				builders.ParseReferredReferences(row.SupplementalReferredSemIds, referenceBuilderRefs)
			}
		}

		// DisplayNames
		err = addDisplayNames(row, submodel)
		if err != nil {
			return nil, err
		}

		// Descriptions
		err = addDescriptions(row, submodel)
		if err != nil {
			return nil, err
		}

		// Embedded Data Specifications
		if isArrayNotEmpty(row.DataSpecReference) {
			builder := builders.NewEmbeddedDataSpecificationsBuilder()
			err := builder.BuildReferences(row.DataSpecReference, row.DataSpecReferenceReferred)
			if err != nil {
				fmt.Println(err)
				return nil, err
			}
			err = builder.BuildContentsIec61360(row.DataSpecIEC61360)
			if err != nil {
				fmt.Println(err)
				return nil, err
			}
			submodel.EmbeddedDataSpecifications = builder.Build()
		}

		// Qualifiers
		if isArrayNotEmpty(row.Qualifiers) {
			builder := builders.NewQualifiersBuilder()
			qualifierRows, err := builders.ParseQualifiersRow(row.Qualifiers)
			if err != nil {
				return nil, err
			}
			for _, qualifierRow := range qualifierRows {
				builder.AddQualifier(qualifierRow.DbId, qualifierRow.Kind, qualifierRow.Type, qualifierRow.ValueType, qualifierRow.Value)

				builder.AddSemanticId(qualifierRow.DbId, qualifierRow.SemanticId, qualifierRow.SemanticIdReferredReferences)

				builder.AddValueId(qualifierRow.DbId, qualifierRow.ValueId, qualifierRow.ValueIdReferredReferences)

				builder.AddSupplementalSemanticIds(qualifierRow.DbId, qualifierRow.SupplementalSemanticIds, qualifierRow.SupplementalSemanticIdsReferredReferences)
			}
			submodel.Qualifier = builder.Build()
		}

		// Extensions
		if isArrayNotEmpty(row.Extensions) {
			builder := builders.NewExtensionsBuilder()
			extensionRows, err := builders.ParseExtensionRows(row.Extensions)
			if err != nil {
				return nil, err
			}
			for _, extensionRow := range extensionRows {
				builder.AddExtension(extensionRow.DbId, extensionRow.Name, extensionRow.ValueType, extensionRow.Value)

				builder.AddSemanticId(extensionRow.DbId, extensionRow.SemanticId, extensionRow.SemanticIdReferredReferences)

				builder.AddRefersTo(extensionRow.DbId, extensionRow.RefersTo, extensionRow.RefersToReferredReferences)

				builder.AddSupplementalSemanticIds(extensionRow.DbId, extensionRow.SupplementalSemanticIds, extensionRow.SupplementalSemanticIdsReferredReferences)
			}
			submodel.Extension = builder.Build()
		}

		// Administration
		if isArrayNotEmpty(row.Administration) {
			adminRow, err := builders.ParseAdministrationRow(row.Administration)
			if err != nil {
				fmt.Println(err)
				return nil, err
			}
			if adminRow != nil {

				admin, err := builders.BuildAdministration(*adminRow)
				if err != nil {
					fmt.Println(err)
					return nil, err
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
	return result, nil
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

// hasSemanticId validates that exactly one semantic ID reference exists.
//
// According to the AAS specification, a submodel should have exactly one semantic ID.
// This function checks that the parsed semantic ID data contains exactly one reference.
//
// Parameters:
//   - semanticIdData: Slice of Reference objects parsed from semantic ID data
//
// Returns:
//   - bool: true if exactly one semantic ID reference exists, false otherwise
func hasSemanticId(semanticIdData []*gen.Reference) bool {
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
//   - submodelId: Optional filter for a specific submodel ID. Empty string retrieves all submodels.
//
// Returns:
//   - *sql.Rows: Result set containing submodel data with JSON-aggregated nested structures
//   - error: An error if query building or execution fails
func getSubmodelDataFromDbWithJSONQuery(db *sql.DB, submodelId string) (*sql.Rows, error) {
	q, err := submodel_query.GetQueryWithGoqu(submodelId)
	// fmt.Println(q)
	// save query in query.txt
	// err = os.WriteFile("query.txt", []byte(q), 0644)
	if err != nil {
		return nil, fmt.Errorf("error saving query to file: %w", err)
	}
	//fmt.Print(q)
	rows, err := db.Query(q)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}
