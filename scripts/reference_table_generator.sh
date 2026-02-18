#!/usr/bin/env bash
set -euo pipefail

fail() {
  local code="$1"
  local msg="$2"
  echo "ERROR [$code] $msg" >&2
  exit 1
}

output_file="${1:-draft_ref.sql}"

contexts=(
"submodel_semantic_id|submodel|id|true"
"submodel_element_semantic_id|submodel_element|id|true"
"submodel_descriptor_semantic_id|submodel_descriptor|descriptor_id|true"
"specific_asset_id_external_subject_id|specific_asset_id|id|true"
"specific_asset_id_supplemental_semantic_id|specific_asset_id|id|true"
"submodel_descriptor_supplemental_semantic_id|submodel_descriptor|descriptor_id|true"
)

[[ ${#contexts[@]} -gt 0 ]] || fail "SMREPO-GENREF-EMPTYCONTEXTS" "No contexts configured"

cat > "$output_file" <<'SQL'
/*
 Auto-generated file. Do not edit manually.
 Naming pattern: <context>_reference and <context>_reference_key.
*/
SQL

i=0
for entry in "${contexts[@]}"; do
  i=$((i + 1))
  [[ -n "$entry" ]] || fail "SMREPO-GENREF-INVALIDCONTEXT" "Encountered empty context"

  IFS='|' read -r ctx ref_table ref_column with_payload <<< "$entry"

  [[ -n "$ctx" ]] || fail "SMREPO-GENREF-MISSINGCTX" "Missing ctx in entry: $entry"
  [[ -n "$ref_table" ]] || fail "SMREPO-GENREF-MISSINGREFTABLE" "Missing ref table in entry: $entry"
  [[ -n "$ref_column" ]] || fail "SMREPO-GENREF-MISSINGREFCOLUMN" "Missing ref column in entry: $entry"
  [[ -n "$with_payload" ]] || fail "SMREPO-GENREF-MISSINGPAYLOADFLAG" "Missing payload flag in entry: $entry"
  [[ "$with_payload" == "true" || "$with_payload" == "false" ]] || fail "SMREPO-GENREF-INVALIDPAYLOADFLAG" "Invalid payload flag '$with_payload' in entry: $entry"

  cat >> "$output_file" <<SQL

-- =========================================================
-- $i) $ctx -> ${ref_table}.${ref_column}
-- =========================================================
CREATE TABLE IF NOT EXISTS ${ctx}_reference (
  id   BIGINT PRIMARY KEY REFERENCES ${ref_table}(${ref_column}) ON DELETE CASCADE,
  type int NOT NULL
);

CREATE TABLE IF NOT EXISTS ${ctx}_reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES ${ctx}_reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  type         int NOT NULL,
  value        TEXT NOT NULL,
  UNIQUE(reference_id, position)
);
SQL

  if [[ "$with_payload" == "true" ]]; then
    cat >> "$output_file" <<SQL

CREATE TABLE IF NOT EXISTS ${ctx}_reference_payload (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES ${ctx}_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);
SQL
  fi
done

echo "Generated: $output_file"