package bench

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

const (
	BaseURL         = "http://127.0.0.1:5004"
	ComposeFilePath = "./docker_compose/docker_compose.yml"
)

var (
	composeAvailable bool
	composeEngine    string
	composeArgsBase  []string
)

// TestMain runs for integration and benchmark tests
func TestMain(m *testing.M) {
	eng, baseArgs, err := testenv.FindCompose()
	if err != nil {
		fmt.Println("compose engine not found:", err)
		composeAvailable = false
		os.Exit(m.Run())
	}
	composeAvailable = true
	composeEngine = eng
	composeArgsBase = baseArgs

	upArgs := append(composeArgsBase, "-f", ComposeFilePath, "up", "-d", "--build")
	if v := getenv("DISC_TEST_BUILD"); v == "1" {
		upArgs = append(upArgs, "--build")
	}

	ctxUp, cancelUp := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelUp()
	if err := testenv.RunCompose(ctxUp, composeEngine, upArgs...); err != nil {
		fmt.Println("failed to start compose:", err)
		composeAvailable = false
	}

	code := m.Run()

	ctxDown, cancelDown := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelDown()
	_ = testenv.RunCompose(ctxDown, composeEngine, append(composeArgsBase, "-f", ComposeFilePath, "down")...)

	os.Exit(code)
}

func mustHaveCompose(tb testing.TB) {
	tb.Helper()
	if !composeAvailable {
		tb.Skip("compose not available in this environment")
	}
}

func waitUntilHealthy(tb testing.TB) {
	tb.Helper()
	testenv.WaitHealthy(tb, BaseURL+"/health", 2*time.Minute)
}

func getenv(k string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return ""
}
