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
	"github.com/FriedJannik/aas-go-sdk/stringification"
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

	result, submodelMap, err := buildSubmodels(submodelRows)
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

func buildSubmodels(submodelRows []model.SubmodelRow) ([]*types.Submodel, map[string]*types.Submodel, error) {
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
				submodel, err := parseSubmodelRow(job.row, referenceBuilderRefs, &refMutex)
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

func parseSubmodelRow(row model.SubmodelRow, referenceBuilderRefs map[int64]*builders.ReferenceBuilder, refMutex *sync.RWMutex) (*types.Submodel, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	// Create submodel using SDK factory
	submodel := types.NewSubmodel(row.ID)

	// Set IDShort if present
	if row.IDShort != "" {
		submodel.SetIDShort(&row.IDShort)
	}

	// Set Kind
	kind := convertToSDKModellingKind(row.Kind)
	submodel.SetKind(&kind)

	// Handle nullable Category field
	if row.Category.Valid {
		submodel.SetCategory(&row.Category.String)
	}

	var (
		semanticID              []*types.IReference
		supplementalSemanticIDs []*types.IReference
		embeddedDataSpecs       []types.EmbeddedDataSpecification
		qualifiers              []types.IQualifier
		extensions              []types.IExtension
		administration          *types.AdministrativeInformation
		displayNames            []types.ILangStringNameType
		descriptions            []types.ILangStringTextType
	)

	localG, _ := errgroup.WithContext(context.Background())

	localG.Go(func() error {
		if common.IsArrayNotEmpty(row.SemanticID) {
			var err error
			semanticID, err = builders.ParseReferences(row.SemanticID, referenceBuilderRefs, refMutex)
			if err != nil {
				return fmt.Errorf("error parsing semantic ID: %w", err)
			}
			if hasSemanticID(semanticID) {
				err = builders.ParseReferredReferences(row.ReferredSemanticIDs, referenceBuilderRefs, refMutex)
				if err != nil {
					return fmt.Errorf("error parsing referred semantic IDs: %w", err)
				}
			}
		}
		return nil
	})

	localG.Go(func() error {
		if common.IsArrayNotEmpty(row.SupplementalSemanticIDs) {
			return json.Unmarshal(row.SupplementalSemanticIDs, &supplementalSemanticIDs)
		}
		return nil
	})

	localG.Go(func() error {
		dn, err := parseDisplayNamesToSDK(row)
		if err == nil {
			displayNames = dn
		}
		return err
	})
	localG.Go(func() error {
		ds, err := parseDescriptionsToSDK(row)
		if err == nil {
			descriptions = ds
		}
		return err
	})

	localG.Go(func() error {
		if common.IsArrayNotEmpty(row.EmbeddedDataSpecification) {
			return json.Unmarshal(row.EmbeddedDataSpecification, &embeddedDataSpecs)
		}
		return nil
	})

	localG.Go(func() error {
		q, err := BuildQualifiers(row)
		if err == nil {
			qualifiers = q
		}
		return err
	})

	localG.Go(func() error {
		if common.IsArrayNotEmpty(row.Extensions) {
			return json.Unmarshal(row.Extensions, &extensions)
		}
		return nil
	})

	localG.Go(func() error {
		a, err := BuildAdministration(row)
		if err == nil {
			administration = a
		}
		return err
	})

	if err := localG.Wait(); err != nil {
		return nil, err
	}

	// Assign parsed data to submodel using SDK setters
	if hasSemanticID(semanticID) {
		submodel.SetSemanticID(*semanticID[0])
	}
	if len(supplementalSemanticIDs) > 0 {
		// Convert from []*types.IReference to []types.IReference
		supplementalRefs := make([]types.IReference, len(supplementalSemanticIDs))
		for i, ref := range supplementalSemanticIDs {
			supplementalRefs[i] = *ref
		}
		submodel.SetSupplementalSemanticIDs(supplementalRefs)
	}
	if len(embeddedDataSpecs) > 0 {
		// Convert to slice of interface
		eds := make([]types.IEmbeddedDataSpecification, len(embeddedDataSpecs))
		for i := range embeddedDataSpecs {
			eds[i] = &embeddedDataSpecs[i]
		}
		submodel.SetEmbeddedDataSpecifications(eds)
	}
	if len(qualifiers) > 0 {
		submodel.SetQualifiers(qualifiers)
	}
	if len(extensions) > 0 {
		submodel.SetExtensions(extensions)
	}
	if administration != nil {
		submodel.SetAdministration(administration)
	}
	if len(displayNames) > 0 {
		submodel.SetDisplayName(displayNames)
	}
	if len(descriptions) > 0 {
		submodel.SetDescription(descriptions)
	}

	return submodel, nil
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
func BuildQualifiers(row model.SubmodelRow) ([]types.IQualifier, error) {
	if common.IsArrayNotEmpty(row.Qualifiers) {
		builder := builders.NewQualifiersBuilder()
		qualifierRows, err := builders.ParseQualifiersRow(row.Qualifiers)
		if err != nil {
			return nil, err
		}
		for _, qualifierRow := range qualifierRows {
			// Convert string enums to SDK enums
			kindEnum, ok := stringification.QualifierKindFromString(qualifierRow.Kind)
			if !ok {
				return nil, fmt.Errorf("invalid QualifierKind: %s", qualifierRow.Kind)
			}
			valueTypeEnum, ok := stringification.DataTypeDefXSDFromString(qualifierRow.ValueType)
			if !ok {
				return nil, fmt.Errorf("invalid DataTypeDefXSD: %s", qualifierRow.ValueType)
			}
			_, err = builder.AddQualifier(qualifierRow.DbID, int64(kindEnum), qualifierRow.Type, int64(valueTypeEnum), qualifierRow.Value, qualifierRow.Position)
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
			result[i] = &q
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
			_, _ = fmt.Println(err)
			return nil, err
		}
		if adminRow != nil {
			admin, err := builders.BuildAdministration(*adminRow)
			if err != nil {
				_, _ = fmt.Println(err)
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
		if err != nil {
			return nil, fmt.Errorf("error parsing display names: %w", err)
		}
		return convertLangStringNameTypesToSDK(displayNames), nil
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
		if err != nil {
			return nil, fmt.Errorf("error parsing descriptions: %w", err)
		}
		return convertLangStringTextTypesToSDK(descriptions), nil
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
func hasSemanticID(semanticIDData []*types.IReference) bool {
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

// ============================================================================
// SDK Type Conversion Functions
// ============================================================================

// convertToSDKModellingKind converts internal ModellingKind string to SDK ModellingKind type.
func convertToSDKModellingKind(kind string) types.ModellingKind {
	switch kind {
	case "Template":
		return types.ModellingKindTemplate
	case "Instance":
		return types.ModellingKindInstance
	default:
		return types.ModellingKindInstance
	}
}

// convertLangStringNameTypesToSDK converts internal LangStringNameType to SDK ILangStringNameType.
func convertLangStringNameTypesToSDK(langStrings []model.LangStringNameType) []types.ILangStringNameType {
	if langStrings == nil {
		return nil
	}
	result := make([]types.ILangStringNameType, len(langStrings))
	for i, ls := range langStrings {
		result[i] = types.NewLangStringNameType(ls.Language, ls.Text)
	}
	return result
}

// convertLangStringTextTypesToSDK converts internal LangStringTextType to SDK ILangStringTextType.
func convertLangStringTextTypesToSDK(langStrings []model.LangStringTextType) []types.ILangStringTextType {
	if langStrings == nil {
		return nil
	}
	result := make([]types.ILangStringTextType, len(langStrings))
	for i, ls := range langStrings {
		result[i] = types.NewLangStringTextType(ls.Language, ls.Text)
	}
	return result
}

// convertReferenceToSDK converts internal Reference to SDK IReference.
func convertReferenceToSDK(ref *model.Reference) types.IReference {
	if ref == nil {
		return nil
	}

	// Convert keys
	sdkKeys := make([]types.IKey, len(ref.Keys))
	for i, key := range ref.Keys {
		keyType := convertKeyTypeToSDK(string(key.Type))
		sdkKeys[i] = types.NewKey(keyType, key.Value)
	}

	// Convert reference type
	refType := convertReferenceTypeToSDK(string(ref.Type))

	sdkRef := types.NewReference(refType, sdkKeys)

	// Handle ReferredSemanticId if present
	if ref.ReferredSemanticID != nil {
		sdkRef.SetReferredSemanticID(convertReferenceToSDK(ref.ReferredSemanticID))
	}

	return sdkRef
}

// convertReferencesToSDK converts a slice of internal References to SDK IReferences.
func convertReferencesToSDK(refs []*model.Reference) []types.IReference {
	if refs == nil {
		return nil
	}
	result := make([]types.IReference, len(refs))
	for i, ref := range refs {
		result[i] = convertReferenceToSDK(ref)
	}
	return result
}

// convertReferencesSliceToSDK converts internal []Reference (value slice) to SDK []IReference.
func convertReferencesSliceToSDK(refs []model.Reference) []types.IReference {
	if refs == nil {
		return nil
	}
	result := make([]types.IReference, len(refs))
	for i := range refs {
		result[i] = convertReferenceToSDK(&refs[i])
	}
	return result
}

// convertKeyTypeToSDK converts internal KeyType string to SDK KeyTypes.
func convertKeyTypeToSDK(keyType string) types.KeyTypes {
	switch keyType {
	case "AnnotatedRelationshipElement":
		return types.KeyTypesAnnotatedRelationshipElement
	case "AssetAdministrationShell":
		return types.KeyTypesAssetAdministrationShell
	case "BasicEventElement":
		return types.KeyTypesBasicEventElement
	case "Blob":
		return types.KeyTypesBlob
	case "Capability":
		return types.KeyTypesCapability
	case "ConceptDescription":
		return types.KeyTypesConceptDescription
	case "DataElement":
		return types.KeyTypesDataElement
	case "Entity":
		return types.KeyTypesEntity
	case "EventElement":
		return types.KeyTypesEventElement
	case "File":
		return types.KeyTypesFile
	case "FragmentReference":
		return types.KeyTypesFragmentReference
	case "GlobalReference":
		return types.KeyTypesGlobalReference
	case "Identifiable":
		return types.KeyTypesIdentifiable
	case "MultiLanguageProperty":
		return types.KeyTypesMultiLanguageProperty
	case "Operation":
		return types.KeyTypesOperation
	case "Property":
		return types.KeyTypesProperty
	case "Range":
		return types.KeyTypesRange
	case "Referable":
		return types.KeyTypesReferable
	case "ReferenceElement":
		return types.KeyTypesReferenceElement
	case "RelationshipElement":
		return types.KeyTypesRelationshipElement
	case "Submodel":
		return types.KeyTypesSubmodel
	case "SubmodelElement":
		return types.KeyTypesSubmodelElement
	case "SubmodelElementCollection":
		return types.KeyTypesSubmodelElementCollection
	case "SubmodelElementList":
		return types.KeyTypesSubmodelElementList
	default:
		return types.KeyTypesGlobalReference
	}
}

// convertReferenceTypeToSDK converts internal ReferenceType string to SDK ReferenceTypes.
func convertReferenceTypeToSDK(refType string) types.ReferenceTypes {
	switch refType {
	case "ExternalReference":
		return types.ReferenceTypesExternalReference
	case "ModelReference":
		return types.ReferenceTypesModelReference
	default:
		return types.ReferenceTypesExternalReference
	}
}

// convertQualifiersToSDK converts internal Qualifiers to SDK IQualifiers.
func convertQualifiersToSDK(qualifiers []model.Qualifier) []types.IQualifier {
	if qualifiers == nil {
		return nil
	}
	result := make([]types.IQualifier, len(qualifiers))
	for i, q := range qualifiers {
		valueType := convertDataTypeDefXSDToSDK(string(q.ValueType))
		sdkQ := types.NewQualifier(q.Type, valueType)

		if q.Kind != "" {
			kind := convertQualifierKindToSDK(string(q.Kind))
			sdkQ.SetKind(&kind)
		}
		if q.Value != "" {
			sdkQ.SetValue(&q.Value)
		}
		if q.SemanticID != nil {
			sdkQ.SetSemanticID(convertReferenceToSDK(q.SemanticID))
		}
		if q.ValueID != nil {
			sdkQ.SetValueID(convertReferenceToSDK(q.ValueID))
		}
		if len(q.SupplementalSemanticIds) > 0 {
			sdkQ.SetSupplementalSemanticIDs(convertReferencesSliceToSDK(q.SupplementalSemanticIds))
		}
		result[i] = sdkQ
	}
	return result
}

// convertQualifierKindToSDK converts internal QualifierKind string to SDK QualifierKind.
func convertQualifierKindToSDK(kind string) types.QualifierKind {
	switch kind {
	case "ValueQualifier":
		return types.QualifierKindValueQualifier
	case "ConceptQualifier":
		return types.QualifierKindConceptQualifier
	case "TemplateQualifier":
		return types.QualifierKindTemplateQualifier
	default:
		return types.QualifierKindConceptQualifier
	}
}

// convertDataTypeDefXSDToSDK converts internal DataTypeDefXsd string to SDK DataTypeDefXSD.
func convertDataTypeDefXSDToSDK(dataType string) types.DataTypeDefXSD {
	switch dataType {
	case "xs:anyURI":
		return types.DataTypeDefXSDAnyURI
	case "xs:base64Binary":
		return types.DataTypeDefXSDBase64Binary
	case "xs:boolean":
		return types.DataTypeDefXSDBoolean
	case "xs:byte":
		return types.DataTypeDefXSDByte
	case "xs:date":
		return types.DataTypeDefXSDDate
	case "xs:dateTime":
		return types.DataTypeDefXSDDateTime
	case "xs:decimal":
		return types.DataTypeDefXSDDecimal
	case "xs:double":
		return types.DataTypeDefXSDDouble
	case "xs:duration":
		return types.DataTypeDefXSDDuration
	case "xs:float":
		return types.DataTypeDefXSDFloat
	case "xs:gDay":
		return types.DataTypeDefXSDGDay
	case "xs:gMonth":
		return types.DataTypeDefXSDGMonth
	case "xs:gMonthDay":
		return types.DataTypeDefXSDGMonthDay
	case "xs:gYear":
		return types.DataTypeDefXSDGYear
	case "xs:gYearMonth":
		return types.DataTypeDefXSDGYearMonth
	case "xs:hexBinary":
		return types.DataTypeDefXSDHexBinary
	case "xs:int":
		return types.DataTypeDefXSDInt
	case "xs:integer":
		return types.DataTypeDefXSDInteger
	case "xs:long":
		return types.DataTypeDefXSDLong
	case "xs:negativeInteger":
		return types.DataTypeDefXSDNegativeInteger
	case "xs:nonNegativeInteger":
		return types.DataTypeDefXSDNonNegativeInteger
	case "xs:nonPositiveInteger":
		return types.DataTypeDefXSDNonPositiveInteger
	case "xs:positiveInteger":
		return types.DataTypeDefXSDPositiveInteger
	case "xs:short":
		return types.DataTypeDefXSDShort
	case "xs:string":
		return types.DataTypeDefXSDString
	case "xs:time":
		return types.DataTypeDefXSDTime
	case "xs:unsignedByte":
		return types.DataTypeDefXSDUnsignedByte
	case "xs:unsignedInt":
		return types.DataTypeDefXSDUnsignedInt
	case "xs:unsignedLong":
		return types.DataTypeDefXSDUnsignedLong
	case "xs:unsignedShort":
		return types.DataTypeDefXSDUnsignedShort
	default:
		return types.DataTypeDefXSDString
	}
}

// convertExtensionsToSDK converts internal Extensions to SDK IExtensions.
func convertExtensionsToSDK(extensions []model.Extension) []types.IExtension {
	if extensions == nil {
		return nil
	}
	result := make([]types.IExtension, len(extensions))
	for i, ext := range extensions {
		sdkExt := types.NewExtension(ext.Name)

		if ext.ValueType != "" {
			valueType := convertDataTypeDefXSDToSDK(string(ext.ValueType))
			sdkExt.SetValueType(&valueType)
		}
		if ext.Value != "" {
			sdkExt.SetValue(&ext.Value)
		}
		if ext.SemanticID != nil {
			sdkExt.SetSemanticID(convertReferenceToSDK(ext.SemanticID))
		}
		if len(ext.RefersTo) > 0 {
			sdkExt.SetRefersTo(convertReferencesSliceToSDK(ext.RefersTo))
		}
		if len(ext.SupplementalSemanticIds) > 0 {
			sdkExt.SetSupplementalSemanticIDs(convertReferencesSliceToSDK(ext.SupplementalSemanticIds))
		}
		result[i] = sdkExt
	}
	return result
}

// convertAdministrationToSDK converts internal AdministrativeInformation to SDK IAdministrativeInformation.
func convertAdministrationToSDK(admin *model.AdministrativeInformation) types.IAdministrativeInformation {
	if admin == nil {
		return nil
	}

	sdkAdmin := types.NewAdministrativeInformation()

	if admin.Version != "" {
		sdkAdmin.SetVersion(&admin.Version)
	}
	if admin.Revision != "" {
		sdkAdmin.SetRevision(&admin.Revision)
	}
	if admin.Creator != nil {
		sdkAdmin.SetCreator(convertReferenceToSDK(admin.Creator))
	}
	if admin.TemplateID != "" {
		sdkAdmin.SetTemplateID(&admin.TemplateID)
	}
	if len(admin.EmbeddedDataSpecifications) > 0 {
		sdkAdmin.SetEmbeddedDataSpecifications(convertEmbeddedDataSpecsToSDK(admin.EmbeddedDataSpecifications))
	}

	return sdkAdmin
}

// convertEmbeddedDataSpecsToSDK converts internal EmbeddedDataSpecifications to SDK IEmbeddedDataSpecifications.
func convertEmbeddedDataSpecsToSDK(specs []model.EmbeddedDataSpecification) []types.IEmbeddedDataSpecification {
	if specs == nil {
		return nil
	}
	result := make([]types.IEmbeddedDataSpecification, len(specs))
	for i, spec := range specs {
		var dataSpecContent types.IDataSpecificationContent
		if spec.DataSpecificationContent != nil {
			dataSpecContent = convertDataSpecificationContentToSDK(spec.DataSpecificationContent)
		}
		var dataSpecRef types.IReference
		if spec.DataSpecification != nil {
			dataSpecRef = convertReferenceToSDK(spec.DataSpecification)
		}
		sdkSpec := types.NewEmbeddedDataSpecification(dataSpecRef, dataSpecContent)
		result[i] = sdkSpec
	}
	return result
}

// convertDataSpecificationContentToSDK converts internal DataSpecificationContent to SDK IDataSpecificationContent.
func convertDataSpecificationContentToSDK(content model.DataSpecificationContent) types.IDataSpecificationContent {
	if content == nil {
		return nil
	}

	// Check if it's IEC61360
	if iec, ok := content.(*model.DataSpecificationIec61360); ok {
		return convertDataSpecificationIEC61360ToSDK(iec)
	}

	return nil
}

// convertDataSpecificationIEC61360ToSDK converts internal DataSpecificationIec61360 to SDK IDataSpecificationIEC61360.
func convertDataSpecificationIEC61360ToSDK(iec *model.DataSpecificationIec61360) types.IDataSpecificationIEC61360 {
	if iec == nil {
		return nil
	}

	// Convert preferred name (required)
	preferredName := make([]types.ILangStringPreferredNameTypeIEC61360, len(iec.PreferredName))
	for i, pn := range iec.PreferredName {
		preferredName[i] = types.NewLangStringPreferredNameTypeIEC61360(pn.Language, pn.Text)
	}

	sdkIEC := types.NewDataSpecificationIEC61360(preferredName)

	// Set optional fields
	if len(iec.ShortName) > 0 {
		shortName := make([]types.ILangStringShortNameTypeIEC61360, len(iec.ShortName))
		for i, sn := range iec.ShortName {
			shortName[i] = types.NewLangStringShortNameTypeIEC61360(sn.Language, sn.Text)
		}
		sdkIEC.SetShortName(shortName)
	}

	if iec.Unit != "" {
		sdkIEC.SetUnit(&iec.Unit)
	}
	if iec.UnitID != nil {
		sdkIEC.SetUnitID(convertReferenceToSDK(iec.UnitID))
	}
	if iec.SourceOfDefinition != "" {
		sdkIEC.SetSourceOfDefinition(&iec.SourceOfDefinition)
	}
	if iec.Symbol != "" {
		sdkIEC.SetSymbol(&iec.Symbol)
	}
	if iec.DataType != "" {
		dataType := convertDataTypeIEC61360ToSDK(string(iec.DataType))
		sdkIEC.SetDataType(&dataType)
	}
	if len(iec.Definition) > 0 {
		definition := make([]types.ILangStringDefinitionTypeIEC61360, len(iec.Definition))
		for i, d := range iec.Definition {
			definition[i] = types.NewLangStringDefinitionTypeIEC61360(d.Language, d.Text)
		}
		sdkIEC.SetDefinition(definition)
	}
	if iec.ValueFormat != "" {
		sdkIEC.SetValueFormat(&iec.ValueFormat)
	}
	if iec.Value != "" {
		sdkIEC.SetValue(&iec.Value)
	}

	return sdkIEC
}

// convertDataTypeIEC61360ToSDK converts internal DataTypeIec61360 string to SDK DataTypeIEC61360.
func convertDataTypeIEC61360ToSDK(dataType string) types.DataTypeIEC61360 {
	switch dataType {
	case "DATE":
		return types.DataTypeIEC61360Date
	case "STRING":
		return types.DataTypeIEC61360String
	case "STRING_TRANSLATABLE":
		return types.DataTypeIEC61360StringTranslatable
	case "INTEGER_MEASURE":
		return types.DataTypeIEC61360IntegerMeasure
	case "INTEGER_COUNT":
		return types.DataTypeIEC61360IntegerCount
	case "INTEGER_CURRENCY":
		return types.DataTypeIEC61360IntegerCurrency
	case "REAL_MEASURE":
		return types.DataTypeIEC61360RealMeasure
	case "REAL_COUNT":
		return types.DataTypeIEC61360RealCount
	case "REAL_CURRENCY":
		return types.DataTypeIEC61360RealCurrency
	case "BOOLEAN":
		return types.DataTypeIEC61360Boolean
	case "IRI":
		return types.DataTypeIEC61360IRI
	case "IRDI":
		return types.DataTypeIEC61360IRDI
	case "RATIONAL":
		return types.DataTypeIEC61360Rational
	case "RATIONAL_MEASURE":
		return types.DataTypeIEC61360RationalMeasure
	case "TIME":
		return types.DataTypeIEC61360Time
	case "TIMESTAMP":
		return types.DataTypeIEC61360Timestamp
	case "FILE":
		return types.DataTypeIEC61360File
	case "HTML":
		return types.DataTypeIEC61360HTML
	case "BLOB":
		return types.DataTypeIEC61360Blob
	default:
		return types.DataTypeIEC61360String
	}
}
