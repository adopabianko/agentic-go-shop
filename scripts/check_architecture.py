#!/usr/bin/env python3
"""Enforce import direction using Go's package graph, not source-text guesses."""
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MODULE = "example.com/agentic-go-shop"
ALLOWED = {
    "domain": set(),
    "contracts": {"domain"},
    "ports": {"domain"},
    "inventory": {"domain", "ports"},
    "orders": {"domain", "ports"},
    "adjustments": {"domain", "contracts"},
    "orderquery": {"domain", "contracts"},
    "adapters": {"domain", "ports", "contracts"},
    "httpapi": {"domain", "ports", "contracts", "inventory", "orders"},
}


def main():
    output = subprocess.check_output(["go", "list", "-json", "./internal/..."], cwd=ROOT, text=True)
    decoder = json.JSONDecoder()
    failures = []
    packages = 0
    while output.strip():
        package, end = decoder.raw_decode(output.lstrip())
        output = output.lstrip()[end:]
        path = package["ImportPath"]
        if path.endswith("/storetest"):
            continue  # Shared adapter test suite intentionally imports testing.
        packages += 1
        layer = path.removeprefix(MODULE + "/internal/").split("/")[0]
        if layer not in ALLOWED:
            failures.append(f"unclassified package: {path}")
            continue
        for dep in package.get("Imports", []):
            if not dep.startswith(MODULE + "/internal/"):
                if dep.startswith(MODULE + "/"):
                    failures.append(f"{path} must not import {dep}")
                elif layer == "domain" and dep in {"net/http", "database/sql"}:
                    failures.append(f"domain must not import {dep}")
                continue
            dependency_layer = dep.removeprefix(MODULE + "/internal/").split("/")[0]
            if dependency_layer != layer and dependency_layer not in ALLOWED[layer]:
                failures.append(f"{path} must not import {dep}")
    if failures:
        raise SystemExit("\n".join(failures))
    print(f"Architecture OK: {packages} production packages checked.")


if __name__ == "__main__":
    main()
