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

package rebac

import (
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// Effective access sources reported by GET …/$access/effective.
const (
	sourceReBAC         = "rebac"
	sourceAdministrator = "administrator"
)

type effectiveAction struct {
	name     string
	relation string
	method   string
	suffix   string
	right    grammar.RightsEnum
}

type effectiveRight struct {
	Action string `json:"action"`
	Source string `json:"source"`
}

type effectiveDocument struct {
	Object accessObject     `json:"object"`
	Rights []effectiveRight `json:"rights"`
}

func effectiveActions(target accessTarget) []effectiveAction {
	actions := []effectiveAction{
		{name: "read", relation: RelationCanRead, method: http.MethodGet, right: grammar.RightsEnumREAD},
		{name: "update", relation: RelationCanUpdate, method: http.MethodPatch, right: grammar.RightsEnumUPDATE},
		{name: "delete", relation: RelationCanDelete, method: http.MethodDelete, right: grammar.RightsEnumDELETE},
		{name: "execute", relation: RelationCanExecute, method: http.MethodPost, suffix: "/invoke", right: grammar.RightsEnumEXECUTE},
		{name: "manage", relation: RelationCanManage},
	}
	switch {
	case target.elementPath != "":
		actions[2].relation = RelationCanUpdate
	case target.kind.ObjectType == TypeConceptDescription:
		actions[1].method = http.MethodPut
		actions = append(actions[:3], actions[4])
	case target.kind.ObjectType == TypeAAS:
		actions[1].method = http.MethodPut
		actions[3].method = ""
	default:
		actions[3].method = ""
	}
	return actions
}

// handleEffective reports the caller's own rights on a target. Callers
// without any right receive 404, so the endpoint never reveals existence.
func (c *Coordinator) handleEffective(w http.ResponseWriter, r *http.Request, request accessRequest) {
	actions := effectiveActions(request.target)
	rebac, err := c.effectiveReBAC(r, request, actions)
	if err != nil {
		writeUnavailable(w, r, err)
		return
	}
	document := effectiveDocument{Object: accessObject{Type: request.target.objectType(), ID: request.target.identifier, IDShortPath: request.target.elementPath}}
	anyRight := false
	for index, action := range actions {
		source := c.effectiveSource(r, request, action, rebac[index])
		anyRight = anyRight || source != string(auth.ABACAccessNone)
		document.Rights = append(document.Rights, effectiveRight{Action: action.name, Source: source})
	}
	if !anyRight {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, document)
}

func (c *Coordinator) effectiveReBAC(r *http.Request, request accessRequest, actions []effectiveAction) ([]bool, error) {
	resolution := &resolution{coordinator: c, principal: request.principal}
	results := make([]bool, len(actions))
	for index, action := range actions {
		object, structure := request.target.objectKey(), request.target.structure()
		if action.name == "delete" && request.target.elementPath != "" {
			parent := ParentElementPath(request.target.elementPath)
			object, structure = ResourceObject(TypeSubmodel, request.target.authUUID), nil
			if parent != "" {
				object, structure = ElementObject(request.target.authUUID, parent), ElementAncestry(request.target.authUUID, parent)
			}
		}
		allowed, err := resolution.checkResource(r.Context(), request.target.kind, action.relation, object, structure)
		if err != nil {
			return nil, err
		}
		results[index] = allowed
	}
	return results, nil
}

func (c *Coordinator) effectiveSource(r *http.Request, request accessRequest, action effectiveAction, rebacAllowed bool) string {
	abac := auth.ABACAccessNone
	if action.method != "" {
		abac = auth.EvaluateABACRight(c.abacProvider, c.abacImplicitCasts, action.method,
			c.resourcePath(request.target)+action.suffix, auth.ClaimsFromContext(r.Context()), action.right)
	}
	switch {
	case abac == auth.ABACAccessUnconditional:
		return string(abac)
	case rebacAllowed:
		return sourceReBAC
	case action.name == "manage" && request.admin:
		return sourceAdministrator
	default:
		return string(abac)
	}
}

// resourcePath returns the API path of a target including the context path.
func (c *Coordinator) resourcePath(target accessTarget) string {
	basePath := ""
	if c.abacProvider != nil {
		if model := c.abacProvider.ActiveAccessModel(); model != nil {
			basePath = common.NormalizeBasePath(model.BasePath())
		}
	}
	if basePath == "/" {
		basePath = ""
	}
	path := basePath + resourcePrefix(target.kind) + "/" + common.EncodeString(target.identifier)
	if target.elementPath != "" {
		path += "/submodel-elements/" + target.elementPath
	}
	return path
}
