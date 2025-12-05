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

// Package builder provides utilities for constructing complex AAS (Asset Administration Shell)
// data structures from database query results.
package builder

import (
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// BuildRegistryAdministration constructs a RegistryAdministrativeInformation object from database query results.
// It processes administrative metadata including version, revision, template ID, creator references, and company information
//
// The function handles the complexity of building nested reference structures for the creator field.
//
// Parameters:
//   - adminRow: An RegistryAdministrationRow containing administrative data from the database, including
//     version information, creator references, and company information
//
// Returns:
//   - *model.RegistryAdministrativeInformation: A pointer to the constructed registry administrative information object
//     with all nested references properly built
//   - error: An error if reference parsing fails, nil otherwise.
//
// Example:
//
//	admin, err := BuildRegistryAdministration(adminRow)
//	if err != nil {
//	    log.Printf("Failed to build administration: %v", err)
//	}
func BuildRegistryAdministration(adminRow model.RegistryAdministrationRow) (*model.RegistryAdministrativeInformation, error) {
	administration := &model.RegistryAdministrativeInformation{
		Version:    adminRow.Version,
		Revision:   adminRow.Revision,
		TemplateID: adminRow.TemplateID,
		Company:    adminRow.Company,
	}

	refBuilderMap := make(map[int64]*ReferenceBuilder)

	refs, err := ParseReferences(adminRow.Creator, refBuilderMap, nil)
	if err != nil {
		return nil, err
	}

	if err = ParseReferredReferences(adminRow.CreatorReferred, refBuilderMap, nil); err != nil {
		return nil, err
	}

	if len(refs) > 0 {
		administration.Creator = refs[0]
	}

	for _, refBuilder := range refBuilderMap {
		refBuilder.BuildNestedStructure()
	}

	return administration, nil
}
