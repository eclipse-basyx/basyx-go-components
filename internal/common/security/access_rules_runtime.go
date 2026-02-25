package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	api "github.com/go-chi/chi/v5"
)

// SecuritySetupOptions configures component-specific security runtime behavior.
type SecuritySetupOptions struct {
	RulesTableName string
}

type accessRuleRecord struct {
	ID   int64                        `json:"id"`
	Rule grammar.AccessPermissionRule `json:"rule"`
}

type accessRuleResponse struct {
	ID int64 `json:"id"`
	grammar.AccessPermissionRule
}

type accessRulesRuntime struct {
	repo      *accessRulesRepository
	store     *AccessModelStore
	apiRouter *api.Mux
	basePath  string
}

func newAccessRulesRuntime(
	ctx context.Context,
	cfg *common.Config,
	apiRouter *api.Mux,
	opts SecuritySetupOptions,
) (*accessRulesRuntime, error) {
	if strings.TrimSpace(opts.RulesTableName) == "" {
		return nil, nil
	}

	db, err := openABACRulesDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("SEC-RULES-OPENDB: %w", err)
	}

	repo, err := newAccessRulesRepository(db, opts.RulesTableName)
	if err != nil {
		return nil, fmt.Errorf("SEC-RULES-INITREPO: %w", err)
	}
	if err := repo.EnsureTable(ctx); err != nil {
		return nil, fmt.Errorf("SEC-RULES-ENSURETABLE: %w", err)
	}

	rt := &accessRulesRuntime{
		repo:      repo,
		store:     NewAccessModelStore(nil),
		apiRouter: apiRouter,
		basePath:  cfg.Server.ContextPath,
	}

	if cfg.ABAC.SyncJSONToDB {
		if err := rt.SyncFromModelFile(ctx, cfg.ABAC.ModelPath); err != nil {
			return nil, fmt.Errorf("SEC-RULES-SYNCJSONTODB: %w", err)
		}
	}

	if err := rt.ReloadModelFromDB(ctx); err != nil {
		return nil, fmt.Errorf("SEC-RULES-RELOADMODEL: %w", err)
	}

	return rt, nil
}

func openABACRulesDB(cfg *common.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.DBName,
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func (r *accessRulesRuntime) ModelStore() *AccessModelStore {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *accessRulesRuntime) SyncFromModelFile(ctx context.Context, modelPath string) error {
	if strings.TrimSpace(modelPath) == "" {
		return errors.New("SEC-RULES-SYNC-READMODELFILE: abac.modelPath is empty")
	}
	//nolint:gosec // modelPath is an explicit deployment configuration input for the ABAC rules file
	data, err := os.ReadFile(modelPath)
	if err != nil {
		return fmt.Errorf("SEC-RULES-SYNC-READMODELFILE: %w", err)
	}
	rules, err := dematerializeRulesFromModelPayload(data)
	if err != nil {
		return fmt.Errorf("SEC-RULES-SYNC-DEMATERIALIZE: %w", err)
	}
	if err := r.repo.ReplaceAll(ctx, rules); err != nil {
		return fmt.Errorf("SEC-RULES-SYNC-REPLACEALL: %w", err)
	}
	return nil
}

func (r *accessRulesRuntime) ReloadModelFromDB(ctx context.Context) error {
	rules, err := r.repo.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("SEC-RULES-LOADRULES: %w", err)
	}
	model, err := ParseDematerializedAccessRules(rules, r.apiRouter, r.basePath)
	if err != nil {
		return fmt.Errorf("SEC-RULES-COMPILEMODEL: %w", err)
	}
	r.store.Set(model)
	return nil
}

func (r *accessRulesRuntime) RegisterRoutes(router *api.Mux) {
	router.Method(http.MethodGet, "/rules", http.HandlerFunc(r.handleListRules))
	router.Method(http.MethodPost, "/rules", http.HandlerFunc(r.handleCreateRule))
	router.Method(http.MethodGet, "/rules/{id}", http.HandlerFunc(r.handleGetRule))
	router.Method(http.MethodPut, "/rules/{id}", http.HandlerFunc(r.handlePutRule))
	router.Method(http.MethodDelete, "/rules/{id}", http.HandlerFunc(r.handleDeleteRule))
}

func (r *accessRulesRuntime) handleListRules(w http.ResponseWriter, req *http.Request) {
	records, err := r.repo.List(req.Context())
	if err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to list access permission rules."), http.StatusInternalServerError, "Rules", "ListQueryDB")
		return
	}
	writeJSON(w, http.StatusOK, toAccessRuleResponses(records))
}

func (r *accessRulesRuntime) handleGetRule(w http.ResponseWriter, req *http.Request) {
	id, ok := parseRuleID(api.URLParam(req, "id"))
	if !ok {
		writeRulesError(w, common.NewErrBadRequest("Rule ID must be a positive integer."), http.StatusBadRequest, "Rules", "GetBadID")
		return
	}
	record, found, err := r.repo.Get(req.Context(), id)
	if err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to load access permission rule."), http.StatusInternalServerError, "Rules", "GetQueryDB")
		return
	}
	if !found {
		writeRulesError(w, common.NewErrNotFound(fmt.Sprintf("Access permission rule %d", id)), http.StatusNotFound, "Rules", "GetNotFound")
		return
	}
	writeJSON(w, http.StatusOK, toAccessRuleResponse(record))
}

func (r *accessRulesRuntime) handleCreateRule(w http.ResponseWriter, req *http.Request) {
	rule, err := decodeRuleRequest(req)
	if err != nil {
		writeRulesError(w, common.NewErrBadRequest("Invalid access permission rule JSON body."), http.StatusBadRequest, "Rules", "CreateBadBody")
		return
	}
	if err := validateDematerializedRule(rule, r.apiRouter, r.basePath); err != nil {
		writeRulesError(w, common.NewErrBadRequest("Invalid access permission rule."), http.StatusBadRequest, "Rules", "CreateInvalidRule")
		return
	}

	record, err := r.repo.Insert(req.Context(), rule)
	if err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to store access permission rule."), http.StatusInternalServerError, "Rules", "CreateInsertDB")
		return
	}
	if err := r.ReloadModelFromDB(req.Context()); err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to reload access permission rules."), http.StatusInternalServerError, "Rules", "CreateReloadModel")
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/rules/%d", record.ID))
	writeJSON(w, http.StatusCreated, toAccessRuleResponse(record))
}

func (r *accessRulesRuntime) handlePutRule(w http.ResponseWriter, req *http.Request) {
	id, ok := parseRuleID(api.URLParam(req, "id"))
	if !ok {
		writeRulesError(w, common.NewErrBadRequest("Rule ID must be a positive integer."), http.StatusBadRequest, "Rules", "PutBadID")
		return
	}
	rule, err := decodeRuleRequest(req)
	if err != nil {
		writeRulesError(w, common.NewErrBadRequest("Invalid access permission rule JSON body."), http.StatusBadRequest, "Rules", "PutBadBody")
		return
	}
	if err := validateDematerializedRule(rule, r.apiRouter, r.basePath); err != nil {
		writeRulesError(w, common.NewErrBadRequest("Invalid access permission rule."), http.StatusBadRequest, "Rules", "PutInvalidRule")
		return
	}

	record, replaced, err := r.repo.ReplaceByID(req.Context(), id, rule)
	if err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to update access permission rule."), http.StatusInternalServerError, "Rules", "PutUpdateDB")
		return
	}
	if !replaced {
		writeRulesError(w, common.NewErrNotFound(fmt.Sprintf("Access permission rule %d", id)), http.StatusNotFound, "Rules", "PutNotFound")
		return
	}
	if err := r.ReloadModelFromDB(req.Context()); err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to reload access permission rules."), http.StatusInternalServerError, "Rules", "PutReloadModel")
		return
	}
	writeJSON(w, http.StatusOK, toAccessRuleResponse(record))
}

func (r *accessRulesRuntime) handleDeleteRule(w http.ResponseWriter, req *http.Request) {
	id, ok := parseRuleID(api.URLParam(req, "id"))
	if !ok {
		writeRulesError(w, common.NewErrBadRequest("Rule ID must be a positive integer."), http.StatusBadRequest, "Rules", "DeleteBadID")
		return
	}
	deleted, err := r.repo.Delete(req.Context(), id)
	if err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to delete access permission rule."), http.StatusInternalServerError, "Rules", "DeleteQueryDB")
		return
	}
	if !deleted {
		writeRulesError(w, common.NewErrNotFound(fmt.Sprintf("Access permission rule %d", id)), http.StatusNotFound, "Rules", "DeleteNotFound")
		return
	}
	if err := r.ReloadModelFromDB(req.Context()); err != nil {
		writeRulesError(w, common.NewInternalServerError("Failed to reload access permission rules."), http.StatusInternalServerError, "Rules", "DeleteReloadModel")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeRuleRequest(req *http.Request) (grammar.AccessPermissionRule, error) {
	defer func() {
		_ = req.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return grammar.AccessPermissionRule{}, err
	}
	var rule grammar.AccessPermissionRule
	if err := common.UnmarshalAndDisallowUnknownFields(body, &rule); err != nil {
		return grammar.AccessPermissionRule{}, err
	}
	return rule, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	_ = commonmodel.EncodeJSONResponse(payload, &status, w)
}

func writeRulesError(w http.ResponseWriter, err error, status int, function string, info string) {
	resp := common.NewErrorResponse(err, status, "Security", function, info)
	_ = commonmodel.EncodeJSONResponse(resp.Body, &resp.Code, w)
}

func validateDematerializedRule(rule grammar.AccessPermissionRule, apiRouter *api.Mux, basePath string) error {
	_, err := ParseDematerializedAccessRules([]grammar.AccessPermissionRule{rule}, apiRouter, basePath)
	return err
}

func dematerializeRulesFromModelPayload(data []byte) ([]grammar.AccessPermissionRule, error) {
	var model grammar.AccessRuleModelSchemaJSON
	if err := common.UnmarshalAndDisallowUnknownFields(data, &model); err != nil {
		return nil, err
	}
	return dematerializeRules(model.AllAccessPermissionRules)
}

func dematerializeRules(all grammar.AccessRuleModelSchemaJSONAllAccessPermissionRules) ([]grammar.AccessPermissionRule, error) {
	index, err := buildDefinitionIndex(all)
	if err != nil {
		return nil, err
	}

	out := make([]grammar.AccessPermissionRule, 0, len(all.Rules))
	for i, rule := range all.Rules {
		mr, err := materializeRule(index, rule)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}
		out = append(out, materializedRuleToAccessPermissionRule(mr))
	}
	return out, nil
}

func materializedRuleToAccessPermissionRule(mr materializedRule) grammar.AccessPermissionRule {
	acl := mr.acl
	if len(mr.attrs) > 0 {
		acl.ATTRIBUTES = append([]grammar.AttributeItem(nil), mr.attrs...)
	}
	acl.USEATTRIBUTES = nil

	var formula *grammar.LogicalExpression
	if mr.lexpr != nil {
		tmp := *mr.lexpr
		formula = &tmp
	}

	rule := grammar.AccessPermissionRule{
		ACL:     &acl,
		FORMULA: formula,
	}
	if len(mr.objs) > 0 {
		rule.OBJECTS = append([]grammar.ObjectItem(nil), mr.objs...)
	}
	if len(mr.filterList) > 0 {
		rule.FILTERLIST = append([]grammar.AccessPermissionRuleFILTER(nil), mr.filterList...)
	}
	return rule
}

type accessRulesRepository struct {
	db       *sql.DB
	dialect  goqu.DialectWrapper
	table    string
	tableSQL string
}

type storedRule struct {
	rule accessRuleRecord
	json string
}

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func newAccessRulesRepository(db *sql.DB, table string) (*accessRulesRepository, error) {
	table = strings.TrimSpace(table)
	if !sqlIdentifierPattern.MatchString(table) {
		return nil, fmt.Errorf("invalid rules table name %q", table)
	}
	return &accessRulesRepository{
		db:       db,
		dialect:  goqu.Dialect(common.Dialect),
		table:    table,
		tableSQL: `"` + table + `"`,
	}, nil
}

func (r *accessRulesRepository) EnsureTable(ctx context.Context) error {
	stmt := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY,
			rule_json JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		r.tableSQL,
	)
	if _, err := r.db.ExecContext(ctx, stmt); err != nil {
		return err
	}
	// Backward-compatible upgrade for earlier hash-based tables created during development.
	alterStmts := []string{
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS id BIGSERIAL`, r.tableSQL),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS rule_json JSONB`, r.tableSQL),
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, r.tableSQL),
	}
	for _, alterStmt := range alterStmts {
		if _, err := r.db.ExecContext(ctx, alterStmt); err != nil {
			return err
		}
	}
	return nil
}

func (r *accessRulesRepository) List(ctx context.Context) ([]accessRuleRecord, error) {
	ds := r.dialect.
		From(goqu.T(r.table)).
		Select(goqu.C("id"), goqu.C("rule_json")).
		Order(goqu.C("id").Asc())
	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []accessRuleRecord
	for rows.Next() {
		var (
			id      int64
			ruleRaw []byte
		)
		if err := rows.Scan(&id, &ruleRaw); err != nil {
			return nil, err
		}
		var rule grammar.AccessPermissionRule
		if err := common.UnmarshalAndDisallowUnknownFields(ruleRaw, &rule); err != nil {
			return nil, err
		}
		out = append(out, accessRuleRecord{ID: id, Rule: rule})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *accessRulesRepository) ListRules(ctx context.Context) ([]grammar.AccessPermissionRule, error) {
	records, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]grammar.AccessPermissionRule, 0, len(records))
	for _, rec := range records {
		rules = append(rules, rec.Rule)
	}
	return rules, nil
}

func (r *accessRulesRepository) Get(ctx context.Context, id int64) (accessRuleRecord, bool, error) {
	ds := r.dialect.
		From(goqu.T(r.table)).
		Select(goqu.C("id"), goqu.C("rule_json")).
		Where(goqu.C("id").Eq(id)).
		Limit(1)
	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return accessRuleRecord{}, false, err
	}
	//nolint:gosec // sqlStr is generated by goqu; table name is validated by sqlIdentifierPattern and values are parameterized
	row := r.db.QueryRowContext(ctx, sqlStr, args...)

	var (
		ruleID  int64
		ruleRaw []byte
	)
	if err := row.Scan(&ruleID, &ruleRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return accessRuleRecord{}, false, nil
		}
		return accessRuleRecord{}, false, err
	}
	var rule grammar.AccessPermissionRule
	if err := common.UnmarshalAndDisallowUnknownFields(ruleRaw, &rule); err != nil {
		return accessRuleRecord{}, false, err
	}
	return accessRuleRecord{ID: ruleID, Rule: rule}, true, nil
}

func (r *accessRulesRepository) Insert(ctx context.Context, rule grammar.AccessPermissionRule) (accessRuleRecord, error) {
	rec, err := newStoredRule(rule)
	if err != nil {
		return accessRuleRecord{}, err
	}

	ds := r.dialect.
		Insert(goqu.T(r.table)).
		Rows(goqu.Record{
			"rule_json":  goqu.L("?::jsonb", rec.json),
			"updated_at": goqu.L("NOW()"),
		}).
		Returning(goqu.C("id"))
	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return accessRuleRecord{}, err
	}
	var id int64
	//nolint:gosec // sqlStr is generated by goqu; table name is validated by sqlIdentifierPattern and values are parameterized
	if err := r.db.QueryRowContext(ctx, sqlStr, args...).Scan(&id); err != nil {
		return accessRuleRecord{}, err
	}
	rec.rule.ID = id
	return rec.rule, nil
}

func (r *accessRulesRepository) ReplaceByID(ctx context.Context, id int64, rule grammar.AccessPermissionRule) (accessRuleRecord, bool, error) {
	rec, err := newStoredRule(rule)
	if err != nil {
		return accessRuleRecord{}, false, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return accessRuleRecord{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	exists, err := r.existsTx(ctx, tx, id)
	if err != nil {
		return accessRuleRecord{}, false, err
	}
	if !exists {
		return accessRuleRecord{}, false, nil
	}

	updateDS := r.dialect.
		Update(goqu.T(r.table)).
		Set(goqu.Record{
			"rule_json":  goqu.L("?::jsonb", rec.json),
			"updated_at": goqu.L("NOW()"),
		}).
		Where(goqu.C("id").Eq(id))
	updateSQL, updateArgs, err := updateDS.ToSQL()
	if err != nil {
		return accessRuleRecord{}, false, err
	}
	//nolint:gosec // updateSQL is generated by goqu; table name is validated by sqlIdentifierPattern and values are parameterized
	if _, err := tx.ExecContext(ctx, updateSQL, updateArgs...); err != nil {
		return accessRuleRecord{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return accessRuleRecord{}, false, err
	}
	rec.rule.ID = id
	return rec.rule, true, nil
}

func (r *accessRulesRepository) Delete(ctx context.Context, id int64) (bool, error) {
	ds := r.dialect.Delete(goqu.T(r.table)).Where(goqu.C("id").Eq(id))
	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return false, err
	}
	//nolint:gosec // sqlStr is generated by goqu; table name is validated by sqlIdentifierPattern and values are parameterized
	result, err := r.db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *accessRulesRepository) ReplaceAll(ctx context.Context, rules []grammar.AccessPermissionRule) error {
	records := make([]storedRule, 0, len(rules))
	for _, rule := range rules {
		rec, err := newStoredRule(rule)
		if err != nil {
			return err
		}
		records = append(records, rec)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	deleteDS := r.dialect.Delete(goqu.T(r.table))
	deleteSQL, deleteArgs, err := deleteDS.ToSQL()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, deleteSQL, deleteArgs...); err != nil {
		return err
	}

	if len(records) > 0 {
		rows := make([]interface{}, 0, len(records))
		for _, rec := range records {
			rows = append(rows, goqu.Record{
				"rule_json":  goqu.L("?::jsonb", rec.json),
				"updated_at": goqu.L("NOW()"),
			})
		}
		insertDS := r.dialect.Insert(goqu.T(r.table)).Rows(rows...)
		insertSQL, insertArgs, err := insertDS.ToSQL()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, insertSQL, insertArgs...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *accessRulesRepository) existsTx(ctx context.Context, tx *sql.Tx, id int64) (bool, error) {
	ds := r.dialect.
		From(goqu.T(r.table)).
		Select(goqu.C("id")).
		Where(goqu.C("id").Eq(id)).
		Limit(1)
	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return false, err
	}
	var found int64
	//nolint:gosec // sqlStr is generated by goqu; table name is validated by sqlIdentifierPattern and values are parameterized
	if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func newStoredRule(rule grammar.AccessPermissionRule) (storedRule, error) {
	raw, err := json.Marshal(rule)
	if err != nil {
		return storedRule{}, err
	}
	var normalized grammar.AccessPermissionRule
	if err := common.UnmarshalAndDisallowUnknownFields(raw, &normalized); err != nil {
		return storedRule{}, err
	}
	normalizedRaw, err := json.Marshal(normalized)
	if err != nil {
		return storedRule{}, err
	}
	return storedRule{
		rule: accessRuleRecord{
			Rule: normalized,
		},
		json: string(normalizedRaw),
	}, nil
}

func parseRuleID(v string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func toAccessRuleResponse(rec accessRuleRecord) accessRuleResponse {
	return accessRuleResponse{
		ID:                   rec.ID,
		AccessPermissionRule: rec.Rule,
	}
}

func toAccessRuleResponses(records []accessRuleRecord) []accessRuleResponse {
	out := make([]accessRuleResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toAccessRuleResponse(rec))
	}
	return out
}
