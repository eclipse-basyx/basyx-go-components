Rule 1 — You never write raw SQL manually

Meaning no more:

DB.Query("SELECT ...")
DB.Exec("DELETE ...")


Goqu generates SQL safely and cleanly.

Rule 2 — Always use the dialect from goqu.New("postgres", db)

Example:

dialect := goqu.New("postgres", p.DB)


This must appear in every persistence method.

Rule 3 — Always build SELECTs like this:
sql, _, err := dialect.
    From("aas").
    Select("id", "id_short", "category", "model_type").
    Order(goqu.I("id").Asc()).
    ToSQL()


Then execute:

rows, err := p.DB.Query(sql)


or use StructScan helpers 

Rule 4 — Always build DELETEs like this:
sql, _, _ := dialect.
    Delete("aas").
    Where(goqu.Ex{"id": id}).
    ToSQL()

Rule 5 — Always build INSERTs like this:
sql, _, _ := dialect.
    Insert("aas").
    Rows(goqu.Record{
        "id": id,
        "id_short": idShort,
        "category": category,
        "model_type": modelType,
    }).
    ToSQL()