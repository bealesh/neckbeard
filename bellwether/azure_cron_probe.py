#!/usr/bin/env python3
"""Execute an Azure scheduled job and verify its image, status and database report.

This checks direct execution of the scheduled definition, not the timer itself.
Requires an initialized fixture and Azure CLI with its containerapp extension.
"""

import argparse
import json
from pathlib import Path
import re
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--env", choices=["dev", "stg", "prd"], default="dev")
    args = parser.parse_args()
    target = json.loads((args.root / f".neckbeard/deploy/{args.env}.json").read_text())
    release = json.loads((args.root / f"releases/{args.env}.json").read_text())
    if target["cloud"] != "azure" or target["runtime"] != "serverless-containers":
        raise ValueError("this probe requires an Azure serverless fixture")
    deployment = json.loads(subprocess.check_output([
        "tofu", f"-chdir={args.root / 'infra/envs' / args.env}", "output", "-json", "deployment"
    ]))

    def az(*command):
        return json.loads(subprocess.check_output([
            "az", *command, "--subscription", target["container"], "--output", "json"
        ]))

    jobs = [s for s in target["services"] if s["kind"] == "cron"]
    if not jobs:
        raise ValueError("fixture has no scheduled workload")
    for service in jobs:
        name = deployment["service_names"][service["name"]]
        scope = ["--name", name, "--resource-group", deployment["resource_group"]]
        job = az("containerapp", "job", "show", *scope)
        if job["properties"]["configuration"]["triggerType"] != "Schedule":
            raise ValueError("job is not the scheduled definition")
        container = next(c for c in job["properties"]["template"]["containers"] if c["name"] == service["name"])
        if container["image"] != release["image"]:
            raise ValueError("scheduled job does not use the committed image")
        execution = az("containerapp", "job", "start", *scope)["name"]
        print(json.dumps({"job": name, "execution": execution, "status": "started"}), flush=True)
        deadline = time.monotonic() + 600
        while True:
            result = az("containerapp", "job", "execution", "show", *scope, "--job-execution-name", execution)
            status = result["properties"]["status"]
            if status == "Succeeded":
                break
            if status in ["Failed", "Stopped", "Degraded"]:
                raise ValueError(f"scheduled workload execution {status}: {execution}")
            if time.monotonic() >= deadline:
                raise TimeoutError(f"scheduled workload did not finish: {execution}")
            time.sleep(5)
        actual = next(c for c in result["properties"]["template"]["containers"] if c["name"] == service["name"])
        if actual["image"] != release["image"]:
            raise ValueError("completed execution did not use the committed immutable image")
        logs = subprocess.check_output([
            "az", "containerapp", "job", "logs", "show", *scope,
            "--subscription", target["container"], "--execution", execution,
            "--container", service["name"], "--format", "text", "--tail", "300",
        ]).decode()
        pattern = rf"bellwether report \(env {args.env}\): [1-9][0-9]* note\(s\), [1-9][0-9]* processed"
        reports = re.findall(pattern, logs)
        if len(reports) != 1:
            raise ValueError("job logs did not prove it read the populated application database")
        print(json.dumps({"job": name, "execution": execution, "image": actual["image"], "report": reports[0]}), flush=True)


if __name__ == "__main__":
    main()
