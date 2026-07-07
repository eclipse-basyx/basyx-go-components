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

//nolint:revive
package testenv

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const composePortLockStaleAfter = 24 * time.Hour

var reservedPortLocks struct {
	sync.Mutex
	locks []*portLock
}

// PortBinding declares one dynamically allocated host port for a compose test.
type PortBinding struct {
	Name   string
	EnvVar string
}

// ComposeRuntime carries the per-test compose project name, ports, and env values.
type ComposeRuntime struct {
	ProjectName string
	ports       map[string]int
	env         []string
	portLocks   []*portLock
}

// NewComposeRuntime creates a unique compose project name and locked host ports.
func NewComposeRuntime(prefix string, ports []PortBinding) (*ComposeRuntime, error) {
	projectName, err := NewComposeProjectName(prefix)
	if err != nil {
		return nil, err
	}

	runtime := &ComposeRuntime{
		ProjectName: projectName,
		ports:       make(map[string]int, len(ports)),
		env:         make([]string, 0, len(ports)),
	}

	usedPorts := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		name := strings.TrimSpace(port.Name)
		envVar := strings.TrimSpace(port.EnvVar)
		if name == "" || envVar == "" {
			runtime.Release()
			return nil, fmt.Errorf("TESTENV-COMPOSERT-BADPORT binding name and env var must not be empty")
		}
		reservedPort, lock, reserveErr := reserveLockedLocalPort(usedPorts)
		if reserveErr != nil {
			runtime.Release()
			return nil, reserveErr
		}
		usedPorts[reservedPort] = struct{}{}
		runtime.ports[name] = reservedPort
		runtime.portLocks = append(runtime.portLocks, lock)
		trackReservedPortLock(lock)
		runtime.env = append(runtime.env, envVar+"="+strconv.Itoa(reservedPort))
		runtime.env = append(runtime.env, composeURLVars(envVar, reservedPort)...)
	}

	return runtime, nil
}

// NewComposeRuntimeOrExit creates a compose runtime and exits the process on failure.
func NewComposeRuntimeOrExit(prefix string, ports []PortBinding) *ComposeRuntime {
	runtime, err := NewComposeRuntime(prefix, ports)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := runtime.ApplyToProcess(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return runtime
}

// Release frees host-port allocation locks held by this runtime.
func (r *ComposeRuntime) Release() {
	if r == nil {
		return
	}
	for _, lock := range r.portLocks {
		lock.Release()
	}
	r.portLocks = nil
}

// NewComposeProjectName returns a Docker Compose-compatible project name with a unique suffix.
func NewComposeProjectName(prefix string) (string, error) {
	sanitized := sanitizeComposeProjectName(prefix)
	if sanitized == "" {
		sanitized = "basyx"
	}

	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("TESTENV-COMPOSERT-RANDOM: %w", err)
	}

	return fmt.Sprintf("%s-%d-%d-%s", sanitized, os.Getpid(), time.Now().UnixNano(), hex.EncodeToString(random)), nil
}

func sanitizeComposeProjectName(prefix string) string {
	var builder strings.Builder
	lastSeparator := false
	for _, char := range strings.ToLower(strings.TrimSpace(prefix)) {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
			lastSeparator = false
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastSeparator = false
		case char == '-' || char == '_' || unicode.IsSpace(char) || char == '/' || char == '.':
			if builder.Len() > 0 && !lastSeparator {
				builder.WriteByte('-')
				lastSeparator = true
			}
		}
	}

	result := strings.Trim(builder.String(), "-_")
	if result == "" {
		return ""
	}
	if result[0] < 'a' || result[0] > 'z' {
		result = "basyx-" + result
	}
	if len(result) > 40 {
		result = strings.Trim(result[:40], "-_")
	}
	return result
}

// ReserveLocalPort returns a currently available localhost TCP port and locks it for this process.
func ReserveLocalPort() (int, error) {
	port, lock, err := reserveLockedLocalPort(nil)
	if err != nil {
		return 0, err
	}

	trackReservedPortLock(lock)
	return port, nil
}

func trackReservedPortLock(lock *portLock) {
	reservedPortLocks.Lock()
	reservedPortLocks.locks = append(reservedPortLocks.locks, lock)
	reservedPortLocks.Unlock()
}

// ReleaseReservedLocalPorts frees locks created for dynamic local port reservations.
func ReleaseReservedLocalPorts() {
	reservedPortLocks.Lock()
	defer reservedPortLocks.Unlock()

	for _, lock := range reservedPortLocks.locks {
		lock.Release()
	}
	reservedPortLocks.locks = nil
}

func reserveAvailableLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("TESTENV-COMPOSERT-RESERVEPORT: %w", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("TESTENV-COMPOSERT-PORTADDR unexpected listener address %T", listener.Addr())
	}
	return addr.Port, nil
}

func reserveLockedLocalPort(usedPorts map[int]struct{}) (int, *portLock, error) {
	var lastErr error
	for attempt := 0; attempt < 100; attempt++ {
		port, err := reserveAvailableLocalPort()
		if err != nil {
			return 0, nil, err
		}
		if _, alreadyUsed := usedPorts[port]; alreadyUsed {
			continue
		}

		lock, err := acquirePortLock(port)
		if err == nil {
			return port, lock, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return 0, nil, fmt.Errorf("TESTENV-COMPOSERT-LOCKPORT: %w", lastErr)
	}
	return 0, nil, fmt.Errorf("TESTENV-COMPOSERT-LOCKPORT could not reserve a unique local port")
}

type portLock struct {
	path string
}

func acquirePortLock(port int) (*portLock, error) {
	lockRoot := filepath.Join(os.TempDir(), "basyx-compose-port-locks")
	if err := os.MkdirAll(lockRoot, 0o700); err != nil {
		return nil, fmt.Errorf("TESTENV-COMPOSERT-LOCKROOT: %w", err)
	}

	lockPath := filepath.Join(lockRoot, strconv.Itoa(port)+".lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if os.IsExist(err) && removeStalePortLock(lockPath) == nil {
			if retryErr := os.Mkdir(lockPath, 0o700); retryErr == nil {
				return writePortLockOwner(lockPath)
			}
		}
		return nil, fmt.Errorf("TESTENV-COMPOSERT-LOCKCREATE: %w", err)
	}
	return writePortLockOwner(lockPath)
}

func removeStalePortLock(lockPath string) error {
	info, err := os.Stat(lockPath)
	if err != nil {
		return err
	}
	if time.Since(info.ModTime()) < composePortLockStaleAfter {
		return fmt.Errorf("TESTENV-COMPOSERT-LOCKACTIVE %s", lockPath)
	}
	return os.RemoveAll(lockPath)
}

func writePortLockOwner(lockPath string) (*portLock, error) {
	owner := fmt.Sprintf("pid=%d\ncreated=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(lockPath, "owner"), []byte(owner), 0o600); err != nil {
		_ = os.RemoveAll(lockPath)
		return nil, fmt.Errorf("TESTENV-COMPOSERT-LOCKOWNER: %w", err)
	}
	return &portLock{path: lockPath}, nil
}

func (l *portLock) Release() {
	if l == nil || l.path == "" {
		return
	}
	_ = os.RemoveAll(l.path)
	l.path = ""
}

func (r *ComposeRuntime) Env() []string {
	if r == nil {
		return nil
	}
	return append([]string{}, r.env...)
}

func (r *ComposeRuntime) EnvWith(extra ...string) []string {
	env := r.Env()
	return append(env, extra...)
}

func (r *ComposeRuntime) ApplyToProcess() error {
	if r == nil {
		return nil
	}
	return applyProcessEnv(r.env)
}

// SetEnvDefaults sets process env vars only when they are not already present.
func SetEnvDefaults(values map[string]string) error {
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("TESTENV-ENVDEFAULTS-SETENV: %w", err)
		}
	}
	return nil
}

// SetEnvDefaultsOrExit sets default env vars and exits the process on failure.
func SetEnvDefaultsOrExit(values map[string]string) {
	if err := SetEnvDefaults(values); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (r *ComposeRuntime) Port(name string) int {
	if r == nil {
		return 0
	}
	return r.ports[name]
}

func (r *ComposeRuntime) LocalURL(name string) string {
	return fmt.Sprintf("http://127.0.0.1:%d", r.Port(name))
}

func (r *ComposeRuntime) LocalhostURL(name string) string {
	return fmt.Sprintf("http://localhost:%d", r.Port(name))
}

func (r *ComposeRuntime) PostgresURL(name string, dbName string) string {
	return fmt.Sprintf("postgres://admin:admin123@127.0.0.1:%d/%s?sslmode=disable", r.Port(name), dbName)
}

func (r *ComposeRuntime) PostgresKeywordDSN(name string, dbName string) string {
	return fmt.Sprintf("host=127.0.0.1 port=%d user=admin password=admin123 dbname=%s sslmode=disable", r.Port(name), dbName)
}

func LocalURLFromEnv(envVar string, defaultPort int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", PortFromEnv(envVar, defaultPort))
}

func LocalhostURLFromEnv(envVar string, defaultPort int) string {
	return fmt.Sprintf("http://localhost:%d", PortFromEnv(envVar, defaultPort))
}

func PostgresURLFromEnv(envVar string, defaultPort int, dbName string) string {
	return fmt.Sprintf("postgres://admin:admin123@127.0.0.1:%d/%s?sslmode=disable", PortFromEnv(envVar, defaultPort), dbName)
}

func PostgresKeywordDSNFromEnv(envVar string, defaultPort int, dbName string) string {
	return fmt.Sprintf("host=127.0.0.1 port=%d user=admin password=admin123 dbname=%s sslmode=disable", PortFromEnv(envVar, defaultPort), dbName)
}

func PortFromEnv(envVar string, defaultPort int) int {
	value := strings.TrimSpace(os.Getenv(envVar))
	if value == "" {
		return defaultPort
	}
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 {
		return defaultPort
	}
	return port
}

func composeURLVars(envVar string, port int) []string {
	portSuffix := "_PORT"
	if !strings.HasSuffix(envVar, portSuffix) {
		return nil
	}
	prefix := strings.TrimSuffix(envVar, portSuffix)
	return []string{
		prefix + "_URL=http://127.0.0.1:" + strconv.Itoa(port),
		prefix + "_LOCALHOST_URL=http://localhost:" + strconv.Itoa(port),
	}
}

func PrepareSecurityEnv(srcDir string, replacements map[string]string) (string, error) {
	sourceInfo, err := os.Stat(srcDir)
	if err != nil {
		return "", fmt.Errorf("TESTENV-SECURITYENV-STAT: %w", err)
	}
	if !sourceInfo.IsDir() {
		return "", fmt.Errorf("TESTENV-SECURITYENV-NOTDIR %s is not a directory", srcDir)
	}

	targetDir, err := os.MkdirTemp("", "basyx-security-env-*")
	if err != nil {
		return "", fmt.Errorf("TESTENV-SECURITYENV-MKDIR: %w", err)
	}
	absoluteTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		_ = os.RemoveAll(targetDir)
		return "", fmt.Errorf("TESTENV-SECURITYENV-ABS: %w", err)
	}
	//nolint:gosec // Docker containers run as non-root users and need to read mounted fixture directories.
	if err := os.Chmod(absoluteTargetDir, 0o755); err != nil {
		_ = os.RemoveAll(absoluteTargetDir)
		return "", fmt.Errorf("TESTENV-SECURITYENV-CHMOD: %w", err)
	}

	if err := copySecurityEnv(srcDir, absoluteTargetDir, replacements); err != nil {
		_ = os.RemoveAll(absoluteTargetDir)
		return "", err
	}
	return absoluteTargetDir, nil
}

func PrepareSecurityEnvOrExit(srcDir string, replacements map[string]string) string {
	targetDir, err := PrepareSecurityEnv(srcDir, replacements)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return targetDir
}

func copySecurityEnv(srcDir string, targetDir string, replacements map[string]string) error {
	return filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("TESTENV-SECURITYENV-WALK: %w", walkErr)
		}

		relativePath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("TESTENV-SECURITYENV-REL: %w", err)
		}
		if relativePath == "." {
			return nil
		}

		targetPath := filepath.Join(targetDir, relativePath)
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("TESTENV-SECURITYENV-INFO: %w", err)
		}

		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		//nolint:gosec // The caller supplies a test fixture directory; WalkDir bounds reads to that tree.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("TESTENV-SECURITYENV-READ: %w", err)
		}
		content := string(data)
		for oldValue, newValue := range replacements {
			content = strings.ReplaceAll(content, oldValue, newValue)
		}
		if err := os.WriteFile(targetPath, []byte(content), info.Mode().Perm()); err != nil {
			return fmt.Errorf("TESTENV-SECURITYENV-WRITE: %w", err)
		}
		return nil
	})
}
