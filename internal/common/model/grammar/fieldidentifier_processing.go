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
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Martin Stemmer ( Fraunhofer IESE )

package grammar

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
)

// ArrayIndexBinding represents a concrete index access on an array-like
// segment of a field path that has been normalized into SQL.
//
// Each binding links a resolved SQL table alias to a specific index
// within that array. This struct is only used when an array index is explicitly
// specified in the FieldIdentifier; open-ended array accesses (e.g. "[]")
// do not produce any ArrayIndexBinding entries.
type ArrayIndexBinding struct {
	// Alias is the SQL identifier for the array position column.
	// Typically this ends with ".position" for array bindings (e.g. "specific_asset_id.position"),
	// but it may also reference other binding columns (e.g. "submodel_element.idshort_path" for $sme).
	Alias string

	// Index is the concrete binding value.
	//
	// For array segments, this is the numeric position. For $sme idShortPath constraints,
	// this contains the extracted idShortPath string.
	Index ArrayIndex
}

// ArrayIndex is a small JSON-union type that can represent either a numeric array index
// or a string identifier (used for $sme idShortPath constraints).
type ArrayIndex struct {
	intValue    *int
	stringValue *string
}

// NewArrayIndexPosition creates an ArrayIndex representing a numeric array position.
func NewArrayIndexPosition(i int) ArrayIndex {
	return ArrayIndex{intValue: &i}
}

// NewArrayIndexString creates an ArrayIndex representing a string constraint (e.g. $sme idShortPath).
func NewArrayIndexString(s string) ArrayIndex {
	return ArrayIndex{stringValue: &s}
}

// MarshalJSON implements json.Marshaler.
//
// It encodes numeric indices as JSON numbers and string indices as JSON strings.
func (a ArrayIndex) MarshalJSON() ([]byte, error) {
	if a.intValue != nil {
		return []byte(fmt.Sprintf("%d", *a.intValue)), nil
	}
	if a.stringValue != nil {
		return json.Marshal(*a.stringValue)
	}
	return []byte("null"), nil
}

// UnmarshalJSON implements json.Unmarshaler.
//
// It accepts either a JSON number (stored as an int) or a JSON string.
func (a *ArrayIndex) UnmarshalJSON(b []byte) error {
	// Accept either JSON number (int) or JSON string.
	var asNumber json.Number
	if err := json.Unmarshal(b, &asNumber); err == nil {
		if i, err := asNumber.Int64(); err == nil {
			iv := int(i)
			a.intValue = &iv
			a.stringValue = nil
			return nil
		}
	}
	var asString string
	if err := json.Unmarshal(b, &asString); err == nil {
		a.stringValue = &asString
		a.intValue = nil
		return nil
	}
	return fmt.Errorf("invalid ArrayIndex JSON: %s", string(b))
}

// ResolvedFieldPath is the SQL-resolved representation of a FieldIdentifier.
//
// It consists of a base SQL column expression and an ordered list of
// array index bindings that must be applied when constructing joins
// or predicates for array-backed structures.
// can be produced by this value: {"$field": "$aasdesc#specificAssetIds[2].externalSubjectId.keys[3].value"}
type ResolvedFieldPath struct {
	// Column is the final SQL column or expression corresponding to
	// the terminal field of the identifier.
	Column string

	// ArrayBindings contains index constraints for each array segment
	// encountered while resolving the field path.
	//
	// The bindings are ordered from outermost to innermost array access
	// as they appear in the original FieldIdentifier.
	ArrayBindings []ArrayIndexBinding
}

// ResolveScalarFieldToSQL converts a FieldIdentifier value into its SQL
// representation.
//
// The function parses a field path DSL of the form:
//
//	"$<root>#<path>"
//
// where <path> may contain nested object accessors and array selectors
// using either wildcard (`[]`) or concrete index (`[n]`) notation.
//
// Example inputs:
//
//  1. {"$field": "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"}
//  2. {"$field": "$aasdesc#specificAssetIds[2].externalSubjectId.keys[5].value"}
//  3. {"$field": "$aasdesc#endpoints[].protocolinformation.href"}
//
// Example outputs:
//
//	1.
//	ResolvedFieldPath{
//	    Column: "external_subject_reference_key.value",
//	    ArrayBindings: nil, // wildcards ("[]") do not add bindings
//	}
//
//	2.
//	ResolvedFieldPath{
//	    Column: "external_subject_reference_key.value",
//	    ArrayBindings: []ArrayIndexBinding{
//	        {Alias: "specific_asset_id.position", Index: 2},
//	        {Alias: "external_subject_reference_key.position", Index: 5},
//	    },
//	}
//
// Fragment FieldIdentifiers (examples 4 and 5) resolve only to array bindings
// without a terminal column and are intended to be used as join or existence
// constraints rather than direct column references.
func ResolveScalarFieldToSQL(field *ModelStringPattern) (ResolvedFieldPath, error) {
	if field == nil {
		return ResolvedFieldPath{}, fmt.Errorf("field is nil")
	}

	fieldStr := string(*field)
	if !strings.Contains(fieldStr, "#") {
		return ResolvedFieldPath{}, fmt.Errorf("invalid field identifier (missing '#'): %q", fieldStr)
	}

	tokens := builder.TokenizeField(fieldStr)
	if len(tokens) == 0 {
		return ResolvedFieldPath{}, fmt.Errorf("invalid field identifier (empty path): %q", fieldStr)
	}

	// Scalar identifiers must end in a concrete terminal field, not an array segment.
	if _, ok := tokens[len(tokens)-1].(builder.ArrayToken); ok {
		return ResolvedFieldPath{}, fmt.Errorf("scalar field identifier must not end in an array segment: %q", fieldStr)
	}

	column, err := ResolveAASQLFieldToSQLColumn(fieldStr)
	if err != nil {
		return ResolvedFieldPath{}, err
	}

	bindings, err := resolveArrayBindings(fieldStr, tokens)
	if err != nil {
		return ResolvedFieldPath{}, err
	}

	return ResolvedFieldPath{Column: column, ArrayBindings: bindings}, nil
}

// ResolveFragmentFieldToSQL resolves a fragment identifier that ends in an array segment.
// Returns only bindings. Errors if the identifier ends in a concrete column.
// Example inputs:
//  1. {"$field": "$aasdesc#endpoints[]"}      // Fragment FieldIdentifier
//  2. {"$field": "$aasdesc#endpoints[2]"}     // Fragment FieldIdentifier
//  3. {"$field": "$aasdesc#endpoints[2].protocolinformation.href"} // should also work but has no main column value still
func ResolveFragmentFieldToSQL(field *FragmentStringPattern) ([]ArrayIndexBinding, error) {
	if field == nil {
		return nil, fmt.Errorf("field is nil")
	}

	fieldStr := string(*field)
	if !strings.Contains(fieldStr, "#") {
		return nil, fmt.Errorf("invalid field identifier (missing '#'): %q", fieldStr)
	}

	tokens := builder.TokenizeField(fieldStr)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("invalid field identifier (empty path): %q", fieldStr)
	}

	return resolveArrayBindings(fieldStr, tokens)
}

type resolveContext int

const (
	ctxUnknown resolveContext = iota
	ctxAASDesc
	ctxSMDesc
	ctxSM
	ctxSME
	ctxSpecificAssetID
	ctxAASDescEndpoint
	ctxSubmodelDescriptor
	ctxSubmodelDescriptorEndpoint
)

type arraySegmentContextMapping struct {
	PositionAlias string
	NextContext   resolveContext
}

type arraySegmentMapping struct {
	// ByContext is used for array segments that don't require a preceding simple token.
	ByContext map[resolveContext]arraySegmentContextMapping

	// ByParent is used for array segments that depend on a preceding simple token
	// (e.g. "semanticId.keys[]" vs "externalSubjectId.keys[]").
	ByParent map[string]map[resolveContext]arraySegmentContextMapping
}

// arraySegmentMappings defines how array-like path segments map to SQL join aliases.
//
// The mapping is intentionally centralized so that supported fragment/scalar field
// identifiers can be extended by adding data rather than growing switch statements.
//
// NOTE: PositionAlias must already include the trailing ".position".
var arraySegmentMappings = map[string]arraySegmentMapping{
	"specificAssetIds": {
		ByContext: map[resolveContext]arraySegmentContextMapping{
			ctxAASDesc: {PositionAlias: "specific_asset_id.position", NextContext: ctxSpecificAssetID},
		},
	},

	"endpoints": {
		ByContext: map[resolveContext]arraySegmentContextMapping{
			ctxAASDesc:            {PositionAlias: "aas_descriptor_endpoint.position", NextContext: ctxAASDescEndpoint},
			ctxSMDesc:             {PositionAlias: "submodel_descriptor_endpoint.position", NextContext: ctxSubmodelDescriptorEndpoint},
			ctxSubmodelDescriptor: {PositionAlias: "submodel_descriptor_endpoint.position", NextContext: ctxSubmodelDescriptorEndpoint},
		},
	},

	"submodelDescriptors": {
		ByContext: map[resolveContext]arraySegmentContextMapping{
			ctxAASDesc: {PositionAlias: "submodel_descriptor.position", NextContext: ctxSubmodelDescriptor},
		},
	},

	"keys": {
		ByParent: map[string]map[resolveContext]arraySegmentContextMapping{
			"externalSubjectId": {
				ctxSpecificAssetID: {PositionAlias: "external_subject_reference_key.position", NextContext: ctxSpecificAssetID},
			},
			"semanticId": {
				ctxSM:                 {PositionAlias: "semantic_id_reference_key.position", NextContext: ctxSM},
				ctxSME:                {PositionAlias: "semantic_id_reference_key.position", NextContext: ctxSME},
				ctxSMDesc:             {PositionAlias: "aasdesc_submodel_descriptor_semantic_id_reference_key.position", NextContext: ctxSMDesc},
				ctxSubmodelDescriptor: {PositionAlias: "aasdesc_submodel_descriptor_semantic_id_reference_key.position", NextContext: ctxSubmodelDescriptor},
			},
		},
	},
}

func contextFromFieldPrefix(fieldStr string) resolveContext {
	// prefix is like "$aasdesc" (before '#')
	parts := strings.SplitN(fieldStr, "#", 2)
	if len(parts) != 2 {
		return ctxUnknown
	}
	// For $sme we allow an idShortPath before '#', e.g. "$sme.temperature".
	prefix := parts[0]
	root := prefix
	if idx := strings.Index(prefix, "."); idx >= 0 {
		root = prefix[:idx]
	}
	switch root {
	case "$aasdesc":
		return ctxAASDesc
	case "$smdesc":
		return ctxSMDesc
	case "$sm":
		return ctxSM
	case "$sme":
		return ctxSME
	default:
		return ctxUnknown
	}
}

func smeIDShortPathFromField(fieldStr string) (string, bool) {
	parts := strings.SplitN(fieldStr, "#", 2)
	if len(parts) != 2 {
		return "", false
	}
	prefix := parts[0]
	if !strings.HasPrefix(prefix, "$sme") {
		return "", false
	}
	path := strings.TrimPrefix(prefix, "$sme")
	path = strings.TrimPrefix(path, ".")
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	return path, true
}

func resolveArrayBindings(fieldStr string, tokens []builder.Token) ([]ArrayIndexBinding, error) {
	ctx := contextFromFieldPrefix(fieldStr)
	if ctx == ctxUnknown {
		// Keep error explicit: this is meant for registry queries today.
		return nil, fmt.Errorf("unsupported field root (expected $aasdesc#, $smdesc#, $sm#, or $sme...#): %q", fieldStr)
	}

	var bindings []ArrayIndexBinding
	if ctx == ctxSME {
		if idShortPath, ok := smeIDShortPathFromField(fieldStr); ok {
			bindings = append(bindings, ArrayIndexBinding{Alias: "submodel_element.idshort_path", Index: NewArrayIndexString(idShortPath)})
		}
	}
	prevSimple := ""
	for _, tok := range tokens {
		switch t := tok.(type) {
		case builder.SimpleToken:
			prevSimple = t.Name

		case builder.ArrayToken:
			positionAlias, nextCtx, err := resolveArrayToken(fieldStr, ctx, prevSimple, t.Name)
			if err != nil {
				return nil, err
			}
			// Only explicit array indices create bindings. Wildcard "[]" produces no binding.
			if t.Index >= 0 {
				bindings = append(bindings, ArrayIndexBinding{Alias: positionAlias, Index: NewArrayIndexPosition(t.Index)})
			}
			ctx = nextCtx
			prevSimple = ""

		default:
			return nil, fmt.Errorf("unsupported token type while resolving: %T", tok)
		}
	}

	return bindings, nil
}

func resolveArrayToken(fieldStr string, ctx resolveContext, prevSimple string, arrayName string) (positionAlias string, next resolveContext, err error) {
	mapping, ok := arraySegmentMappings[arrayName]
	if !ok {
		return "", ctx, fmt.Errorf("unsupported array segment %q in field %q", arrayName, fieldStr)
	}

	if mapping.ByParent != nil {
		parentMappings, ok := mapping.ByParent[prevSimple]
		if !ok {
			return "", ctx, fmt.Errorf("cannot resolve %s[] array without a known parent (got %q) for field %q", arrayName, prevSimple, fieldStr)
		}
		ctxMapping, ok := parentMappings[ctx]
		if !ok {
			return "", ctx, fmt.Errorf("%s.%s not valid in this context for field %q", prevSimple, arrayName, fieldStr)
		}
		return ctxMapping.PositionAlias, ctxMapping.NextContext, nil
	}

	ctxMapping, ok := mapping.ByContext[ctx]
	if !ok {
		return "", ctx, fmt.Errorf("%s not valid in this context for field %q", arrayName, fieldStr)
	}
	return ctxMapping.PositionAlias, ctxMapping.NextContext, nil
}
