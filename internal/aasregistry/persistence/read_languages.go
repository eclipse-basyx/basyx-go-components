package aasregistrydatabase

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

// GetLangStringTextTypesByIDs fetches LangStringTextType rows for the given
// reference IDs and groups them by their reference ID. An empty input returns
// an empty map.
func GetLangStringTextTypesByIDs(
	db *sql.DB,
	textTypeIDs []int64,
) (map[int64][]model.LangStringTextType, error) {

	out := make(map[int64][]model.LangStringTextType, len(textTypeIDs))
	if len(textTypeIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")

    arr := pq.Array(textTypeIDs)
    ds := dialect.
        From("lang_string_text_type").
        Select("lang_string_text_type_reference_id", "text", "language").
        Where(goqu.L("lang_string_text_type_reference_id = ANY(?::bigint[])", arr))

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var refID int64
		var text, language string
		if err := rows.Scan(&refID, &text, &language); err != nil {
			return nil, err
		}
		out[refID] = append(out[refID], model.LangStringTextType{
			Text:     text,
			Language: language,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// GetLangStringNameTypesByIDs fetches LangStringNameType rows for the given
// reference IDs and groups them by their reference ID. An empty input returns
// an empty map.
func GetLangStringNameTypesByIDs(
	db *sql.DB,
	nameTypeIDs []int64,
) (map[int64][]model.LangStringNameType, error) {

	out := make(map[int64][]model.LangStringNameType, len(nameTypeIDs))
	if len(nameTypeIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")

	// Build query
    arr := pq.Array(nameTypeIDs)
    ds := dialect.
        From("lang_string_name_type").
        Select("lang_string_name_type_reference_id", "text", "language").
        Where(goqu.L("lang_string_name_type_reference_id = ANY(?::bigint[])", arr))

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var refID int64
		var text, language string
		if err := rows.Scan(&refID, &text, &language); err != nil {
			return nil, err
		}
		out[refID] = append(out[refID], model.LangStringNameType{
			Text:     text,
			Language: language,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
