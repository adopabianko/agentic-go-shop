"""Structural checks, not a replacement for full OpenAPI schema validation."""
import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class ContractStructureTest(unittest.TestCase):
    def test_local_references_and_operation_ids(self):
        for filename in ["openapi.json", "next.openapi.json"]:
            with self.subTest(filename=filename):
                document = json.loads((ROOT / "api" / filename).read_text())
                self.assertEqual(document["openapi"], "3.1.0")
                seen = set()

                def walk(value):
                    if isinstance(value, list):
                        for child in value:
                            walk(child)
                    elif isinstance(value, dict):
                        if "$ref" in value:
                            reference = value["$ref"]
                            self.assertTrue(reference.startswith("#/"))
                            node = document
                            for part in reference[2:].split("/"):
                                node = node[part.replace("~1", "/").replace("~0", "~")]
                        for child in value.values():
                            walk(child)

                walk(document)
                for path, operations in document["paths"].items():
                    for method, operation in operations.items():
                        self.assertIn(method, {"get", "post"})
                        self.assertNotIn(operation["operationId"], seen)
                        seen.add(operation["operationId"])
                        path_params = {p["name"] for p in operation.get("parameters", []) if p["in"] == "path" and p["required"]}
                        self.assertEqual(set(re.findall(r"\{([^}]+)\}", path)), path_params)
                        self.assertTrue(any(code.startswith("2") for code in operation["responses"]))
                        for response in operation["responses"].values():
                            self.assertIn("description", response)
                            self.assertIn("application/json", response["content"])


if __name__ == "__main__":
    unittest.main()
