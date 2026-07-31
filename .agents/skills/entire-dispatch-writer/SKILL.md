---
name: entire-dispatch-writer
description: Use when researching, drafting, editing, or publishing an Entire Dispatch from recent work across Entire products and open-source projects.
---

# Entire Dispatch Writer

Create an accurate, approachable Entire Dispatch that explains what changed, why it matters, and who contributed without exposing private work or turning the introduction into a changelog.

## Source Authority

1. Treat the user's latest approved copy and corrections as authoritative.
2. Use recent dispatches for structure and length, not as permission to revive discarded tone.
3. Research facts from primary sources: merged pull requests, releases, commits, repository files, and Entire sessions or checkpoints.
4. When an automated workflow supplies a factual source brief, treat that brief as the only factual source. Use published dispatch examples only for style and structure.
5. Never invent releases, benefits, visibility, links, or contributor attribution.

## Coverage and Privacy

Establish the window from the latest published dispatch through the current date. Check every relevant public project, including newly active repositories. The usual set is:

- Entire CLI
- entire.io
- EntireDB
- external agent integrations
- go-git
- go-nuts
- git-sync
- ForgeMark

Record why an inspected project was omitted: no notable activity, private work, duplication, or an explicit user exclusion.

Confirm that the work itself is public before mentioning it. Repository visibility is not the deciding factor: publicly shipped Entire.io or EntireDB behavior may come from a private source repository. Exclude private products, internal-only features, feature-flagged work that is not public, and unreleased work that the dispatch should not announce. Do not include private Entire CI or Trails work unless the source explicitly establishes that it has become public. Maintain a per-dispatch exclusion list instead of assuming any other old exclusion is permanent.

## Contributor Credits

Audit contributor attribution before drafting. The merged pull request author may not be the original contributor. Check, when the source data is available:

- organization membership and `author_association`
- pull-request descriptions
- commit authors
- carried, superseded, or maintainer-owned pull requests
- preserved authorship from an earlier pull request

Do not thank organization members, employees, or bots as external contributors. Credit an external contributor directly beneath the relevant bullet:

```markdown
  - Thank you for your contribution, [@handle](https://github.com/handle)!
```

For multiple contributors on one bullet:

```markdown
  - Thank you for your contributions, [@first](https://github.com/first) and [@second](https://github.com/second)!
```

If contributor metadata is incomplete, flag the credit for review rather than guessing.

## Introduction

Read at least the three latest dispatch introductions before drafting. Use this shape:

```markdown
Beep, boop. Marvin here. [One sardonic sentence tied to a real headline.]

[One compact paragraph explaining the main change and its benefit, followed by only the most useful supporting highlights.]

For humans and agents, details can be read (or scraped) below:
```

Rules:

- The sardonic line is one sentence after “Marvin here.”
- Tie it to something that actually shipped.
- Keep it dry, understated, specific, and not corny.
- Do not joke about “the humans.”
- Never imply that little or nothing shipped.
- Do not turn the introduction into a catalog of every project.
- Keep approved sentences unchanged unless the user asks to revisit them.
- When tone is unresolved, offer genuinely different one-line angles before editing. Do not repeat the same joke with minor wording changes.

## Change Bullets

Every bullet must explain both what changed and how it helps. A useful default construction is:

```markdown
- [Project or feature] now [concrete change], so you can [practical benefit] ([PR \#123](https://github.com/org/repo/pull/123)).
```

Vary the sentence naturally; do not force every bullet to say “so you can.” Use “you” when helpful instead of repeatedly saying “users.”

Translate implementation details into observable effects such as:

- fewer manual steps
- clearer failures
- faster navigation
- safer credentials
- more reliable checkpoints
- reduced memory or resource use
- better compatibility
- easier automation

Avoid vague claims, unexplained internal names, raw pull-request titles, and benefits unsupported by the change. Combine related pull requests when they produce one reader-facing outcome, but do not collapse distinct changes into a vague summary.

Describe work on external projects as work by that project or its contributors, not as work owned by the Entire team.

## Structure and Markdown

Match the newest published dispatch's heading hierarchy. The usual structure is:

```markdown
## **CLI**

### **Entire CLI**

#### **Feature Theme**

- Change and benefit.

## **Web**

### **Entire.io**

### **EntireDB**

## **OSS Projects**

### **Project Name**
```

Place external agent integrations with the Entire CLI unless the current dispatch establishes a separate product section. Place go-git, go-nuts, git-sync, and ForgeMark under OSS Projects. Omit empty products and sections.

Formatting rules:

- Backtick commands, flags, configuration keys, paths, refs, APIs, and code identifiers.
- Leave product and agent names as ordinary text.
- Link every referenced pull request and release.
- Use real Markdown headings.
- Remove internal checklists, drafting notes, and media placeholders.
- Preserve the current frontmatter and MDX conventions.
- Distinguish stable CLI releases from nightly-only work.

## Closing

Keep the closing short:

```markdown
That's the dispatch. As always, bring questions, bugs, PRs, and constructive dread to [Discord](https://discord.com/invite/jZJs3Tue4S) or [GitHub issues](https://github.com/entireio/cli/issues).

Boop.
```

Do not add another full recap before the closing.

## Publishing

Current dispatch posts live in:

```text
website/src/routes/_content/blog/-content/
```

Use the established `YYYY-MM-DD-entire-dispatch-0xNNNN.mdx` filename and frontmatter fields: `title`, `description`, `category: Dispatch`, and `author: Marvin`.

When publishing in the entire.io repository, validate with:

```bash
pnpm --filter entire-website run lint
pnpm run website:build
pnpm run format:check
```

Preserve unrelated worktree changes. Do not commit, push, or open a pull request unless asked.

## Final Review

Before handing off the draft, verify:

- dispatch number, date, versions, and links
- coverage of every relevant public project
- requested and private exclusions are absent
- every bullet contains a concrete change and benefit
- stable and nightly work are labeled correctly
- external contributors are credited
- the introduction is compact and product-forward
- the Marvin line is one sardonic, non-corny sentence
- no internal notes or placeholders remain
- `Boop.` closes the post

If visibility, attribution, or the dispatch window cannot be determined safely, flag it for review instead of guessing.
