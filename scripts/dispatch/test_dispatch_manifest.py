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
                }
            ],
        )
        self.assertEqual(dispatch_manifest.previous_tag(release), "v5.19.1")

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


if __name__ == "__main__":
    unittest.main()
