package openapi

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// isEmptyReference checks if a Reference is empty (zero value)
func isEmptyReference(ref Reference) bool {
	return reflect.DeepEqual(ref, Reference{})
}

// UnmarshalJSON implements polymorphic deserialization for SubmodelElement
// based on the modelType field
func (se *SubmodelElement) UnmarshalJSON(data []byte) error {
	// First, unmarshal into a generic map to read the modelType
	var temp map[string]interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	modelType, ok := temp["modelType"].(string)
	if !ok {
		return fmt.Errorf("modelType field is missing or not a string")
	}

	// Based on modelType, unmarshal into the appropriate concrete type
	switch modelType {
	case "Property":
		var property Property
		if err := json.Unmarshal(data, &property); err != nil {
			return err
		}
		// Copy common fields from Property to SubmodelElement
		se.Extensions = property.Extensions
		se.Category = property.Category
		se.IdShort = property.IdShort
		se.DisplayName = property.DisplayName
		se.Description = property.Description
		se.ModelType = ModelType(property.ModelType)
		// Only copy non-empty references and slices
		if !isEmptyReference(property.SemanticId) {
			se.SemanticId = property.SemanticId
		}
		if len(property.SupplementalSemanticIds) > 0 {
			se.SupplementalSemanticIds = property.SupplementalSemanticIds
		}
		if len(property.Qualifiers) > 0 {
			se.Qualifiers = property.Qualifiers
		}
		if len(property.EmbeddedDataSpecifications) > 0 {
			se.EmbeddedDataSpecifications = property.EmbeddedDataSpecifications
		}
		return nil

	case "MultiLanguageProperty":
		var mlProperty MultiLanguageProperty
		if err := json.Unmarshal(data, &mlProperty); err != nil {
			return err
		}
		// Copy common fields from MultiLanguageProperty to SubmodelElement
		se.Extensions = mlProperty.Extensions
		se.Category = mlProperty.Category
		se.IdShort = mlProperty.IdShort
		se.DisplayName = mlProperty.DisplayName
		se.Description = mlProperty.Description
		se.ModelType = ModelType(mlProperty.ModelType)
		if !isEmptyReference(mlProperty.SemanticId) {
			se.SemanticId = mlProperty.SemanticId
		}
		se.SupplementalSemanticIds = mlProperty.SupplementalSemanticIds
		se.Qualifiers = mlProperty.Qualifiers
		se.EmbeddedDataSpecifications = mlProperty.EmbeddedDataSpecifications
		return nil

	case "Blob":
		var blob Blob
		if err := json.Unmarshal(data, &blob); err != nil {
			return err
		}
		// Copy common fields from Blob to SubmodelElement
		se.Extensions = blob.Extensions
		se.Category = blob.Category
		se.IdShort = blob.IdShort
		se.DisplayName = blob.DisplayName
		se.Description = blob.Description
		se.ModelType = ModelType(blob.ModelType)
		if !isEmptyReference(blob.SemanticId) {
			se.SemanticId = blob.SemanticId
		}
		se.SupplementalSemanticIds = blob.SupplementalSemanticIds
		se.Qualifiers = blob.Qualifiers
		se.EmbeddedDataSpecifications = blob.EmbeddedDataSpecifications
		return nil

	case "File":
		var file File
		if err := json.Unmarshal(data, &file); err != nil {
			return err
		}
		// Copy common fields from File to SubmodelElement
		se.Extensions = file.Extensions
		se.Category = file.Category
		se.IdShort = file.IdShort
		se.DisplayName = file.DisplayName
		se.Description = file.Description
		se.ModelType = ModelType(file.ModelType)
		if !isEmptyReference(file.SemanticId) {
			se.SemanticId = file.SemanticId
		}
		se.SupplementalSemanticIds = file.SupplementalSemanticIds
		se.Qualifiers = file.Qualifiers
		se.EmbeddedDataSpecifications = file.EmbeddedDataSpecifications
		return nil

	case "Range":
		var rangeElement Range
		if err := json.Unmarshal(data, &rangeElement); err != nil {
			return err
		}
		// Copy common fields from Range to SubmodelElement
		se.Extensions = rangeElement.Extensions
		se.Category = rangeElement.Category
		se.IdShort = rangeElement.IdShort
		se.DisplayName = rangeElement.DisplayName
		se.Description = rangeElement.Description
		se.ModelType = ModelType(rangeElement.ModelType)
		if !isEmptyReference(rangeElement.SemanticId) {
			se.SemanticId = rangeElement.SemanticId
		}
		se.SupplementalSemanticIds = rangeElement.SupplementalSemanticIds
		se.Qualifiers = rangeElement.Qualifiers
		se.EmbeddedDataSpecifications = rangeElement.EmbeddedDataSpecifications
		return nil

	case "ReferenceElement":
		var refElement ReferenceElement
		if err := json.Unmarshal(data, &refElement); err != nil {
			return err
		}
		// Copy common fields from ReferenceElement to SubmodelElement
		se.Extensions = refElement.Extensions
		se.Category = refElement.Category
		se.IdShort = refElement.IdShort
		se.DisplayName = refElement.DisplayName
		se.Description = refElement.Description
		se.ModelType = ModelType(refElement.ModelType)
		if !isEmptyReference(refElement.SemanticId) {
			se.SemanticId = refElement.SemanticId
		}
		se.SupplementalSemanticIds = refElement.SupplementalSemanticIds
		se.Qualifiers = refElement.Qualifiers
		se.EmbeddedDataSpecifications = refElement.EmbeddedDataSpecifications
		return nil

	case "RelationshipElement":
		var relElement RelationshipElement
		if err := json.Unmarshal(data, &relElement); err != nil {
			return err
		}
		// Copy common fields from RelationshipElement to SubmodelElement
		se.Extensions = relElement.Extensions
		se.Category = relElement.Category
		se.IdShort = relElement.IdShort
		se.DisplayName = relElement.DisplayName
		se.Description = relElement.Description
		se.ModelType = ModelType(relElement.ModelType)
		if !isEmptyReference(relElement.SemanticId) {
			se.SemanticId = relElement.SemanticId
		}
		se.SupplementalSemanticIds = relElement.SupplementalSemanticIds
		se.Qualifiers = relElement.Qualifiers
		se.EmbeddedDataSpecifications = relElement.EmbeddedDataSpecifications
		return nil

	case "AnnotatedRelationshipElement":
		var annotatedRelElement AnnotatedRelationshipElement
		if err := json.Unmarshal(data, &annotatedRelElement); err != nil {
			return err
		}
		// Copy common fields from AnnotatedRelationshipElement to SubmodelElement
		se.Extensions = annotatedRelElement.Extensions
		se.Category = annotatedRelElement.Category
		se.IdShort = annotatedRelElement.IdShort
		se.DisplayName = annotatedRelElement.DisplayName
		se.Description = annotatedRelElement.Description
		se.ModelType = ModelType(annotatedRelElement.ModelType)
		if !isEmptyReference(annotatedRelElement.SemanticId) {
			se.SemanticId = annotatedRelElement.SemanticId
		}
		se.SupplementalSemanticIds = annotatedRelElement.SupplementalSemanticIds
		se.Qualifiers = annotatedRelElement.Qualifiers
		se.EmbeddedDataSpecifications = annotatedRelElement.EmbeddedDataSpecifications
		return nil

	case "SubmodelElementCollection":
		var collection SubmodelElementCollection
		if err := json.Unmarshal(data, &collection); err != nil {
			return err
		}
		// Copy common fields from SubmodelElementCollection to SubmodelElement
		se.Extensions = collection.Extensions
		se.Category = collection.Category
		se.IdShort = collection.IdShort
		se.DisplayName = collection.DisplayName
		se.Description = collection.Description
		se.ModelType = ModelType(collection.ModelType)
		if !isEmptyReference(collection.SemanticId) {
			se.SemanticId = collection.SemanticId
		}
		se.SupplementalSemanticIds = collection.SupplementalSemanticIds
		se.Qualifiers = collection.Qualifiers
		se.EmbeddedDataSpecifications = collection.EmbeddedDataSpecifications
		return nil

	case "SubmodelElementList":
		var list SubmodelElementList
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		// Copy common fields from SubmodelElementList to SubmodelElement
		se.Extensions = list.Extensions
		se.Category = list.Category
		se.IdShort = list.IdShort
		se.DisplayName = list.DisplayName
		se.Description = list.Description
		se.ModelType = ModelType(list.ModelType)
		if !isEmptyReference(list.SemanticId) {
			se.SemanticId = list.SemanticId
		}
		se.SupplementalSemanticIds = list.SupplementalSemanticIds
		se.Qualifiers = list.Qualifiers
		se.EmbeddedDataSpecifications = list.EmbeddedDataSpecifications
		return nil

	case "Operation":
		var operation Operation
		if err := json.Unmarshal(data, &operation); err != nil {
			return err
		}
		// Copy common fields from Operation to SubmodelElement
		se.Extensions = operation.Extensions
		se.Category = operation.Category
		se.IdShort = operation.IdShort
		se.DisplayName = operation.DisplayName
		se.Description = operation.Description
		se.ModelType = ModelType(operation.ModelType)
		if !isEmptyReference(operation.SemanticId) {
			se.SemanticId = operation.SemanticId
		}
		se.SupplementalSemanticIds = operation.SupplementalSemanticIds
		se.Qualifiers = operation.Qualifiers
		se.EmbeddedDataSpecifications = operation.EmbeddedDataSpecifications
		return nil

	case "Capability":
		var capability Capability
		if err := json.Unmarshal(data, &capability); err != nil {
			return err
		}
		// Copy common fields from Capability to SubmodelElement
		se.Extensions = capability.Extensions
		se.Category = capability.Category
		se.IdShort = capability.IdShort
		se.DisplayName = capability.DisplayName
		se.Description = capability.Description
		se.ModelType = ModelType(capability.ModelType)
		if !isEmptyReference(capability.SemanticId) {
			se.SemanticId = capability.SemanticId
		}
		se.SupplementalSemanticIds = capability.SupplementalSemanticIds
		se.Qualifiers = capability.Qualifiers
		se.EmbeddedDataSpecifications = capability.EmbeddedDataSpecifications
		return nil

	case "BasicEventElement":
		var basicEvent BasicEventElement
		if err := json.Unmarshal(data, &basicEvent); err != nil {
			return err
		}
		// Copy common fields from BasicEventElement to SubmodelElement
		se.Extensions = basicEvent.Extensions
		se.Category = basicEvent.Category
		se.IdShort = basicEvent.IdShort
		se.DisplayName = basicEvent.DisplayName
		se.Description = basicEvent.Description
		se.ModelType = ModelType(basicEvent.ModelType)
		if !isEmptyReference(basicEvent.SemanticId) {
			se.SemanticId = basicEvent.SemanticId
		}
		se.SupplementalSemanticIds = basicEvent.SupplementalSemanticIds
		se.Qualifiers = basicEvent.Qualifiers
		se.EmbeddedDataSpecifications = basicEvent.EmbeddedDataSpecifications
		return nil

	case "Entity":
		var entity Entity
		if err := json.Unmarshal(data, &entity); err != nil {
			return err
		}
		// Copy common fields from Entity to SubmodelElement
		se.Extensions = entity.Extensions
		se.Category = entity.Category
		se.IdShort = entity.IdShort
		se.DisplayName = entity.DisplayName
		se.Description = entity.Description
		se.ModelType = ModelType(entity.ModelType)
		if !isEmptyReference(entity.SemanticId) {
			se.SemanticId = entity.SemanticId
		}
		se.SupplementalSemanticIds = entity.SupplementalSemanticIds
		se.Qualifiers = entity.Qualifiers
		se.EmbeddedDataSpecifications = entity.EmbeddedDataSpecifications
		return nil

	default:
		return fmt.Errorf("unknown modelType: %s", modelType)
	}
}
