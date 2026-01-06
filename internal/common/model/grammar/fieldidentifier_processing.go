package grammar

import (
	"fmt"
	"strings"

	builder "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
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
	// It always ends with ".position" (e.g. "specific_asset_id.position").
	Alias string

	// Index is the concrete array position.
	Index int
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

	column := ParseAASQLFieldToSQLColumn(fieldStr)
	// If no mapping exists, ParseAASQLFieldToSQLColumn returns the input unchanged.
	if column == fieldStr || strings.Contains(column, "$") {
		return ResolvedFieldPath{}, fmt.Errorf("unsupported or unmapped field identifier: %q", fieldStr)
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

	// Fragment identifiers must end in an array segment.
	if _, ok := tokens[len(tokens)-1].(builder.ArrayToken); !ok {
		return nil, fmt.Errorf("fragment field identifier must end in an array segment: %q", fieldStr)
	}

	return resolveArrayBindings(fieldStr, tokens)
}

type resolveContext int

const (
	ctxUnknown resolveContext = iota
	ctxAASDesc
	ctxSMDesc
	ctxSM
	ctxSpecificAssetID
	ctxAASDescEndpoint
	ctxSubmodelDescriptor
	ctxSubmodelDescriptorEndpoint
)

func contextFromFieldPrefix(fieldStr string) resolveContext {
	// prefix is like "$aasdesc" (before '#')
	parts := strings.SplitN(fieldStr, "#", 2)
	if len(parts) != 2 {
		return ctxUnknown
	}
	switch parts[0] {
	case "$aasdesc":
		return ctxAASDesc
	case "$smdesc":
		return ctxSMDesc
	case "$sm":
		return ctxSM
	default:
		return ctxUnknown
	}
}

func resolveArrayBindings(fieldStr string, tokens []builder.Token) ([]ArrayIndexBinding, error) {
	ctx := contextFromFieldPrefix(fieldStr)
	if ctx == ctxUnknown {
		// Keep error explicit: this is meant for registry queries today.
		return nil, fmt.Errorf("unsupported field root (expected $aasdesc#, $smdesc#, or $sm#): %q", fieldStr)
	}

	var bindings []ArrayIndexBinding
	prevSimple := ""
	for _, tok := range tokens {
		switch t := tok.(type) {
		case builder.SimpleToken:
			prevSimple = t.Name

		case builder.ArrayToken:
			alias, nextCtx, err := resolveArrayToken(fieldStr, ctx, prevSimple, t.Name)
			if err != nil {
				return nil, err
			}
			// Only explicit array indices create bindings. Wildcard "[]" produces no binding.
			if t.Index >= 0 {
				bindings = append(bindings, ArrayIndexBinding{Alias: alias + ".position", Index: t.Index})
			}
			ctx = nextCtx
			prevSimple = ""

		default:
			return nil, fmt.Errorf("unsupported token type while resolving: %T", tok)
		}
	}

	return bindings, nil
}

func resolveArrayToken(fieldStr string, ctx resolveContext, prevSimple string, arrayName string) (alias string, next resolveContext, err error) {
	// Aliases must match the ones used by descriptor SQL builders.
	switch arrayName {
	case "specificAssetIds":
		if ctx != ctxAASDesc {
			return "", ctx, fmt.Errorf("specificAssetIds not valid in this context for field %q", fieldStr)
		}
		return "specific_asset_id", ctxSpecificAssetID, nil

	case "endpoints":
		// endpoints can be on $aasdesc, $smdesc, or inside a submodelDescriptor (which is also a descriptor)
		switch ctx {
		case ctxAASDesc:
			return "aas_descriptor_endpoint", ctxAASDescEndpoint, nil
		case ctxSMDesc, ctxSubmodelDescriptor:
			// same physical table, but joined under a different alias
			return "submodel_descriptor_endpoint", ctxSubmodelDescriptorEndpoint, nil
		default:
			return "", ctx, fmt.Errorf("endpoints not valid in this context for field %q", fieldStr)
		}

	case "submodelDescriptors":
		if ctx != ctxAASDesc {
			return "", ctx, fmt.Errorf("submodelDescriptors not valid in this context for field %q", fieldStr)
		}
		return "submodel_descriptor", ctxSubmodelDescriptor, nil

	case "keys":
		// keys always refers to a reference_key table; which alias depends on the reference being accessed.
		// We rely on the preceding simple token to disambiguate.
		switch prevSimple {
		case "externalSubjectId":
			if ctx != ctxSpecificAssetID {
				return "", ctx, fmt.Errorf("externalSubjectId.keys not valid in this context for field %q", fieldStr)
			}
			return "external_subject_reference_key", ctx, nil
		case "semanticId":
			switch ctx {
			case ctxSM:
				return "semantic_id_reference_key", ctx, nil
			case ctxSMDesc, ctxSubmodelDescriptor:
				return "aasdesc_submodel_descriptor_semantic_id_reference_key", ctx, nil
			default:
				return "", ctx, fmt.Errorf("semanticId.keys not valid in this context for field %q", fieldStr)
			}
		default:
			return "", ctx, fmt.Errorf("cannot resolve keys[] array without a known parent (got %q) for field %q", prevSimple, fieldStr)
		}

	default:
		return "", ctx, fmt.Errorf("unsupported array segment %q in field %q", arrayName, fieldStr)
	}
}
