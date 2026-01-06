package grammar

// ArrayIndexBinding represents a concrete index access on an array-like
// segment of a field path that has been normalized into SQL.
//
// Each binding links a resolved SQL table alias to a specific index
// within that array. This struct is only used when an array index is explicitly
// specified in the FieldIdentifier; open-ended array accesses (e.g. "[]")
// do not produce any ArrayIndexBinding entries.
type ArrayIndexBinding struct {
	// Alias is the SQL alias (or table name) representing the array segment.
	Alias string

	// Index is the concrete array position.
	Index int
}

// ResolvedFieldPath is the SQL-resolved representation of a FieldIdentifier.
//
// It consists of a base SQL column expression and an ordered list of
// array index bindings that must be applied when constructing joins
// or predicates for array-backed structures.
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
//   "$<root>#<path>"
//
// where <path> may contain nested object accessors and array selectors
// using either wildcard (`[]`) or concrete index (`[n]`) notation.
//
// Example inputs:
//
//   1. {"$field": "$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"}
//   2. {"$field": "$aasdesc#specificAssetIds[2].externalSubjectId.keys[5].value"}
//   3. {"$field": "$aasdesc#endpoints[].protocolinformation.href"}
//
// Example outputs:
//
//   1.
//   ResolvedFieldPath{
//       Column: "external_subject_reference_key.value",
//       ArrayBindings: []ArrayIndexBinding{
//           {Alias: "specific_asset_id.position", Index: -1},
//           {Alias: "external_subject_reference_key.position", Index: -1},
//       },
//   }
//
//   2.
//   ResolvedFieldPath{
//       Column: "external_subject_reference_key.value",
//       ArrayBindings: []ArrayIndexBinding{
//           {Alias: "specific_asset_id.position", Index: 2},
//           {Alias: "external_subject_reference_key.position", Index: 5},
//       },
//   }
//
// Fragment FieldIdentifiers (examples 4 and 5) resolve only to array bindings
// without a terminal column and are intended to be used as join or existence
// constraints rather than direct column references.
func ResolveScalarFieldToSQL(field *ModelStringPattern) (ResolvedFieldPath, error)

// ResolveFragmentFieldToSQL resolves a fragment identifier that ends in an array segment.
// Returns only bindings. Errors if the identifier ends in a concrete column.
// Example inputs:
//   1. {"$field": "$aasdesc#endpoints[]"}      // Fragment FieldIdentifier
//   2. {"$field": "$aasdesc#endpoints[2]"}     // Fragment FieldIdentifier
func ResolveFragmentFieldToSQL(field *FragmentStringPattern) ([]ArrayIndexBinding, error)
