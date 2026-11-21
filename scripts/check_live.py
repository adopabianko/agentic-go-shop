#!/usr/bin/env python3
"""Build-independent local process check: smoke real HTTP, then verify SIGTERM exit."""
import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    binary = ROOT / "bin/api"
    if not binary.is_file():
        raise SystemExit("run make build first")
    env = {**os.environ, "HTTP_ADDR": "127.0.0.1:0"}
    # A file avoids deadlocking the server on a full stdout pipe during the smoke.
    log_dir = ROOT / ".cache"
    log_dir.mkdir(exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w+b", dir=log_dir) as log:
        process = subprocess.Popen([str(binary)], cwd=ROOT, env=env, stdout=log, stderr=log)
        try:
            address = None
            deadline = time.monotonic() + 10
            while time.monotonic() < deadline:
                # Open a separate reader; seeking a descriptor shared with the
                # child would move the child's stdout offset as well.
                for line in Path(log.name).read_text().splitlines():
                    try:
                        event = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if event.get("msg") == "server started":
                        address = event["address"]
                if address:
                    break
                if process.poll() is not None:
                    raise RuntimeError("server exited before readiness:\n" + Path(log.name).read_text())
                time.sleep(0.05)
            if address is None:
                raise RuntimeError("server did not start within 10 seconds")
            subprocess.run([sys.executable, "scripts/smoke.py", "--url", "http://" + address], cwd=ROOT, check=True, timeout=20)
        finally:
            process.terminate()
            try:
                code = process.wait(timeout=12)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
                raise RuntimeError("graceful shutdown timed out")
        if code != 0:
            raise RuntimeError(f"server exited with status {code} after SIGTERM")
    print("PASS: compiled binary, isolated loopback instance, smoke journey, graceful SIGTERM exit")


if __name__ == "__main__":
    main()
