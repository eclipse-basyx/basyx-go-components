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
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// EventRecordAuthorizer hides feed rows the caller cannot read as AAS or Submodel.
type EventRecordAuthorizer struct {
	Settings ABACSettings
}

// BindEventFeedAuthorizer installs event-level ABAC filtering when ABAC is on.
func BindEventFeedAuthorizer(settings ABACSettings) {
	if !settings.Enabled {
		eventfeed.SetRecordAuthorizer(nil)
		return
	}
	eventfeed.SetRecordAuthorizer(EventRecordAuthorizer{Settings: settings})
}

// Allow reports whether the caller may see an event for subject.
func (a EventRecordAuthorizer) Allow(ctx context.Context, eventType, subject string) bool {
	if !a.Settings.Enabled {
		return true
	}
	model := activeAccessModel(a.Settings)
	if model == nil {
		return false
	}
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return false
	}
	path := eventReadPath(model.BasePath(), eventType, subject)
	if path == "" {
		return false
	}
	opts := grammar.DefaultSimplifyOptions()
	opts.EnableImplicitCasts = a.Settings.EnableImplicitCasts
	evaluation := model.AuthorizeWithFilterWithOptions(EvalInput{
		Method:    httpMethodGet,
		Path:      path,
		RoutePath: path,
		Claims:    claims,
	}, opts)
	return evaluation.Allowed && !queryFilterRestricts(evaluation.QueryFilter)
}

const httpMethodGet = "GET"

func eventReadPath(basePath, eventType, subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	encoded := common.EncodeString(subject)
	var route string
	switch {
	case strings.HasPrefix(eventType, "io.admin-shell.aas."):
		route = "/shells/" + encoded
	case strings.HasPrefix(eventType, "io.admin-shell.submodel."), eventType == eventfeed.TypePCN:
		route = "/submodels/" + encoded
	case strings.HasPrefix(eventType, "io.admin-shell.asset."):
		route = "/lookup/shells"
	default:
		return ""
	}
	return joinBasePath(basePath, route)
}

func queryFilterRestricts(filter *QueryFilter) bool {
	if filter == nil {
		return false
	}
	if len(filter.Filters) > 0 {
		return true
	}
	if formulaRestricts(filter.Formula) {
		return true
	}
	for _, expr := range filter.FormulasByRight {
		e := expr
		if formulaRestricts(&e) {
			return true
		}
	}
	return false
}

func formulaRestricts(expr *grammar.LogicalExpression) bool {
	if expr == nil {
		return false
	}
	return expr.Boolean == nil || !*expr.Boolean
}
