import contextlib
import copy
import io
import json
import subprocess
import tempfile
import unittest
from pathlib import Path

import sprint


class SprintTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / "repo"
        self.root.mkdir()
        self.manifest = {"name": "test", "max_parallel": 2, "tasks": []}
        for number in [1, 2]:
            task = {"id": f"T{number:02}", "title": "example", "status": "planned", "branch": f"feat/t{number:02}-example", "port": 8100+number, "sds": f"docs/sds/T{number:02}.md", "start_after": [], "integrate_after": [], "owns": [f"internal/task{number}/*"], "checks": ["go test ./..."], "commit": None}
            path = self.root / task["sds"]
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("# Example SDS\n")
            self.manifest["tasks"].append(task)
        self.save()

    def save(self):
        path = self.root / "docs/sprint/tasks.json"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(self.manifest))

    def init(self):
        sprint.git(self.root, "init", "-b", "main")
        sprint.git(self.root, "config", "user.name", "Test Fixture")
        sprint.git(self.root, "config", "user.email", "test@example.invalid")
        sprint.git(self.root, "config", "commit.gpgsign", "false")
        self.commit()

    def commit(self):
        sprint.git(self.root, "add", ".")
        sprint.git(self.root, "commit", "-m", "test fixture")

    def test_manifest_rejects_cycles_unknown_and_missing_sds(self):
        original = copy.deepcopy(self.manifest)
        for change in ["cycle", "unknown", "sds", "port", "status", "path"]:
            with self.subTest(change=change):
                self.manifest = copy.deepcopy(original)
                a, b = self.manifest["tasks"]
                if change == "cycle": a["start_after"] = ["T02"]; b["integrate_after"] = ["T01"]
                if change == "unknown": a["start_after"] = ["T99"]
                if change == "sds": a["sds"] = "missing.md"
                if change == "port": b["port"] = a["port"]
                if change == "status": a["status"] = "implemented"
                if change == "path": a["owns"] = ["../escape"]
                self.save()
                with self.assertRaises(ValueError): sprint.load(self.root)

    def test_worktree_isolated_and_no_overwrite(self):
        self.init()
        _, tasks = sprint.load(self.root)
        destination = Path(self.tmp.name) / "worker"
        with contextlib.redirect_stdout(io.StringIO()):
            sprint.create_worktree(self.root, tasks, tasks["T01"], "main", destination)
        (destination / "worker-only").write_text("isolated")
        self.assertFalse((self.root / "worker-only").exists())
        self.assertEqual(sprint.git(destination, "branch", "--show-current"), tasks["T01"]["branch"])
        with self.assertRaises(ValueError): sprint.create_worktree(self.root, tasks, tasks["T01"], "main", destination)

    def test_dependency_commit_must_be_on_base(self):
        self.init()
        sprint.git(self.root, "checkout", "-b", "foundation")
        (self.root / "foundation").write_text("code")
        self.commit()
        dependency_commit = sprint.git(self.root, "rev-parse", "HEAD")
        sprint.git(self.root, "checkout", "main")
        a, b = self.manifest["tasks"]
        a.update(status="implemented", commit=dependency_commit)
        b["start_after"] = ["T01"]
        self.save(); self.commit()
        _, tasks = sprint.load(self.root)
        destination = Path(self.tmp.name) / "worker"
        with self.assertRaisesRegex(ValueError, "does not contain"):
            sprint.create_worktree(self.root, tasks, tasks["T02"], "main", destination)
        with contextlib.redirect_stdout(io.StringIO()):
            sprint.create_worktree(self.root, tasks, tasks["T02"], "foundation", destination)

    def test_worktree_rejects_dirty_parent_and_waiting_task(self):
        self.init()
        _, tasks = sprint.load(self.root)
        (self.root / "uncommitted").write_text("keep me")
        with self.assertRaisesRegex(ValueError, "dirty"):
            sprint.create_worktree(self.root, tasks, tasks["T01"], "main", Path(self.tmp.name)/"worker")
        self.commit()
        tasks["T02"]["start_after"] = ["T01"]
        with self.assertRaisesRegex(ValueError, "prerequisites"):
            sprint.create_worktree(self.root, tasks, tasks["T02"], "main", Path(self.tmp.name)/"worker")
        with self.assertRaisesRegex(ValueError, "outside"):
            sprint.create_worktree(self.root, tasks, tasks["T01"], "main", self.root/"nested")

    def test_scope_checks_commits_untracked_and_rename_sources(self):
        self.init()
        _, tasks = sprint.load(self.root)
        owned = self.root / "internal/task1"
        owned.mkdir(parents=True)
        (owned / "ok.go").write_text("package task1\n")
        with contextlib.redirect_stdout(io.StringIO()): sprint.check_scope(self.root, tasks["T01"], "main")
        (self.root / "outside.txt").write_text("unexpected")
        with self.assertRaisesRegex(ValueError, "outside.txt"): sprint.check_scope(self.root, tasks["T01"], "main")
        self.commit()
        sprint.git(self.root, "checkout", "-b", "worker")
        sprint.git(self.root, "mv", "outside.txt", "internal/task1/moved.txt")
        self.commit()
        with self.assertRaisesRegex(ValueError, "outside.txt"): sprint.check_scope(self.root, tasks["T01"], "main")

    def test_update_manifest_sets_status_and_resolves_full_commit(self):
        self.init()
        manifest, tasks = sprint.load(self.root)
        sprint.update_manifest(self.root, manifest, tasks, "T01", "in_progress")
        manifest, tasks = sprint.load(self.root)
        self.assertEqual(tasks["T01"]["status"], "in_progress")
        self.assertIsNone(tasks["T01"]["commit"])

        head = sprint.git(self.root, "rev-parse", "HEAD")
        sprint.update_manifest(self.root, manifest, tasks, "T01", "implemented", "HEAD")
        manifest, tasks = sprint.load(self.root)
        self.assertEqual(tasks["T01"]["status"], "implemented")
        self.assertEqual(tasks["T01"]["commit"], head)
        self.assertTrue((self.root / "docs/sprint/tasks.json").read_text().endswith("\n"))

    def test_update_manifest_enforces_commit_and_dependencies(self):
        self.init()
        manifest, tasks = sprint.load(self.root)
        with self.assertRaisesRegex(ValueError, "requires --commit"):
            sprint.update_manifest(self.root, manifest, tasks, "T01", "implemented")
        with self.assertRaisesRegex(ValueError, "does not accept --commit"):
            sprint.update_manifest(self.root, manifest, tasks, "T01", "in_progress", "HEAD")

        tasks["T02"]["start_after"] = ["T01"]
        with self.assertRaisesRegex(ValueError, "start prerequisites"):
            sprint.update_manifest(self.root, manifest, tasks, "T02", "in_progress")

        tasks["T02"]["start_after"] = []
        tasks["T02"]["integrate_after"] = ["T01"]
        with self.assertRaisesRegex(ValueError, "integration prerequisites"):
            sprint.update_manifest(self.root, manifest, tasks, "T02", "integrated", "HEAD")

    def test_update_manifest_rejects_integrated_commit_outside_head(self):
        self.init()
        sprint.git(self.root, "checkout", "-b", "worker")
        (self.root / "worker").write_text("code")
        self.commit()
        worker_commit = sprint.git(self.root, "rev-parse", "HEAD")
        sprint.git(self.root, "checkout", "main")
        manifest, tasks = sprint.load(self.root)
        with self.assertRaisesRegex(ValueError, "not an ancestor"):
            sprint.update_manifest(self.root, manifest, tasks, "T01", "integrated", worker_commit)


if __name__ == "__main__":
    unittest.main()
