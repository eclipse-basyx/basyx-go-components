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

// LangStringNameTypesBuilder constructs a collection of LangStringNameType objects from
// flattened database rows. LangStringNameTypes are used for multilingual names in AAS
// (Asset Administration Shell) elements such as display names.
//
// The builder tracks database IDs to avoid duplicate entries when processing multiple
// rows that may contain the same language string repeated due to SQL joins.
type LangStringNameTypesBuilder struct {
	nameTypeIds         []int64                   // Database IDs of name types already added to prevent duplicates
	langStringNameTypes *[]gen.LangStringNameType // Pointer to the slice being populated
}

// NewLangStringNameTypesBuilder creates a new LangStringNameTypesBuilder instance and
// initializes an empty slice for collecting LangStringNameType objects.
//
// Returns:
//   - *[]gen.LangStringNameType: A pointer to the slice that will be populated with language strings
//   - *LangStringNameTypesBuilder: A pointer to the builder for constructing the collection
//
// The returned slice pointer can be assigned to an AAS element's DisplayName field, and as
// the builder processes database rows, it will automatically populate the slice.
//
// Example:
//
//	displayName, builder := NewLangStringNameTypesBuilder()
//	submodel.DisplayName = *displayName
//	// Later, when processing database rows:
//	builder.CreateLangStringNameType(1, "en", "Temperature Sensor")
//	builder.CreateLangStringNameType(2, "de", "Temperatursensor")
//	// Now submodel.DisplayName contains both language variants
func NewLangStringNameTypesBuilder() (*[]gen.LangStringNameType, *LangStringNameTypesBuilder) {
	langStringNameTypes := []gen.LangStringNameType{}
	return &langStringNameTypes, &LangStringNameTypesBuilder{langStringNameTypes: &langStringNameTypes, nameTypeIds: []int64{}}
}

// CreateLangStringNameType adds a new language string to the collection. Duplicate entries
// (based on database ID) are automatically skipped to prevent duplication when processing
// multiple database rows that contain repeated data due to SQL joins.
//
// Parameters:
//   - nameTypeId: The database ID of the language string for duplicate detection
//   - language: The language code (e.g., "en", "de", "fr", "zh")
//   - text: The name text in the specified language
//
// The method is safe to call multiple times with the same nameTypeId - it will only add
// the entry once. This is essential when processing flattened SQL join results where the
// same language string may appear in multiple rows.
//
// Example:
//
//	displayName, builder := NewLangStringNameTypesBuilder()
//	// Processing database rows in a loop
//	for rows.Next() {
//	    var nameId int64
//	    var lang, text string
//	    rows.Scan(&nameId, &lang, &text)
//	    builder.CreateLangStringNameType(nameId, lang, text)
//	}
//	// Result: displayName contains unique entries for each language
func (lsntb *LangStringNameTypesBuilder) CreateLangStringNameType(nameTypeId int64, language string, text string) {
	skip := slices.Contains(lsntb.nameTypeIds, nameTypeId)
	if !skip {
		lsntb.nameTypeIds = append(lsntb.nameTypeIds, nameTypeId)
		*lsntb.langStringNameTypes = append(*lsntb.langStringNameTypes, gen.LangStringNameType{
			Language: language,
			Text:     text,
		})
	}
}
