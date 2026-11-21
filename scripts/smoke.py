#!/usr/bin/env python3
"""Exercise the real local API. This creates and cancels one order."""
import argparse
import json
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:8080")
    args = parser.parse_args()
    key = "smoke-" + uuid.uuid4().hex

    def request(method, path, data=None, expected=200, idem=None):
        headers = {}
        body = None
        if data is not None:
            body = json.dumps(data).encode()
            headers["Content-Type"] = "application/json"
        if idem:
            headers["Idempotency-Key"] = idem
        req = urllib.request.Request(args.url.rstrip("/") + path, body, headers, method=method)
        try:
            response = urllib.request.urlopen(req, timeout=10)
        except urllib.error.HTTPError as exc:
            response = exc
        with response:
            payload = json.load(response)
            if response.status != expected:
                raise RuntimeError(f"{method} {path}: expected {expected}, got {response.status}: {payload}")
            return payload, response.headers

    def ensure(condition, message):
        if not condition:
            raise RuntimeError(message)

    request("GET", "/healthz")
    before, _ = request("GET", "/v1/inventory/SKU-001")
    payload = {"customer_id": "smoke-customer", "items": [{"sku": "SKU-001", "quantity": 1}]}
    order, _ = request("POST", "/v1/orders", payload, 201, key)
    try:
        replay, headers = request("POST", "/v1/orders", payload, 201, key)
        ensure(replay == order and headers["Idempotency-Replayed"] == "true", "replay mismatch")
        conflict = {**payload, "customer_id": "different-customer"}
        failure, _ = request("POST", "/v1/orders", conflict, 409, key)
        ensure(failure["error"]["code"] == "idempotency_conflict", "wrong conflict code")
        current, _ = request("GET", "/v1/orders/" + order["id"])
        ensure(current == order, "GET order mismatch")
        reserved, _ = request("GET", "/v1/inventory/SKU-001")
        ensure(reserved["available"] == before["available"] - 1, "stock not reserved")
    finally:
        cancelled, _ = request("POST", "/v1/orders/" + order["id"] + "/cancel")
    again, _ = request("POST", "/v1/orders/" + order["id"] + "/cancel")
    ensure(cancelled == again and again["status"] == "cancelled", "cancel not idempotent")
    after, _ = request("GET", "/v1/inventory/SKU-001")
    ensure(before == after, "stock not restored; use an isolated demo instance")
    print("PASS: live HTTP create, replay, conflict, get, stock reservation, cancel, repeated cancel, stock restoration")


if __name__ == "__main__":
    main()
