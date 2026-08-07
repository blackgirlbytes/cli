#!/bin/sh
set -eu

: "${SINCE:?SINCE is required}"
: "${CURRENT_DATE:?CURRENT_DATE is required}"

python3 scripts/dispatch/dispatch_manifest.py combine \
  --collections /tmp/collections \
  --previous /tmp/previous-dispatch.md \
  --posthog /tmp/posthog_flags.json \
  --output-manifest /tmp/dispatch-manifest.json \
  --output-source /tmp/dispatch-source.md

{
  for example in /tmp/dispatch-examples/*; do
    echo "# Style reference: $(basename "$example")"
    cat "$example"
    echo
  done
} > /tmp/dispatch-style.md

rm -rf /tmp/project-sources /tmp/project-fragments
mkdir -p /tmp/project-sources /tmp/project-fragments
python3 scripts/dispatch/dispatch_manifest.py split \
  --manifest /tmp/dispatch-manifest.json \
  --output /tmp/project-sources

for key in $(jq -r '.[].key' /tmp/project-sources/projects.json); do
  repo=$(jq -r --arg key "$key" '.[] | select(.key == $key) | .repo' /tmp/project-sources/projects.json)
  area=$(jq -r --arg key "$key" '.[] | select(.key == $key) | .area' /tmp/project-sources/projects.json)
  product=$(jq -r --arg key "$key" '.[] | select(.key == $key) | .product' /tmp/project-sources/projects.json)
  source_path="/tmp/project-sources/${key}.md"
  fragment_path="/tmp/project-fragments/${key}.md"
  exclusions_path="/tmp/project-fragments/${key}.exclusions.json"
  recipe_path="/tmp/project-${key}-recipe.yaml"
  log_path="/tmp/project-${key}.log"
  pid_path="/tmp/project-${key}.pid"

  cat > "$recipe_path" <<'RECIPE_EOF'
version: "1.0.0"
title: "Curate Dispatch Project"
description: "Turn one project manifest into a polished dispatch fragment"

extensions:
  - type: builtin
    name: developer

instructions: |
  Curate the PROJECT_REPO section of an Entire Dispatch.
  Read PROJECT_SOURCE once and do not inspect GitHub, git, curl, or any other source.
  Write the finished files directly with the developer extension. Do not return their contents through chat or a final-output tool.

  Requirements:
  - Write PROJECT_FRAGMENT as Markdown beginning with exactly `### PROJECT_PRODUCT`.
  - Do not add a `##` platform heading; the assembler adds `## PROJECT_AREA`.
  - Use benefit-oriented `####` theme headings and concise user-facing bullets.
  - Mention and link every stable and nightly release in the source.
  - Include every meaningful change with its exact PR, commit, or release link.
  - Write PROJECT_EXCLUSIONS as a JSON array containing every omitted source id and one short reason: dependency-only, internal-only, feature-flagged, duplicate, no user-visible effect, or maintenance-only.
  - Never exclude a release id.
  - Verify both files, then respond only with DONE.

prompt: |
  Create PROJECT_FRAGMENT and PROJECT_EXCLUSIONS for PROJECT_REPO now, verify them, then respond only with DONE.
RECIPE_EOF

  sed -i \
    -e "s|PROJECT_REPO|${repo}|g" \
    -e "s|PROJECT_SOURCE|${source_path}|g" \
    -e "s|PROJECT_FRAGMENT|${fragment_path}|g" \
    -e "s|PROJECT_EXCLUSIONS|${exclusions_path}|g" \
    -e "s|PROJECT_PRODUCT|${product}|g" \
    -e "s|PROJECT_AREA|${area}|g" \
    "$recipe_path"

  rm -f "$fragment_path" "$exclusions_path"
  echo "Starting bounded curation for ${repo}."
  timeout 8m goose run --recipe "$recipe_path" > "$log_path" 2>&1 &
  model_pid=$!
  printf '%s\n' "$model_pid" > "$pid_path"
done

for key in $(jq -r '.[].key' /tmp/project-sources/projects.json); do
  repo=$(jq -r --arg key "$key" '.[] | select(.key == $key) | .repo' /tmp/project-sources/projects.json)
  manifest_path="/tmp/project-sources/${key}.json"
  fragment_path="/tmp/project-fragments/${key}.md"
  exclusions_path="/tmp/project-fragments/${key}.exclusions.json"
  coverage_path="/tmp/project-fragments/${key}.coverage.md"
  log_path="/tmp/project-${key}.log"
  pid_path="/tmp/project-${key}.pid"

  model_pid=$(cat "$pid_path")
  wait "$model_pid" || true
  cat "$log_path"

  valid=true
  repairable=false
  if [ ! -s "$fragment_path" ] || ! jq -e 'type == "array" and all(.[]; (.id | type == "string") and (.reason | type == "string"))' "$exclusions_path" > /dev/null 2>&1; then
    valid=false
  elif ! python3 scripts/dispatch/dispatch_manifest.py validate \
    --manifest "$manifest_path" \
    --draft "$fragment_path" \
    --exclusions "$exclusions_path" \
    --output "$coverage_path"; then
    valid=false
    repairable=true
  fi

  if [ "$repairable" = true ]; then
    repair_recipe_path="/tmp/project-${key}-repair-recipe.yaml"
    repair_log_path="/tmp/project-${key}-repair.log"
    cat > "$repair_recipe_path" <<'REPAIR_RECIPE_EOF'
version: "1.0.0"
title: "Repair Dispatch Project Coverage"
description: "Account for missing source links in a curated project fragment"

extensions:
  - type: builtin
    name: developer

instructions: |
  Repair the coverage of an already-curated Entire Dispatch project.
  Read REPAIR_COVERAGE, REPAIR_FRAGMENT, and REPAIR_EXCLUSIONS only.
  Update REPAIR_FRAGMENT and REPAIR_EXCLUSIONS directly with the developer extension.

  Requirements:
  - Preserve the existing polished themes and wording.
  - For every item under `## Missing` in REPAIR_COVERAGE, either add its exact link to the appropriate theme in REPAIR_FRAGMENT or add its exact id and one allowed reason to REPAIR_EXCLUSIONS.
  - Allowed reasons: dependency-only, internal-only, feature-flagged, duplicate, no user-visible effect, or maintenance-only.
  - Never exclude a release id.
  - Keep REPAIR_EXCLUSIONS a valid JSON array and do not remove existing entries.
  - Verify both files, then respond only with DONE.

prompt: |
  Account for every missing item now, verify both files, then respond only with DONE.
REPAIR_RECIPE_EOF

    sed -i \
      -e "s|REPAIR_COVERAGE|${coverage_path}|g" \
      -e "s|REPAIR_FRAGMENT|${fragment_path}|g" \
      -e "s|REPAIR_EXCLUSIONS|${exclusions_path}|g" \
      "$repair_recipe_path"

    echo "Repairing incomplete coverage for ${repo}."
    timeout 2m goose run --recipe "$repair_recipe_path" > "$repair_log_path" 2>&1 || true
    cat "$repair_log_path"
    if [ -s "$fragment_path" ] \
      && jq -e 'type == "array" and all(.[]; (.id | type == "string") and (.reason | type == "string"))' "$exclusions_path" > /dev/null 2>&1 \
      && python3 scripts/dispatch/dispatch_manifest.py validate \
        --manifest "$manifest_path" \
        --draft "$fragment_path" \
        --exclusions "$exclusions_path" \
        --output "$coverage_path"; then
      valid=true
    fi
  fi

  if [ "$valid" != true ]; then
    echo "::warning::Using deterministic fallback for ${repo}; model curation did not cover its complete manifest."
    python3 scripts/dispatch/dispatch_manifest.py fallback \
      --project-manifest "$manifest_path" \
      --output-fragment "$fragment_path" \
      --output-exclusions "$exclusions_path"
  fi
done

python3 scripts/dispatch/dispatch_manifest.py assemble \
  --manifest /tmp/dispatch-manifest.json \
  --previous /tmp/previous-dispatch.md \
  --projects /tmp/project-sources/projects.json \
  --fragments /tmp/project-fragments \
  --output-draft /tmp/dispatch-draft.md \
  --output-exclusions /tmp/dispatch-exclusions.json

python3 scripts/dispatch/dispatch_manifest.py validate \
  --manifest /tmp/dispatch-manifest.json \
  --draft /tmp/dispatch-draft.md \
  --exclusions /tmp/dispatch-exclusions.json \
  --output /tmp/dispatch-coverage.md

mkdir -p /tmp/dispatch-bundle
cp /tmp/dispatch-draft.md /tmp/dispatch-manifest.json \
  /tmp/dispatch-exclusions.json /tmp/dispatch-coverage.md \
  /tmp/dispatch-repository-audit.md \
  /tmp/dispatch-bundle/
