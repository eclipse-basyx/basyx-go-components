/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

import (
	"fmt"
	"sort"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
)

// ToValueOnly converts a Submodel to its Value-Only representation
func SubmodelToValueOnly(s types.ISubmodel) (SubmodelValue, error) {
	result := make(SubmodelValue)

	for _, element := range s.SubmodelElements() {
		idShort := element.IDShort()
		if idShort == nil || *idShort == "" {
			continue // Skip elements without idShort
		}

		valueOnly, err := SubmodelElementToValueOnly(element)
		if err != nil {
			elementID := "<unknown element>"
			if idShort != nil {
				elementID = *idShort
			}
			return nil, fmt.Errorf("failed to convert element '%s': %w", elementID, err)
		}

		if valueOnly != nil {
			result[*idShort] = valueOnly
		}
	}

	return result, nil
}

// SubmodelElementToValueOnly converts any SubmodelElement to its Value-Only representation
func SubmodelElementToValueOnly(element types.ISubmodelElement) (SubmodelElementValue, error) {
	switch e := element.(type) {
	case *types.Property:
		return PropertyToValueOnly(e), nil
	case *types.MultiLanguageProperty:
		return MultiLanguagePropertyToValueOnly(e), nil
	case *types.Range:
		return RangeToValueOnly(e), nil
	case *types.File:
		return FileToValueOnly(e), nil
	case *types.Blob:
		return BlobToValueOnly(e), nil
	case *types.ReferenceElement:
		return ReferenceElementToValueOnly(e), nil
	case *types.RelationshipElement:
		return RelationshipElementToValueOnly(e), nil
	case *types.AnnotatedRelationshipElement:
		return AnnotatedRelationshipElementToValueOnly(e), nil
	case *types.Entity:
		return EntityToValueOnly(e)
	case *types.BasicEventElement:
		return BasicEventElementToValueOnly(e), nil
	case *types.SubmodelElementCollection:
		return SubmodelElementCollectionToValueOnly(e)
	case *types.SubmodelElementList:
		return SubmodelElementListToValueOnly(e)
	default:
		// Capability and Operation are not serialized in Value-Only format
		return nil, nil
	}
}

// PropertyToValueOnly converts a Property to PropertyValue
func PropertyToValueOnly(p *types.Property) PropertyValue {
	if p.Value() == nil {
		return PropertyValue{}
	}
	return PropertyValue{Value: *p.Value()}
}

// MultiLanguagePropertyToValueOnly converts a MultiLanguageProperty to MultiLanguagePropertyValue
// Preserves the original order of language strings from the input
func MultiLanguagePropertyToValueOnly(mlp *types.MultiLanguageProperty) MultiLanguagePropertyValue {
	// Create a copy to avoid mutating input order
	vals := make([]types.ILangStringTextType, len(mlp.Value()))
	copy(vals, mlp.Value())

	// Ensure deterministic order by language code, then text as tie-breaker
	sort.SliceStable(vals, func(i, j int) bool {
		if vals[i].Language() == vals[j].Language() {
			return vals[i].Text() < vals[j].Text()
		}
		return vals[i].Language() < vals[j].Language()
	})

	result := make(MultiLanguagePropertyValue, 0, len(vals))
	for i := 0; i < len(vals); i++ {
		langString := vals[i]
		langText := make(map[string]string)
		langText[langString.Language()] = langString.Text()
		result = append(result, langText)
	}
	return result
}

// RangeToValueOnly converts a Range to RangeValue
func RangeToValueOnly(r *types.Range) RangeValue {
	return RangeValue{
		Min: r.Min(),
		Max: r.Max(),
	}
}

// FileToValueOnly converts a File to FileValue
func FileToValueOnly(f *types.File) FileValue {
	fileValue := FileValue{}
	if f.ContentType() != nil {
		fileValue.ContentType = *f.ContentType()
	} else {
		fileValue.ContentType = ""
	}
	if f.Value() != nil {
		fileValue.Value = *f.Value()
	} else {
		fileValue.Value = ""
	}
	return fileValue
}

// BlobToValueOnly converts a Blob to BlobValue
func BlobToValueOnly(b *types.Blob) BlobValue {
	blobValue := BlobValue{}
	if b.ContentType() != nil {
		blobValue.ContentType = *b.ContentType()
	} else {
		blobValue.ContentType = ""
	}
	if b.Value() != nil {
		blobValue.Value = b.Value()
	} else {
		blobValue.Value = nil
	}
	return blobValue
}

// ReferenceElementToValueOnly converts a ReferenceElement to ReferenceElementValue
func ReferenceElementToValueOnly(re *types.ReferenceElement) ReferenceElementValue {
	if re.Value() == nil {
		return ReferenceElementValue{}
	}

	refElemVal := ReferenceElementValue{}

	refType, ok := stringification.ReferenceTypesToString(re.Value().Type())
	if ok {
		refElemVal.Type = refType
	}

	var keys []map[string]any
	for _, key := range re.Value().Keys() {
		key, err := jsonization.ToJsonable(key)
		if err == nil {
			keys = append(keys, key)
		}
		refElemVal.Keys = keys
	}
	return refElemVal
}

// RelationshipElementToValueOnly converts a RelationshipElement to RelationshipElementValue
func RelationshipElementToValueOnly(re *types.RelationshipElement) RelationshipElementValue {
	result := RelationshipElementValue{}

	firstJsonable, err := jsonization.ToJsonable(re.First())
	if err == nil {
		result.First = firstJsonable
	}

	secondJsonable, err := jsonization.ToJsonable(re.Second())
	if err == nil {
		result.Second = secondJsonable
	}

	return result
}

// AnnotatedRelationshipElementToValueOnly converts an AnnotatedRelationshipElement to AnnotatedRelationshipElementValue
func AnnotatedRelationshipElementToValueOnly(are *types.AnnotatedRelationshipElement) AnnotatedRelationshipElementValue {
	result := AnnotatedRelationshipElementValue{}

	firstJsonable, err := jsonization.ToJsonable(are.First())
	if err == nil {
		result.First = firstJsonable
	}

	secondJsonable, err := jsonization.ToJsonable(are.Second())
	if err == nil {
		result.Second = secondJsonable
	}

	// Convert annotations
	if len(are.Annotations()) > 0 {
		result.Annotations = make(map[string]SubmodelElementValue)
		for _, annotation := range are.Annotations() {
			idShort := annotation.IDShort()
			if idShort == nil || *idShort == "" {
				continue
			}
			if annotationValue, err := SubmodelElementToValueOnly(annotation); err == nil && annotationValue != nil {
				result.Annotations[*idShort] = annotationValue
			}
		}
	}

	return result
}

// EntityToValueOnly converts an Entity to EntityValue
func EntityToValueOnly(e *types.Entity) (EntityValue, error) {
	result := EntityValue{}
	if e.EntityType() != nil {
		entityType, ok := stringification.EntityTypeToString(*e.EntityType())
		if !ok {
			return EntityValue{}, fmt.Errorf("unknown entity type: %v", e.EntityType())
		}
		result.EntityType = entityType
	}

	if e.GlobalAssetID() != nil {
		result.GlobalAssetID = *e.GlobalAssetID()
	}

	// Convert SpecificAssetIds
	if len(e.SpecificAssetIDs()) > 0 {
		result.SpecificAssetIds = make([]map[string]any, 0, len(e.SpecificAssetIDs()))
		for _, assetID := range e.SpecificAssetIDs() {
			assetIDMap := map[string]any{
				"name":  assetID.Name(),
				"value": assetID.Value(),
			}
			if assetID.ExternalSubjectID() != nil {
				assetIDMap["externalSubjectId"] = assetID.ExternalSubjectID()
			}
			result.SpecificAssetIds = append(result.SpecificAssetIds, assetIDMap)
		}
	}

	// Convert Statements
	if len(e.Statements()) > 0 {
		statementsMap := make(map[string]SubmodelElementValue)
		for _, statement := range e.Statements() {
			idShort := statement.IDShort()
			if idShort == nil || *idShort == "" {
				continue
			}

			valueOnly, err := SubmodelElementToValueOnly(statement)
			if err != nil {
				return result, fmt.Errorf("failed to convert statement '%s': %w", *idShort, err)
			}

			if valueOnly != nil {
				statementsMap[*idShort] = valueOnly
			}
		}
		result.Statements = statementsMap
	}

	return result, nil
}

// BasicEventElementToValueOnly converts a BasicEventElement to BasicEventElementValue
func BasicEventElementToValueOnly(bee *types.BasicEventElement) BasicEventElementValue {
	result := BasicEventElementValue{}

	if bee.Observed() != nil {
		observedJsonable, err := jsonization.ToJsonable(bee.Observed())
		if err == nil {
			result.Observed = observedJsonable
		}
	}

	return result
}

// SubmodelElementCollectionToValueOnly converts a SubmodelElementCollection to SubmodelElementCollectionValue
func SubmodelElementCollectionToValueOnly(sec *types.SubmodelElementCollection) (SubmodelElementCollectionValue, error) {
	result := make(SubmodelElementCollectionValue)

	for _, element := range sec.Value() {
		idShort := element.IDShort()
		if idShort == nil || *idShort == "" {
			continue
		}

		valueOnly, err := SubmodelElementToValueOnly(element)
		if err != nil {
			return nil, fmt.Errorf("failed to convert element '%s': %w", *idShort, err)
		}

		if valueOnly != nil {
			result[*idShort] = valueOnly
		}
	}

	return result, nil
}

// SubmodelElementListToValueOnly converts a SubmodelElementList to SubmodelElementListValue
func SubmodelElementListToValueOnly(sel *types.SubmodelElementList) (SubmodelElementListValue, error) {
	result := make(SubmodelElementListValue, 0, len(sel.Value()))

	for i, element := range sel.Value() {
		valueOnly, err := SubmodelElementToValueOnly(element)
		if err != nil {
			return nil, fmt.Errorf("failed to convert element at index %d: %w", i, err)
		}

		if valueOnly != nil {
			result = append(result, valueOnly)
		}
	}

	return result, nil
}
