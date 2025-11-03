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

// Author: Jannik Fried ( Fraunhofer IESE )
package builder

import (
	"log"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func BuildAdministration(adminRow AdministrationRow) (*gen.AdministrativeInformation, error) {
	administration := &gen.AdministrativeInformation{
		Version:    adminRow.Version,
		Revision:   adminRow.Revision,
		TemplateId: adminRow.TemplateId,
	}

	refBuilderMap := make(map[int64]*ReferenceBuilder)

	refs, err := ParseReferences(adminRow.Creator, refBuilderMap)
	if err != nil {
		return nil, err
	}

	ParseReferredReferences(adminRow.CreatorReferred, refBuilderMap)

	if len(refs) > 0 {
		administration.Creator = refs[0]
	}

	builder := NewEmbeddedDataSpecificationsBuilder()

	err = builder.BuildContentsIec61360(adminRow.EdsDataSpecificationIEC61360)
	if err != nil {
		log.Printf("Failed to build contents: %v", err)
	}

	err = builder.BuildReferences(adminRow.EdsDataSpecifications, adminRow.EdsDataSpecificationsReferred)
	if err != nil {
		log.Printf("Failed to build references: %v", err)
	}

	eds := builder.Build()

	if len(eds) > 0 {
		administration.EmbeddedDataSpecifications = eds
	}

	return administration, nil
}
