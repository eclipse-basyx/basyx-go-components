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

package rebac

import (
	"context"
	"strings"
	"testing"
)

type recordingChecker struct {
	objects []string
	allow   bool
}

func (c *recordingChecker) Check(_ context.Context, _ string, _ string, object string, _ []Tuple) (bool, error) {
	c.objects = append(c.objects, object)
	return c.allow, nil
}

func TestUniqueAASIdentifiersRejectsUnsafeInput(t *testing.T) {
	identifiers, err := uniqueAASIdentifiers([]string{"aas-b", "aas-a"})
	if err != nil || len(identifiers) != 2 || identifiers[0] != "aas-a" {
		t.Fatalf("uniqueAASIdentifiers() = %#v, %v", identifiers, err)
	}
	for _, input := range [][]string{{""}, {"aas-a", "aas-a"}} {
		if _, err = uniqueAASIdentifiers(input); err == nil {
			t.Fatalf("uniqueAASIdentifiers(%#v) accepted invalid input", input)
		}
	}
}

func TestAuthorizeInheritanceRequiresSubmodelAndAllParents(t *testing.T) {
	checker := &recordingChecker{allow: true}
	service := InheritanceService{Checker: checker}
	actor := GrantActor{User: "user:alice"}
	old := []Tuple{{User: "aas:old", Relation: "aas_parent", Object: "submodel:one"}}
	desired := []Tuple{{User: "aas:new", Relation: "aas_parent", Object: "submodel:one"}}
	if err := service.authorizeInheritance(t.Context(), actor, "submodel:one", old, desired); err != nil {
		t.Fatalf("authorizeInheritance() error = %v", err)
	}
	joined := strings.Join(checker.objects, ",")
	for _, object := range []string{"submodel:one", "aas:old", "aas:new"} {
		if !strings.Contains(joined, object) {
			t.Fatalf("missing management check for %s: %#v", object, checker.objects)
		}
	}
}

func TestAuthorizeInheritanceAdministratorDoesNotNeedChecker(t *testing.T) {
	service := InheritanceService{}
	if err := service.authorizeInheritance(t.Context(), GrantActor{Administrator: true}, "submodel:one", []Tuple{{User: "aas:old"}}, []Tuple{{User: "aas:new"}}); err != nil {
		t.Fatalf("administrator authorization error = %v", err)
	}
}
