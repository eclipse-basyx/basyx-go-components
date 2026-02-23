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

package common

import "github.com/doug-martin/goqu/v9"

// SQL dialect used for goqu builders in this package.
// Currently fixed to PostgreSQL.
const (
	Dialect = "postgres"
)

// Tables holds the table names used by descriptor queries. These are grouped
// here to provide a single source of truth for SQL builders throughout this
// package and to keep SQL literals out of the query code.
const (
	TblDescriptor                     = "descriptor"
	TblAASDescriptor                  = "aas_descriptor"
	TblAASDescriptorEndpoint          = "aas_descriptor_endpoint"
	TblAASIdentifier                  = "aas_identifier"
	TblSpecificAssetID                = "specific_asset_id"
	TblSpecificAssetIDPayload         = "specific_asset_id_payload"
	TblSpecificAssetIDSuppSemantic    = "specific_asset_id_supplemental_semantic_id_reference"
	TblSubmodelDescriptor             = "submodel_descriptor"
	TblSubmodelDescriptorSuppSemantic = "submodel_descriptor_supplemental_semantic_id_reference"
	TblDescriptorPayload              = "descriptor_payload"
	TblExtension                      = "extension"
	TblDescriptorExtension            = "descriptor_extension"
	TblExtensionSuppSemantic          = "extension_supplemental_semantic_id"
	TblExtensionRefersTo              = "extension_refers_to"
	TblLangStringTextType             = "lang_string_text_type"
	TblLangStringNameType             = "lang_string_name_type"
	TblReference                      = "reference"
	TblReferenceKey                   = "reference_key"
	TblInfrastructureDescriptor       = "infrastructure_descriptor"
)

// Common table aliases used across descriptor queries. Keeping them here avoids
// scattering literal table names throughout the query builders.
const (
	AliasSpecificAssetID                          = TblSpecificAssetID
	AliasExternalSubjectReference                 = "external_subject_reference"
	AliasExternalSubjectReferenceKey              = "external_subject_reference_key"
	AliasAASDescriptorEndpoint                    = TblAASDescriptorEndpoint
	AliasSubmodelDescriptor                       = TblSubmodelDescriptor
	AliasSubmodelDescriptorEndpoint               = "submodel_descriptor_endpoint"
	AliasSubmodelDescriptorSemanticIDReference    = "aasdesc_submodel_descriptor_semantic_id_reference"
	AliasSubmodelDescriptorSemanticIDReferenceKey = "aasdesc_submodel_descriptor_semantic_id_reference_key"
	AliasInfrastructureDescriptor                 = TblInfrastructureDescriptor
	AliasInfrastructureDescriptorEndpoint         = "infrastructure_descriptor_endpoint"
)

// Columns holds the column names used by descriptor queries. Centralizing the
// names makes SQL generation more robust to refactors and reduces stringly‑typed
// errors in the query code.
const (
	ColPosition                  = "position" // this column is needed for the Query Language
	ColID                        = "id"
	ColDescriptorID              = "descriptor_id"
	ColAASDescriptorID           = "aas_descriptor_id"
	ColDescriptionID             = "description_id"
	ColDisplayNameID             = "displayname_id"
	ColAdminInfoID               = "administrative_information_id"
	ColDescriptionPayload        = "description_payload"
	ColDisplayNamePayload        = "displayname_payload"
	ColAdministrativeInfoPayload = "administrative_information_payload"
	ColExtensionsPayload         = "extensions_payload"
	ColAssetKind                 = "asset_kind"
	ColAssetType                 = "asset_type"
	ColGlobalAssetID             = "global_asset_id"
	ColIDShort                   = "id_short"
	ColCreatedAt                 = "created_at"
	ColAASID                     = "id"
	ColInfDescID                 = "id"
	ColHref                      = "href"
	ColEndpointProtocol          = "endpoint_protocol"
	ColSubProtocol               = "sub_protocol"
	ColSubProtocolBody           = "sub_protocol_body"
	ColSubProtocolBodyEncoding   = "sub_protocol_body_encoding"
	ColInterface                 = "interface"

	ColEndpointProtocolVersion = "endpoint_protocol_version"
	ColSecurityAttributes      = "security_attributes"

	ColSemanticID              = "semantic_id"
	ColSupplementalSemanticIDs = "supplemental_semantic_ids"
	ColName                    = "name"
	ColValue                   = "value"
	ColExternalSubjectRef      = "external_subject_ref"
	ColAASRef                  = "aasref"

	ColSpecificAssetIDID = "specific_asset_id_id"
	ColSpecificAssetID   = "specific_asset_id"
	ColReferenceID       = "reference_id"

	ColCompany = "company"

	// Generic/common column names used in descriptor queries
	ColType            = "type"
	ColParentReference = "parentreference"
	ColRootReference   = "rootreference"
)

// Goqu table helpers (convenience for Returning/Col) to avoid repetitively
// constructing the table builders in call sites.
var (
	TDescriptor               = goqu.T(TblDescriptor)
	TAASDescriptor            = goqu.T(TblAASDescriptor)
	TAASDescriptorEndpoint    = goqu.T(TblAASDescriptorEndpoint)
	TSpecificAssetID          = goqu.T(TblSpecificAssetID)
	TDescriptorPayload        = goqu.T(TblDescriptorPayload)
	TInfrastructureDescriptor = goqu.T(TblInfrastructureDescriptor)
)
