#!/usr/bin/env python3
"""Live HTTPS/database/worker probe; retain the note file across releases.

This is application evidence, not a substitute for CI, digest, cron, or gate checks.
"""

import argparse
import json
from pathlib import Path
import time
import urllib.parse
import urllib.request
import uuid


def request(url, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req, timeout=15) as response:
        if response.url != url:
            raise ValueError("probe endpoint redirected")
        return json.load(response)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release", required=True, type=Path)
    parser.add_argument("--note", required=True, type=Path)
    parser.add_argument("--create", action="store_true")
    args = parser.parse_args()
    release = json.loads(args.release.read_text())
    url = urllib.parse.urlsplit(release["health_url"])
    if url.scheme != "https" or not url.hostname or url.username or url.password:
        raise ValueError("a credential-free HTTPS endpoint is required")
    health = request(release["health_url"])
    expected = {
        "status": "ok",
        "db": "ok",
        "env": release["environment"],
        "version": release["expected_version"],
    }
    if any(health.get(key) != value for key, value in expected.items()):
        raise ValueError(f"health assertion failed: {health!r}; expected {expected!r}")
    base = urllib.parse.urlunsplit((url.scheme, url.netloc, "/notes", "", ""))
    if args.create:
        # Reserve the evidence path before creating a row: reruns cannot silently
        # replace the persistence probe with a new record after losing the old DB.
        with args.note.open("x") as output:
            body = "neckbeard-live-" + str(uuid.uuid4())
            note = request(base, {"body": body})
            if note.get("body") != body or not isinstance(note.get("id"), int):
                raise ValueError("create response did not identify the probe note")
            json.dump({"endpoint": base, "note": note}, output, indent=2)
            output.write("\n")
    saved = json.loads(args.note.read_text())
    if saved["endpoint"] != base:
        raise ValueError("saved note belongs to a different environment endpoint")
    note = saved["note"]
    deadline = time.monotonic() + 60
    while True:
        notes = request(base)
        found = [row for row in notes if row.get("id") == note["id"]]
        if len(found) != 1 or found[0].get("body") != note["body"]:
            raise ValueError("the original database record is missing or changed")
        if found[0].get("processed") is True:
            print(json.dumps({"health": health, "note_id": note["id"], "worker": "processed"}))
            return
        if time.monotonic() >= deadline:
            raise TimeoutError("worker did not process the record within 60 seconds")
        time.sleep(3)


if __name__ == "__main__":
    main()
