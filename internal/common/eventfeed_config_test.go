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

package common

import (
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEventFeedConfigAdvertisesServedSchemas(t *testing.T) {
	cfg := &Config{Eventing: EventingConfig{Feed: EventFeedConfig{Enabled: true, SourceBaseURL: "https://public.example/api/v3"}}}
	actual := NewEventFeedConfig(cfg)
	require.Equal(t, "https://public.example/api/v3"+eventfeed.SchemaPath, actual.SchemaBaseURL)
}

func TestEventFeedConfigPublicBaseURL(t *testing.T) {
	cases := []struct {
		name           string
		cfg            Config
		source, schema string
	}{
		{name: "external prefix", cfg: Config{General: GeneralConfig{ExternalURL: "https://public.example/api/v3"}}, source: "https://public.example/api/v3", schema: "https://public.example/api/v3" + eventfeed.SchemaPath},
		{name: "local prefix", cfg: Config{Server: ServerConfig{Port: 6004, ContextPath: "/api/v3"}}, source: "http://localhost:6004/api/v3", schema: "http://localhost:6004/api/v3" + eventfeed.SchemaPath},
		{name: "explicit schema host", cfg: Config{Eventing: EventingConfig{Feed: EventFeedConfig{SourceBaseURL: "https://public.example/api", SchemaBaseURL: "https://schemas.example/feed/v1/"}}}, source: "https://public.example/api", schema: "https://schemas.example/feed/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := NewEventFeedConfig(&tc.cfg)
			require.Equal(t, tc.source, actual.SourceBaseURL)
			require.Equal(t, tc.schema, actual.SchemaBaseURL)
			require.False(t, actual.Enabled)
		})
	}
}

func TestEventFeedConfigZeroGraceIsPreserved(t *testing.T) {
	cfg := &Config{Eventing: EventingConfig{Feed: EventFeedConfig{Enabled: true, HardDeleteGraceDays: 0}}}
	require.Zero(t, NewEventFeedConfig(cfg).HardDeleteGrace)
}
