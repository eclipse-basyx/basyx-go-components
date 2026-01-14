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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import "golang.org/x/sync/errgroup"

// GoAssign runs fn within the provided errgroup and assigns its successful
// result into dst. If fn returns an error, the assignment is skipped and the
// error is propagated to the group's Wait(). This helps reduce boilerplate
// around common patterns of spawning a goroutine, collecting a value, and
// handling the error.
//
// Example:
//
//	var out map[int64][]T
//	GoAssign(g, func() (map[int64][]T, error) { return load(... ) }, &out)
//
// The function is generic and can be used for any result type.
func GoAssign[T any](g *errgroup.Group, fn func() (T, error), dst *T) {
	g.Go(func() error {
		v, err := fn()
		if err != nil {
			return err
		}
		*dst = v
		return nil
	})
}
