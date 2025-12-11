#!/usr/bin/env bash
set -euo pipefail

# Export the Keycloak realm from a running container and copy it to the host.
# Defaults: runtime=docker, output=./export, auto-detect a single running Keycloak container.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

runtime="${CONTAINER_RUNTIME:-docker}"
container_name=""
output_dir="export"

usage() {
  cat <<EOF
Usage: $0 [-r|--runtime docker|podman] [-c|--container <name>] [-o|--output <path>]

Options:
  -r, --runtime    Container runtime to use (default: docker; respects CONTAINER_RUNTIME).
  -c, --container  Container name/ID. If omitted, the script auto-detects a single running Keycloak container.
  -o, --output     Destination on the host for the exported realm directory (default: ./export).
  -h, --help       Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -r|--runtime)
      runtime="$2"
      shift 2
      ;;
    -c|--container)
      container_name="$2"
      shift 2
      ;;
    -o|--output)
      output_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v "$runtime" >/dev/null 2>&1; then
  echo "Runtime '$runtime' not found on PATH. Install it or set --runtime/CONTAINER_RUNTIME." >&2
  exit 1
fi

auto_detect_container() {
  mapfile -t matches < <("$runtime" ps --format '{{.Names}} {{.Image}}' | grep -iE 'keycloak')
  if [[ "${#matches[@]}" -eq 0 ]]; then
    echo "No running Keycloak container found; provide one with --container." >&2
    exit 1
  fi
  if [[ "${#matches[@]}" -gt 1 ]]; then
    echo "Multiple Keycloak containers found; please choose one with --container." >&2
    for line in "${matches[@]}"; do
      echo "  $line" >&2
    done
    exit 1
  fi
  echo "${matches[0]%% *}"
}

if [[ -z "$container_name" ]]; then
  container_name="$(auto_detect_container)"
fi

if [[ -e "$output_dir" ]]; then
  echo "Output path '$output_dir' already exists. Remove it or choose a different --output." >&2
  exit 1
fi

echo "Using runtime: $runtime"
echo "Container: $container_name"
echo "Output directory: $output_dir"
echo "Exporting realm inside container..."

export_status=0
set +e
$runtime exec "$container_name" sh -c "set -euo pipefail
~/bin/kcadm.sh config credentials --server http://localhost:8080 --realm master --user admin --password admin
rm -rf /tmp/export
~/bin/kc.sh export --dir /tmp/export --users realm_file
" || export_status=$?
set -e
if [[ "$export_status" -ne 0 ]]; then
  echo "Warning: export inside container failed (exit $export_status). Continuing..." >&2
fi

echo "Copying export to host..."
mkdir -p "$(dirname "$output_dir")"
$runtime cp "${container_name}:/tmp/export" "$output_dir"

echo "Done. Export available at: $output_dir"
