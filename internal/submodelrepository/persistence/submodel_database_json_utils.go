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
// Author: Jannik Fried (Fraunhofer IESE), Aaron Zielstorff (Fraunhofer IESE)

package persistence

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	jsoniter "github.com/json-iterator/go"
)

type submodelPayloadJSON struct {
	displayName               *string
	description               *string
	administrativeInformation *string
	embeddedDataSpecification *string
	supplementalSemanticIDs   *string
	extensions                *string
	qualifiers                *string
}

func jsonizeSubmodelPayload(submodel types.ISubmodel) (*submodelPayloadJSON, error) {
	jsonAPI := jsoniter.ConfigCompatibleWithStandardLibrary
	result := &submodelPayloadJSON{}
	var err error

	if submodelHasDisplayName(submodel) {
		result.displayName, err = common.JsonStringFromJsonableSlice(jsonAPI, submodel.DisplayName())
		if err != nil {
			return nil, err
		}
	}

	if submodelHasDescription(submodel) {
		result.description, err = common.JsonStringFromJsonableSlice(jsonAPI, submodel.Description())
		if err != nil {
			return nil, err
		}
	}

	if submodelHasAdministration(submodel) {
		administration := submodel.Administration()
		if administration != nil {
			result.administrativeInformation, err = common.JsonStringFromJsonableObject(jsonAPI, administration)
			if err != nil {
				return nil, err
			}
		}
	}

	if submodelHasEmbeddedDataSpecification(submodel) {
		result.embeddedDataSpecification, err = common.JsonStringFromJsonableSlice(jsonAPI, submodel.EmbeddedDataSpecifications())
		if err != nil {
			return nil, err
		}
	}

	if submodelHasSupplementalSemanticID(submodel) {
		result.supplementalSemanticIDs, err = common.JsonStringFromJsonableSlice(jsonAPI, submodel.SupplementalSemanticIDs())
		if err != nil {
			return nil, err
		}
	}

	if submodelHasExtension(submodel) {
		result.extensions, err = common.JsonStringFromJsonableSlice(jsonAPI, submodel.Extensions())
		if err != nil {
			return nil, err
		}
	}

	if submodelHasQualifier(submodel) {
		result.qualifiers, err = common.JsonStringFromJsonableSlice(jsonAPI, submodel.Qualifiers())
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func jsonPayloadToInstance(descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString sql.NullString, submodel types.ISubmodel) (types.ISubmodel, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	if descriptionJsonString.Valid {
		description, iSubmodel, err2 := unmarshalDescription(descriptionJsonString, json)
		if err2 != nil {
			return iSubmodel, err2
		}
		submodel.SetDescription(description)
	}

	if displayNameJsonString.Valid {
		displayName, iSubmodel, err2 := unmarshalDisplayName(json, displayNameJsonString)
		if err2 != nil {
			return iSubmodel, err2
		}
		submodel.SetDisplayName(displayName)
	}

	if administrativeInformationJsonString.Valid {
		administrativeInformation, iSubmodel, err2 := unmarshalAdministrativeInformation(json, administrativeInformationJsonString)
		if err2 != nil {
			return iSubmodel, err2
		}
		submodel.SetAdministration(administrativeInformation)
	}

	if embeddedDataSpecificationJsonString.Valid {
		eds, iSubmodel, err2 := unmarshalEmbeddedDataSpecification(json, embeddedDataSpecificationJsonString)
		if err2 != nil {
			return iSubmodel, err2
		}
		submodel.SetEmbeddedDataSpecifications(eds)
	}

	if supplementalSemanticIDsJsonString.Valid {
		suplSemIds, iSubmodel, err2 := unmarshalSupplementalSemanticIDs(json, supplementalSemanticIDsJsonString)
		if err2 != nil {
			return iSubmodel, err2
		}
		submodel.SetSupplementalSemanticIDs(suplSemIds)
	}

	if extensionsJsonString.Valid {
		extensions, iSubmodel, err2 := unmarshalExtensions(json, extensionsJsonString)
		if err2 != nil {
			return iSubmodel, err2
		}
		submodel.SetExtensions(extensions)
	}

	if qualifiersJsonString.Valid {
		qualifiers, iSubmodel, err2 := unmarshalQualifiers(json, qualifiersJsonString)
		if err2 != nil {
			return iSubmodel, err2
		}
		submodel.SetQualifiers(qualifiers)
	}
	return submodel, nil
}

func unmarshalQualifiers(json jsoniter.API, qualifiersJsonString sql.NullString) ([]types.IQualifier, types.ISubmodel, error) {
	var qualifiersJsonable []map[string]any
	err := json.Unmarshal([]byte(qualifiersJsonString.String), &qualifiersJsonable)
	if err != nil {
		return nil, nil, err
	}
	qualifiers := make([]types.IQualifier, 0, len(qualifiersJsonable))
	for _, qualifierItem := range qualifiersJsonable {
		qualifier, err := jsonization.QualifierFromJsonable(qualifierItem)
		if err != nil {
			return nil, nil, err
		}
		qualifiers = append(qualifiers, qualifier)
	}
	return qualifiers, nil, nil
}

func unmarshalExtensions(json jsoniter.API, extensionsJsonString sql.NullString) ([]types.IExtension, types.ISubmodel, error) {
	var extensionsJsonable []map[string]any
	err := json.Unmarshal([]byte(extensionsJsonString.String), &extensionsJsonable)
	if err != nil {
		return nil, nil, err
	}
	extensions := make([]types.IExtension, 0, len(extensionsJsonable))
	for _, extensionItem := range extensionsJsonable {
		extension, err := jsonization.ExtensionFromJsonable(extensionItem)
		if err != nil {
			return nil, nil, err
		}
		extensions = append(extensions, extension)
	}
	return extensions, nil, nil
}

func unmarshalSupplementalSemanticIDs(json jsoniter.API, supplementalSemanticIDsJsonString sql.NullString) ([]types.IReference, types.ISubmodel, error) {
	var suplSemIdsJsonable []map[string]any
	err := json.Unmarshal([]byte(supplementalSemanticIDsJsonString.String), &suplSemIdsJsonable)
	if err != nil {
		return nil, nil, err
	}
	suplSemIds := make([]types.IReference, 0, len(suplSemIdsJsonable))
	for _, suplSemIdItem := range suplSemIdsJsonable {
		suplSemId, err := jsonization.ReferenceFromJsonable(suplSemIdItem)
		if err != nil {
			return nil, nil, err
		}
		suplSemIds = append(suplSemIds, suplSemId)
	}
	return suplSemIds, nil, nil
}

func unmarshalDescription(descriptionJsonString sql.NullString, json jsoniter.API) ([]types.ILangStringTextType, types.ISubmodel, error) {
	var descriptionJsonable []map[string]any
	err := json.Unmarshal([]byte(descriptionJsonString.String), &descriptionJsonable)
	if err != nil {
		return nil, nil, err
	}
	description := make([]types.ILangStringTextType, 0, len(descriptionJsonable))
	for _, desc := range descriptionJsonable {
		langStringTextType, err := jsonization.LangStringTextTypeFromJsonable(desc)
		if err != nil {
			return nil, nil, err
		}
		description = append(description, langStringTextType)
	}
	return description, nil, nil
}

func unmarshalDisplayName(json jsoniter.API, displayNameJsonString sql.NullString) ([]types.ILangStringNameType, types.ISubmodel, error) {
	var displayNameJsonable []map[string]any
	err := json.Unmarshal([]byte(displayNameJsonString.String), &displayNameJsonable)
	if err != nil {
		return nil, nil, err
	}
	displayName := make([]types.ILangStringNameType, 0, len(displayNameJsonable))
	for _, dispName := range displayNameJsonable {
		langStringNameType, err := jsonization.LangStringNameTypeFromJsonable(dispName)
		if err != nil {
			return nil, nil, err
		}
		displayName = append(displayName, langStringNameType)
	}
	return displayName, nil, nil
}

func unmarshalAdministrativeInformation(json jsoniter.API, administrativeInformationJsonString sql.NullString) (types.IAdministrativeInformation, types.ISubmodel, error) {
	var administrativeInformationJsonable map[string]any
	err := json.Unmarshal([]byte(administrativeInformationJsonString.String), &administrativeInformationJsonable)
	if err != nil {
		return nil, nil, err
	}
	administrativeInformation, err := jsonization.AdministrativeInformationFromJsonable(administrativeInformationJsonable)
	if err != nil {
		return nil, nil, err
	}
	return administrativeInformation, nil, nil
}

func unmarshalEmbeddedDataSpecification(json jsoniter.API, embeddedDataSpecificationJsonString sql.NullString) ([]types.IEmbeddedDataSpecification, types.ISubmodel, error) {
	var edsJsonable []map[string]any
	err := json.Unmarshal([]byte(embeddedDataSpecificationJsonString.String), &edsJsonable)
	if err != nil {
		return nil, nil, err
	}
	eds := make([]types.IEmbeddedDataSpecification, 0, len(edsJsonable))
	for _, edsItem := range edsJsonable {
		embeddedDataSpecification, err := jsonization.EmbeddedDataSpecificationFromJsonable(edsItem)
		if err != nil {
			return nil, nil, err
		}
		eds = append(eds, embeddedDataSpecification)
	}
	return eds, nil, nil
}

func submodelHasQualifier(submodel types.ISubmodel) bool {
	return submodel.Qualifiers() != nil && len(submodel.Qualifiers()) > 0
}

func submodelHasExtension(submodel types.ISubmodel) bool {
	return submodel.Extensions() != nil && len(submodel.Extensions()) > 0
}

func submodelHasSupplementalSemanticID(submodel types.ISubmodel) bool {
	return submodel.SupplementalSemanticIDs() != nil && len(submodel.SupplementalSemanticIDs()) > 0
}

func submodelHasEmbeddedDataSpecification(submodel types.ISubmodel) bool {
	return submodel.EmbeddedDataSpecifications() != nil && len(submodel.EmbeddedDataSpecifications()) > 0
}

func submodelHasAdministration(submodel types.ISubmodel) bool {
	return submodel.Administration() != nil
}

func submodelHasDescription(submodel types.ISubmodel) bool {
	return submodel.Description() != nil && len(submodel.Description()) > 0
}

func submodelHasDisplayName(submodel types.ISubmodel) bool {
	return submodel.DisplayName() != nil && len(submodel.DisplayName()) > 0
}
