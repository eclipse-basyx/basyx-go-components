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

package descriptors

import (
	"github.com/doug-martin/goqu/v9"
)

var (
	specificAssetIDAlias                  = goqu.T(tblSpecificAssetID).As(aliasSpecificAssetID)
	aasDescriptorEndpointAlias            = goqu.T(tblAASDescriptorEndpoint).As(aliasAASDescriptorEndpoint)
	submodelDescriptorAlias               = goqu.T(tblSubmodelDescriptor).As(aliasSubmodelDescriptor)
	submodelDescriptorEndpointAlias       = goqu.T(tblAASDescriptorEndpoint).As(aliasSubmodelDescriptorEndpoint)
	infrastructureDescriptorAlias         = goqu.T(tblInfrastructureDescriptor).As(aliasInfrastructureDescriptor)
	infrastructureDescriptorEndpointAlias = goqu.T(tblAASDescriptorEndpoint).As(aliasInfrastructureDescriptorEndpoint)
)
