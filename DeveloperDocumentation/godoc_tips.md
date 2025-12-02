# Godoc Tips for BaSyx Go Components

## Writing Effective GoDoc Comments

- **Package Comment**: Start each package with a block comment describing its purpose.
- **Function/Method Comments**: Begin with the function name. Example:
  ```go
  // CreateSubmodel creates a new submodel in the repository.
  func CreateSubmodel(...) {...}
  ```
- **Type Comments**: Describe the struct or interface and its fields.
- **Parameter and Return Descriptions**: Use `Parameters:` and `Returns:` for clarity.
- **Example Blocks**: Add usage examples for complex functions.

## Viewing Documentation Locally

1. Run `godoc -http=:6060`
2. Open [http://localhost:6060/pkg/](http://localhost:6060/pkg/) in your browser

## Best Practices

- Keep comments concise but informative
- Document edge cases and error handling
- Use Markdown in comments for lists and code blocks
- Update comments when changing function signatures

## Example: FileHandler

```go
// PostgreSQLFileHandler handles persistence for File submodel elements.
// It supports upload, download, and deletion of large objects in PostgreSQL.
type PostgreSQLFileHandler struct {
    db        *sql.DB
    decorated *PostgreSQLSMECrudHandler
}
```

## Useful Links
- [Effective Go](https://go.dev/doc/effective_go#commentary)
- [GoDoc Guidelines](https://blog.golang.org/godoc)

---
For more examples, see the comments in `internal/submodelrepository/persistence/Submodel/submodelElements/FileHandler.go` and other core files.
