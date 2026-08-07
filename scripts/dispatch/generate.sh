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
    awk '/^## / { exit } { print }' "$example"
    echo
  done
} > /tmp/dispatch-style.md

rm -rf /tmp/project-sources /tmp/project-fragments
mkdir -p /tmp/project-sources /tmp/project-fragments
python3 scripts/dispatch/dispatch_manifest.py split \
  --manifest /tmp/dispatch-manifest.json \
  --output /tmp/project-sources

for key in $(jq -r '.[] | select(.empty) | .key' /tmp/project-sources/projects.json); do
  python3 scripts/dispatch/dispatch_manifest.py empty \
    --project-manifest "/tmp/project-sources/${key}.json" \
    --output-fragment "/tmp/project-fragments/${key}.md" \
    --output-exclusions "/tmp/project-fragments/${key}.exclusions.json"
done

for key in $(jq -r '.[] | select(.empty | not) | .key' /tmp/project-sources/projects.json); do
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

for key in $(jq -r '.[] | select(.empty | not) | .key' /tmp/project-sources/projects.json); do
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
  if [ ! -s "$fragment_path" ]; then
    valid=false
  else
    if ! jq -e 'type == "array" and all(.[]; (.id | type == "string") and (.reason | type == "string"))' "$exclusions_path" > /dev/null 2>&1; then
      echo "Model produced a fragment without valid exclusions for ${repo}; initializing the exclusion list for coverage repair."
      printf '%s\n' '[]' > "$exclusions_path"
    fi
    if ! python3 scripts/dispatch/dispatch_manifest.py validate \
      --manifest "$manifest_path" \
      --draft "$fragment_path" \
      --exclusions "$exclusions_path" \
      --output "$coverage_path"; then
      valid=false
      repairable=true
    fi
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
    timeout 4m goose run --recipe "$repair_recipe_path" > "$repair_log_path" 2>&1 || true
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

python3 scripts/dispatch/dispatch_manifest.py intro-source \
  --projects /tmp/project-sources/projects.json \
  --fragments /tmp/project-fragments \
  --output /tmp/dispatch-intro-source.md

cat > /tmp/dispatch-intro-recipe.yaml <<'INTRO_RECIPE_EOF'
version: "1.0.0"
title: "Write Dispatch Introduction"
description: "Write frontmatter highlights and a Marvin-style Dispatch introduction"

extensions:
  - type: builtin
    name: developer

instructions: |
  Write the introduction for an Entire Dispatch using only DISPATCH_STYLE and INTRO_SOURCE.
  DISPATCH_STYLE contains introductions from the four most recent published Dispatches. Use them only for voice and structure; never copy their facts, links, embeds, or sentences.
  INTRO_SOURCE contains a small set of highlights from the current Dispatch. Do not inspect any other source.

  Write DESCRIPTION_FILE as one concise sentence naming the strongest reader-facing highlights, similar to the published frontmatter descriptions. Do not merely list product names.

  Write INTRO_FILE as Markdown with no frontmatter or headings. It must:
  - begin exactly with `Beep, boop. Marvin here.`
  - open with a dry Marvin observation tied to the strongest current highlight
  - use two or three short paragraphs that cohesively mention the major current highlights across active products
  - never mention projects with no releases or merged pull requests
  - never frame repositories or teams as competing, leading, quiet, or failing to ship
  - end exactly with `For humans and agents, details can be read (or scraped) below:`
  - contain no recap, closing, HTML, JSX, image, or video embed

  Verify both files, then respond only with DONE.

prompt: |
  Read DISPATCH_STYLE and INTRO_SOURCE, write DESCRIPTION_FILE and INTRO_FILE, verify them, then respond only with DONE.
INTRO_RECIPE_EOF

sed -i \
  -e 's|DISPATCH_STYLE|/tmp/dispatch-style.md|g' \
  -e 's|INTRO_SOURCE|/tmp/dispatch-intro-source.md|g' \
  -e 's|DESCRIPTION_FILE|/tmp/dispatch-description.txt|g' \
  -e 's|INTRO_FILE|/tmp/dispatch-intro.md|g' \
  /tmp/dispatch-intro-recipe.yaml

rm -f /tmp/dispatch-description.txt /tmp/dispatch-intro.md
timeout 2m goose run --recipe /tmp/dispatch-intro-recipe.yaml > /tmp/dispatch-intro.log 2>&1 || true
cat /tmp/dispatch-intro.log

if [ ! -s /tmp/dispatch-description.txt ] \
  || [ ! -s /tmp/dispatch-intro.md ] \
  || ! grep -q '^Beep, boop\. Marvin here\.' /tmp/dispatch-intro.md \
  || [ "$(tail -1 /tmp/dispatch-intro.md)" != 'For humans and agents, details can be read (or scraped) below:' ] \
  || grep -Eq '^#|<(div|figure|iframe)' /tmp/dispatch-intro.md; then
  echo "::warning::Discarding an introduction that does not follow the published Dispatch opening."
  rm -f /tmp/dispatch-description.txt /tmp/dispatch-intro.md
fi

python3 scripts/dispatch/dispatch_manifest.py assemble \
  --manifest /tmp/dispatch-manifest.json \
  --previous /tmp/previous-dispatch.md \
  --projects /tmp/project-sources/projects.json \
  --fragments /tmp/project-fragments \
  --description /tmp/dispatch-description.txt \
  --intro /tmp/dispatch-intro.md \
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
