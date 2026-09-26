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
		{name: "read", relation: PermissionRead, method: http.MethodGet, right: grammar.RightsEnumREAD},
		{name: "update", relation: PermissionUpdate, method: http.MethodPatch, right: grammar.RightsEnumUPDATE},
		{name: "delete", relation: PermissionDelete, method: http.MethodDelete, right: grammar.RightsEnumDELETE},
		{name: "execute", relation: PermissionExecute, method: http.MethodPost, suffix: "/invoke", right: grammar.RightsEnumEXECUTE},
		{name: "manage", relation: PermissionManage},
	}
	switch target.kind.ObjectType {
	case TypeSubmodel:
		if target.elementPath != "" {
			actions[2].relation = PermissionUpdate
		} else {
			actions[3].method = ""
		}
	case TypeAAS:
		actions[1].method = http.MethodPut
		actions[3].method = ""
	case TypeAssetLinks:
		actions[1].method = http.MethodPost
		actions = append(actions[:3], actions[4])
	default:
		actions[1].method = http.MethodPut
		actions = append(actions[:3], actions[4])
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
	keys := request.principal.SubjectKeys()
	results := make([]bool, len(actions))
	for index, action := range actions {
		target := request.target
		if action.name == "delete" && target.elementPath != "" {
			target.elementPath = ParentElementPath(target.elementPath)
		}
		allowed, err := c.effectivePermission(r, keys, target, action.relation)
		if err != nil {
			return nil, err
		}
		results[index] = allowed
	}
	return results, nil
}

// effectivePermission evaluates a permission; an element target whose path
// became empty addresses the containing Submodel.
func (c *Coordinator) effectivePermission(r *http.Request, keys []string, target accessTarget, permission string) (bool, error) {
	if target.kind.ObjectType == TypeSubmodel {
		return hasElementPermission(r.Context(), c.db, target.authUUID, target.elementPath, keys, permission)
	}
	return hasPermission(r.Context(), c.db, target.kind, target.authUUID, keys, permission)
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
	path := basePath + target.kind.Prefix + "/" + common.EncodeString(target.identifier)
	if target.elementPath != "" {
		path += "/submodel-elements/" + target.elementPath
	}
	return path
}
