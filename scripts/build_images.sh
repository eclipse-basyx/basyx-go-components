#!/usr/bin/env bash

set -euo pipefail

readonly SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
readonly REPO_ROOT="$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)"
readonly DEFAULT_REGISTRY="eclipsebasyx"
readonly DEFAULT_TAG="SNAPSHOT"

SERVICES=(
  "aasenvironmentservice|AAS Environment Service|eclipsebasyx/aasenvironment-go|cmd/aasenvironmentservice/Dockerfile"
  "aasregistryservice|AAS Registry Service|eclipsebasyx/aasregistry-go|cmd/aasregistryservice/Dockerfile"
  "discoveryservice|Discovery Service|eclipsebasyx/aasdiscovery-go|cmd/discoveryservice/Dockerfile"
  "digitaltwinregistryservice|Digital Twin Registry Service|eclipsebasyx/digitaltwinregistry-go|cmd/digitaltwinregistryservice/Dockerfile"
  "companylookupservice|Company Lookup Service|eclipsebasyx/companylookup-go|cmd/companylookupservice/Dockerfile"
  "submodelrepositoryservice|Submodel Repository Service|eclipsebasyx/submodelrepository-go|cmd/submodelrepositoryservice/Dockerfile"
  "submodelregistryservice|Submodel Registry Service|eclipsebasyx/submodelregistry-go|cmd/submodelregistryservice/Dockerfile"
  "conceptdescriptionrepositoryservice|Concept Description Repository Service|eclipsebasyx/conceptdescriptionrepository-go|cmd/conceptdescriptionrepositoryservice/Dockerfile"
  "aasrepositoryservice|AAS Repository Service|eclipsebasyx/aasrepository-go|cmd/aasrepositoryservice/Dockerfile"
  "aasxfileserverservice|AASX File Server Service|eclipsebasyx/aasxfileserver-go|cmd/aasxfileserverservice/Dockerfile"
  "basyxconfigurationservice|BaSyx Configuration Service|eclipsebasyx/basyxconfigurationservice-go|cmd/basyxconfigurationservice/Dockerfile"
  "dppapiservice|DPP API Service|eclipsebasyx/dppapi-go|cmd/dppapiservice/Dockerfile"
)

selection="all"
tag="$DEFAULT_TAG"
method="docker"
output="load"
platform="native"
repository="$DEFAULT_REGISTRY"
non_interactive=false
assume_yes=false
dry_run=false

die() { echo "ERROR [BUILDIMG-$1] $2" >&2; exit 1; }
cancel() { echo; echo "Build cancelled." >&2; exit 130; }

usage() {
  sed -n 's/^# Usage: //p; s/^#   //p' "$0"
  cat <<'EOF'
Usage: scripts/build_images.sh [options]
  --services all|aas-environment-config|service[,service...]
  --tag TAG                         Image tag (default: SNAPSHOT)
  --method docker|buildx            Build method (default: docker)
  --output load|push                BuildX output (default: load)
  --platform native|linux/amd64|linux/arm64|linux/arm/v7|PLATFORM[,PLATFORM...]
  --repository REPOSITORY           Image repository prefix (default: eclipsebasyx)
  --registry REGISTRY/NAMESPACE     Alias for --repository
  --non-interactive                 Skip the wizard and use options/defaults
  --yes                             Start without confirmation
  --dry-run                         Print commands without running Docker
  -h, --help                        Show this help
EOF
}

valid_tag() { [[ "$1" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]*$ ]]; }
valid_platform() { [[ "$1" == native || "$1" == linux/amd64 || "$1" == linux/arm64 || "$1" == linux/arm/v7 ]]; }
valid_registry() { [[ "$1" =~ ^[A-Za-z0-9.-]+(:[0-9]+)?(/[A-Za-z0-9._-]+)*$ ]]; }

service_index() {
  local wanted="$1" i fields
  for i in "${!SERVICES[@]}"; do
    IFS='|' read -r fields _ <<< "${SERVICES[$i]}"
    [[ "$fields" == "$wanted" ]] && { echo "$i"; return 0; }
  done
  return 1
}

selected_indices() {
  local item index
  if [[ "$selection" == all ]]; then
    printf '%s\n' "${!SERVICES[@]}"
  elif [[ "$selection" == aas-environment-config ]]; then
    printf '%s\n' "$(service_index aasenvironmentservice)" "$(service_index basyxconfigurationservice)"
  else
    IFS=',' read -ra requested <<< "$selection"
    for item in "${requested[@]}"; do
      index=$(service_index "$item") || die "SERVICES" "Unknown service '$item'"
      printf '%s\n' "$index"
    done
  fi
}

prompt_number() {
  local label="$1" default="$2" max="$3" answer
  while true; do
    printf '%s [%s]: ' "$label" "$default" >&2
    IFS= read -r answer || cancel
    answer="${answer:-$default}"
    if [[ "$answer" =~ ^[0-9]+$ ]] && (( answer >= 1 && answer <= max )); then
      printf '%s\n' "$answer"; return
    fi
    echo "Please enter a number from 1 to $max." >&2
  done
}

wizard() {
  local answer
  echo "Build BaSyx Images for Local Use"
  echo
  echo "1. Select services"
  echo "   1) All services (12)"
  echo "   2) AAS Environment + Configuration Service (2)"
  echo "   3) Select individual services"
  answer=$(prompt_number "Select" 1 3)
  case "$answer" in
    1) selection=all ;;
    2) selection=aas-environment-config ;;
    3) selection=$(select_individual) ;;
  esac

  echo
  echo "2. Image tag"
  while true; do
    printf 'Tag [%s]: ' "$DEFAULT_TAG" >&2
    IFS= read -r answer || cancel
    tag="${answer:-$DEFAULT_TAG}"
    valid_tag "$tag" && break
    echo "Tags may contain letters, numbers, '_', '.', and '-' and must start with a letter or number." >&2
  done

  echo
  echo "3. Image repository"
  while true; do
    printf 'Repository [%s]: ' "$DEFAULT_REGISTRY" >&2
    IFS= read -r answer || cancel
    repository="${answer:-$DEFAULT_REGISTRY}"
    valid_registry "$repository" && break
    echo "Enter a repository such as eclipsebasyx or ghcr.io/eclipse-basyx." >&2
  done

  echo
  echo "4. Build method"
  echo "   1) Docker Build"
  echo "   2) Docker BuildX"
  answer=$(prompt_number "Select" 1 2)
  [[ "$answer" == 2 ]] && method=buildx || method=docker

  if [[ "$method" == buildx ]]; then
    echo
    echo "5. Output"
    echo "   1) Load into local Docker"
    echo "   2) Push to registry"
    answer=$(prompt_number "Select" 1 2)
    [[ "$answer" == 2 ]] && output=push || output=load

    echo
    echo "6. Target platform"
    echo "   1) Native platform"
    echo "   2) linux/amd64"
    echo "   3) linux/arm64"
    echo "   4) linux/arm/v7"
    echo "   5) Multiple platforms (push only)"
    answer=$(prompt_number "Select" 1 5)
    case "$answer" in
      1) platform=native ;; 2) platform=linux/amd64 ;; 3) platform=linux/arm64 ;; 4) platform=linux/arm/v7 ;;
      5) platform=$(prompt_platforms) ;;
    esac
    [[ "$platform" == *,* && "$output" != push ]] && { echo "Multiple platforms require push output." >&2; output=push; }
  fi
  if [[ "$output" == push ]]; then
    prompt_registry
  fi
}

select_individual() {
  local item index result=""; echo "Enter service names separated by commas (for example: aasenvironmentservice,aasregistryservice)." >&2
  while true; do
    printf 'Services: ' >&2; IFS= read -r result || cancel
    [[ -n "$result" ]] || { echo "Select at least one service." >&2; continue; }
    IFS=',' read -ra items <<< "$result"
    for item in "${items[@]}"; do service_index "$item" >/dev/null || { echo "Unknown service '$item'." >&2; result=""; break; }; done
    [[ -n "$result" ]] && { printf '%s\n' "$result"; return; }
  done
}

prompt_platforms() {
  local result item valid; while true; do
    printf 'Platforms (comma-separated): ' >&2; IFS= read -r result || cancel
    valid=true; IFS=',' read -ra items <<< "$result"
    for item in "${items[@]}"; do valid_platform "$item" && [[ "$item" != native ]] || valid=false; done
    $valid && [[ "${#items[@]}" -gt 1 ]] && { printf '%s\n' "$result"; return; }
    echo "Enter at least two supported non-native platforms, separated by commas." >&2
  done
}

prompt_registry() {
  while true; do
    printf 'Registry/namespace [%s]: ' "$repository" >&2; IFS= read -r repository || cancel
    repository="${repository:-$DEFAULT_REGISTRY}"; valid_registry "$repository" && return
    echo "Enter a registry/namespace such as ghcr.io/eclipse-basyx." >&2
  done
}

parse_args() {
  while (($#)); do
    case "$1" in
      --services) [[ $# -ge 2 ]] || die "ARGS" "--services requires a value"; selection=$2; shift 2 ;;
      --tag) [[ $# -ge 2 ]] || die "ARGS" "--tag requires a value"; tag=$2; shift 2 ;;
      --method) [[ $# -ge 2 ]] || die "ARGS" "--method requires a value"; method=$2; shift 2 ;;
      --output) [[ $# -ge 2 ]] || die "ARGS" "--output requires a value"; output=$2; shift 2 ;;
      --platform) [[ $# -ge 2 ]] || die "ARGS" "--platform requires a value"; platform=$2; shift 2 ;;
      --repository) [[ $# -ge 2 ]] || die "ARGS" "--repository requires a value"; repository=$2; shift 2 ;;
      --registry) [[ $# -ge 2 ]] || die "ARGS" "--registry requires a value"; repository=$2; shift 2 ;;
      --non-interactive) non_interactive=true; shift ;;
      --yes) assume_yes=true; shift ;;
      --dry-run) dry_run=true; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "ARGS" "Unknown option '$1'" ;;
    esac
  done
}

validate() {
  valid_tag "$tag" || die "TAG" "Invalid image tag '$tag'"
  [[ "$method" == docker || "$method" == buildx ]] || die "METHOD" "Method must be docker or buildx"
  [[ "$output" == load || "$output" == push ]] || die "OUTPUT" "Output must be load or push"
  [[ "$method" == buildx || "$output" == load ]] || die "OUTPUT" "--output push requires --method buildx"
  [[ "$method" == buildx && "$output" == load && "$platform" != native ]] && die "PLATFORM" "Multiple or explicit platforms require push output"
  if [[ "$platform" == *,* ]]; then
    [[ "$output" == push ]] || die "PLATFORM" "Multiple platforms require push output"
    IFS=',' read -ra platforms <<< "$platform"; (( ${#platforms[@]} > 1 )) || die "PLATFORM" "Multiple platforms require a comma-separated list"
    for item in "${platforms[@]}"; do valid_platform "$item" || die "PLATFORM" "Unsupported platform '$item'"; done
  else valid_platform "$platform" || die "PLATFORM" "Unsupported platform '$platform"; fi
  valid_registry "$repository" || die "REPOSITORY" "Invalid image repository '$repository'"
  selected_indices >/dev/null
}

image_ref() { local image; IFS='|' read -r _ _ image _ <<< "${SERVICES[$1]}"; echo "$repository/${image#*/}:$tag"; }
print_summary() {
  echo; echo "Build summary"; echo "Services:"; local index fields name
  while IFS= read -r index; do IFS='|' read -r _ name _ _ <<< "${SERVICES[$index]}"; echo "  - $name ($(image_ref "$index"))"; done < <(selected_indices)
  echo "Build method: $method"; [[ "$method" == buildx ]] && { echo "Output: $output"; echo "Platforms: $platform"; } || echo "Output: local Docker"; echo
}

build() {
  local index fields service_name dockerfile command
  while IFS= read -r index; do
    IFS='|' read -r _ service_name _ dockerfile <<< "${SERVICES[$index]}"
    if [[ "$method" == docker ]]; then
      command=(docker build -t "$(image_ref "$index")" -f "$dockerfile" .)
    else
      command=(docker buildx build "--$output" -t "$(image_ref "$index")" -f "$dockerfile" .)
      [[ "$platform" == native ]] || command=(docker buildx build --platform "$platform" "--$output" -t "$(image_ref "$index")" -f "$dockerfile" .)
    fi
    printf '+ '; printf '%q ' "${command[@]}"; echo
    if ! $dry_run && ! (cd "$REPO_ROOT" && "${command[@]}"); then
      echo "FAILED: $service_name ($(image_ref "$index"))" >&2
      return 1
    fi
  done < <(selected_indices)
}

trap cancel INT
parse_args "$@"
if ! $non_interactive; then wizard; fi
validate
print_summary
if ! $assume_yes; then printf 'Start building? [Y/n] '; IFS= read -r answer || cancel; [[ -z "$answer" || "$answer" =~ ^[Yy]$ ]] || { echo "Build cancelled."; exit 0; }; fi
echo "Starting build..."
if build; then echo "Build successful: all selected images completed."; else echo "Build failed." >&2; exit 1; fi
