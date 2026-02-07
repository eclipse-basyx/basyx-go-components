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
// Author: Martin Stemmer ( Fraunhofer IESE )

package grammar

import (
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
)

type terminalColumnMapping struct {
	// ByContext resolves a terminal field directly from the current context.
	ByContext map[resolveContext]string

	// ByParentSimple resolves a terminal field based on the immediately preceding
	// simple segment (e.g. "semanticId.type", "protocolinformation.href").
	ByParentSimple map[string]map[resolveContext]string

	// ByArrayParentSimple resolves a terminal field that follows an array segment,
	// with disambiguation via the simple parent of that array segment.
	//
	// Example:
	//   "externalSubjectId.keys[].value" vs "semanticId.keys[].value"
	//   terminal="value", arrayName="keys", arrayParentSimple="externalSubjectId"|"semanticId".
	ByArrayParentSimple map[string]map[string]map[resolveContext]string
}

// terminalColumnMappings defines how terminal path segments map to SQL column expressions.
//
// The mapping is intentionally centralized (similar to arraySegmentMappings) so supported
// scalar field identifiers can be extended by adding data rather than growing switch statements.
var terminalColumnMappings = map[string]terminalColumnMapping{
	"idShort": {
		ByContext: map[resolveContext]string{
			ctxSM:                 "s.id_short",
			ctxSME:                "submodel_element.id_short",
			ctxAASDesc:            "aas_descriptor.id_short",
			ctxSMDesc:             "submodel_descriptor.id_short",
			ctxSubmodelDescriptor: "submodel_descriptor.id_short",
		},
	},

	"id": {
		ByContext: map[resolveContext]string{
			ctxSM:                 "s.id",
			ctxAASDesc:            "aas_descriptor.id",
			ctxSMDesc:             "submodel_descriptor.id",
			ctxSubmodelDescriptor: "submodel_descriptor.id",
		},
	},

	"createdAt": {
		ByContext: map[resolveContext]string{
			ctxAASDesc:            "aas_descriptor.created_at",
			ctxBD:                 "aas_identifier.created_at",
			ctxSMDesc:             "submodel_descriptor.created_at",
			ctxSubmodelDescriptor: "submodel_descriptor.created_at",
		},
	},

	"assetKind": {
		ByContext: map[resolveContext]string{
			ctxAASDesc: "aas_descriptor.asset_kind",
		},
	},

	"assetType": {
		ByContext: map[resolveContext]string{
			ctxAASDesc: "aas_descriptor.asset_type",
		},
	},

	"globalAssetId": {
		ByContext: map[resolveContext]string{
			ctxAASDesc: "aas_descriptor.global_asset_id",
		},
	},

	"name": {
		ByContext: map[resolveContext]string{
			ctxSpecificAssetID: "specific_asset_id.name",
		},
	},

	"value": {
		ByContext: map[resolveContext]string{
			ctxSpecificAssetID: "specific_asset_id.value",
			ctxSME:             "COALESCE(property_element.value_text, property_element.value_num::text, property_element.value_bool::text, property_element.value_time::text, property_element.value_datetime::text)",
		},
		ByParentSimple: map[string]map[resolveContext]string{
			// submodelDescriptor semanticId mapping (used by $aasdesc#submodelDescriptors[].semanticId.* and $smdesc#semanticId.*).
			"semanticId": {
				ctxSMDesc:             "aasdesc_submodel_descriptor_semantic_id_reference.value",
				ctxSubmodelDescriptor: "aasdesc_submodel_descriptor_semantic_id_reference.value",
			},
		},
		ByArrayParentSimple: map[string]map[string]map[resolveContext]string{
			"keys": {
				"semanticId": {
					ctxSM:                 "semantic_id_reference_key.value",
					ctxSME:                "sme_semantic_id_reference_key.value",
					ctxSMDesc:             "aasdesc_submodel_descriptor_semantic_id_reference_key.value",
					ctxSubmodelDescriptor: "aasdesc_submodel_descriptor_semantic_id_reference_key.value",
				},
				"externalSubjectId": {
					ctxSpecificAssetID: "external_subject_reference_key.value",
				},
			},
		},
	},

	"type": {
		ByParentSimple: map[string]map[resolveContext]string{
			"semanticId": {
				ctxSM:                 "semantic_id_reference.type",
				ctxSME:                "sme_semantic_id_reference.type",
				ctxSMDesc:             "aasdesc_submodel_descriptor_semantic_id_reference.type",
				ctxSubmodelDescriptor: "aasdesc_submodel_descriptor_semantic_id_reference.type",
			},
		},
		ByArrayParentSimple: map[string]map[string]map[resolveContext]string{
			"keys": {
				"semanticId": {
					ctxSM:                 "semantic_id_reference_key.type",
					ctxSME:                "sme_semantic_id_reference_key.type",
					ctxSMDesc:             "aasdesc_submodel_descriptor_semantic_id_reference_key.type",
					ctxSubmodelDescriptor: "aasdesc_submodel_descriptor_semantic_id_reference_key.type",
				},
				"externalSubjectId": {
					ctxSpecificAssetID: "external_subject_reference_key.type",
				},
			},
		},
	},

	"valueType": {
		ByContext: map[resolveContext]string{
			ctxSME: "property_element.value_type",
		},
	},

	"language": {
		ByContext: map[resolveContext]string{
			ctxSME: "multilanguage_property_value.language",
		},
	},

	"interface": {
		ByContext: map[resolveContext]string{
			ctxAASDescEndpoint:            "aas_descriptor_endpoint.interface",
			ctxSubmodelDescriptorEndpoint: "submodel_descriptor_endpoint.interface",
		},
	},

	"href": {
		ByParentSimple: map[string]map[resolveContext]string{
			"protocolinformation": {
				ctxAASDescEndpoint:            "aas_descriptor_endpoint.href",
				ctxSubmodelDescriptorEndpoint: "submodel_descriptor_endpoint.href",
			},
		},
	},
}

// ResolveAASQLFieldToSQLColumn resolves a normalized AAS query language field identifier to a SQL column.
//
// It supports array selectors with wildcards ([]) and concrete indices ([n]); indices do not affect
// the returned SQL column but are expected to be handled separately via array bindings / constraints.
func ResolveAASQLFieldToSQLColumn(fieldStr string) (string, error) {
	ctx := contextFromFieldPrefix(fieldStr)
	if ctx == ctxUnknown {
		return "", fmt.Errorf("unsupported field root (expected $aasdesc#, $smdesc#, $sm#, $sme...#, or $bd#): %q", fieldStr)
	}

	tokens := builder.TokenizeField(fieldStr)
	if len(tokens) == 0 {
		return "", fmt.Errorf("invalid field identifier (empty path): %q", fieldStr)
	}
	// Scalar identifiers must end in a simple terminal field.
	if _, ok := tokens[len(tokens)-1].(builder.ArrayToken); ok {
		return "", fmt.Errorf("scalar field identifier must not end in an array segment: %q", fieldStr)
	}

	prevSimple := ""
	lastArrayName := ""
	lastArrayParentSimple := ""

	for i, tok := range tokens {
		switch t := tok.(type) {
		case builder.SimpleToken:
			if i == len(tokens)-1 {
				terminal := t.Name
				parentSimple := prevSimple
				parentArray := lastArrayName
				arrayParentSimple := lastArrayParentSimple
				return resolveTerminalColumn(fieldStr, ctx, terminal, parentSimple, parentArray, arrayParentSimple)
			}
			prevSimple = t.Name

		case builder.ArrayToken:
			// Resolve and advance context for the array segment.
			_, nextCtx, err := resolveArrayToken(fieldStr, ctx, prevSimple, t.Name)
			if err != nil {
				return "", err
			}

			lastArrayName = t.Name
			if mapping, ok := arraySegmentMappings[t.Name]; ok && mapping.ByParent != nil {
				lastArrayParentSimple = prevSimple
			} else {
				lastArrayParentSimple = ""
			}

			ctx = nextCtx
			prevSimple = ""

		default:
			return "", fmt.Errorf("unsupported token type while resolving: %T", tok)
		}
	}

	return "", fmt.Errorf("invalid field identifier (missing terminal field): %q", fieldStr)
}

func resolveTerminalColumn(fieldStr string, ctx resolveContext, terminal string, parentSimple string, parentArray string, arrayParentSimple string) (string, error) {
	mapping, ok := terminalColumnMappings[terminal]
	if !ok {
		return "", fmt.Errorf("unsupported terminal field %q in field %q", terminal, fieldStr)
	}

	if parentSimple != "" && mapping.ByParentSimple != nil {
		if byCtx, ok := mapping.ByParentSimple[parentSimple]; ok {
			if col, ok := byCtx[ctx]; ok {
				return col, nil
			}
		}
	}

	if parentArray != "" && arrayParentSimple != "" && mapping.ByArrayParentSimple != nil {
		if byParentSimple, ok := mapping.ByArrayParentSimple[parentArray]; ok {
			if byCtx, ok := byParentSimple[arrayParentSimple]; ok {
				if col, ok := byCtx[ctx]; ok {
					return col, nil
				}
			}
		}
	}

	if mapping.ByContext != nil {
		if col, ok := mapping.ByContext[ctx]; ok {
			return col, nil
		}
	}

	return "", fmt.Errorf("unsupported field identifier path (terminal=%q, parentSimple=%q, parentArray=%q, arrayParentSimple=%q) in context %d for field %q", terminal, parentSimple, parentArray, arrayParentSimple, ctx, fieldStr)
}
