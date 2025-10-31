package persistence_postgresql

import "github.com/doug-martin/goqu/v9"

const (
	dialect = "postgres"
)

// Tables
const (
	tblDescriptor                     = "descriptor"
	tblAASDescriptor                  = "aas_descriptor"
	tblAASDescriptorEndpoint          = "aas_descriptor_endpoint"
	tblAdministrativeInformation      = "administrative_information"
	tblSecurityAttributes             = "security_attributes"
	tblEndpointProtocolVersion        = "endpoint_protocol_version"
	tblSpecificAssetID                = "specific_asset_id"
	tblSpecificAssetIDSuppSemantic    = "specific_asset_id_supplemental_semantic_id"
	tblSubmodelDescriptor             = "submodel_descriptor"
	tblSubmodelDescriptorSuppSemantic = "submodel_descriptor_supplemental_semantic_id"
	tblExtension                      = "extension"
	tblExtensionReference             = "extension_reference"
	tblDescriptorExtension            = "descriptor_extension"
)

// Columns
const (
	colID                      = "id"
	colDescriptorID            = "descriptor_id"
	colAASDescriptorID         = "aas_descriptor_id"
	colDescriptionID           = "description_id"
	colDisplayNameID           = "displayname_id"
	colAdminInfoID             = "administrative_information_id"
	colAssetKind               = "asset_kind"
	colAssetType               = "asset_type"
	colGlobalAssetID           = "global_asset_id"
	colIdShort                 = "id_short"
	colAASID                   = "id"
	colHref                    = "href"
	colEndpointProtocol        = "endpoint_protocol"
	colSubProtocol             = "sub_protocol"
	colSubProtocolBody         = "sub_protocol_body"
	colSubProtocolBodyEncoding = "sub_protocol_body_encoding"
	colInterface               = "interface"

	colVersion    = "version"
	colRevision   = "revision"
	colTemplateId = "templateid"
	colCreator    = "creator"

	colEndpointID              = "endpoint_id"
	colSecurityType            = "security_type"
	colSecurityKey             = "security_key"
	colSecurityValue           = "security_value"
	colEndpointProtocolVersion = "endpoint_protocol_version"

	colSemanticID         = "semantic_id"
	colName               = "name"
	colValue              = "value"
	colExternalSubjectRef = "external_subject_ref"

	colSpecificAssetIDID = "specific_asset_id_id"
	colReferenceID       = "reference_id"

	colValueType     = "value_type"
	colValueText     = "value_text"
	colValueNum      = "value_num"
	colValueBool     = "value_bool"
	colValueTime     = "value_time"
	colValueDatetime = "value_datetime"

	colExtensionID = "extension_id"
)

// Goqu table helpers (convenience for Returning/Col)
var (
	tDescriptor            = goqu.T(tblDescriptor)
	tAASDescriptor         = goqu.T(tblAASDescriptor)
	tAASDescriptorEndpoint = goqu.T(tblAASDescriptorEndpoint)
	tSpecificAssetID       = goqu.T(tblSpecificAssetID)
	tSubmodelDescriptor    = goqu.T(tblSubmodelDescriptor)
	tExtension             = goqu.T(tblExtension)
)
