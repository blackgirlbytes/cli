# Dispatch automation

`release-summary-slack-v2.yml` is the canonical weekly Dispatch generator. Its
pipeline deliberately separates factual discovery from editorial writing:

1. `.github/dispatch-projects.json` defines every monitored repository, its
   product placement, and its release mode.
2. `dispatch_manifest.py collect` builds deterministic per-repository source
   manifests from GitHub releases or merged pull requests.
3. `dispatch_manifest.py split` creates one bounded source file per project, so
   Goose never has to process the full cross-project inventory in one pass.
   Release-note paragraphs shared by several PR links are grouped into one
   editorial item while retaining every source URL for validation.
   PostHog data is reduced to flag identity, lifecycle state, and rollout
   percentages so the Entire Web inventory stays below model tool-output limits.
   Configured OSS projects with `show_when_empty` retain their headline and get
   a deterministic no-releases or no-merged-PRs notice without invoking Goose.
4. Goose curates the non-empty projects in parallel into public-facing
   fragments and records an explicit ledger for intentionally excluded source
   items. Each project has its own bounded execution window.
5. Every fragment is validated for source coverage and editorial shape. If
   curation times out, misses an item, uses a generic fallback heading, or
   produces more than 30 top-level bullets, a targeted repair pass sees the
   coverage and quality ledger and patches the fragment. If repair still
   fails, a deterministic fallback preserves every release and source link in
   the diagnostic artifact, the job fails, and the draft is not posted to
   Slack as if it were finished.
6. `dispatch_manifest.py assemble` builds the final Marvin-formatted draft and
   global validation requires every release and source item to be included or
   accounted for.

The assembler also adds published-style thank-you lines after bullets backed by
external contributors. Contributor status follows the earlier Dispatch rule:
accounts are internal when they are visible members of `entireio` or
`entirehq`, or expose an `@entire.io` email; human go-git contributors are
always treated as community contributors. A bounded intro-only pass uses the
four latest published introductions for voice and structure, with a
deterministic Marvin-formatted fallback.

The resulting `dispatch-bundle` artifact contains:

- `dispatch-draft.md`
- `dispatch-manifest.json`
- `dispatch-exclusions.json`
- `dispatch-coverage.md`
- `dispatch-repository-audit.md`
- `dispatch-generation-status.txt`

Manual runs upload the bundle to Slack by default. Clear `post_to_slack` when
testing artifact generation without posting another draft. Scheduled runs
upload the validated draft. Runs that require a deterministic project fallback
retain a diagnostic artifact in Actions but do not post it to Slack.

## Adding a project

Add one entry to `.github/dispatch-projects.json` and one manual-dispatch choice
to `release-summary-slack-v2.yml`.

Use `github-releases` when GitHub stable/prerelease objects define what shipped.
Use `public-merges` for continuously deployed or launch-oriented repositories;
the writing pass will include public, user-visible work and record explicit
reasons for excluded internal work.

Run the focused tests with:

```bash
mise run test:dispatch
```
