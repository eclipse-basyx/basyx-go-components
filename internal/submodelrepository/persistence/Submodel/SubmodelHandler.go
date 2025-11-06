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

// Package submodelpersistence provides functions to persist and retrieve Submodel entities from a PostgreSQL database.
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package submodelpersistence

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	// nolint:all
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	submodel_query "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/submodelQueries"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// GetSubmodelByID retrieves a single submodel by its ID from the database with full nested structures.
//
// This function is a wrapper around getSubmodels that fetches a single submodel
// based on the provided submodel ID. It returns an error if the submodel is not found.
//
// Parameters:
//   - db: Database connection to execute the query against
//   - submodelIdFilter: The ID of the submodel to retrieve
//
// Returns:
//   - *model.Submodel: Fully populated Submodel object with all nested structures
//   - error: An error if database query fails, scanning fails, data parsing fails, or submodel is not found
func GetSubmodelByID(db *sql.DB, submodelIDFilter string) (*model.Submodel, error) {
	submodels, _, err := getSubmodels(db, submodelIDFilter, 1, "", nil)
	if err != nil {
		return nil, err
	}
	if len(submodels) == 0 {
		return nil, common.NewErrNotFound(submodelIDFilter)
	}
	return submodels[0], nil
}

// GetAllSubmodels retrieves all submodels from the database with full nested structures.
//
// This function is a wrapper around getSubmodels that fetches all submodels without
// any specific ID filter. It supports pagination and optional AAS QueryLanguage filtering.
//
// Parameters:
//   - db: Database connection to execute the query against
//   - limit: Maximum number of results to return (0 means no limit)
//   - cursor: The submodel ID to start pagination from (empty string means start from beginning)
//   - query: Optional AAS QueryLanguage filtering
//
// Returns:
//   - []*model.Submodel: Slice of fully populated Submodel objects with all nested structures
//   - string: Next cursor for pagination (empty string if no more pages)
//   - error: An error if database query fails, scanning fails, or data parsing fails
func GetAllSubmodels(db *sql.DB, limit int64, cursor string, query *grammar.QueryWrapper) ([]*model.Submodel, string, error) {
	return getSubmodels(db, "", limit, cursor, query)
}

// SubmodelElementSubmodelMetadata holds metadata for a SubmodelElement including its database ID.
type SubmodelElementSubmodelMetadata struct {
	SubmodelElement model.SubmodelElement
	DatabaseID      int
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
//   - []*model.Submodel: Slice of fully populated Submodel objects with all nested structures
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
func getSubmodels(db *sql.DB, submodelIDFilter string, limit int64, cursor string, query *grammar.QueryWrapper) ([]*model.Submodel, string, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var result []*model.Submodel
	var submodelCount int
	err := db.QueryRow("SELECT COUNT(*) FROM submodel").Scan(&submodelCount)
	if err != nil {
		return nil, "", fmt.Errorf("error getting submodel count: %w", err)
	}
	if limit > 0 && submodelCount < int(limit) {
		limit = int64(submodelCount)
	}
	referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	start := time.Now().Local().UnixMilli()
	rows, err := getSubmodelDataFromDbWithJSONQuery(db, submodelIDFilter, limit, cursor, query)
	end := time.Now().Local().UnixMilli()
	fmt.Printf("Total Query Only time: %d milliseconds\n", end-start)
	if err != nil {
		return nil, "", fmt.Errorf("error getting submodel data from DB: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var count int64
	var row model.SubmodelRow
	var EmbeddedDataSpecifications []model.EmbeddedDataSpecification
	var semanticID []*model.Reference
	for rows.Next() {
		count++
		if err := rows.Scan(
			&row.ID, &row.IDShort, &row.Category, &row.Kind,
			&row.EmbeddedDataSpecification,
			&row.DisplayNames, &row.Descriptions,
			&row.SemanticID, &row.ReferredSemanticIDs,
			&row.SupplementalSemanticIDs, &row.SupplementalReferredSemIDs,
			&row.Qualifiers, &row.Extensions, &row.Administration, &row.RootSubmodelElements, &row.ChildSubmodelElements, &row.TotalSubmodels,
		); err != nil {
			return nil, "", fmt.Errorf("error scanning row: %w", err)
		}

		if result == nil {
			result = make([]*model.Submodel, 0, row.TotalSubmodels)
		}

		submodel := &model.Submodel{
			ModelType:        "Submodel",
			ID:               row.ID,
			IdShort:          row.IDShort,
			Category:         row.Category,
			Kind:             model.ModellingKind(row.Kind),
			SubmodelElements: []model.SubmodelElement{},
		}
		if count > limit {
			result = append(result, submodel)
			break
		}
		if common.IsArrayNotEmpty(row.SemanticID) {
			semanticID, err = builders.ParseReferences(row.SemanticID, referenceBuilderRefs)
			if err != nil {
				return nil, "", err
			}
			if hasSemanticID(semanticID) {
				submodel.SemanticID = semanticID[0]
				err = builders.ParseReferredReferences(row.ReferredSemanticIDs, referenceBuilderRefs)
				if err != nil {
					return nil, "", err
				}
			}
		}

		// SupplementalSemanticIDs
		if common.IsArrayNotEmpty(row.SupplementalSemanticIDs) {
			supplementalSemanticIDs, err := builders.ParseReferences(row.SupplementalSemanticIDs, referenceBuilderRefs)
			if err != nil {
				return nil, "", err
			}
			if moreThanZeroReferences(supplementalSemanticIDs) {
				submodel.SupplementalSemanticIds = supplementalSemanticIDs
				err = builders.ParseReferredReferences(row.SupplementalReferredSemIDs, referenceBuilderRefs)
				if err != nil {
					return nil, "", err
				}
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
		if common.IsArrayNotEmpty(row.EmbeddedDataSpecification) {
			err = json.Unmarshal(row.EmbeddedDataSpecification, &EmbeddedDataSpecifications)
			// Print size of EmbeddedDataSpecifications in bytes
			if err != nil {
				fmt.Println(err)
				return nil, "", err
			}
			submodel.EmbeddedDataSpecifications = EmbeddedDataSpecifications
		}

		// Qualifiers
		if common.IsArrayNotEmpty(row.Qualifiers) {
			builder := builders.NewQualifiersBuilder()
			qualifierRows, err := builders.ParseQualifiersRow(row.Qualifiers)
			if err != nil {
				return nil, "", err
			}
			for _, qualifierRow := range qualifierRows {
				_, err = builder.AddQualifier(qualifierRow.DbID, qualifierRow.Kind, qualifierRow.Type, qualifierRow.ValueType, qualifierRow.Value, qualifierRow.Position)
				if err != nil {
					return nil, "", err
				}

				_, err = builder.AddSemanticID(qualifierRow.DbID, qualifierRow.SemanticID, qualifierRow.SemanticIDReferredReferences)
				if err != nil {
					return nil, "", err
				}

				_, err = builder.AddValueID(qualifierRow.DbID, qualifierRow.ValueID, qualifierRow.ValueIDReferredReferences)
				if err != nil {
					return nil, "", err
				}

				_, err = builder.AddSupplementalSemanticIDs(qualifierRow.DbID, qualifierRow.SupplementalSemanticIDs, qualifierRow.SupplementalSemanticIDsReferredReferences)
				if err != nil {
					return nil, "", err
				}
			}
			submodel.Qualifier = builder.Build()
		}

		// Extensions
		if common.IsArrayNotEmpty(row.Extensions) {
			builder := builders.NewExtensionsBuilder()
			extensionRows, err := builders.ParseExtensionRows(row.Extensions)
			if err != nil {
				return nil, "", err
			}
			for _, extensionRow := range extensionRows {
				_, err = builder.AddExtension(extensionRow.DbID, extensionRow.Name, extensionRow.ValueType, extensionRow.Value, extensionRow.Position)
				if err != nil {
					return nil, "", err
				}

				_, err = builder.AddSemanticID(extensionRow.DbID, extensionRow.SemanticID, extensionRow.SemanticIDReferredReferences)
				if err != nil {
					return nil, "", err
				}

				_, err = builder.AddRefersTo(extensionRow.DbID, extensionRow.RefersTo, extensionRow.RefersToReferredReferences)
				if err != nil {
					return nil, "", err
				}

				_, err = builder.AddSupplementalSemanticIDs(extensionRow.DbID, extensionRow.SupplementalSemanticIDs, extensionRow.SupplementalSemanticIDsReferredReferences)
				if err != nil {
					return nil, "", err
				}
			}
			submodel.Extension = builder.Build()
		}

		// Administration
		if common.IsArrayNotEmpty(row.Administration) {
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

		nodes := make(map[int64]*node, 256)
		children := make(map[int64][]*node, 256)
		roots := make([]*node, 0, 16)

		// RootSubmodelElements
		smeBuilderMap := make(map[int64]*builders.SubmodelElementBuilder)
		if common.IsArrayNotEmpty(row.RootSubmodelElements) {
			var RootSubmodelElements []model.SubmodelElementRow
			err := json.Unmarshal(row.RootSubmodelElements, &RootSubmodelElements)
			if err != nil {
				return nil, "", err
			}
			for _, smeRow := range RootSubmodelElements {
				_, exists := smeBuilderMap[smeRow.DbID]
				if !exists {
					sme, builder, err := builders.BuildSubmodelElement(smeRow)
					if err != nil {
						return nil, "", err
					}
					smeBuilderMap[smeRow.DbID] = builder
					n := &node{
						id:       smeRow.DbID,
						parentID: 0, // Root elements have no parent
						path:     smeRow.IDShortPath,
						position: smeRow.Position,
						element:  *sme,
					}
					nodes[smeRow.DbID] = n
					roots = append(roots, n)
				}
			}
		}
		if common.IsArrayNotEmpty(row.ChildSubmodelElements) {
			var ChildSubmodelElements []model.SubmodelElementRow
			err := json.Unmarshal(row.ChildSubmodelElements, &ChildSubmodelElements)
			if err != nil {
				return nil, "", err
			}
			for _, smeRow := range ChildSubmodelElements {
				_, exists := smeBuilderMap[smeRow.DbID]
				if !exists {
					sme, builder, err := builders.BuildSubmodelElement(smeRow)
					if err != nil {
						return nil, "", err
					}
					smeBuilderMap[smeRow.DbID] = builder
					n := &node{
						id:       smeRow.DbID,
						parentID: *smeRow.ParentID,
						path:     smeRow.IDShortPath,
						position: smeRow.Position,
						element:  *sme,
					}
					nodes[smeRow.DbID] = n
					children[*smeRow.ParentID] = append(children[*smeRow.ParentID], n)
				}
			}
		}

		attachChildrenToSubmodelElements(nodes, children)

		sort.SliceStable(roots, func(i, j int) bool {
			a, b := roots[i], roots[j]
			return a.id < b.id
		})

		res := make([]model.SubmodelElement, 0, len(roots))
		for _, r := range roots {
			res = append(res, r.element)
		}
		submodel.SubmodelElements = res

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
func addDisplayNames(row model.SubmodelRow, submodel *model.Submodel) error {
	if common.IsArrayNotEmpty(row.DisplayNames) {
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
func addDescriptions(row model.SubmodelRow, submodel *model.Submodel) error {
	if common.IsArrayNotEmpty(row.Descriptions) {
		descriptions, err := builders.ParseLangStringTextType(row.Descriptions)
		if err != nil {
			return fmt.Errorf("error parsing descriptions: %w", err)
		}
		submodel.Description = descriptions
	}
	return nil
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
func hasSemanticID(semanticIDData []*model.Reference) bool {
	return len(semanticIDData) == 1
}

// moreThanZeroReferences checks if References exist.
//
// Parameters:
//   - referenceArray: Slice of Reference objects
//
// Returns:
//   - bool: true if at least one Reference exists, false otherwise
func moreThanZeroReferences(referenceArray []*model.Reference) bool {
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
	err = os.WriteFile("submodel_query.sql", []byte(q), 0644)
	if err != nil {
		fmt.Printf("Error writing query to file: %v\n", err)
		return nil, err
	}
	rows, err := db.Query(q)
	if err != nil {
		fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}

// attachChildrenToSubmodelElements reconstructs the hierarchical structure of submodel elements.
//
// This function attaches child elements to their parent containers (SubmodelElementCollection
// or SubmodelElementList) in the proper order. It performs a stable sort based on position
// (if set) with path as tie-breaker, ensuring consistent ordering of children.
//
// The function operates in O(n log n) time where n is the number of children, using an
// efficient sorting algorithm and direct map lookups.
//
// Parameters:
//   - nodes: Map of all nodes indexed by their database ID
//   - children: Map of parent ID to slice of child nodes
//
// Note: Only SubmodelElementCollection and SubmodelElementList types support children.
// Other element types are silently skipped even if they have entries in the children map.
func attachChildrenToSubmodelElements(nodes map[int64]*node, children map[int64][]*node) {
	for id, parent := range nodes {
		kids := children[id]
		if len(kids) == 0 {
			continue
		}

		// Stable order: by position (if set), otherwise by path as tie-breaker
		sort.SliceStable(kids, func(i, j int) bool {
			a, b := kids[i], kids[j]
			return a.position < b.position
		})

		switch p := parent.element.(type) {
		case *model.SubmodelElementCollection:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		case *model.SubmodelElementList:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		}
	}
}

// node is a helper struct to build the hierarchical structure of SubmodelElements.
//
// It holds metadata such as database ID, parent ID, path, position, and the actual
// SubmodelElement data. This struct is used during the reconstruction of the
// nested structure of submodel elements from flat database rows.
type node struct {
	id       int64                 // Database ID of the element
	parentID int64                 // Parent element ID for hierarchy
	path     string                // Full path for navigation
	position int                   // Position within parent for ordering
	element  model.SubmodelElement // The actual submodel element data
}
