# Dispatch automation

`release-summary-slack-v2.yml` is the canonical weekly Dispatch generator. Its
pipeline deliberately separates factual discovery from editorial writing:

1. `.github/dispatch-projects.json` defines every monitored repository, its
   product placement, and its release mode.
2. `dispatch_manifest.py collect` builds deterministic per-repository source
   manifests from GitHub releases or merged pull requests.
3. Goose rewrites that inventory into the public Dispatch and returns an
   explicit ledger for intentionally excluded source items.
4. `dispatch_manifest.py validate` requires every release and every source item
   to be included or accounted for. One automatic repair pass runs when the
   first draft is incomplete.

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
