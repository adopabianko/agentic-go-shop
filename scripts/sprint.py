#!/usr/bin/env python3
"""Local sprint planner and worktree helper. Never launches AI, pushes, or merges."""
import argparse
import fnmatch
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
STATES = {"planned", "in_progress", "implemented", "integrated"}


def git(root, *args):
    result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True)
    if result.returncode:
        raise ValueError(result.stderr.strip() or "git command failed")
    return result.stdout.rstrip("\n")


def load(root):
    manifest = json.loads((root / "docs/sprint/tasks.json").read_text())
    tasks = manifest["tasks"]
    by_id = {}
    branches = set()
    ports = set()
    for task in tasks:
        task_id = task["id"]
        if not re.fullmatch(r"T[0-9]{2}", task_id) or task_id in by_id:
            raise ValueError(f"invalid/duplicate task ID: {task_id}")
        if task["status"] not in STATES:
            raise ValueError(f"invalid status for {task_id}")
        if task["status"] in {"implemented", "integrated"} and not re.fullmatch(r"[a-f0-9]{40}(?:[a-f0-9]{24})?", task.get("commit") or ""):
            raise ValueError(f"{task_id}: completed status requires a full implementation commit")
        if not re.fullmatch(r"feat/t[0-9]{2}-[a-z0-9-]+", task["branch"]) or task["branch"] in branches:
            raise ValueError(f"invalid/duplicate branch for {task_id}")
        if not isinstance(task["port"], int) or not 1024 <= task["port"] <= 65535 or task["port"] in ports:
            raise ValueError(f"invalid/duplicate port for {task_id}")
        branches.add(task["branch"])
        ports.add(task["port"])
        for path in [task["sds"], *task["owns"]]:
            if Path(path).is_absolute() or ".." in Path(path).parts or path.startswith(".git/"):
                raise ValueError(f"unsafe path: {path}")
        if not task["owns"] or not (root / task["sds"]).is_file():
            raise ValueError(f"{task_id}: missing ownership or SDS")
        if not task["checks"] or any(not isinstance(c, str) or not c.strip() for c in task["checks"]):
            raise ValueError(f"{task_id}: missing checks")
        by_id[task_id] = task
    visiting, visited = set(), set()

    def visit(task_id):
        if task_id in visiting:
            raise ValueError(f"dependency cycle at {task_id}")
        if task_id in visited:
            return
        visiting.add(task_id)
        task = by_id[task_id]
        for dep in set(task["start_after"] + task["integrate_after"]):
            if dep not in by_id:
                raise ValueError(f"{task_id}: unknown dependency {dep}")
            visit(dep)
        visiting.remove(task_id)
        visited.add(task_id)

    for task_id in by_id:
        visit(task_id)
    return manifest, by_id


def pending(tasks, task, field):
    accepted = {"implemented", "integrated"} if field == "start_after" else {"integrated"}
    return [dep for dep in task[field] if tasks[dep]["status"] not in accepted]


def plan(manifest, tasks):
    print(f"{manifest['name']} — status is declared metadata, not a substitute for CI/review.")
    for task in tasks.values():
        waiting = pending(tasks, task, "start_after")
        ready = "wait " + ",".join(waiting) if waiting else "ready to implement"
        merge_wait = pending(tasks, task, "integrate_after")
        merge = ",".join(merge_wait) if merge_wait else "none (checks/review still required)"
        print(f"{task['id']} [{task['status']}] {task['title']}\n  start: {ready}; integration dependencies: {merge}")
    print(f"Suggested concurrency: {manifest['max_parallel']}; worktrees do not start agents.")


def brief(root, task):
    print(f"Task: {task['id']} — {task['title']}\nBranch: {task['branch']}\nHTTP_ADDR=127.0.0.1:{task['port']}")
    print(f"Read AGENTS.md, docs/architecture.md, docs/contracts/baseline.md, and {task['sds']}.")
    print("Ownership: " + ", ".join(task["owns"]))
    print("Start prerequisites: " + (", ".join(task["start_after"]) or "none"))
    print("Integration prerequisites: " + (", ".join(task["integrate_after"]) or "none"))
    print("Checks (inspect before execution):")
    for command in task["checks"]:
        print("  " + command)
    print(f"Write handoff to docs/handoffs/{task['id']}.md using TEMPLATE.md. Record actual evidence and unresolved integration.")
    print("Do not edit another worktree, change shared contracts unilaterally, push, or merge without task authorization.")


def require_repo(root):
    top = Path(git(root, "rev-parse", "--show-toplevel")).resolve()
    if top != root.resolve():
        raise ValueError("project must be the repository root; initialize/commit this project first")
    git(root, "rev-parse", "--verify", "HEAD")


def create_worktree(root, tasks, task, base, destination):
    require_repo(root)
    if task["status"] != "planned":
        raise ValueError("task is not planned; inspect its existing worktree instead of creating another")
    if git(root, "status", "--porcelain"):
        raise ValueError("coordinator worktree is dirty; commit or reconcile changes before branching")
    waiting = pending(tasks, task, "start_after")
    if waiting:
        raise ValueError("start prerequisites not implemented: " + ", ".join(waiting))
    # Resolve the branch/ref once to avoid silently following a moving base.
    base_sha = git(root, "rev-parse", "--verify", base + "^{commit}")
    for dep in task["start_after"]:
        dep_sha = tasks[dep]["commit"]
        try:
            git(root, "merge-base", "--is-ancestor", dep_sha, base_sha)
        except ValueError as exc:
            raise ValueError(f"base does not contain required implementation commit for {dep}") from exc
    destination = destination.resolve()
    if destination == root or root in destination.parents:
        raise ValueError("worktrees must be outside the current checkout")
    if destination.exists():
        raise ValueError(f"destination already exists: {destination}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    git(root, "worktree", "add", "-b", task["branch"], str(destination), base_sha)
    print(f"Created {destination}\nBranch: {task['branch']}\nBase commit: {base_sha}")
    brief(root, task)


def changed_paths(root, base):
    require_repo(root)
    paths = set()
    for args in [
        ("diff", "--name-only", "--no-renames", "-z", base + "...HEAD"),
        ("diff", "--name-only", "--no-renames", "-z", "HEAD"),
        ("ls-files", "--others", "--exclude-standard", "-z"),
    ]:
        paths.update(p for p in git(root, *args).split("\0") if p)
    return paths


def check_scope(root, task, base):
    paths = changed_paths(root, base)
    unexpected = sorted(p for p in paths if not any(fnmatch.fnmatchcase(p, pattern) for pattern in task["owns"]))
    if unexpected:
        raise ValueError("outside task ownership:\n" + "\n".join(unexpected))
    print(f"Scope OK: {len(paths)} changed paths within {task['id']} ownership. This is a review check, not a security sandbox.")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT, help="repository root")
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("validate")
    sub.add_parser("plan")
    p = sub.add_parser("brief"); p.add_argument("task")
    p = sub.add_parser("worktree"); p.add_argument("task"); p.add_argument("--base", default="main"); p.add_argument("--destination", type=Path)
    p = sub.add_parser("check-scope"); p.add_argument("task"); p.add_argument("--base", default="main")
    args = parser.parse_args(argv)
    root = args.root.resolve()
    try:
        manifest, tasks = load(root)
        if args.command == "validate":
            print(f"Valid manifest: {len(tasks)} tasks, acyclic dependencies, unique branches/ports, SDS present.")
        elif args.command == "plan":
            plan(manifest, tasks)
        else:
            if args.task not in tasks:
                raise ValueError(f"unknown task: {args.task}")
            task = tasks[args.task]
            if args.command == "brief":
                brief(root, task)
            elif args.command == "worktree":
                destination = args.destination or root.parent / (root.name + "-worktrees") / args.task.lower()
                create_worktree(root, tasks, task, args.base, destination)
            elif args.command == "check-scope":
                check_scope(root, task, args.base)
    except (ValueError, KeyError, TypeError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
