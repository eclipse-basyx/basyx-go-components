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

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

// RefreshReBACExecutionContext refreshes a queued operation's validated identity and revision.
// The caller must authorize each concrete target in its writer transaction.
func RefreshReBACExecutionContext(ctx context.Context) (context.Context, error) {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return ctx, nil
	}
	ctx = context.WithValue(ctx, rebacReadTransactionKey{}, (*sql.Tx)(nil))
	ctx = context.WithValue(ctx, rebacWriterTransactionKey{}, (*sql.Tx)(nil))
	original, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || original == nil || original.security == nil {
		return ctx, common.NewErrServiceUnavailable("REBAC-ASYNC-CONTEXT validated initiating authorization is required")
	}
	identity, err := ReBACIdentityFromContext(ctx, cfg.ReBAC)
	if err != nil {
		return ctx, err
	}
	request := *original
	request.readGrants = &rebacReadGrantCache{}
	request.identity = identity
	request.actor = rebacAsyncActor(identity)
	model := activeAccessModel(request.security.settings)
	if model == nil {
		return ctx, common.NewErrServiceUnavailable("REBAC-ASYNC-MODEL active authorization model is required")
	}
	options := grammar.DefaultSimplifyOptions()
	options.EnableImplicitCasts = request.security.settings.EnableImplicitCasts
	var refreshed context.Context
	refresh := func(tx *sql.Tx) error {
		state, lockErr := request.security.runtime.State.Lock(ctx, tx, false)
		if lockErr != nil {
			return lockErr
		}
		request.revision = state.Applied
		refreshed = context.WithValue(ctx, rebacRequestKey{}, &request)
		refreshed = context.WithValue(refreshed, authorizationSessionContextKey{}, newAuthorizationSession(model, ClaimsFromContext(ctx), nil, options))
		user := ""
		if identity.Principal != nil {
			user = request.actor.User
		}
		refreshed = rebac.ContextWithLifecycleActor(refreshed, rebac.LifecycleActor{User: user, Revision: request.revision})
		return nil
	}
	err = request.security.runtime.State.WithApplied(ctx, refresh)
	if err != nil {
		return ctx, common.NewErrServiceUnavailable("REBAC-ASYNC-REFRESH " + err.Error())
	}
	return refreshed, nil
}

func rebacAsyncActor(identity ReBACIdentity) rebac.GrantActor {
	actor := rebac.GrantActor{User: "user:anonymous", Groups: identity.ContextualTuples, Administrator: identity.Administrator}
	if identity.Principal == nil {
		return actor
	}
	user, err := identity.Principal.Object()
	if err == nil {
		actor.User = user
	}
	return actor
}

// RefreshReBACReadContext reuses an active HTTP read barrier or refreshes detached work.
func RefreshReBACReadContext(ctx context.Context) (context.Context, error) {
	if tx, ok := ctx.Value(rebacReadTransactionKey{}).(*sql.Tx); ok && tx != nil {
		return ctx, nil
	}
	return RefreshReBACExecutionContext(ctx)
}
