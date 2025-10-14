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

type LangStringTextTypesBuilder struct {
	nameTypeIds         []int64
	langStringTextTypes *[]gen.LangStringTextType
}

func NewLangStringTextTypesBuilder() (*[]gen.LangStringTextType, *LangStringTextTypesBuilder) {
	langStringTextTypes := []gen.LangStringTextType{}
	return &langStringTextTypes, &LangStringTextTypesBuilder{langStringTextTypes: &langStringTextTypes, nameTypeIds: []int64{}}
}

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
