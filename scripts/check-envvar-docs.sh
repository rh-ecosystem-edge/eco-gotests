#!/usr/bin/env bash
#
# Verify that every ECO_* environment variable declared in Go source code
# is mentioned in the corresponding README.md, and vice versa.
#
# Exit codes:
#   0  all vars documented and no stale references
#   1  undocumented or stale vars found

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TESTS_DIR="$REPO_ROOT/tests"
errors=0

declare -A GLOBAL_VARS
while IFS= read -r v; do
    [[ -n "$v" ]] && GLOBAL_VARS["$v"]=1
done < <({
    grep -ohP 'ECO_[A-Z0-9_]+' "$REPO_ROOT/scripts/test-runner.sh" 2>/dev/null || true
    grep -ohP 'envconfig:"ECO_[A-Z0-9_]+"' "$TESTS_DIR/internal/config/config.go" 2>/dev/null \
        | sed 's/envconfig:"//;s/"//' || true
} | sort -u)

readme_for_config() {
    local config_file="$1"
    echo "${config_file%/internal/*}/README.md"
}

vars_from_code() {
    local file="$1"
    {
        grep -ohP 'envconfig:"ECO_[A-Z0-9_]+"' "$file" 2>/dev/null \
            | sed 's/envconfig:"//;s/"//' || true
        grep -ohP 'os\.Getenv\("ECO_[A-Z0-9_]+"\)' "$file" 2>/dev/null \
            | sed 's/os\.Getenv("//;s/")//' || true
    } | sort -u
}

vars_from_readme() {
    local file="$1"
    if [[ ! -f "$file" ]]; then
        return
    fi
    grep -ohP 'ECO_[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+(?![_*A-Z0-9])' "$file" | sort -u
}

config_files=$(grep -rl 'envconfig:"ECO_\|os\.Getenv("ECO_' "$TESTS_DIR" --include="*.go" \
    | grep -E '(config|env).*\.go$' \
    | sort -u)

declare -A readme_code_vars

for config_file in $config_files; do
    readme=$(readme_for_config "$config_file")

    code_vars=$(vars_from_code "$config_file")
    if [[ -n "${readme_code_vars[$readme]:-}" ]]; then
        readme_code_vars["$readme"]="${readme_code_vars[$readme]}"$'\n'"$code_vars"
    else
        readme_code_vars["$readme"]="$code_vars"
    fi
done

for readme in "${!readme_code_vars[@]}"; do
    rel_readme="${readme#$REPO_ROOT/}"

    declare -A code_set=()
    while IFS= read -r var; do
        [[ -n "$var" ]] && code_set["$var"]=1
    done < <(echo "${readme_code_vars[$readme]}" | sort -u)

    declare -A doc_set=()
    while IFS= read -r var; do
        [[ -n "$var" ]] && doc_set["$var"]=1
    done < <(vars_from_readme "$readme")

    if [[ ! -f "$readme" ]]; then
        echo "ERROR: $rel_readme does not exist (${#code_set[@]} vars undocumented)"
        errors=1
        unset code_set doc_set
        continue
    fi

    for var in "${!code_set[@]}"; do
        if [[ -z "${doc_set[$var]:-}" ]]; then
            echo "UNDOCUMENTED: $var not in $rel_readme"
            errors=1
        fi
    done

    for var in "${!doc_set[@]}"; do
        if [[ -z "${code_set[$var]:-}" && -z "${GLOBAL_VARS[$var]:-}" ]]; then
            echo "STALE: $var in $rel_readme but not in code"
            errors=1
        fi
    done

    unset code_set doc_set
done

if [[ "$errors" -eq 0 ]]; then
    echo "OK: all ECO_* environment variables are documented."
fi

exit "$errors"
