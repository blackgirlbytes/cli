#!/usr/bin/env python3
"""Build and validate deterministic source manifests for Entire Dispatch."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any


PR_URL_RE = re.compile(r"https://github\.com/([^/]+/[^/]+)/pull/(\d+)")
COMPARE_RE = re.compile(r"github\.com/[^/]+/[^/]+/compare/([^\s)]+)\.\.\.([^\s)]+)")
CHANGES_SINCE_RE = re.compile(r"Changes since\s+([^:\s]+)", re.IGNORECASE)
COMMIT_LINE_RE = re.compile(r"^\s*([0-9a-f]{7,40})\s+(.+?)\s*$", re.IGNORECASE)


def read_json(path: Path) -> Any:
    return json.loads(path.read_text())


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")


def run_json(command: list[str], cwd: Path | None = None) -> Any:
    output = subprocess.check_output(command, cwd=cwd, text=True)
    return json.loads(output)


def parse_date(value: str) -> dt.date:
    return dt.date.fromisoformat(value[:10])


def in_window(value: str | None, since: dt.date, until: dt.date) -> bool:
    return bool(value) and since <= parse_date(value) <= until


def project_for(config: dict[str, Any], repo: str) -> dict[str, Any]:
    for project in config["repositories"]:
        if project["repo"] == repo:
            return project
    raise SystemExit(f"repository is not configured: {repo}")


def matrix(config: dict[str, Any], selection: str) -> dict[str, Any]:
    projects = config["repositories"]
    if selection != "all":
        projects = [project_for(config, selection)]
    return {
        "include": [
            {
                "repo": project["repo"],
                "name": project["name"],
                "mode": project["mode"],
                "feature_flags": project["feature_flags"],
            }
            for project in projects
        ]
    }


def merged_prs(repo: str, since: str, until: str) -> list[dict[str, Any]]:
    return run_json(
        [
            "gh",
            "pr",
            "list",
            "--repo",
            repo,
            "--state",
            "merged",
            "--limit",
            "1000",
            "--search",
            f"merged:>={since} merged:<={until}",
            "--json",
            "number,title,body,author,mergedAt,url,labels",
        ]
    )


def clean_release_title(line: str, url: str) -> str:
    title = line.replace(url, "").strip()
    title = re.sub(r"^[-*+]+\s*", "", title)
    title = re.sub(r"\s+by\s+@[^\s]+\s+in\s*$", "", title, flags=re.IGNORECASE)
    return title.strip() or f"Pull request {url.rsplit('/', 1)[-1]}"


def release_body_changes(repo: str, release: dict[str, Any]) -> list[dict[str, Any]]:
    body = release.get("body") or ""
    changes: list[dict[str, Any]] = []
    seen: set[str] = set()
    for line in body.splitlines():
        for match in PR_URL_RE.finditer(line):
            matched_repo, number = match.groups()
            if matched_repo.lower() != repo.lower():
                continue
            url = match.group(0)
            item_id = f"{repo}#{number}"
            if item_id in seen:
                continue
            seen.add(item_id)
            changes.append(
                {
                    "id": item_id,
                    "kind": "pull_request",
                    "number": int(number),
                    "title": clean_release_title(line, url),
                    "url": url,
                }
            )

    for line in body.splitlines():
        match = COMMIT_LINE_RE.match(line)
        if not match:
            continue
        commit, title = match.groups()
        url = f"https://github.com/{repo}/commit/{commit}"
        item_id = f"{repo}@{commit}"
        if item_id in seen:
            continue
        seen.add(item_id)
        changes.append(
            {
                "id": item_id,
                "kind": "commit",
                "commit": commit,
                "title": title,
                "url": url,
            }
        )
    return changes


def previous_tag(release: dict[str, Any]) -> str | None:
    body = release.get("body") or ""
    match = COMPARE_RE.search(body)
    if match:
        return match.group(1)
    match = CHANGES_SINCE_RE.search(body)
    return match.group(1) if match else None


def git_range_changes(repo: str, checkout: Path, release: dict[str, Any]) -> list[dict[str, Any]]:
    previous = previous_tag(release)
    tag = release["tag_name"]
    if not previous:
        return []
    try:
        output = subprocess.check_output(
            ["git", "log", "--format=%H%x09%s%x09%b%x00", f"{previous}..{tag}"],
            cwd=checkout,
            text=True,
            stderr=subprocess.DEVNULL,
        )
    except subprocess.CalledProcessError:
        return []

    changes: list[dict[str, Any]] = []
    seen: set[str] = set()
    for record in output.split("\x00"):
        if not record.strip():
            continue
        commit, title, *body = record.strip().split("\t", 2)
        numbers = re.findall(r"#(\d+)", "\t".join(body) + " " + title)
        if numbers:
            for number in numbers:
                item_id = f"{repo}#{number}"
                if item_id in seen:
                    continue
                seen.add(item_id)
                changes.append(
                    {
                        "id": item_id,
                        "kind": "pull_request",
                        "number": int(number),
                        "title": title,
                        "url": f"https://github.com/{repo}/pull/{number}",
                    }
                )
            continue
        item_id = f"{repo}@{commit[:12]}"
        changes.append(
            {
                "id": item_id,
                "kind": "commit",
                "commit": commit,
                "title": title,
                "url": f"https://github.com/{repo}/commit/{commit}",
            }
        )
    return changes


def resolve_commit_pull_requests(repo: str, changes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    resolved: list[dict[str, Any]] = []
    seen: set[str] = set()
    for change in changes:
        if change["kind"] != "commit":
            if change["id"] not in seen:
                seen.add(change["id"])
                resolved.append(change)
            continue
        try:
            pulls = run_json(["gh", "api", f"repos/{repo}/commits/{change['commit']}/pulls"])
        except subprocess.CalledProcessError:
            pulls = []
        merged = next((pull for pull in pulls if pull.get("merged_at")), None)
        if merged:
            item_id = f"{repo}#{merged['number']}"
            replacement = {
                "id": item_id,
                "kind": "pull_request",
                "number": merged["number"],
                "title": merged["title"],
                "url": merged["html_url"],
            }
        else:
            item_id = change["id"]
            replacement = change
        if item_id not in seen:
            seen.add(item_id)
            resolved.append(replacement)
    return resolved


def collect(config: dict[str, Any], repo: str, checkout: Path, since: str, until: str) -> dict[str, Any]:
    project = project_for(config, repo)
    result: dict[str, Any] = {
        "project": project,
        "window": {"since": since, "until": until},
        "releases": [],
        "candidates": [],
    }
    if project["mode"] == "github-releases":
        releases = run_json(["gh", "api", f"repos/{repo}/releases?per_page=100"])
        start, end = parse_date(since), parse_date(until)
        for release in sorted(
            (item for item in releases if not item.get("draft") and in_window(item.get("published_at"), start, end)),
            key=lambda item: item["published_at"],
        ):
            changes = release_body_changes(repo, release)
            if not changes:
                changes = git_range_changes(repo, checkout, release)
            changes = resolve_commit_pull_requests(repo, changes)
            result["releases"].append(
                {
                    "id": f"{repo}@{release['tag_name']}",
                    "tag": release["tag_name"],
                    "name": release.get("name") or release["tag_name"],
                    "url": release["html_url"],
                    "published_at": release["published_at"],
                    "channel": "nightly" if release.get("prerelease") else "stable",
                    "previous_tag": previous_tag(release),
                    "changes": changes,
                }
            )
    else:
        for pr in merged_prs(repo, since, until):
            author = pr.get("author") or {}
            result["candidates"].append(
                {
                    "id": f"{repo}#{pr['number']}",
                    "kind": "pull_request",
                    "number": pr["number"],
                    "title": pr["title"],
                    "body": (pr.get("body") or "")[:1200],
                    "url": pr["url"],
                    "merged_at": pr.get("mergedAt"),
                    "author": author.get("login", "unknown"),
                    "labels": [label["name"] for label in pr.get("labels", [])],
                }
            )

    flags = checkout / "feature-flags.json"
    if project.get("feature_flags") and flags.exists():
        result["feature_flag_definitions"] = read_json(flags)
    return result


def all_manifest_files(collections: Path) -> list[Path]:
    return sorted(collections.rglob("manifest.json"))


def filter_previously_published(manifest: dict[str, Any], previous: str) -> None:
    for repo in manifest["repositories"]:
        kept_releases = []
        for release in repo["releases"]:
            if release["url"] in previous or release["tag"] in previous:
                continue
            release["changes"] = [change for change in release["changes"] if change["url"] not in previous]
            kept_releases.append(release)
        repo["releases"] = kept_releases
        repo["candidates"] = [candidate for candidate in repo["candidates"] if candidate["url"] not in previous]


def compact_posthog_flags(value: Any) -> list[dict[str, Any]]:
    flags = value.get("results", []) if isinstance(value, dict) else value
    if not isinstance(flags, list):
        return []
    compact: list[dict[str, Any]] = []
    for flag in flags:
        if not isinstance(flag, dict):
            continue
        filters = flag.get("filters") or {}
        groups = filters.get("groups") or []
        multivariate = filters.get("multivariate") or {}
        variants = multivariate.get("variants") or []
        compact.append(
            {
                "key": flag.get("key"),
                "name": flag.get("name"),
                "active": flag.get("active"),
                "status": flag.get("status"),
                "archived": flag.get("archived"),
                "deleted": flag.get("deleted"),
                "rollout_percentages": sorted(
                    {
                        group["rollout_percentage"]
                        for group in groups
                        if isinstance(group, dict) and group.get("rollout_percentage") is not None
                    }
                ),
                "variants": [
                    {
                        "key": variant.get("key"),
                        "rollout_percentage": variant.get("rollout_percentage"),
                    }
                    for variant in variants
                    if isinstance(variant, dict)
                ],
            }
        )
    return compact


def render_source(manifest: dict[str, Any]) -> str:
    lines = [
        "# Deterministic Dispatch source manifest",
        "",
        f"Window: {manifest['window']['since']} through {manifest['window']['until']}",
        "",
        "Every release below must be named and linked in the draft. Every change or candidate must be included with its source link or returned in the structured exclusion list with a concrete reason.",
        "",
    ]
    for repo in manifest["repositories"]:
        project = repo["project"]
        lines.extend(
            [
                f"## {project['repo']}",
                "",
                f"Placement: `## {project['area']}` / `### {project['product']}`",
                f"Release mode: `{project['mode']}`",
                "",
            ]
        )
        for release in repo["releases"]:
            lines.extend(
                [
                    f"### {release['channel'].title()} release: [{release['tag']}]({release['url']})",
                    f"Published: {release['published_at']}",
                    "",
                ]
            )
            if not release["changes"]:
                lines.append("- Release published without individually linked changes; summarize the release itself.")
            for change in release["changes"]:
                lines.append(f"- `{change['id']}` [{change['title']}]({change['url']})")
            lines.append("")
        if repo["candidates"]:
            lines.extend(["### Public-merge candidates", ""])
            for candidate in repo["candidates"]:
                labels = ", ".join(candidate.get("labels", [])) or "none"
                lines.append(
                    f"- `{candidate['id']}` [{candidate['title']}]({candidate['url']}) "
                    f"by @{candidate.get('author', 'unknown')}; labels: {labels}"
                )
                if candidate.get("body"):
                    summary = " ".join(candidate["body"].split())[:180]
                    lines.append(f"  PR context: {summary}")
            lines.append("")
        if repo.get("feature_flag_definitions"):
            lines.extend(
                [
                    "### Repository feature-flag definitions",
                    "",
                    "```json",
                    json.dumps(repo["feature_flag_definitions"], separators=(",", ":")),
                    "```",
                    "",
                ]
            )
        if not repo["releases"] and not repo["candidates"]:
            lines.extend(["- No eligible source items in this window.", ""])
    if manifest.get("posthog_flags"):
        lines.extend(
            [
                "## Current PostHog feature-flag state",
                "",
                "Use this only to exclude Entire.io changes that are disabled, internal-only, or partially rolled out.",
                "",
                "```json",
                json.dumps(compact_posthog_flags(manifest["posthog_flags"]), separators=(",", ":")),
                "```",
                "",
            ]
        )
    return "\n".join(lines)


def combine(collections: Path, previous_path: Path, posthog_path: Path | None) -> dict[str, Any]:
    repositories = [read_json(path) for path in all_manifest_files(collections)]
    repositories.sort(key=lambda item: (item["project"]["area"], item["project"]["product"], item["project"]["repo"]))
    if not repositories:
        raise SystemExit(f"no repository manifests found under {collections}")
    manifest: dict[str, Any] = {
        "window": repositories[0]["window"],
        "repositories": repositories,
    }
    if posthog_path and posthog_path.exists():
        manifest["posthog_flags"] = read_json(posthog_path)
    filter_previously_published(manifest, previous_path.read_text())
    return manifest


def split_projects(manifest: dict[str, Any], output: Path) -> list[dict[str, str]]:
    output.mkdir(parents=True, exist_ok=True)
    projects: list[dict[str, str]] = []
    for index, repository in enumerate(manifest["repositories"]):
        if not repository["releases"] and not repository["candidates"]:
            continue
        key = f"{index:02d}"
        project_manifest: dict[str, Any] = {
            "window": manifest["window"],
            "repositories": [repository],
        }
        if repository["project"].get("feature_flags") and manifest.get("posthog_flags"):
            project_manifest["posthog_flags"] = manifest["posthog_flags"]
        write_json(output / f"{key}.json", project_manifest)
        (output / f"{key}.md").write_text(render_source(project_manifest))
        project = repository["project"]
        projects.append(
            {
                "key": key,
                "repo": project["repo"],
                "area": project["area"],
                "product": project["product"],
            }
        )
    write_json(output / "projects.json", projects)
    return projects


def fallback_fragment(project_manifest: dict[str, Any]) -> str:
    repository = project_manifest["repositories"][0]
    project = repository["project"]
    lines = [f"### {project['product']}", "", "#### Release and Project Updates", ""]
    for release in repository["releases"]:
        lines.append(
            f"- **{release['channel'].title()} release [{release['tag']}]({release['url']}).**"
        )
        for change in release["changes"]:
            lines.append(f"  - [{change['title']}]({change['url']})")
    for candidate in repository["candidates"]:
        lines.append(f"- [{candidate['title']}]({candidate['url']})")
    lines.append("")
    return "\n".join(lines)


def next_dispatch_title(previous: str) -> str:
    match = re.search(r"Entire Dispatch 0x([0-9a-fA-F]+)", previous)
    if not match:
        return "Entire Dispatch"
    width = len(match.group(1))
    return f"Entire Dispatch 0x{int(match.group(1), 16) + 1:0{width}x}"


def assemble_draft(
    manifest: dict[str, Any], previous: str, projects: list[dict[str, str]], fragments: Path
) -> tuple[str, list[dict[str, str]]]:
    products = [project["product"] for project in projects]
    description = "Updates across " + ", ".join(products) + "."
    lines = [
        f"title: {next_dispatch_title(previous)}",
        f"description: {description}",
        "category: Dispatch",
        "author: Marvin",
        "",
        "Beep, boop. Marvin here. The machines have been busy again. I have arranged their output into something humans can inspect without opening every repository themselves.",
        "",
        f"Here is what changed from {manifest['window']['since']} through {manifest['window']['until']}:",
        "",
    ]
    for area in ("CLI", "Web", "OSS Projects"):
        area_projects = [project for project in projects if project["area"] == area]
        if not area_projects:
            continue
        lines.extend([f"## {area}", ""])
        for project in area_projects:
            lines.extend([(fragments / f"{project['key']}.md").read_text().strip(), ""])
    lines.extend(
        [
            "That's the dispatch. The repositories have been counted, the releases have been linked, and nothing has been permitted to vanish merely because the list was inconveniently long.",
            "",
            "As always, bring questions, bugs, PRs, and constructive dread to [Discord](https://discord.com/invite/jZJs3Tue4S) or [GitHub issues](https://github.com/entireio/cli/issues).",
            "",
            "Boop.",
            "",
        ]
    )
    valid_exclusion_ids = {item["id"] for item in coverage_items(manifest)}
    exclusions: list[dict[str, str]] = []
    exclusion_ids: set[str] = set()
    for project in projects:
        path = fragments / f"{project['key']}.exclusions.json"
        if path.exists():
            for exclusion in read_json(path):
                if exclusion["id"] not in valid_exclusion_ids or exclusion["id"] in exclusion_ids:
                    continue
                exclusion_ids.add(exclusion["id"])
                exclusions.append(exclusion)
    return "\n".join(lines), exclusions


def coverage_items(manifest: dict[str, Any]) -> list[dict[str, str]]:
    items: list[dict[str, str]] = []
    for repo in manifest["repositories"]:
        project = repo["project"]
        for release in repo["releases"]:
            items.append(
                {
                    "id": release["id"],
                    "url": release["url"],
                    "label": f"{project['repo']} release {release['tag']}",
                    "required": "release",
                }
            )
            for change in release["changes"]:
                items.append(
                    {
                        "id": change["id"],
                        "url": change["url"],
                        "label": f"{project['repo']}: {change['title']}",
                        "required": "change",
                    }
                )
        for candidate in repo["candidates"]:
            items.append(
                {
                    "id": candidate["id"],
                    "url": candidate["url"],
                    "label": f"{project['repo']}: {candidate['title']}",
                    "required": "change",
                }
            )
    return items


def validate(manifest: dict[str, Any], draft: str, exclusions: list[dict[str, str]]) -> tuple[str, int]:
    excluded = {item.get("id"): item.get("reason", "") for item in exclusions}
    included_items: list[dict[str, str]] = []
    excluded_items: list[dict[str, str]] = []
    missing_items: list[dict[str, str]] = []
    for item in coverage_items(manifest):
        if item["url"] in draft or item["id"] in draft:
            included_items.append(item)
        elif item["required"] != "release" and item["id"] in excluded and excluded[item["id"]].strip():
            excluded_items.append(item | {"reason": excluded[item["id"]]})
        else:
            missing_items.append(item)

    lines = [
        "# Dispatch coverage report",
        "",
        f"- Included: {len(included_items)}",
        f"- Intentionally excluded: {len(excluded_items)}",
        f"- Missing: {len(missing_items)}",
        "",
    ]
    for heading, values in (
        ("Included", included_items),
        ("Intentionally excluded", excluded_items),
        ("Missing", missing_items),
    ):
        lines.extend([f"## {heading}", ""])
        if not values:
            lines.extend(["- None", ""])
            continue
        for item in values:
            reason = f" — {item['reason']}" if item.get("reason") else ""
            lines.append(f"- `{item['id']}` [{item['label']}]({item['url']}){reason}")
        lines.append("")
    return "\n".join(lines), len(missing_items)


def command_matrix(args: argparse.Namespace) -> None:
    print(json.dumps(matrix(read_json(args.config), args.selection), separators=(",", ":")))


def command_collect(args: argparse.Namespace) -> None:
    value = collect(read_json(args.config), args.repo, args.checkout, args.since, args.until)
    write_json(args.output, value)


def command_combine(args: argparse.Namespace) -> None:
    value = combine(args.collections, args.previous, args.posthog)
    write_json(args.output_manifest, value)
    args.output_source.write_text(render_source(value))


def command_split(args: argparse.Namespace) -> None:
    split_projects(read_json(args.manifest), args.output)


def command_fallback(args: argparse.Namespace) -> None:
    args.output_fragment.write_text(fallback_fragment(read_json(args.project_manifest)))
    write_json(args.output_exclusions, [])


def command_assemble(args: argparse.Namespace) -> None:
    draft, exclusions = assemble_draft(
        read_json(args.manifest),
        args.previous.read_text(),
        read_json(args.projects),
        args.fragments,
    )
    args.output_draft.write_text(draft)
    write_json(args.output_exclusions, exclusions)


def command_validate(args: argparse.Namespace) -> None:
    exclusions = read_json(args.exclusions) if args.exclusions.exists() else []
    report, missing = validate(read_json(args.manifest), args.draft.read_text(), exclusions)
    args.output.write_text(report)
    if missing:
        print(f"dispatch draft is missing {missing} required source items", file=sys.stderr)
        raise SystemExit(1)


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser()
    subparsers = result.add_subparsers(dest="command", required=True)

    matrix_parser = subparsers.add_parser("matrix")
    matrix_parser.add_argument("--config", type=Path, required=True)
    matrix_parser.add_argument("--selection", required=True)
    matrix_parser.set_defaults(func=command_matrix)

    collect_parser = subparsers.add_parser("collect")
    collect_parser.add_argument("--config", type=Path, required=True)
    collect_parser.add_argument("--repo", required=True)
    collect_parser.add_argument("--checkout", type=Path, required=True)
    collect_parser.add_argument("--since", required=True)
    collect_parser.add_argument("--until", required=True)
    collect_parser.add_argument("--output", type=Path, required=True)
    collect_parser.set_defaults(func=command_collect)

    combine_parser = subparsers.add_parser("combine")
    combine_parser.add_argument("--collections", type=Path, required=True)
    combine_parser.add_argument("--previous", type=Path, required=True)
    combine_parser.add_argument("--posthog", type=Path)
    combine_parser.add_argument("--output-manifest", type=Path, required=True)
    combine_parser.add_argument("--output-source", type=Path, required=True)
    combine_parser.set_defaults(func=command_combine)

    split_parser = subparsers.add_parser("split")
    split_parser.add_argument("--manifest", type=Path, required=True)
    split_parser.add_argument("--output", type=Path, required=True)
    split_parser.set_defaults(func=command_split)

    fallback_parser = subparsers.add_parser("fallback")
    fallback_parser.add_argument("--project-manifest", type=Path, required=True)
    fallback_parser.add_argument("--output-fragment", type=Path, required=True)
    fallback_parser.add_argument("--output-exclusions", type=Path, required=True)
    fallback_parser.set_defaults(func=command_fallback)

    assemble_parser = subparsers.add_parser("assemble")
    assemble_parser.add_argument("--manifest", type=Path, required=True)
    assemble_parser.add_argument("--previous", type=Path, required=True)
    assemble_parser.add_argument("--projects", type=Path, required=True)
    assemble_parser.add_argument("--fragments", type=Path, required=True)
    assemble_parser.add_argument("--output-draft", type=Path, required=True)
    assemble_parser.add_argument("--output-exclusions", type=Path, required=True)
    assemble_parser.set_defaults(func=command_assemble)

    validate_parser = subparsers.add_parser("validate")
    validate_parser.add_argument("--manifest", type=Path, required=True)
    validate_parser.add_argument("--draft", type=Path, required=True)
    validate_parser.add_argument("--exclusions", type=Path, required=True)
    validate_parser.add_argument("--output", type=Path, required=True)
    validate_parser.set_defaults(func=command_validate)
    return result


def main() -> None:
    args = parser().parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
