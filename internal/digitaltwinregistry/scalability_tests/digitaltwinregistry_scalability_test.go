package scalability_tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

const (
	composeFilePath    = "docker_compose/docker_compose.yml"
	requestTimeout     = 10 * time.Second
	keycloakClientID   = "basyx-ui"
	globalAssetIDName  = "globalAssetId"
	defaultPageLimit   = 50
	defaultRepetitions = 5
	defaultConcurrency = 2
	anonymousUserName  = "anonymous"
)

var (
	dtrBaseURL              string
	keycloakTokenURL        string
	scalabilityResultReport *scalabilityReport
)

type fixture struct {
	aasID              string
	submodelID         string
	primaryAssetLink   assetLink
	secondaryAssetLink assetLink
	globalAssetLink    assetLink
}

type assetLink struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type testUser struct {
	name      string
	password  string
	anonymous bool
}

type scenario struct {
	name string
	run  func(context.Context) (responseMetadata, error)
}

type requestResult struct {
	duration  time.Duration
	status    int
	bodyBytes int64
	err       error
}

type responseMetadata struct {
	status    int
	bodyBytes int64
}

func TestMain(m *testing.M) {
	scalabilityResultReport = newScalabilityReport(time.Now())
	if err := loadEnvironmentFile(".env"); err != nil {
		exitWithScalabilityReport("DTRSCALE-TESTMAIN-LOADENV: %v", err)
	}

	runtime, err := testenv.NewComposeRuntime("dtr-scalability", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "keycloak", EnvVar: "BASYX_IT_KEYCLOAK_PORT"},
	})
	if err != nil {
		exitWithScalabilityReport("DTRSCALE-TESTMAIN-COMPOSERUNTIME: %v", err)
	}
	if err := runtime.ApplyToProcess(); err != nil {
		runtime.Release()
		exitWithScalabilityReport("DTRSCALE-TESTMAIN-APPLYRUNTIME: %v", err)
	}
	dtrBaseURL = runtime.LocalURL("api") + "/api/v3"
	keycloakTokenURL = runtime.LocalhostURL("keycloak") + "/realms/basyx/protocol/openid-connect/token"
	securityEnvironment, err := testenv.PrepareSecurityEnv("../integration_tests/docker_compose/security_env", map[string]string{
		"http://localhost:8080": runtime.LocalhostURL("keycloak"),
	})
	if err != nil {
		runtime.Release()
		exitWithScalabilityReport("DTRSCALE-TESTMAIN-SECURITYENV: %v", err)
	}

	code := testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: composeFilePath,
		ProjectName: runtime.ProjectName,
		Env: runtime.EnvWith(
			"BASYX_IT_SECURITY_ENV=" + securityEnvironment,
		),
		HealthURL: dtrBaseURL + "/health",
	})
	_ = os.RemoveAll(securityEnvironment)
	if code != 0 {
		scalabilityResultReport.addFailure("The scalability process exited unsuccessfully; review the test output for the original error.")
	}
	writeScalabilityReport(code)
	os.Exit(code)
}

func exitWithScalabilityReport(format string, arguments ...any) {
	message := fmt.Sprintf(format, arguments...)
	_, _ = fmt.Fprintln(os.Stderr, message)
	scalabilityResultReport.addFailure(message)
	writeScalabilityReport(1)
	os.Exit(1)
}

func writeScalabilityReport(exitCode int) {
	path, err := scalabilityResultReport.write(exitCode)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "DTRSCALE-TESTMAIN-WRITEREPORT: %v\n", err)
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, "DTR scalability report written to %s\n", path)
}

func TestDTRScalability(t *testing.T) {
	fixtures := fixturesFromEnvironment(t)
	client := &http.Client{}
	tokenProvider := testenv.NewPasswordGrantTokenProvider(keycloakTokenURL, keycloakClientID, requestTimeout)
	users := testUsersFromEnvironment(t)
	referenceUser := referenceTestUser(t, users)
	referenceToken := accessToken(t, tokenProvider, referenceUser)
	pageLimit := environmentInt(t, "DTR_SCALE_PAGE_LIMIT", defaultPageLimit)
	repetitions := environmentInt(t, "DTR_SCALE_REQUEST_REPETITIONS", defaultRepetitions)
	concurrency := environmentInt(t, "DTR_SCALE_REQUEST_CONCURRENCY", defaultConcurrency)
	scalabilityResultReport.configure(fixtures, users, pageLimit, repetitions, concurrency)
	for index, item := range fixtures {
		t.Run(fmt.Sprintf("fixture_%d", index+1), func(t *testing.T) {
			primary, secondary, globalAssetLink := loadFixtureAssetLinks(t, client, referenceToken, item)
			for _, user := range users {
				user := user
				t.Run("user_"+user.name, func(t *testing.T) {
					token := accessToken(t, tokenProvider, user)
					for _, scenarioItem := range buildScenarios(t, client, token, item, primary, secondary, globalAssetLink, pageLimit) {
						runScenario(t, scenarioItem, repetitions, concurrency, scenarioReportContext{
							fixtureIndex: index + 1,
							user:         user.name,
						})
					}
				})
			}
		})
	}
}

func buildScenarios(t *testing.T, client *http.Client, token string, item fixture, primary, secondary, globalAssetLink assetLink, pageLimit int) []scenario {
	t.Helper()
	aasIdentifier := encodeIdentifier(item.aasID)
	submodelIdentifier := encodeIdentifier(item.submodelID)
	primaryParameter := encodedAssetLink(t, primary)
	secondaryParameter := encodedAssetLink(t, secondary)
	globalParameter := encodedAssetLink(t, globalAssetLink)
	pageQuery := url.Values{"limit": []string{strconv.Itoa(pageLimit)}}.Encode()
	primaryQuery := url.Values{"limit": []string{strconv.Itoa(pageLimit)}, "assetIds": []string{primaryParameter}}.Encode()
	multiQuery := url.Values{"limit": []string{strconv.Itoa(pageLimit)}, "assetIds": []string{primaryParameter, secondaryParameter}}.Encode()
	globalQuery := url.Values{"limit": []string{strconv.Itoa(pageLimit)}, "assetIds": []string{globalParameter}}.Encode()
	globalAndPrimaryQuery := url.Values{"limit": []string{strconv.Itoa(pageLimit)}, "assetIds": []string{globalParameter, primaryParameter}}.Encode()
	primaryPayload := assetLinkPayload(t, primary)
	globalPayload := assetLinkPayload(t, globalAssetLink)

	return []scenario{
		{name: "list_shell_descriptors", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors?"+pageQuery, nil)},
		{name: "get_shell_descriptor", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors/"+aasIdentifier, nil)},
		{name: "get_asset_links", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/lookup/shells/"+aasIdentifier, nil)},
		{name: "filter_shell_descriptors_by_asset_id", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors?"+primaryQuery, nil)},
		{name: "filter_shell_descriptors_by_two_asset_ids", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors?"+multiQuery, nil)},
		{name: "filter_shell_descriptors_by_global_asset_id", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors?"+globalQuery, nil)},
		{name: "filter_shell_descriptors_by_global_and_specific_asset_id", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors?"+globalAndPrimaryQuery, nil)},
		{name: "lookup_shells_by_asset_id", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/lookup/shells?"+primaryQuery, nil)},
		{name: "lookup_shells_by_two_asset_ids", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/lookup/shells?"+multiQuery, nil)},
		{name: "lookup_shells_by_global_asset_id", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/lookup/shells?"+globalQuery, nil)},
		{name: "lookup_shells_by_global_and_specific_asset_id", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/lookup/shells?"+globalAndPrimaryQuery, nil)},
		{name: "search_shells_by_asset_link", run: requestRunner(client, token, http.MethodPost, dtrBaseURL+"/lookup/shellsByAssetLink?"+pageQuery, primaryPayload)},
		{name: "search_shells_by_global_asset_id", run: requestRunner(client, token, http.MethodPost, dtrBaseURL+"/lookup/shellsByAssetLink?"+pageQuery, globalPayload)},
		{name: "list_submodel_descriptors", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors/"+aasIdentifier+"/submodel-descriptors?"+pageQuery, nil)},
		{name: "get_submodel_descriptor", run: requestRunner(client, token, http.MethodGet, dtrBaseURL+"/shell-descriptors/"+aasIdentifier+"/submodel-descriptors/"+submodelIdentifier, nil)},
	}
}

func requestRunner(client *http.Client, token, method, endpoint string, payload []byte) func(context.Context) (responseMetadata, error) {
	return func(ctx context.Context) (responseMetadata, error) {
		var body io.Reader
		if len(payload) > 0 {
			body = bytes.NewReader(payload)
		}
		request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
		if err != nil {
			return responseMetadata{}, fmt.Errorf("DTRSCALE-REQUEST-BUILD: %w", err)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		if len(payload) > 0 {
			request.Header.Set("Content-Type", "application/json")
		}

		response, err := client.Do(request)
		if err != nil {
			return responseMetadata{}, fmt.Errorf("DTRSCALE-REQUEST-EXECUTE: %w", err)
		}
		defer func() { _ = response.Body.Close() }()
		bodyBytes, err := io.Copy(io.Discard, response.Body)
		if err != nil {
			return responseMetadata{}, fmt.Errorf("DTRSCALE-REQUEST-READBODY: %w", err)
		}
		return responseMetadata{status: response.StatusCode, bodyBytes: bodyBytes}, nil
	}
}

func loadFixtureAssetLinks(t *testing.T, client *http.Client, token string, item fixture) (assetLink, assetLink, assetLink) {
	t.Helper()
	primary, secondary := item.primaryAssetLink, item.secondaryAssetLink
	links, lookupStatus, err := loadAssetLinks(t, client, token, item.aasID)
	if err != nil {
		t.Fatalf("DTRSCALE-LOOKUP-EXECUTE: %v", err)
	}
	if lookupStatus >= http.StatusInternalServerError {
		t.Fatalf("DTRSCALE-LOOKUP-STATUS received server error %d", lookupStatus)
	}
	if lookupStatus == http.StatusOK {
		primary, secondary = selectAssetLinks(t, links, primary, secondary)
	} else {
		t.Logf("bootstrap asset-link lookup returned %d; using sampled fixture links", lookupStatus)
	}

	globalAssetLink, descriptorStatus, err := loadGlobalAssetLink(t, client, token, item.aasID)
	if err != nil {
		t.Fatalf("DTRSCALE-GLOBAL-EXECUTE: %v", err)
	}
	if descriptorStatus >= http.StatusInternalServerError {
		t.Fatalf("DTRSCALE-GLOBAL-STATUS received server error %d", descriptorStatus)
	}
	if descriptorStatus != http.StatusOK {
		t.Logf("bootstrap shell-descriptor request returned %d; using sampled global asset link", descriptorStatus)
		globalAssetLink = item.globalAssetLink
	}
	return primary, secondary, globalAssetLink
}

func loadAssetLinks(t *testing.T, client *http.Client, token, aasID string) ([]assetLink, int, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.TODO(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, dtrBaseURL+"/lookup/shells/"+encodeIdentifier(aasID), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("DTRSCALE-LOOKUP-BUILD: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("DTRSCALE-LOOKUP-EXECUTE: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, nil
	}

	var links []assetLink
	if err := json.NewDecoder(response.Body).Decode(&links); err != nil {
		return nil, response.StatusCode, fmt.Errorf("DTRSCALE-LOOKUP-DECODE: %w", err)
	}
	return links, response.StatusCode, nil
}

func loadGlobalAssetLink(t *testing.T, client *http.Client, token, aasID string) (assetLink, int, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.TODO(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, dtrBaseURL+"/shell-descriptors/"+encodeIdentifier(aasID), nil)
	if err != nil {
		return assetLink{}, 0, fmt.Errorf("DTRSCALE-GLOBAL-BUILD: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return assetLink{}, 0, fmt.Errorf("DTRSCALE-GLOBAL-EXECUTE: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return assetLink{}, response.StatusCode, nil
	}

	var descriptor struct {
		GlobalAssetID string `json:"globalAssetId"`
	}
	if err := json.NewDecoder(response.Body).Decode(&descriptor); err != nil {
		return assetLink{}, response.StatusCode, fmt.Errorf("DTRSCALE-GLOBAL-DECODE: %w", err)
	}
	if descriptor.GlobalAssetID == "" {
		return assetLink{}, response.StatusCode, fmt.Errorf("DTRSCALE-GLOBAL-VALIDATE no globalAssetId returned for fixture")
	}
	return assetLink{Name: globalAssetIDName, Value: descriptor.GlobalAssetID}, response.StatusCode, nil
}

func selectAssetLinks(t *testing.T, links []assetLink, fallbackPrimary, fallbackSecondary assetLink) (assetLink, assetLink) {
	t.Helper()
	primaryName := requiredEnvironment(t, "DTR_SCALE_ASSET_ID_NAME")
	secondaryName := requiredEnvironment(t, "DTR_SCALE_SECOND_ASSET_ID_NAME")
	primary, primaryFound := assetLinkByName(links, primaryName)
	if !primaryFound {
		return fallbackPrimary, fallbackSecondary
	}
	secondary, secondaryFound := assetLinkByName(links, secondaryName)
	if secondaryFound && secondary != primary {
		return primary, secondary
	}
	for _, candidate := range links {
		if candidate != primary {
			return primary, candidate
		}
	}
	return primary, fallbackSecondary
}

func assetLinkByName(links []assetLink, name string) (assetLink, bool) {
	for _, link := range links {
		if link.Name == name && link.Value != "" {
			return link, true
		}
	}
	return assetLink{}, false
}

func runScenario(t *testing.T, item scenario, repetitions, concurrency int, reportContext scenarioReportContext) {
	t.Helper()
	if repetitions <= 0 || concurrency <= 0 {
		t.Fatalf("DTRSCALE-SCENARIO-VALIDATE repetitions and concurrency must be greater than zero")
	}
	results := make(chan requestResult, repetitions)
	jobs := make(chan struct{})
	var workers sync.WaitGroup
	for range min(repetitions, concurrency) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range jobs {
				started := time.Now()
				ctx, cancel := context.WithTimeout(context.TODO(), requestTimeout)
				response, err := item.run(ctx)
				cancel()
				results <- requestResult{duration: time.Since(started), status: response.status, bodyBytes: response.bodyBytes, err: err}
			}
		}()
	}
	for range repetitions {
		jobs <- struct{}{}
	}
	close(jobs)
	workers.Wait()
	close(results)

	durations := make([]time.Duration, 0, repetitions)
	statusCounts := make(map[int]int)
	totalBodyBytes := int64(0)
	var firstError error
	serverErrorStatus := 0
	for result := range results {
		if result.err != nil {
			if firstError == nil {
				firstError = result.err
			}
		}
		if result.status >= http.StatusInternalServerError && serverErrorStatus == 0 {
			serverErrorStatus = result.status
		}
		statusCounts[result.status]++
		totalBodyBytes += result.bodyBytes
		durations = append(durations, result.duration)
	}
	sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })
	p50 := percentile(durations, 50)
	p95 := percentile(durations, 95)
	maximum := durations[len(durations)-1]
	statusSummary := formatStatusCounts(statusCounts)
	scalabilityResultReport.addRow(scenarioReportRow{
		context:      reportContext,
		scenario:     item.name,
		requests:     len(durations),
		statusCounts: statusSummary,
		bodyBytes:    totalBodyBytes,
		p50:          p50,
		p95:          p95,
		maximum:      maximum,
	})
	t.Logf("%s requests=%d statuses=%s bytes=%d p50=%s p95=%s max=%s", item.name, len(durations), statusSummary, totalBodyBytes, p50, p95, maximum)
	if firstError != nil {
		scalabilityResultReport.addFailure(fmt.Sprintf("fixture %d user %s scenario %s: %v", reportContext.fixtureIndex, reportContext.user, item.name, firstError))
		t.Fatalf("DTRSCALE-SCENARIO-%s: %v", item.name, firstError)
	}
	if serverErrorStatus != 0 {
		scalabilityResultReport.addFailure(fmt.Sprintf("fixture %d user %s scenario %s: server returned HTTP status %d", reportContext.fixtureIndex, reportContext.user, item.name, serverErrorStatus))
		t.Fatalf("DTRSCALE-SCENARIO-%s: server returned HTTP status %d", item.name, serverErrorStatus)
	}
}

func formatStatusCounts(statusCounts map[int]int) string {
	statuses := make([]int, 0, len(statusCounts))
	for status := range statusCounts {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, strconv.Itoa(status)+":"+strconv.Itoa(statusCounts[status]))
	}
	return strings.Join(parts, ",")
}

func percentile(durations []time.Duration, percentage int) time.Duration {
	index := (len(durations)*percentage + 99) / 100
	if index > 0 {
		index--
	}
	return durations[index]
}

func fixturesFromEnvironment(t *testing.T) []fixture {
	t.Helper()
	count := environmentInt(t, "DTR_SCALE_FIXTURE_COUNT", 0)
	if count <= 0 {
		t.Fatal("DTRSCALE-FIXTURES-VALIDATE run scripts/dtr_scalability_fixtures first")
	}
	fixtures := make([]fixture, 0, count)
	for index := 1; index <= count; index++ {
		fixtures = append(fixtures, fixture{
			aasID:              requiredEnvironment(t, "DTR_SCALE_AAS_ID_"+strconv.Itoa(index)),
			submodelID:         requiredEnvironment(t, "DTR_SCALE_SUBMODEL_ID_"+strconv.Itoa(index)),
			primaryAssetLink:   assetLinkFromEnvironment(t, "DTR_SCALE_P_"+strconv.Itoa(index)),
			secondaryAssetLink: assetLinkFromEnvironment(t, "DTR_SCALE_Q_"+strconv.Itoa(index)),
			globalAssetLink:    assetLinkFromEnvironment(t, "DTR_SCALE_G_"+strconv.Itoa(index)),
		})
	}
	return fixtures
}

func assetLinkFromEnvironment(t *testing.T, key string) assetLink {
	t.Helper()
	payload, err := base64.RawURLEncoding.DecodeString(requiredEnvironment(t, key))
	if err != nil {
		t.Fatalf("DTRSCALE-FIXTURE-DECODE %s: %v", key, err)
	}
	var link assetLink
	if err := json.Unmarshal(payload, &link); err != nil {
		t.Fatalf("DTRSCALE-FIXTURE-UNMARSHAL %s: %v", key, err)
	}
	if link.Name == "" || link.Value == "" {
		t.Fatalf("DTRSCALE-FIXTURE-VALIDATE %s must encode an asset link with name and value", key)
	}
	return link
}

func testUsersFromEnvironment(t *testing.T) []testUser {
	t.Helper()
	password := requiredEnvironment(t, "DTR_SCALE_KEYCLOAK_PASSWORD")
	names := strings.Split(requiredEnvironment(t, "DTR_SCALE_KEYCLOAK_USERS"), ",")
	users := make([]testUser, 0, len(names)+1)
	seen := make(map[string]struct{}, len(names)+1)
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			t.Fatal("DTRSCALE-USERS-VALIDATE DTR_SCALE_KEYCLOAK_USERS must not contain an empty user")
		}
		if _, exists := seen[name]; exists {
			t.Fatalf("DTRSCALE-USERS-VALIDATE duplicate user %q", name)
		}
		seen[name] = struct{}{}
		users = append(users, testUser{name: name, password: password, anonymous: name == anonymousUserName})
	}
	if environmentBool(t, "DTR_SCALE_INCLUDE_ANONYMOUS", true) {
		if _, exists := seen[anonymousUserName]; !exists {
			users = append(users, testUser{name: anonymousUserName, anonymous: true})
		}
	}
	return users
}

func referenceTestUser(t *testing.T, users []testUser) testUser {
	t.Helper()
	referenceName := envValue("DTR_SCALE_KEYCLOAK_REFERENCE_USER", "admin")
	for _, user := range users {
		if user.name == referenceName {
			if user.anonymous {
				t.Fatal("DTRSCALE-USERS-VALIDATE reference user must not be anonymous")
			}
			return user
		}
	}
	t.Fatalf("DTRSCALE-USERS-VALIDATE reference user %q is not configured", referenceName)
	return testUser{}
}

func accessToken(t *testing.T, provider *testenv.PasswordGrantTokenProvider, user testUser) string {
	t.Helper()
	if user.anonymous {
		return ""
	}
	token, err := provider.GetAccessToken(&testenv.TokenCredentials{User: user.name, Password: user.password})
	if err != nil {
		t.Fatalf("DTRSCALE-TOKEN-%s: %v", user.name, err)
	}
	return token
}

func requiredEnvironment(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("DTRSCALE-ENV-VALIDATE %s must be set", key)
	}
	return value
}

func environmentInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		t.Fatalf("DTRSCALE-ENV-VALIDATE %s must be a positive integer", key)
	}
	return parsed
}

func environmentBool(t *testing.T, key string, fallback bool) bool {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		t.Fatalf("DTRSCALE-ENV-VALIDATE %s must be a boolean", key)
	}
	return parsed
}

func encodedAssetLink(t *testing.T, link assetLink) string {
	t.Helper()
	payload, err := json.Marshal(link)
	if err != nil {
		t.Fatalf("DTRSCALE-LINK-MARSHAL: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func assetLinkPayload(t *testing.T, links ...assetLink) []byte {
	t.Helper()
	payload, err := json.Marshal(links)
	if err != nil {
		t.Fatalf("DTRSCALE-LINKS-MARSHAL: %v", err)
	}
	return payload
}

func encodeIdentifier(identifier string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(identifier))
}

func envValue(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loadEnvironmentFile(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found || key == "" || strings.HasPrefix(key, "#") {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("DTRSCALE-ENV-SET %s: %w", key, err)
		}
	}
	return nil
}
