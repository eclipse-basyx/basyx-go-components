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

package api

import (
	"encoding/json"

	"github.com/FriedJannik/aas-go-sdk/types"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type operationScalarValue struct {
	value     any
	modelType types.ModelType
}

// MarshalValueOnly encodes a scalar or range using its declared JSON value types.
func (v operationScalarValue) MarshalValueOnly() ([]byte, error) { return json.Marshal(v.value) }

// MarshalJSON encodes the value-only representation in operation result envelopes.
func (v operationScalarValue) MarshalJSON() ([]byte, error) { return v.MarshalValueOnly() }

// GetModelType returns the declared submodel element type.
func (v operationScalarValue) GetModelType() types.ModelType { return v.modelType }

func operationElementToValueOnly(element types.ISubmodelElement) (gen.SubmodelElementValue, error) {
	switch value := element.(type) {
	case *types.Property:
		scalar, err := operationScalarOutput(value.Value(), value.ValueType())
		return operationScalarValue{value: scalar, modelType: types.ModelTypeProperty}, err
	case *types.Range:
		return operationRangeToValueOnly(value)
	case *types.SubmodelElementCollection:
		return operationNamedValues(value.Value())
	case *types.SubmodelElementList:
		return operationListToValueOnly(value)
	case *types.Entity:
		return operationEntityToValueOnly(value)
	case *types.AnnotatedRelationshipElement:
		return operationAnnotationsToValueOnly(value)
	default:
		return gen.SubmodelElementToValueOnly(element)
	}
}

func operationRangeToValueOnly(value *types.Range) (gen.SubmodelElementValue, error) {
	minimum, err := operationScalarOutput(value.Min(), value.ValueType())
	if err != nil {
		return nil, err
	}
	maximum, err := operationScalarOutput(value.Max(), value.ValueType())
	if err != nil {
		return nil, err
	}
	return operationScalarValue{value: map[string]any{"min": minimum, "max": maximum}, modelType: types.ModelTypeRange}, nil
}

func operationNamedValues[T types.ISubmodelElement](elements []T) (gen.SubmodelElementCollectionValue, error) {
	values := make(gen.SubmodelElementCollectionValue, len(elements))
	for _, element := range elements {
		name := element.IDShort()
		if name == nil || *name == "" {
			continue
		}
		value, err := operationElementToValueOnly(element)
		if err != nil {
			return nil, err
		}
		if value != nil {
			values[*name] = value
		}
	}
	return values, nil
}

func operationListToValueOnly(list *types.SubmodelElementList) (gen.SubmodelElementValue, error) {
	values := make(gen.SubmodelElementListValue, 0, len(list.Value()))
	for _, element := range list.Value() {
		value, err := operationElementToValueOnly(element)
		if err != nil {
			return nil, err
		}
		if value != nil {
			values = append(values, value)
		}
	}
	return values, nil
}

func operationEntityToValueOnly(entity *types.Entity) (gen.SubmodelElementValue, error) {
	shallow := *entity
	shallow.SetStatements(nil)
	value, err := gen.EntityToValueOnly(&shallow)
	if err != nil {
		return nil, err
	}
	value.Statements, err = operationNamedValues(entity.Statements())
	return value, err
}

func operationAnnotationsToValueOnly(relationship *types.AnnotatedRelationshipElement) (gen.SubmodelElementValue, error) {
	shallow := *relationship
	shallow.SetAnnotations(nil)
	value := gen.AnnotatedRelationshipElementToValueOnly(&shallow)
	var err error
	value.Annotations, err = operationNamedValues(relationship.Annotations())
	return value, err
}
