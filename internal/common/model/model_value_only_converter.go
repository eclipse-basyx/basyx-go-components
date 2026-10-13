/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.2.0
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

type valueOnlyConverter struct {
	strict bool
}

// SubmodelToValueOnly converts a Submodel to its Value-Only representation
func SubmodelToValueOnly(s types.ISubmodel) (SubmodelValue, error) {
	result, err := valueOnlyConverter{}.namedElements(s.SubmodelElements())
	return SubmodelValue(result), err
}

// SubmodelElementToValueOnly converts any SubmodelElement to its Value-Only representation.
// Scalar values without a JSON-native representation are serialized as their lexical string.
func SubmodelElementToValueOnly(element types.ISubmodelElement) (SubmodelElementValue, error) {
	return valueOnlyConverter{}.element(element)
}

// SubmodelElementToValueOnlyStrict converts any SubmodelElement to its Value-Only representation
// and rejects scalar values that are not consistent with their valueType.
func SubmodelElementToValueOnlyStrict(element types.ISubmodelElement) (SubmodelElementValue, error) {
	return valueOnlyConverter{strict: true}.element(element)
}

func (c valueOnlyConverter) element(element types.ISubmodelElement) (SubmodelElementValue, error) {
	switch e := element.(type) {
	case *types.Property:
		return c.property(e)
	case *types.MultiLanguageProperty:
		return MultiLanguagePropertyToValueOnly(e), nil
	case *types.Range:
		return c.rangeValue(e)
	case *types.File:
		return FileToValueOnly(e), nil
	case *types.Blob:
		return BlobToValueOnly(e), nil
	case *types.ReferenceElement:
		return ReferenceElementToValueOnly(e), nil
	case *types.RelationshipElement:
		return RelationshipElementToValueOnly(e), nil
	case *types.AnnotatedRelationshipElement:
		return c.annotatedRelationshipElement(e)
	case *types.Entity:
		return c.entity(e)
	case *types.BasicEventElement:
		return BasicEventElementToValueOnly(e), nil
	case *types.SubmodelElementCollection:
		value, err := c.namedElements(e.Value())
		return SubmodelElementCollectionValue(value), err
	case *types.SubmodelElementList:
		return c.list(e)
	default:
		// Capability and Operation are not serialized in Value-Only format
		return nil, nil
	}
}

func (c valueOnlyConverter) property(p *types.Property) (SubmodelElementValue, error) {
	if err := c.verifyScalar(p.Value(), p.ValueType()); err != nil {
		return nil, err
	}
	return PropertyValue{Value: p.Value(), ValueType: p.ValueType()}, nil
}

func (c valueOnlyConverter) rangeValue(r *types.Range) (SubmodelElementValue, error) {
	if err := c.verifyScalar(r.Min(), r.ValueType()); err != nil {
		return nil, fmt.Errorf("min: %w", err)
	}
	if err := c.verifyScalar(r.Max(), r.ValueType()); err != nil {
		return nil, fmt.Errorf("max: %w", err)
	}
	return RangeValue{Min: r.Min(), Max: r.Max(), ValueType: r.ValueType()}, nil
}

func (c valueOnlyConverter) verifyScalar(lexical *string, valueType types.DataTypeDefXSD) error {
	if !c.strict {
		return nil
	}
	_, err := ValueOnlyScalarToJSONStrict(lexical, valueType)
	return err
}

func (c valueOnlyConverter) namedElements(elements []types.ISubmodelElement) (map[string]SubmodelElementValue, error) {
	result := make(map[string]SubmodelElementValue, len(elements))
	for _, element := range elements {
		idShort := element.IDShort()
		if idShort == nil || *idShort == "" {
			continue
		}
		valueOnly, err := c.element(element)
		if err != nil {
			return nil, fmt.Errorf("failed to convert element '%s': %w", *idShort, err)
		}
		if valueOnly != nil {
			result[*idShort] = valueOnly
		}
	}
	return result, nil
}

func (c valueOnlyConverter) list(sel *types.SubmodelElementList) (SubmodelElementValue, error) {
	result := make(SubmodelElementListValue, 0, len(sel.Value()))
	for i, element := range sel.Value() {
		valueOnly, err := c.element(element)
		if err != nil {
			return nil, fmt.Errorf("failed to convert element at index %d: %w", i, err)
		}
		if valueOnly != nil {
			result = append(result, valueOnly)
		}
	}
	return result, nil
}

func (c valueOnlyConverter) annotatedRelationshipElement(are *types.AnnotatedRelationshipElement) (SubmodelElementValue, error) {
	result := AnnotatedRelationshipElementValue{
		First:  referenceJsonable(are.First()),
		Second: referenceJsonable(are.Second()),
	}
	if len(are.Annotations()) == 0 {
		return result, nil
	}
	annotations := make([]types.ISubmodelElement, 0, len(are.Annotations()))
	for _, annotation := range are.Annotations() {
		annotations = append(annotations, annotation)
	}
	values, err := c.namedElements(annotations)
	if err != nil {
		return nil, fmt.Errorf("failed to convert annotations: %w", err)
	}
	result.Annotations = values
	return result, nil
}

func (c valueOnlyConverter) entity(e *types.Entity) (SubmodelElementValue, error) {
	result := EntityValue{}
	if e.EntityType() != nil {
		entityType, ok := stringification.EntityTypeToString(*e.EntityType())
		if !ok {
			return nil, fmt.Errorf("unknown entity type: %v", e.EntityType())
		}
		result.EntityType = entityType
	}
	if e.GlobalAssetID() != nil {
		result.GlobalAssetID = *e.GlobalAssetID()
	}
	result.SpecificAssetIds = specificAssetIDsToValueOnly(e.SpecificAssetIDs())
	if len(e.Statements()) == 0 {
		return result, nil
	}
	statements, err := c.namedElements(e.Statements())
	if err != nil {
		return nil, fmt.Errorf("failed to convert statements: %w", err)
	}
	result.Statements = statements
	return result, nil
}

func specificAssetIDsToValueOnly(assetIDs []types.ISpecificAssetID) []map[string]any {
	if len(assetIDs) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		jsonable, err := jsonization.ToJsonable(assetID)
		if err == nil {
			result = append(result, jsonable)
		}
	}
	return result
}

func referenceJsonable(reference types.IReference) map[string]any {
	if reference == nil {
		return nil
	}
	jsonable, err := jsonization.ToJsonable(reference)
	if err != nil {
		return nil
	}
	return jsonable
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

	if referredSemanticID := re.Value().ReferredSemanticID(); referredSemanticID != nil {
		referredSemanticIDJSON, err := jsonization.ToJsonable(referredSemanticID)
		if err == nil {
			refElemVal.ReferredSemanticId = referredSemanticIDJSON
		}
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
	return RelationshipElementValue{
		First:  referenceJsonable(re.First()),
		Second: referenceJsonable(re.Second()),
	}
}

// BasicEventElementToValueOnly converts a BasicEventElement to BasicEventElementValue
func BasicEventElementToValueOnly(bee *types.BasicEventElement) BasicEventElementValue {
	return BasicEventElementValue{Observed: referenceJsonable(bee.Observed())}
}
