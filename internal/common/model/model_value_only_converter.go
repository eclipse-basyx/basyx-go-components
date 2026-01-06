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

import "fmt"

// ToValueOnly converts a Submodel to its Value-Only representation
func (s *Submodel) ToValueOnly() (SubmodelValue, error) {
	result := make(SubmodelValue)

	for _, element := range s.SubmodelElements {
		idShort := element.GetIdShort()
		if idShort == "" {
			continue // Skip elements without idShort
		}

		valueOnly, err := SubmodelElementToValueOnly(element)
		if err != nil {
			return nil, fmt.Errorf("failed to convert element '%s': %w", idShort, err)
		}

		if valueOnly != nil {
			result[idShort] = valueOnly
		}
	}

	return result, nil
}

// SubmodelElementToValueOnly converts any SubmodelElement to its Value-Only representation
func SubmodelElementToValueOnly(element SubmodelElement) (SubmodelElementValue, error) {
	switch e := element.(type) {
	case *Property:
		return PropertyToValueOnly(e), nil
	case *MultiLanguageProperty:
		return MultiLanguagePropertyToValueOnly(e), nil
	case *Range:
		return RangeToValueOnly(e), nil
	case *File:
		return FileToValueOnly(e), nil
	case *Blob:
		return BlobToValueOnly(e), nil
	case *ReferenceElement:
		return ReferenceElementToValueOnly(e), nil
	case *RelationshipElement:
		return RelationshipElementToValueOnly(e), nil
	case *AnnotatedRelationshipElement:
		return AnnotatedRelationshipElementToValueOnly(e), nil
	case *Entity:
		return EntityToValueOnly(e)
	case *BasicEventElement:
		return BasicEventElementToValueOnly(e), nil
	case *SubmodelElementCollection:
		return SubmodelElementCollectionToValueOnly(e)
	case *SubmodelElementList:
		return SubmodelElementListToValueOnly(e)
	default:
		// Capability and Operation are not serialized in Value-Only format
		return nil, nil
	}
}

// PropertyToValueOnly converts a Property to PropertyValue
func PropertyToValueOnly(p *Property) PropertyValue {
	return PropertyValue{Value: p.Value}
}

// MultiLanguagePropertyToValueOnly converts a MultiLanguageProperty to MultiLanguagePropertyValue
func MultiLanguagePropertyToValueOnly(mlp *MultiLanguageProperty) MultiLanguagePropertyValue {
	result := make(MultiLanguagePropertyValue, len(mlp.Value))
	for i, langString := range mlp.Value {
		langText := make(map[string]string)
		langText[langString.Language] = langString.Text
		result[i] = langText
	}
	return result
}

// RangeToValueOnly converts a Range to RangeValue
func RangeToValueOnly(r *Range) RangeValue {
	return RangeValue{
		Min: r.Min,
		Max: r.Max,
	}
}

// FileToValueOnly converts a File to FileValue
func FileToValueOnly(f *File) FileValue {
	return FileValue{
		ContentType: f.ContentType,
		Value:       f.Value,
	}
}

// BlobToValueOnly converts a Blob to BlobValue
func BlobToValueOnly(b *Blob) BlobValue {
	return BlobValue{
		ContentType: b.ContentType,
		Value:       b.Value,
	}
}

// ReferenceElementToValueOnly converts a ReferenceElement to ReferenceElementValue
func ReferenceElementToValueOnly(re *ReferenceElement) ReferenceElementValue {
	if re.Value == nil {
		return ReferenceElementValue{}
	}
	return ReferenceElementValue{
		Type: re.Value.Type,
		Keys: re.Value.Keys,
	}
}

// RelationshipElementToValueOnly converts a RelationshipElement to RelationshipElementValue
func RelationshipElementToValueOnly(re *RelationshipElement) RelationshipElementValue {
	result := RelationshipElementValue{}

	if re.First != nil {
		result.First = re.First
	}

	if re.Second != nil {
		result.Second = re.Second
	}

	return result
}

// AnnotatedRelationshipElementToValueOnly converts an AnnotatedRelationshipElement to AnnotatedRelationshipElementValue
func AnnotatedRelationshipElementToValueOnly(are *AnnotatedRelationshipElement) AnnotatedRelationshipElementValue {
	result := AnnotatedRelationshipElementValue{}

	if are.First != nil {
		result.First = *are.First
	}

	if are.Second != nil {
		result.Second = *are.Second
	}

	// Convert annotations
	if len(are.Annotations) > 0 {
		result.Annotations = make(map[string]SubmodelElementValue)
		for _, annotation := range are.Annotations {
			idShort := annotation.GetIdShort()
			if idShort == "" {
				continue
			}
			if annotationValue, err := SubmodelElementToValueOnly(annotation); err == nil && annotationValue != nil {
				result.Annotations[idShort] = annotationValue
			}
		}
	}

	return result
}

// EntityToValueOnly converts an Entity to EntityValue
func EntityToValueOnly(e *Entity) (EntityValue, error) {
	result := EntityValue{
		EntityType:    e.EntityType,
		GlobalAssetID: e.GlobalAssetID,
	}

	// Convert SpecificAssetIds
	if len(e.SpecificAssetIds) > 0 {
		result.SpecificAssetIds = make([]map[string]interface{}, 0, len(e.SpecificAssetIds))
		for _, assetID := range e.SpecificAssetIds {
			assetIDMap := map[string]interface{}{
				"name":  assetID.Name,
				"value": assetID.Value,
			}
			if assetID.ExternalSubjectID != nil {
				assetIDMap["externalSubjectId"] = assetID.ExternalSubjectID
			}
			result.SpecificAssetIds = append(result.SpecificAssetIds, assetIDMap)
		}
	}

	// Convert Statements
	if len(e.Statements) > 0 {
		statementsMap := make(map[string]SubmodelElementValue)
		for _, statement := range e.Statements {
			idShort := statement.GetIdShort()
			if idShort == "" {
				continue
			}

			valueOnly, err := SubmodelElementToValueOnly(statement)
			if err != nil {
				return result, fmt.Errorf("failed to convert statement '%s': %w", idShort, err)
			}

			if valueOnly != nil {
				statementsMap[idShort] = valueOnly
			}
		}
		result.Statements = statementsMap
	}

	return result, nil
}

// BasicEventElementToValueOnly converts a BasicEventElement to BasicEventElementValue
func BasicEventElementToValueOnly(bee *BasicEventElement) BasicEventElementValue {
	result := BasicEventElementValue{}

	if bee.Observed != nil {
		result.Observed = *bee.Observed
	}

	return result
}

// SubmodelElementCollectionToValueOnly converts a SubmodelElementCollection to SubmodelElementCollectionValue
func SubmodelElementCollectionToValueOnly(sec *SubmodelElementCollection) (SubmodelElementCollectionValue, error) {
	result := make(SubmodelElementCollectionValue)

	for _, element := range sec.Value {
		idShort := element.GetIdShort()
		if idShort == "" {
			continue
		}

		valueOnly, err := SubmodelElementToValueOnly(element)
		if err != nil {
			return nil, fmt.Errorf("failed to convert element '%s': %w", idShort, err)
		}

		if valueOnly != nil {
			result[idShort] = valueOnly
		}
	}

	return result, nil
}

// SubmodelElementListToValueOnly converts a SubmodelElementList to SubmodelElementListValue
func SubmodelElementListToValueOnly(sel *SubmodelElementList) (SubmodelElementListValue, error) {
	result := make(SubmodelElementListValue, 0, len(sel.Value))

	for i, element := range sel.Value {
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
