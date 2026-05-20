package steps

import (
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// ExecutionContext holds shared state across initialization steps.
type ExecutionContext struct {
	Config *common.Config
	DB     *sql.DB
}
