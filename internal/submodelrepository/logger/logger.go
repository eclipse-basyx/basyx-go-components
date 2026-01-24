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

// Package logger provides centralized logging functionality for the submodel repository.
package logger

import (
	"log"
	"os"

	"github.com/FriedJannik/aas-go-sdk/types"
)

// Logger provides structured logging for the submodel repository.
var logger = log.New(os.Stderr, "[SubmodelRepo] ", log.LstdFlags|log.Lshortfile)

// LogError logs an error with context information.
//
// Parameters:
//   - context: A description of where/when the error occurred
//   - err: The error that occurred
func LogError(context string, err error) {
	if err != nil {
		logger.Printf("ERROR: %s: %v", context, err)
	}
}

// LogInfo logs an informational message.
//
// Parameters:
//   - message: The message to log
func LogInfo(message string) {
	logger.Printf("INFO: %s", message)
}

// LogWarning logs a warning message.
//
// Parameters:
//   - message: The warning message to log
func LogWarning(message string) {
	logger.Printf("WARN: %s", message)
}

// LogDebug logs a debug message.
//
// Parameters:
//   - message: The debug message to log
func LogDebug(message string) {
	logger.Printf("DEBUG: %s", message)
}

// LogHandlerCreationError logs a handler creation error with the handler type.
//
// Parameters:
//   - handlerType: The type of handler that failed to create
//   - err: The error that occurred
func LogHandlerCreationError(handlerType types.ModelType, err error) {
	logger.Printf("ERROR: creating %s handler: %v", handlerType, err)
}
