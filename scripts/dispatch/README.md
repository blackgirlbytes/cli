# Dispatch automation

`release-summary-slack-v2.yml` is the canonical weekly Dispatch generator. Its
pipeline deliberately separates factual discovery from editorial writing:

1. `.github/dispatch-projects.json` defines every monitored repository, its
   product placement, and its release mode.
2. `dispatch_manifest.py collect` builds deterministic per-repository source
   manifests from GitHub releases or merged pull requests.
3. `dispatch_manifest.py split` creates one bounded source file per project, so
   Goose never has to process the full cross-project inventory in one pass.
4. Goose curates each project into a public-facing fragment and records an
   explicit ledger for intentionally excluded source items.
5. Every fragment is validated against its project manifest. If curation times
   out or misses an item, a deterministic fallback includes every release and
   source link instead of producing an incomplete artifact.
6. `dispatch_manifest.py assemble` builds the final Marvin-formatted draft and
   global validation requires every release and source item to be included or
   accounted for.

The resulting `dispatch-bundle` artifact contains:

- `dispatch-draft.md`
- `dispatch-manifest.json`
- `dispatch-exclusions.json`
- `dispatch-coverage.md`
- `dispatch-repository-audit.md`

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
