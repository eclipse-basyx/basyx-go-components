# INSTRUCTION SET FOR AI AGENTS - DO NOT DELETE THIS LINE.
# This file contains instructions for AI agents on how to handle code modifications.
# Follow these instructions carefully when making changes to the codebase.

## General Guidelines:
- Don't output unnecessary explanations or comments unless specifically requested or really mandatory.

## Code Style:
- DRY (Don't Repeat Yourself): Avoid code duplication by reusing existing functions and modules.
- Clean Code: If you need comments, it indicates that the code may need refactoring for clarity and should be extracted into well-named functions or modules.
- Consistent Naming Conventions: Use clear and consistent naming conventions for variables, functions, and modules.
- Error Handling: Implement robust error handling to manage potential issues gracefully.
  - Use Error Codes for all Errors (COMPONENTNAME-FUNCTION-CURRENTSTEP) e.g.: SMREPO-UPDPROP-EXECDBQUERY
- Code Readability: Ensure the code is easy to read and understand, with proper indentation and spacing
- Ensure that the Cognitive Complexity of functions does not exceed 15. Refactor complex functions into smaller, manageable pieces.
- MANDATORY: Use GOQU for SQL Queries in go. No Plain Text.
- New source files must use the repository's complete license header for their file type; copy it from a neighboring file.
- Do not add license headers to Markdown (`.md`), YAML (`.yml`, `.yaml`), or configuration (`.conf`) files.
- SQL patches must use the descriptive `--` banner header and formatting shown in `database/patches/1_1_18.sql`.
- Try to implement scalable and performant code - no prototyping.

## Pre-Task Steps:
1. Check if you understood the task requirements.
2. Ask Questions if any part of the task is unclear.

## Workflow:
1. Create a ToDo List
2. Collect Context around semantically near files/methods/functions
3. Implement Test Cases in integration_tests if applicable
4. Modify the Codebase
5. Run Tests and Ensure All Pass
6. Perform Code Review and Refactoring
7. Perform Post-Task Steps

## General Notes:
- Use `database/base.sql` and `database/patches/` for reference when modifying database-related code.
- Never modify `database/base.sql`; it is reference-only. Implement every database schema change exclusively through a versioned file under `database/patches/`.
- Ensure that database schema changes add a versioned patch under `database/patches/`, are registered in `cmd/basyxconfigurationservice/main.go`, and update the schema version metadata in `internal/common/database.go` when they become the current schema.
- When modifying code, consider the impact on related modules and ensure that changes are consistent across the codebase.
- Always run integration tests after making changes to ensure that the modifications do not break existing functionality. Important: Clean Testcache before running tests to avoid false positives/negatives.
- When adding queryable columns to the schema, you must never add them to a *_payload table.
- Never skip questions in plan mode and wait until the user has answered

## Changelog and Database Patch Rules:

- Review the complete proposed PR diff against its base branch, including pending edits. Do not use the previous commit or an intermediate PR revision as the baseline for release notes.
- Changie fragments describe the final user-visible or security-relevant differences from that base. Do not add separate entries for bugs, regressions, missing indexes, or other issues introduced and fixed entirely within the same unmerged PR.
- For a new feature, describe its final behavior in its feature entry. Put relevant security properties in SecurityImpact; do not present hardening of the new feature as a fix for a vulnerability in the base branch.
- Multiple Changie fragments per PR are allowed and encouraged for distinct notable changes. Group related changes where useful; do not create one fragment per commit or review iteration, and do not force unrelated changes into one fragment.
- When a fragment mixes a real base-branch fix with PR-internal work, rewrite it to retain only the actual release change. Remove redundant or entirely PR-internal fragments.
- Follow `.changie.yaml` and populate every required field, including Impact, PullRequest, and SecurityImpact. Use `None.` for SecurityImpact when appropriate; never omit the field.
- A PR may change at most ONE versioned file under `database/patches/`. Put all schema and index changes for that PR in its single new, unreleased patch, including follow-up fixes. Do not create another patch for each iteration, and never amend a patch already present in the PR base.
- Keep that patch registered in `cmd/basyxconfigurationservice/main.go` and keep `internal/common/database.go` consistent with the resulting schema version. `database/base.sql` remains reference-only.
- Before committing or pushing, audit all Changie fragments belonging to the PR, validate their schema, and count changed database patch files across the complete PR diff. Do not check only the latest commit.

## Post-Task Steps:
- Run linter and formatter on the modified files to ensure code quality and consistency.
- Also fix linting errors of unrelated files if you encounter them while running the linter.
- Do a Self-Code Review to ensure that the changes meet the task requirements and adhere to coding standards. (CC, DRY)

## Run Integration Tests
- go test -v ./internal/submodelrepository/integration_tests
- NEVER USE ADDITIONAL FLAGS (e.g. -run) UNLESS SPECIFICALLY REQUESTED IN THE TASK. ALL TESTS MUST BE RUN TO ENSURE CODE QUALITY AND INTEGRITY.

## Security Relevant Notes:
- do not use context.Background() in live code. Security needs information passed by context. Unit tests are allowed to create a context but they need a parameter so security is disabled.
