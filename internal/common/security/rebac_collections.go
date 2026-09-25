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

package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

func semanticReBACKind(kind string) (SemanticResourceKind, bool) {
	switch kind {
	case "aas":
		return SemanticResourceAAS, true
	case "submodel":
		return SemanticResourceSM, true
	case "aas_descriptor":
		return SemanticResourceAASDesc, true
	case "submodel_descriptor":
		return SemanticResourceSMDesc, true
	case "concept_description":
		return SemanticResourceCD, true
	case "element":
		return SemanticResourceSME, true
	case "discovery":
		return SemanticResourceBD, true
	case "embedded_submodel_descriptor":
		return SemanticResourceSMDesc, true
	default:
		return "", false
	}
}

func (s *rebacSecurity) authorizeCollection(r *http.Request, tx *sql.Tx, request *rebacRequest, session *AuthorizationSession, evaluation AuthorizationEvaluation) (*http.Request, error) {
	semantic, ok := semanticReBACKind(string(request.route.Kind))
	if !ok {
		return nil, fmt.Errorf("REBAC-COLLECTION-KIND unsupported collection authorization")
	}
	grants, err := s.requestReadGrantSets(r.Context(), tx, request)
	if err != nil {
		return nil, err
	}
	evaluation, err = UnionAuthorizationEvaluationWithReBACReadGrants(semantic, evaluation, grants[semantic])
	if err != nil {
		return nil, err
	}
	if !evaluation.Allowed {
		return nil, rebac.ErrForbidden
	}
	session.rebacRead = grants
	if err = s.recordDecision(r.Context(), string(request.route.Kind), "read", "coordinator", "conditional"); err != nil {
		return nil, err
	}
	return withReBACEvaluation(r, session, evaluation), nil
}

func (s *rebacSecurity) readGrantSets(ctx context.Context, tx *sql.Tx, actor rebac.GrantActor) (map[SemanticResourceKind]ReBACReadGrantSet, error) {
	result := map[SemanticResourceKind]ReBACReadGrantSet{}
	for _, kind := range []string{"aas", "submodel", "aas_descriptor", "submodel_descriptor", "concept_description", "discovery", "element", "embedded_submodel_descriptor"} {
		semantic, _ := semanticReBACKind(kind)
		ids, err := s.readableIdentifiers(ctx, tx, actor, kind)
		if err != nil {
			return nil, err
		}
		grants, err := appendReadGrantIDs(result[semantic], kind, ids)
		if err != nil {
			return nil, err
		}
		result[semantic] = grants
	}
	return result, nil
}

func (s *rebacSecurity) readableIdentifiers(ctx context.Context, tx *sql.Tx, actor rebac.GrantActor, kind string) ([]string, error) {
	objectType := kind
	if kind == "embedded_submodel_descriptor" {
		objectType = "submodel_descriptor"
	}
	objectKeys, err := s.runtime.Client.StreamedListObjects(ctx, actor.User, "read", objectType, actor.Groups)
	if err != nil {
		return nil, err
	}
	resources, err := s.runtime.State.ResourcesByObjectKeys(ctx, tx, kind, objectKeys)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(resources))
	for _, resource := range resources {
		result = append(result, resource.Identifier)
	}
	return result, nil
}

func (s *AuthorizationSession) semanticView(resource, outer SemanticResourceKind) SemanticAccessView {
	view := s.abacSemanticView(resource, outer)
	if s == nil || s.rebacRead == nil {
		return view
	}
	grants := s.rebacRead[resource]
	if len(grants.Identifiers) == 0 && len(grants.Elements) == 0 && len(grants.EmbeddedSubmodelDescriptors) == 0 {
		return view
	}
	combined, err := UnionSemanticAccessViewWithReBACReadGrants(view, grants)
	if err != nil {
		return SemanticAccessView{resource: resource, decision: AccessViewDenied}
	}
	return combined
}

func appendReadGrantIDs(grants ReBACReadGrantSet, kind string, ids []string) (ReBACReadGrantSet, error) {
	if kind != "element" && kind != "embedded_submodel_descriptor" {
		grants.Identifiers = append(grants.Identifiers, ids...)
		return grants, nil
	}
	for _, id := range ids {
		var parts []string
		if err := json.Unmarshal([]byte(id), &parts); err != nil || len(parts) != 2 {
			return grants, fmt.Errorf("REBAC-COLLECTION-IDENTITY invalid compound identity")
		}
		if kind == "element" {
			grants.Elements = append(grants.Elements, ReBACElementReadGrant{SubmodelID: parts[0], ElementPath: parts[1]})
		} else {
			grants.EmbeddedSubmodelDescriptors = append(grants.EmbeddedSubmodelDescriptors, ReBACEmbeddedSubmodelDescriptorReadGrant{AASDescriptorID: parts[0], SubmodelID: parts[1]})
		}
	}
	return grants, nil
}

type rebacReadGrantCache struct {
	once   sync.Once
	grants map[SemanticResourceKind]ReBACReadGrantSet
	err    error
}

func (s *rebacSecurity) requestReadGrantSets(ctx context.Context, tx *sql.Tx, request *rebacRequest) (map[SemanticResourceKind]ReBACReadGrantSet, error) {
	if request.readGrants == nil {
		return s.readGrantSets(ctx, tx, request.actor)
	}
	cache := request.readGrants
	cache.once.Do(func() { cache.grants, cache.err = s.readGrantSets(ctx, tx, request.actor) })
	return cache.grants, cache.err
}
