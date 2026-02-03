/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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
	"context"
	"database/sql"
	"fmt"
	"sync"

	// nolint:all

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	submodel_query "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/queries"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
	"golang.org/x/sync/errgroup"
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
func GetSubmodelByID(db *sql.DB, submodelIDFilter string) (*types.Submodel, error) {
	submodels, _, _, err := getSubmodels(db, submodelIDFilter, 1, "", nil)
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
//   - []*types.Submodel: Slice of fully populated Submodel objects with all nested structures
//   - string: Next cursor for pagination (empty string if no more pages)
//   - error: An error if database query fails, scanning fails, or data parsing fails
func GetAllSubmodels(db *sql.DB, limit int64, cursor string, query *grammar.QueryWrapper) ([]*types.Submodel, map[string]*types.Submodel, string, error) {
	return getSubmodels(db, "", limit, cursor, query)
}

// SubmodelElementSubmodelMetadata holds metadata for a SubmodelElement including its database ID.
type SubmodelElementSubmodelMetadata struct {
	SubmodelElement types.ISubmodelElement
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
func getSubmodels(db *sql.DB, submodelIDFilter string, limit int64, cursor string, query *grammar.QueryWrapper) ([]*types.Submodel, map[string]*types.Submodel, string, error) {
	rows, err := GetSubmodelDataFromDbWithJSONQuery(db, submodelIDFilter, limit, cursor, query, false)
	if err != nil {
		return nil, nil, "", fmt.Errorf("error getting submodel data from DB: %w", err)
	}
	defer func() { _ = rows.Close() }()

	submodelRows, err := scanSubmodelRows(rows)
	if err != nil {
		return nil, nil, "", err
	}

	result, submodelMap, err := buildSubmodels(submodelRows, db)
	if err != nil {
		return nil, nil, "", err
	}

	nextCursor := calculateNextCursor(result, limit)
	return result, submodelMap, nextCursor, nil
}

func scanSubmodelRows(rows *sql.Rows) ([]model.SubmodelRow, error) {
	var submodelRows []model.SubmodelRow
	for rows.Next() {
		var row model.SubmodelRow
		if err := rows.Scan(
			&row.ID,
			&row.IDShort,
			&row.Category,
			&row.Kind,
			&row.EmbeddedDataSpecification,
			&row.SupplementalSemanticIDs,
			&row.Extensions,
			&row.DisplayNames,
			&row.Descriptions,
			&row.SemanticID,
			&row.ReferredSemanticIDs,
			&row.Qualifiers,
			&row.Administration,
		); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		submodelRows = append(submodelRows, row)
	}
	return submodelRows, nil
}

func buildSubmodels(submodelRows []model.SubmodelRow, db *sql.DB) ([]*types.Submodel, map[string]*types.Submodel, error) {
	referenceBuilderRefs := make(map[int64]*builders.ReferenceBuilder)
	var refMutex sync.RWMutex

	type parseJob struct {
		row   model.SubmodelRow
		index int
	}

	type parseResult struct {
		submodel *types.Submodel
		index    int
		err      error
	}

	numWorkers := 100
	jobs := make(chan parseJob, len(submodelRows))
	results := make(chan parseResult, len(submodelRows))

	// Start workers
	for i := 0; i < numWorkers; i++ {
		go func() {
			for job := range jobs {
				submodel, err := parseSubmodelRow(job.row, referenceBuilderRefs, &refMutex, db)
				results <- parseResult{submodel: submodel, index: job.index, err: err}
			}
		}()
	}

	// Send jobs
	for i, row := range submodelRows {
		jobs <- parseJob{row: row, index: i}
	}
	close(jobs)

	// Collect results
	result := make([]*types.Submodel, len(submodelRows))
	submodelMap := make(map[string]*types.Submodel)
	for i := 0; i < len(submodelRows); i++ {
		res := <-results
		if res.err != nil {
			return nil, nil, res.err
		}
		result[res.index] = res.submodel
		submodelMap[res.submodel.ID()] = res.submodel
	}

	if err := buildAllNestedReferences(referenceBuilderRefs); err != nil {
		return nil, nil, err
	}

	return result, submodelMap, nil
}

// submodelParsedData holds all parsed data for a submodel row.
type submodelParsedData struct {
	semanticID              []types.IReference
	supplementalSemanticIDs []types.IReference
	embeddedDataSpecs       []types.IEmbeddedDataSpecification
	qualifiers              []types.IQualifier
	extensions              []types.IExtension
	administration          *types.AdministrativeInformation
	displayNames            []types.ILangStringNameType
	descriptions            []types.ILangStringTextType
}

func parseSubmodelRow(row model.SubmodelRow, referenceBuilderRefs map[int64]*builders.ReferenceBuilder, refMutex *sync.RWMutex, db *sql.DB) (*types.Submodel, error) {
	submodel := types.NewSubmodel(row.ID)
	setSubmodelBasicFields(submodel, row)

	data, err := parseSubmodelRowData(row, referenceBuilderRefs, refMutex, db)
	if err != nil {
		return nil, err
	}

	assignParsedDataToSubmodel(submodel, data)
	return submodel, nil
}

func setSubmodelBasicFields(submodel *types.Submodel, row model.SubmodelRow) {
	if row.IDShort != "" {
		submodel.SetIDShort(&row.IDShort)
	}
	if row.Kind.Valid {
		kind := types.ModellingKind(row.Kind.Int64)
		submodel.SetKind(&kind)
	}
	if row.Category.Valid {
		submodel.SetCategory(&row.Category.String)
	}
}

func parseSubmodelRowData(row model.SubmodelRow, referenceBuilderRefs map[int64]*builders.ReferenceBuilder, refMutex *sync.RWMutex, db *sql.DB) (*submodelParsedData, error) {
	data := &submodelParsedData{}
	localG, _ := errgroup.WithContext(context.Background())

	localG.Go(func() error {
		semanticID, err := parseSemanticID(row, referenceBuilderRefs, refMutex)
		data.semanticID = semanticID
		return err
	})

	localG.Go(func() error {
		supplementalIDs, err := parseSupplementalSemanticIDs(row.SupplementalSemanticIDs)
		data.supplementalSemanticIDs = supplementalIDs
		return err
	})

	localG.Go(func() error {
		dn, err := parseDisplayNamesToSDK(row)
		data.displayNames = dn
		return err
	})

	localG.Go(func() error {
		ds, err := parseDescriptionsToSDK(row)
		data.descriptions = ds
		return err
	})

	localG.Go(func() error {
		eds, err := parseEmbeddedDataSpecifications(row.EmbeddedDataSpecification)
		data.embeddedDataSpecs = eds
		return err
	})

	localG.Go(func() error {
		q, err := BuildQualifiers(row, db)
		data.qualifiers = q
		return err
	})

	localG.Go(func() error {
		ext, err := parseExtensionsFromJSON(row.Extensions)
		data.extensions = ext
		return err
	})

	localG.Go(func() error {
		admin, err := BuildAdministration(row)
		data.administration = admin
		return err
	})

	return data, localG.Wait()
}

func parseSemanticID(row model.SubmodelRow, referenceBuilderRefs map[int64]*builders.ReferenceBuilder, refMutex *sync.RWMutex) ([]types.IReference, error) {
	if !common.IsArrayNotEmpty(row.SemanticID) {
		return nil, nil
	}
	semanticID, err := builders.ParseReferences(row.SemanticID, referenceBuilderRefs, refMutex)
	if err != nil {
		return nil, fmt.Errorf("error parsing semantic ID: %w", err)
	}
	if hasSemanticID(semanticID) {
		err = builders.ParseReferredReferences(row.ReferredSemanticIDs, referenceBuilderRefs, refMutex)
		if err != nil {
			return nil, fmt.Errorf("error parsing referred semantic IDs: %w", err)
		}
	}
	return semanticID, nil
}

func parseSupplementalSemanticIDs(data []byte) ([]types.IReference, error) {
	if !common.IsArrayNotEmpty(data) {
		return nil, nil
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var jsonable []map[string]any
	if err := json.Unmarshal(data, &jsonable); err != nil {
		return nil, fmt.Errorf("error unmarshaling supplemental semantic IDs for Submodel: %w", err)
	}
	result := make([]types.IReference, 0, len(jsonable))
	for i, item := range jsonable {
		semanticID, err := jsonization.ReferenceFromJsonable(item)
		if err != nil {
			// Log the problematic JSON for debugging
			jsonBytes, _ := json.Marshal(item)
			_, _ = fmt.Printf("[DEBUG] parseSupplementalSemanticIDs: Error at index %d, JSON: %s, Error: %v\n", i, string(jsonBytes), err)
			return nil, fmt.Errorf("error converting supplemental semantic ID from jsonable (index %d, data: %s): %w", i, string(jsonBytes), err)
		}
		result = append(result, semanticID)
	}
	return result, nil
}

func parseEmbeddedDataSpecifications(data []byte) ([]types.IEmbeddedDataSpecification, error) {
	if !common.IsArrayNotEmpty(data) {
		return nil, nil
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var jsonable []map[string]any
	if err := json.Unmarshal(data, &jsonable); err != nil {
		return nil, fmt.Errorf("error unmarshaling embedded data specifications for Submodel: %w", err)
	}
	result := make([]types.IEmbeddedDataSpecification, 0, len(jsonable))
	for i, item := range jsonable {
		eds, err := jsonization.EmbeddedDataSpecificationFromJsonable(item)
		if err != nil {
			// Log the problematic JSON for debugging
			jsonBytes, _ := json.Marshal(item)
			_, _ = fmt.Printf("[DEBUG] parseEmbeddedDataSpecifications: Error at index %d, JSON: %s, Error: %v\n", i, string(jsonBytes), err)
			return nil, fmt.Errorf("error converting embedded data specification from jsonable (index %d, data: %s): %w", i, string(jsonBytes), err)
		}
		result = append(result, eds)
	}
	return result, nil
}

func parseExtensionsFromJSON(data []byte) ([]types.IExtension, error) {
	if !common.IsArrayNotEmpty(data) {
		return nil, nil
	}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var jsonable []map[string]any
	if err := json.Unmarshal(data, &jsonable); err != nil {
		return nil, fmt.Errorf("error unmarshaling extensions for Submodel: %w", err)
	}
	result := make([]types.IExtension, 0, len(jsonable))
	for i, item := range jsonable {
		ext, err := jsonization.ExtensionFromJsonable(item)
		if err != nil {
			// Log the problematic JSON for debugging
			jsonBytes, _ := json.Marshal(item)
			_, _ = fmt.Printf("[DEBUG] parseExtensionsFromJSON: Error at index %d, JSON: %s, Error: %v\n", i, string(jsonBytes), err)
			return nil, fmt.Errorf("error converting extension from jsonable (index %d, data: %s): %w", i, string(jsonBytes), err)
		}
		result = append(result, ext)
	}
	return result, nil
}

func assignParsedDataToSubmodel(submodel *types.Submodel, data *submodelParsedData) {
	if hasSemanticID(data.semanticID) {
		submodel.SetSemanticID(data.semanticID[0])
	}
	if len(data.supplementalSemanticIDs) > 0 {
		submodel.SetSupplementalSemanticIDs(data.supplementalSemanticIDs)
	}
	if len(data.embeddedDataSpecs) > 0 {
		submodel.SetEmbeddedDataSpecifications(data.embeddedDataSpecs)
	}
	if len(data.qualifiers) > 0 {
		submodel.SetQualifiers(data.qualifiers)
	}
	if len(data.extensions) > 0 {
		submodel.SetExtensions(data.extensions)
	}
	if data.administration != nil {
		submodel.SetAdministration(data.administration)
	}
	if len(data.displayNames) > 0 {
		submodel.SetDisplayName(data.displayNames)
	}
	if len(data.descriptions) > 0 {
		submodel.SetDescription(data.descriptions)
	}
}

func buildAllNestedReferences(referenceBuilderRefs map[int64]*builders.ReferenceBuilder) error {
	var wg sync.WaitGroup
	for _, builder := range referenceBuilderRefs {
		wg.Add(1)
		go func(b *builders.ReferenceBuilder) {
			defer wg.Done()
			b.BuildNestedStructure()
		}(builder)
	}
	wg.Wait()
	return nil
}

func calculateNextCursor(result []*types.Submodel, limit int64) string {
	if limit > 0 && len(result) > int(limit) {
		return result[limit].ID()
	}
	return ""
}

// BuildQualifiers builds qualifiers from the database row.
func BuildQualifiers(row model.SubmodelRow, db *sql.DB) ([]types.IQualifier, error) {
	if common.IsArrayNotEmpty(row.Qualifiers) {
		builder := builders.NewQualifiersBuilder()
		qualifierRows, err := builders.ParseQualifiersRow(row.Qualifiers)
		if err != nil {
			return nil, err
		}
		for _, qualifierRow := range qualifierRows {
			// Convert string enums to SDK enums
			valueTypeEnum := types.DataTypeDefXSD(qualifierRow.ValueType)
			_, err = builder.AddQualifier(qualifierRow.DbID, qualifierRow.Type, int64(valueTypeEnum), qualifierRow.Value, qualifierRow.Position, db)
			if err != nil {
				return nil, err
			}

			_, err = builder.AddSemanticID(qualifierRow.DbID, qualifierRow.SemanticID, qualifierRow.SemanticIDReferredReferences)
			if err != nil {
				return nil, err
			}

			_, err = builder.AddValueID(qualifierRow.DbID, qualifierRow.ValueID, qualifierRow.ValueIDReferredReferences)
			if err != nil {
				return nil, err
			}

			_, err = builder.AddSupplementalSemanticIDs(qualifierRow.DbID, qualifierRow.SupplementalSemanticIDs, qualifierRow.SupplementalSemanticIDsReferredReferences)
			if err != nil {
				return nil, err
			}
		}
		built := builder.Build()
		// Convert concrete types to interfaces - need pointers since methods have pointer receivers
		result := make([]types.IQualifier, len(built))
		for i := range built {
			q := built[i] // Copy to get addressable value
			result[i] = q
		}
		return result, nil
	}
	return nil, nil
}

// BuildAdministration builds administrative information from the database row.
func BuildAdministration(row model.SubmodelRow) (*types.AdministrativeInformation, error) {
	if common.IsArrayNotEmpty(row.Administration) {
		adminRow, err := builders.ParseAdministrationRow(row.Administration)
		if err != nil {
			_, _ = fmt.Println("SMREPO-BLD-PADMIN " + err.Error())
			return nil, err
		}
		if adminRow != nil {
			admin, err := builders.BuildAdministration(*adminRow)
			if err != nil {
				_, _ = fmt.Println("SMREPO-BLD-BADMIN " + err.Error())
				return nil, err
			}
			return admin, nil
		}
		return nil, nil
	}
	return nil, nil
}

// BuildExtensions builds extensions from the database row.
func BuildExtensions(row model.SubmodelRow) ([]types.IExtension, error) {
	if common.IsArrayNotEmpty(row.Extensions) {
		builder := builders.NewExtensionsBuilder()
		extensionRows, err := builders.ParseExtensionRows(row.Extensions)
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
		return builder.Build(), nil
	}
	return nil, nil
}

// parseDisplayNamesToSDK parses display names from the database row to SDK types.
//
// This helper function extracts language-specific display names from the database
// row and converts them to SDK ILangStringNameType.
//
// Parameters:
//   - row: SubmodelRow containing JSON-encoded display names data
//
// Returns:
//   - []types.ILangStringNameType: Slice of SDK language string name types
//   - error: An error if parsing the language strings fails, nil otherwise
func parseDisplayNamesToSDK(row model.SubmodelRow) ([]types.ILangStringNameType, error) {
	if common.IsArrayNotEmpty(row.DisplayNames) {
		displayNames, err := builders.ParseLangStringNameType(row.DisplayNames)
		return displayNames, err
	}
	return nil, nil
}

// parseDescriptionsToSDK parses descriptions from the database row to SDK types.
//
// This helper function extracts language-specific descriptions from the database
// row and converts them to SDK ILangStringTextType.
//
// Parameters:
//   - row: SubmodelRow containing JSON-encoded descriptions data
//
// Returns:
//   - []types.ILangStringTextType: Slice of SDK language string text types
//   - error: An error if parsing the language strings fails, nil otherwise
func parseDescriptionsToSDK(row model.SubmodelRow) ([]types.ILangStringTextType, error) {
	if common.IsArrayNotEmpty(row.Descriptions) {
		descriptions, err := builders.ParseLangStringTextType(row.Descriptions)
		return descriptions, err
	}
	return nil, nil
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
func hasSemanticID(semanticIDData []types.IReference) bool {
	return len(semanticIDData) == 1
}

// GetSubmodelDataFromDbWithJSONQuery executes the submodel query against the database.
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
func GetSubmodelDataFromDbWithJSONQuery(db *sql.DB, submodelID string, limit int64, cursor string, query *grammar.QueryWrapper, onlyIDs bool) (*sql.Rows, error) {
	q, err := submodel_query.GetQueryWithGoqu(submodelID, limit, cursor, query, onlyIDs)
	if err != nil {
		_, _ = fmt.Printf("Error building query: %v\n", err)
		return nil, err
	}

	rows, err := db.Query(q)
	if err != nil {
		_, _ = fmt.Printf("Error querying database: %v\n", err)
		return nil, err
	}
	return rows, nil
}
