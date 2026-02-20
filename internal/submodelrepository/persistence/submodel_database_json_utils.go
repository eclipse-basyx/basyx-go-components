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
	submodel_repository_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
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

	if submodel.DisplayName() != nil && len(submodel.DisplayName()) > 0 {
		result.displayName, err = submodel_repository_utils.JsonStringFromJsonableSlice(jsonAPI, submodel.DisplayName())
		if err != nil {
			return nil, err
		}
	}

	if submodel.Description() != nil && len(submodel.Description()) > 0 {
		result.description, err = submodel_repository_utils.JsonStringFromJsonableSlice(jsonAPI, submodel.Description())
		if err != nil {
			return nil, err
		}
	}

	if submodel.Administration() != nil {
		administration := submodel.Administration()
		if administration != nil {
			result.administrativeInformation, err = submodel_repository_utils.JsonStringFromJsonableObject(jsonAPI, administration)
			if err != nil {
				return nil, err
			}
		}
	}

	if submodel.EmbeddedDataSpecifications() != nil && len(submodel.EmbeddedDataSpecifications()) > 0 {
		result.embeddedDataSpecification, err = submodel_repository_utils.JsonStringFromJsonableSlice(jsonAPI, submodel.EmbeddedDataSpecifications())
		if err != nil {
			return nil, err
		}
	}

	if submodel.SupplementalSemanticIDs() != nil && len(submodel.SupplementalSemanticIDs()) > 0 {
		result.supplementalSemanticIDs, err = submodel_repository_utils.JsonStringFromJsonableSlice(jsonAPI, submodel.SupplementalSemanticIDs())
		if err != nil {
			return nil, err
		}
	}

	if submodel.Extensions() != nil && len(submodel.Extensions()) > 0 {
		result.extensions, err = submodel_repository_utils.JsonStringFromJsonableSlice(jsonAPI, submodel.Extensions())
		if err != nil {
			return nil, err
		}
	}

	if submodel.Qualifiers() != nil && len(submodel.Qualifiers()) > 0 {
		result.qualifiers, err = submodel_repository_utils.JsonStringFromJsonableSlice(jsonAPI, submodel.Qualifiers())
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func jsonPayloadToInstance(descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString sql.NullString, submodel types.ISubmodel) (types.ISubmodel, error) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	if descriptionJsonString.Valid {
		var descriptionJsonable []map[string]any
		err := json.Unmarshal([]byte(descriptionJsonString.String), &descriptionJsonable)
		if err != nil {
			return nil, err
		}
		description := make([]types.ILangStringTextType, 0, len(descriptionJsonable))
		for _, desc := range descriptionJsonable {
			langStringTextType, err := jsonization.LangStringTextTypeFromJsonable(desc)
			if err != nil {
				return nil, err
			}
			description = append(description, langStringTextType)
		}
		submodel.SetDescription(description)
	}

	if displayNameJsonString.Valid {
		var displayNameJsonable []map[string]any
		err := json.Unmarshal([]byte(displayNameJsonString.String), &displayNameJsonable)
		if err != nil {
			return nil, err
		}
		displayName := make([]types.ILangStringNameType, 0, len(displayNameJsonable))
		for _, dispName := range displayNameJsonable {
			langStringNameType, err := jsonization.LangStringNameTypeFromJsonable(dispName)
			if err != nil {
				return nil, err
			}
			displayName = append(displayName, langStringNameType)
		}
		submodel.SetDisplayName(displayName)
	}

	if administrativeInformationJsonString.Valid {
		var administrativeInformationJsonable map[string]any
		err := json.Unmarshal([]byte(administrativeInformationJsonString.String), &administrativeInformationJsonable)
		if err != nil {
			return nil, err
		}
		administrativeInformation, err := jsonization.AdministrativeInformationFromJsonable(administrativeInformationJsonable)
		if err != nil {
			return nil, err
		}
		submodel.SetAdministration(administrativeInformation)
	}

	if embeddedDataSpecificationJsonString.Valid {
		var edsJsonable []map[string]any
		err := json.Unmarshal([]byte(embeddedDataSpecificationJsonString.String), &edsJsonable)
		if err != nil {
			return nil, err
		}
		eds := make([]types.IEmbeddedDataSpecification, 0, len(edsJsonable))
		for _, edsItem := range edsJsonable {
			embeddedDataSpecification, err := jsonization.EmbeddedDataSpecificationFromJsonable(edsItem)
			if err != nil {
				return nil, err
			}
			eds = append(eds, embeddedDataSpecification)
		}
		submodel.SetEmbeddedDataSpecifications(eds)
	}

	if supplementalSemanticIDsJsonString.Valid {
		var suplSemIdsJsonable []map[string]any
		err := json.Unmarshal([]byte(supplementalSemanticIDsJsonString.String), &suplSemIdsJsonable)
		if err != nil {
			return nil, err
		}
		suplSemIds := make([]types.IReference, 0, len(suplSemIdsJsonable))
		for _, suplSemIdItem := range suplSemIdsJsonable {
			suplSemId, err := jsonization.ReferenceFromJsonable(suplSemIdItem)
			if err != nil {
				return nil, err
			}
			suplSemIds = append(suplSemIds, suplSemId)
		}
		submodel.SetSupplementalSemanticIDs(suplSemIds)
	}

	if extensionsJsonString.Valid {
		var extensionsJsonable []map[string]any
		err := json.Unmarshal([]byte(extensionsJsonString.String), &extensionsJsonable)
		if err != nil {
			return nil, err
		}
		extensions := make([]types.IExtension, 0, len(extensionsJsonable))
		for _, extensionItem := range extensionsJsonable {
			extension, err := jsonization.ExtensionFromJsonable(extensionItem)
			if err != nil {
				return nil, err
			}
			extensions = append(extensions, extension)
		}
		submodel.SetExtensions(extensions)
	}

	if qualifiersJsonString.Valid {
		var qualifiersJsonable []map[string]any
		err := json.Unmarshal([]byte(qualifiersJsonString.String), &qualifiersJsonable)
		if err != nil {
			return nil, err
		}
		qualifiers := make([]types.IQualifier, 0, len(qualifiersJsonable))
		for _, qualifierItem := range qualifiersJsonable {
			qualifier, err := jsonization.QualifierFromJsonable(qualifierItem)
			if err != nil {
				return nil, err
			}
			qualifiers = append(qualifiers, qualifier)
		}
		submodel.SetQualifiers(qualifiers)
	}
	return submodel, nil
}
