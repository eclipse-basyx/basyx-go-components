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

// Package logging configures process-wide structured logging for BaSyx commands.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
)

const (
	// FormatText selects slog's human-readable text handler.
	FormatText = "text"
	// FormatJSON selects slog's newline-delimited JSON handler.
	FormatJSON = "json"

	// LevelDebug emits all supported log levels.
	LevelDebug = "debug"
	// LevelInfo filters debug events.
	LevelInfo = "info"
	// LevelWarn filters debug and info events.
	LevelWarn = "warn"
	// LevelError emits only error events.
	LevelError = "error"
)

// Config controls the format and minimum severity of emitted log records.
type Config struct {
	Format string `mapstructure:"format" yaml:"format" json:"format"`
	Level  string `mapstructure:"level" yaml:"level" json:"level"`
}

// Normalize validates configuration values and returns their canonical form.
func Normalize(cfg Config) (Config, error) {
	cfg.Format = strings.ToLower(strings.TrimSpace(cfg.Format))
	switch cfg.Format {
	case FormatText, FormatJSON:
	default:
		return Config{}, fmt.Errorf("CONFIG-LOGGING-FORMAT unsupported logging.format %q", cfg.Format)
	}

	cfg.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
	switch cfg.Level {
	case LevelDebug, LevelInfo, LevelWarn, LevelError:
	default:
		return Config{}, fmt.Errorf("CONFIG-LOGGING-LEVEL unsupported logging.level %q", cfg.Level)
	}

	return cfg, nil
}

// Configure installs the process-wide slog logger and bridges standard log output.
func Configure(cfg Config, serviceName string, output io.Writer) (*slog.Logger, error) {
	normalized, err := Normalize(cfg)
	if err != nil {
		return nil, err
	}
	trimmedServiceName := strings.TrimSpace(serviceName)
	if serviceName != trimmedServiceName || !validServiceName(trimmedServiceName) {
		return nil, fmt.Errorf("LOGGING-CONFIG-SERVICENAME invalid service name %q", serviceName)
	}
	serviceName = trimmedServiceName
	if isNilWriter(output) {
		return nil, fmt.Errorf("LOGGING-CONFIG-OUTPUT output must not be nil")
	}

	options := &slog.HandlerOptions{
		AddSource: false,
		Level:     parseLevel(normalized.Level),
	}
	var handler slog.Handler
	if normalized.Format == FormatJSON {
		handler = slog.NewJSONHandler(output, options)
	} else {
		handler = slog.NewTextHandler(output, options)
	}

	logger := slog.New(handler).With("service.name", serviceName)
	slog.SetDefault(logger)
	return logger, nil
}

func validServiceName(serviceName string) bool {
	if serviceName == "" {
		return false
	}
	for index, character := range serviceName {
		isLowercaseLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		isSeparator := character == '-'
		if !isLowercaseLetter && !isDigit && !isSeparator {
			return false
		}
		if index == 0 && isSeparator {
			return false
		}
	}
	return !strings.HasSuffix(serviceName, "-")
}

func parseLevel(level string) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func isNilWriter(output io.Writer) bool {
	if output == nil {
		return true
	}
	value := reflect.ValueOf(output)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
