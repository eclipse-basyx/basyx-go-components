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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package descriptors

import (
	"context"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// FillSubmodelDescriptorCreator fills the missing administrative creator of
// a Submodel descriptor created under ctx with the authenticated caller.
func FillSubmodelDescriptorCreator(ctx context.Context, descriptor model.SubmodelDescriptor) model.SubmodelDescriptor {
	descriptor.Administration = auth.FillMissingCreator(descriptor.Administration, auth.ReBACCreatorReference(ctx))
	return descriptor
}

func fillAASDescriptorCreators(ctx context.Context, created []model.AssetAdministrationShellDescriptor) []model.AssetAdministrationShellDescriptor {
	creator := auth.ReBACCreatorReference(ctx)
	if creator == nil {
		return created
	}
	filled := make([]model.AssetAdministrationShellDescriptor, len(created))
	for index, descriptor := range created {
		filled[index] = fillAASDescriptorCreator(descriptor, creator)
	}
	return filled
}

// fillAASDescriptorCreator fills the missing creator of a new AAS descriptor
// and of the Submodel descriptors created with it.
func fillAASDescriptorCreator(descriptor model.AssetAdministrationShellDescriptor, creator types.IReference) model.AssetAdministrationShellDescriptor {
	if creator == nil {
		return descriptor
	}
	descriptor.Administration = auth.FillMissingCreator(descriptor.Administration, creator)
	descriptor.SubmodelDescriptors = fillSubmodelDescriptorCreators(descriptor.SubmodelDescriptors, creator)
	return descriptor
}

func fillSubmodelDescriptorCreators(created []model.SubmodelDescriptor, creator types.IReference) []model.SubmodelDescriptor {
	if creator == nil || len(created) == 0 {
		return created
	}
	filled := make([]model.SubmodelDescriptor, len(created))
	for index, descriptor := range created {
		descriptor.Administration = auth.FillMissingCreator(descriptor.Administration, creator)
		filled[index] = descriptor
	}
	return filled
}
