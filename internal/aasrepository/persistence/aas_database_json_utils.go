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

package persistence

import (
	"github.com/FriedJannik/aas-go-sdk/types"
	aas_repository_utils "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
)

type aasPayloadJSON struct {
	displayName               *string
	description               *string
	administrativeInformation *string
	embeddedDataSpecification *string
	extensions                *string
	derivedFrom               *string
}

func jsonizeAssetAdministrationShellPayload(aas types.IAssetAdministrationShell) (*aasPayloadJSON, error) {
	jsonAPI := jsoniter.ConfigCompatibleWithStandardLibrary
	result := &aasPayloadJSON{}
	var err error

	if aas.DisplayName() != nil && len(aas.DisplayName()) > 0 {
		result.displayName, err = aas_repository_utils.JsonStringFromJsonableSlice(jsonAPI, aas.DisplayName())
		if err != nil {
			return nil, err
		}
	}

	if aas.Description() != nil && len(aas.Description()) > 0 {
		result.description, err = aas_repository_utils.JsonStringFromJsonableSlice(jsonAPI, aas.Description())
		if err != nil {
			return nil, err
		}
	}

	if aas.Administration() != nil {
		administration := aas.Administration()
		if administration != nil {
			result.administrativeInformation, err = aas_repository_utils.JsonStringFromJsonableObject(jsonAPI, administration)
			if err != nil {
				return nil, err
			}
		}
	}

	if aas.EmbeddedDataSpecifications() != nil && len(aas.EmbeddedDataSpecifications()) > 0 {
		result.embeddedDataSpecification, err = aas_repository_utils.JsonStringFromJsonableSlice(jsonAPI, aas.EmbeddedDataSpecifications())
		if err != nil {
			return nil, err
		}
	}

	if aas.Extensions() != nil && len(aas.Extensions()) > 0 {
		result.extensions, err = aas_repository_utils.JsonStringFromJsonableSlice(jsonAPI, aas.Extensions())
		if err != nil {
			return nil, err
		}
	}

	if aas.DerivedFrom() != nil {
		result.derivedFrom, err = aas_repository_utils.JsonStringFromJsonableObject(jsonAPI, aas.DerivedFrom())
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

