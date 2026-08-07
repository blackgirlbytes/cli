#!/usr/bin/env python3

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("dispatch_manifest.py")
SPEC = importlib.util.spec_from_file_location("dispatch_manifest", MODULE_PATH)
assert SPEC and SPEC.loader
dispatch_manifest = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(dispatch_manifest)


class DispatchManifestTest(unittest.TestCase):
    def test_matrix_contains_mise_and_has_unique_repositories(self):
        config = dispatch_manifest.read_json(
            MODULE_PATH.parents[2] / ".github" / "dispatch-projects.json"
        )

        result = dispatch_manifest.matrix(config, "all")
        repositories = [item["repo"] for item in result["include"]]

        self.assertIn("jdx/mise", repositories)
        self.assertIn("entireio/external-agents", repositories)
        self.assertEqual(len(repositories), len(set(repositories)))

    def test_release_body_uses_exact_pull_request_links(self):
        release = {
            "body": """
## What's Changed
* safer worktrees by @dev in https://github.com/go-git/go-git/pull/2277
* other project in https://github.com/elsewhere/project/pull/12
**Full Changelog**: https://github.com/go-git/go-git/compare/v5.19.1...v5.19.2
"""
        }

        changes = dispatch_manifest.release_body_changes("go-git/go-git", release)

        self.assertEqual(
            changes,
            [
                {
                    "id": "go-git/go-git#2277",
                    "kind": "pull_request",
                    "number": 2277,
                    "title": "safer worktrees",
                    "url": "https://github.com/go-git/go-git/pull/2277",
                    "author": "dev",
                }
            ],
        )
        self.assertEqual(dispatch_manifest.previous_tag(release), "v5.19.1")

    def test_release_body_attributes_grouped_pull_requests_to_their_segment_author(self):
        release = {
            "body": (
                "* fixes https://github.com/jdx/mise/pull/1 by @alice in notes; "
                "follow-up https://github.com/jdx/mise/pull/2 by @bob in notes"
            )
        }

        changes = dispatch_manifest.release_body_changes("jdx/mise", release)

        self.assertEqual(
            [(change["number"], change["author"]) for change in changes],
            [(1, "alice"), (2, "bob")],
        )

    def test_validation_requires_releases_and_accounts_for_exclusions(self):
        manifest = {
            "window": {"since": "2026-08-01", "until": "2026-08-03"},
            "repositories": [
                {
                    "project": {"repo": "jdx/mise"},
                    "releases": [
                        {
                            "id": "jdx/mise@v2026.8.1",
                            "tag": "v2026.8.1",
                            "url": "https://github.com/jdx/mise/releases/tag/v2026.8.1",
                            "changes": [
                                {
                                    "id": "jdx/mise#123",
                                    "title": "Visible change",
                                    "url": "https://github.com/jdx/mise/pull/123",
                                },
                                {
                                    "id": "jdx/mise#124",
                                    "title": "Dependency bump",
                                    "url": "https://github.com/jdx/mise/pull/124",
                                },
                            ],
                        }
                    ],
                    "candidates": [],
                }
            ],
        }
        draft = "\n".join(
            [
                "[v2026.8.1](https://github.com/jdx/mise/releases/tag/v2026.8.1)",
                "[Visible change](https://github.com/jdx/mise/pull/123)",
            ]
        )

        report, missing = dispatch_manifest.validate(
            manifest,
            draft,
            [{"id": "jdx/mise#124", "reason": "dependency-only"}],
        )

        self.assertEqual(missing, 0)
        self.assertIn("Included: 2", report)
        self.assertIn("Intentionally excluded: 1", report)

    def test_combine_filters_links_from_previous_dispatch(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            collection = root / "collections" / "repo"
            collection.mkdir(parents=True)
            previous = root / "previous.md"
            previous.write_text("https://github.com/entireio/external-agents/pull/43")
            manifest = {
                "project": {
                    "repo": "entireio/external-agents",
                    "area": "CLI",
                    "product": "Entire CLI",
                },
                "window": {"since": "2026-08-01", "until": "2026-08-03"},
                "releases": [],
                "candidates": [
                    {
                        "id": "entireio/external-agents#43",
                        "url": "https://github.com/entireio/external-agents/pull/43",
                    },
                    {
                        "id": "entireio/external-agents#47",
                        "url": "https://github.com/entireio/external-agents/pull/47",
                    },
                ],
            }
            (collection / "manifest.json").write_text(json.dumps(manifest))

            combined = dispatch_manifest.combine(root / "collections", previous, None)

            self.assertEqual(
                [item["id"] for item in combined["repositories"][0]["candidates"]],
                ["entireio/external-agents#47"],
            )

    def test_project_fallback_and_assembly_preserve_coverage(self):
        manifest = {
            "window": {"since": "2026-08-03", "until": "2026-08-06"},
            "repositories": [
                {
                    "project": {
                        "repo": "jdx/mise",
                        "area": "OSS Projects",
                        "product": "mise",
                        "mode": "github-releases",
                        "feature_flags": False,
                    },
                    "releases": [
                        {
                            "id": "jdx/mise@v2026.8.2",
                            "tag": "v2026.8.2",
                            "channel": "stable",
                            "url": "https://github.com/jdx/mise/releases/tag/v2026.8.2",
                            "published_at": "2026-08-05T00:00:00Z",
                            "changes": [
                                {
                                    "id": "jdx/mise#125",
                                    "title": "Upgrade all tools",
                                    "url": "https://github.com/jdx/mise/pull/125",
                                    "author": "community-dev",
                                    "external_contributor": True,
                                }
                            ],
                        }
                    ],
                    "candidates": [],
                }
            ],
        }

        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            projects = dispatch_manifest.split_projects(manifest, root / "sources")
            fragment = dispatch_manifest.fallback_fragment(
                dispatch_manifest.read_json(root / "sources" / "00.json")
            )
            fragments = root / "fragments"
            fragments.mkdir()
            (fragments / "00.md").write_text(fragment)
            (fragments / "00.exclusions.json").write_text(
                '[{"id":"jdx/mise#125","reason":"duplicate"},'
                '{"id":"jdx/mise#125","reason":"duplicate"},'
                '{"id":"jdx/mise#999","reason":"maintenance-only"}]'
            )

            draft, exclusions = dispatch_manifest.assemble_draft(
                manifest,
                "title: Entire Dispatch 0x0018\n",
                projects,
                fragments,
            )
            _, missing = dispatch_manifest.validate(manifest, draft, exclusions)

            self.assertIn("title: Entire Dispatch 0x0019", draft)
            self.assertIn("https://github.com/jdx/mise/releases/tag/v2026.8.2", draft)
            self.assertIn("https://github.com/jdx/mise/pull/125", draft)
            self.assertIn(
                "Thank you for your contribution, [@community-dev](https://github.com/community-dev)!",
                draft,
            )
            self.assertIn(
                "That’s the dispatch. As always, bring questions, bugs, PRs, and constructive dread",
                draft,
            )
            self.assertNotIn("Boop.", draft)
            self.assertEqual(exclusions, [{"id": "jdx/mise#125", "reason": "duplicate"}])
            self.assertEqual(missing, 0)

    def test_render_source_compacts_posthog_flags(self):
        manifest = {
            "window": {"since": "2026-08-03", "until": "2026-08-06"},
            "repositories": [],
            "posthog_flags": {
                "count": 1,
                "results": [
                    {
                        "key": "new-home",
                        "name": "New home",
                        "active": True,
                        "status": "ACTIVE",
                        "archived": False,
                        "deleted": False,
                        "large_irrelevant_payload": "x" * 60_000,
                        "filters": {
                            "groups": [{"rollout_percentage": 25, "properties": ["large"]}],
                            "multivariate": {
                                "variants": [{"key": "control", "rollout_percentage": 75}]
                            },
                        },
                    }
                ],
            },
        }

        source = dispatch_manifest.render_source(manifest)

        self.assertIn('"key":"new-home"', source)
        self.assertIn('"rollout_percentages":[25]', source)
        self.assertIn('"variants":[{"key":"control","rollout_percentage":75}]', source)
        self.assertNotIn("large_irrelevant_payload", source)
        self.assertLess(len(source), 2_000)

    def test_empty_oss_projects_keep_headlines_without_affecting_description(self):
        manifest = {
            "window": {"since": "2026-08-03", "until": "2026-08-06"},
            "repositories": [
                {
                    "project": {
                        "repo": "go-git/go-git",
                        "area": "OSS Projects",
                        "product": "go-git",
                        "mode": "github-releases",
                        "show_when_empty": True,
                    },
                    "releases": [],
                    "candidates": [],
                },
                {
                    "project": {
                        "repo": "entireio/forgemark",
                        "area": "OSS Projects",
                        "product": "ForgeMark",
                        "mode": "public-merges",
                        "show_when_empty": True,
                    },
                    "releases": [],
                    "candidates": [],
                },
                {
                    "project": {
                        "repo": "entireio/external-agents",
                        "area": "CLI",
                        "product": "Entire CLI",
                        "mode": "public-merges",
                    },
                    "releases": [],
                    "candidates": [],
                },
            ],
        }

        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            projects = dispatch_manifest.split_projects(manifest, root / "sources")
            fragments = root / "fragments"
            fragments.mkdir()
            for project in projects:
                project_manifest = dispatch_manifest.read_json(
                    root / "sources" / f"{project['key']}.json"
                )
                (fragments / f"{project['key']}.md").write_text(
                    dispatch_manifest.empty_fragment(project_manifest)
                )

            draft, _ = dispatch_manifest.assemble_draft(
                manifest,
                "title: Entire Dispatch 0x0018\n",
                projects,
                fragments,
            )

            self.assertEqual([project["product"] for project in projects], ["go-git", "ForgeMark"])
            self.assertIn("### go-git", draft)
            self.assertIn("No stable or nightly releases shipped from 2026-08-03 through 2026-08-06.", draft)
            self.assertIn("### ForgeMark", draft)
            self.assertIn("No pull requests were merged from 2026-08-03 through 2026-08-06.", draft)
            self.assertNotIn("Updates across go-git", draft)
            self.assertIn("description: No releases or merged pull requests", draft)


if __name__ == "__main__":
    unittest.main()
