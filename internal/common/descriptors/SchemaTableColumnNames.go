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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import "github.com/doug-martin/goqu/v9"

// SQL dialect used for goqu builders in this package.
// Currently fixed to PostgreSQL.
const (
	dialect = "postgres"
)

// Tables holds the table names used by descriptor queries. These are grouped
// here to provide a single source of truth for SQL builders throughout this
// package and to keep SQL literals out of the query code.
const (
	tblDescriptor                     = "descriptor"
	tblAASDescriptor                  = "aas_descriptor"
	tblAASDescriptorEndpoint          = "aas_descriptor_endpoint"
	tblSecurityAttributes             = "security_attributes"
	tblEndpointProtocolVersion        = "endpoint_protocol_version"
	tblSpecificAssetID                = "specific_asset_id"
	tblSpecificAssetIDSuppSemantic    = "specific_asset_id_supplemental_semantic_id"
	tblSubmodelDescriptor             = "submodel_descriptor"
	tblSubmodelDescriptorSuppSemantic = "submodel_descriptor_supplemental_semantic_id"
	tblExtension                      = "extension"
	tblDescriptorExtension            = "descriptor_extension"
	tblExtensionSuppSemantic          = "extension_supplemental_semantic_id"
	tblExtensionRefersTo              = "extension_refers_to"
	tblLangStringTextType             = "lang_string_text_type"
	tblLangStringNameType             = "lang_string_name_type"
	tblReference                      = "reference"
	tblReferenceKey                   = "reference_key"
	tblRegistryDescriptor             = "registry_descriptor"
)

// Common table aliases used across descriptor queries. Keeping them here avoids
// scattering literal table names throughout the query builders.
const (
	aliasSpecificAssetID                          = tblSpecificAssetID
	aliasExternalSubjectReference                 = "external_subject_reference"
	aliasExternalSubjectReferenceKey              = "external_subject_reference_key"
	aliasAASDescriptorEndpoint                    = tblAASDescriptorEndpoint
	aliasSubmodelDescriptor                       = tblSubmodelDescriptor
	aliasSubmodelDescriptorEndpoint               = "submodel_descriptor_endpoint"
	aliasSubmodelDescriptorSemanticIDReference    = "aasdesc_submodel_descriptor_semantic_id_reference"
	aliasSubmodelDescriptorSemanticIDReferenceKey = "aasdesc_submodel_descriptor_semantic_id_reference_key"
)

// Columns holds the column names used by descriptor queries. Centralizing the
// names makes SQL generation more robust to refactors and reduces stringly‑typed
// errors in the query code.
const (
	colPosition                = "position" // this column is needed for the Query Language
	colID                      = "id"
	colDescriptorID            = "descriptor_id"
	colAASDescriptorID         = "aas_descriptor_id"
	colDescriptionID           = "description_id"
	colDisplayNameID           = "displayname_id"
	colAdminInfoID             = "administrative_information_id"
	colAssetKind               = "asset_kind"
	colAssetType               = "asset_type"
	colGlobalAssetID           = "global_asset_id"
	colIDShort                 = "id_short"
	colAASID                   = "id"
	colRegDescID               = "id"
	colHref                    = "href"
	colEndpointProtocol        = "endpoint_protocol"
	colSubProtocol             = "sub_protocol"
	colSubProtocolBody         = "sub_protocol_body"
	colSubProtocolBodyEncoding = "sub_protocol_body_encoding"
	colInterface               = "interface"

	colEndpointID              = "endpoint_id"
	colSecurityType            = "security_type"
	colSecurityKey             = "security_key"
	colSecurityValue           = "security_value"
	colEndpointProtocolVersion = "endpoint_protocol_version"

	colSemanticID              = "semantic_id"
	colSupplementalSemanticIDs = "supplemental_semantic_ids"
	colName                    = "name"
	colValue                   = "value"
	colExternalSubjectRef      = "external_subject_ref"

	colSpecificAssetIDID = "specific_asset_id_id"
	colReferenceID       = "reference_id"

	colValueType     = "value_type"
	colValueText     = "value_text"
	colValueNum      = "value_num"
	colValueBool     = "value_bool"
	colValueTime     = "value_time"
	colValueDatetime = "value_datetime"

	colExtensionID = "extension_id"

	colRegistryType = "registry_type"
	colCompany      = "company"

	// Generic/common column names used in descriptor queries
	colType            = "type"
	colParentReference = "parentreference"
	colRootReference   = "rootreference"

	// Language string tables columns
	colLangStringTextTypeReferenceID = "lang_string_text_type_reference_id"
	colLangStringNameTypeReferenceID = "lang_string_name_type_reference_id"
	colText                          = "text"
	colLanguage                      = "language"
)

// Goqu table helpers (convenience for Returning/Col) to avoid repetitively
// constructing the table builders in call sites.
var (
	tDescriptor            = goqu.T(tblDescriptor)
	tAASDescriptor         = goqu.T(tblAASDescriptor)
	tAASDescriptorEndpoint = goqu.T(tblAASDescriptorEndpoint)
	tSpecificAssetID       = goqu.T(tblSpecificAssetID)
)
