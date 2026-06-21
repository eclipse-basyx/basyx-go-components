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

package smregistryapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestSMBulkCreateErrorStatusCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, smBulkCreateErrorStatusCode(common.NewErrBadRequest("bad request")))
	require.Equal(t, http.StatusConflict, smBulkCreateErrorStatusCode(common.NewErrConflict("conflict")))
	require.Equal(t, http.StatusForbidden, smBulkCreateErrorStatusCode(common.NewErrDenied("denied")))
	require.Equal(t, http.StatusInternalServerError, smBulkCreateErrorStatusCode(errors.New("unknown")))
}

func TestValidateBulkCreateSubmodelDescriptorsRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	failure := validateBulkCreateSubmodelDescriptors([]model.SubmodelDescriptor{
		{Id: "urn:example:submodel:1"},
		{Id: "urn:example:submodel:1"},
	})

	require.Equal(t, 1, failure.Index)
	require.Equal(t, http.StatusConflict, failure.StatusCode)
}

func TestValidateBulkCreateSubmodelDescriptorsNormalizesIDs(t *testing.T) {
	t.Parallel()

	descriptors := []model.SubmodelDescriptor{
		{Id: " urn:example:submodel:1 "},
		{Id: "\turn:example:submodel:2\n"},
	}

	failure := validateBulkCreateSubmodelDescriptors(descriptors)

	require.Zero(t, failure.StatusCode)
	require.Equal(t, "urn:example:submodel:1", descriptors[0].Id)
	require.Equal(t, "urn:example:submodel:2", descriptors[1].Id)
}

func TestValidateBulkCreateSubmodelDescriptorGraphsReportsItemIndex(t *testing.T) {
	t.Parallel()

	failure := asyncbulk.ItemFailure{}
	err := validateBulkCreateSubmodelDescriptorGraphs([]model.SubmodelDescriptor{
		{Id: "urn:example:submodel:1", Endpoints: []model.Endpoint{{Interface: "SUBMODEL-3.0"}}},
		{Id: "urn:example:submodel:2"},
	}, &failure)

	require.Error(t, err)
	require.Equal(t, 1, failure.Index)
	require.Equal(t, "urn:example:submodel:2", failure.Identifier)
	require.Equal(t, http.StatusBadRequest, failure.StatusCode)
}
