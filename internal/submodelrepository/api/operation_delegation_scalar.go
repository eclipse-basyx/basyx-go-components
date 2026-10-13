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
	"errors"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func applyOperationScalarValue(element map[string]any, field string, value any) error {
	valueTypeName, _ := element["valueType"].(string)
	valueType, ok := stringification.DataTypeDefXSDFromString(valueTypeName)
	if !ok {
		return fmt.Errorf("SMREPO-OPVALREQ-VALUETYPE invalid declared valueType %q", valueTypeName)
	}
	lexical, err := gen.ValueOnlyScalarFromJSON(value, valueType)
	if errors.Is(err, gen.ErrValueOnlyScalarKind) {
		return fmt.Errorf("SMREPO-OPVALREQ-PROPERTYSHAPE %w", err)
	}
	if err != nil {
		return fmt.Errorf("SMREPO-OPVALREQ-SCALARVALUE %w", err)
	}
	if lexical == nil {
		delete(element, field)
		return nil
	}
	element[field] = *lexical
	return nil
}

func applyOperationRangeValue(element map[string]any, value any) error {
	fields, err := valueOnlyObject(value, "Range")
	if err != nil {
		return err
	}
	if err := validateObjectFields(fields, []string{"min", "max"}, []string{"min", "max"}, "Range"); err != nil {
		return err
	}
	for _, field := range []string{"min", "max"} {
		if err := applyOperationScalarValue(element, field, fields[field]); err != nil {
			return fmt.Errorf("SMREPO-OPVALREQ-RANGE-%s %w", field, err)
		}
	}
	return nil
}
