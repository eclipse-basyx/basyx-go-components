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

package events

import (
	"math/rand/v2"
	"time"
)

// RetryDelay calculates exponential retry backoff with jitter.
//
// Parameters:
//   - attempt: One-based attempt number; values below one use the initial delay.
//
// Returns:
//   - time.Duration: Delay starting at one second and capped at one minute.
func RetryDelay(attempt int) time.Duration {
	exponent := min(max(attempt-1, 0), 6)
	base := min(time.Second*time.Duration(1<<exponent), time.Minute)
	span := min(base/2, time.Minute-base)
	// #nosec G404 -- retry jitter is scheduling noise, not a security value.
	return base + time.Duration(rand.Int64N(int64(span)+1))
}
