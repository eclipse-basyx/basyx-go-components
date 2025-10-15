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

package builder

import (
	"slices"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// LangStringTextTypesBuilder constructs a collection of LangStringTextType objects from
// flattened database rows. LangStringTextTypes are used for multilingual text descriptions
// in AAS (Asset Administration Shell) elements such as descriptions.
//
// The builder tracks database IDs to avoid duplicate entries when processing multiple
// rows that may contain the same language string repeated due to SQL joins.
type LangStringTextTypesBuilder struct {
	nameTypeIds         []int64                   // Database IDs of text types already added to prevent duplicates
	langStringTextTypes *[]gen.LangStringTextType // Pointer to the slice being populated
}

// NewLangStringTextTypesBuilder creates a new LangStringTextTypesBuilder instance and
// initializes an empty slice for collecting LangStringTextType objects.
//
// Returns:
//   - *[]gen.LangStringTextType: A pointer to the slice that will be populated with language strings
//   - *LangStringTextTypesBuilder: A pointer to the builder for constructing the collection
//
// The returned slice pointer can be assigned to an AAS element's Description field, and as
// the builder processes database rows, it will automatically populate the slice.
//
// Example:
//
//	description, builder := NewLangStringTextTypesBuilder()
//	submodel.Description = *description
//	// Later, when processing database rows:
//	builder.CreateLangStringTextType(1, "en", "This sensor measures the ambient temperature in degrees Celsius")
//	builder.CreateLangStringTextType(2, "de", "Dieser Sensor misst die Umgebungstemperatur in Grad Celsius")
//	// Now submodel.Description contains both language variants
func NewLangStringTextTypesBuilder() (*[]gen.LangStringTextType, *LangStringTextTypesBuilder) {
	langStringTextTypes := []gen.LangStringTextType{}
	return &langStringTextTypes, &LangStringTextTypesBuilder{langStringTextTypes: &langStringTextTypes, nameTypeIds: []int64{}}
}

// CreateLangStringTextType adds a new language string to the collection. Duplicate entries
// (based on database ID) are automatically skipped to prevent duplication when processing
// multiple database rows that contain repeated data due to SQL joins.
//
// Parameters:
//   - nameTypeId: The database ID of the language string for duplicate detection
//   - language: The language code (e.g., "en", "de", "fr", "zh")
//   - text: The description text in the specified language
//
// The method is safe to call multiple times with the same nameTypeId - it will only add
// the entry once. This is essential when processing flattened SQL join results where the
// same language string may appear in multiple rows.
//
// Example:
//
//	description, builder := NewLangStringTextTypesBuilder()
//	// Processing database rows in a loop
//	for rows.Next() {
//	    var textId int64
//	    var lang, text string
//	    rows.Scan(&textId, &lang, &text)
//	    builder.CreateLangStringTextType(textId, lang, text)
//	}
//	// Result: description contains unique entries for each language
func (lsttb *LangStringTextTypesBuilder) CreateLangStringTextType(nameTypeId int64, language string, text string) {
	skip := slices.Contains(lsttb.nameTypeIds, nameTypeId)
	if !skip {
		lsttb.nameTypeIds = append(lsttb.nameTypeIds, nameTypeId)
		*lsttb.langStringTextTypes = append(*lsttb.langStringTextTypes, gen.LangStringTextType{
			Language: language,
			Text:     text,
		})
	}
}
