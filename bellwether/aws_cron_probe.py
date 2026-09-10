#!/usr/bin/env python3
"""Execute the generated EventBridge task definition and verify its report.

Checks the scheduled workload, not EventBridge's timed invocation or IAM role.
Requires an initialized fixture and its normal AWS CLI/OpenTofu credentials.
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
    if target["cloud"] != "aws" or target["runtime"] != "serverless-containers":
        raise ValueError("this probe requires an AWS serverless fixture")

    def aws(*command):
        return json.loads(subprocess.check_output([
            "aws", *command, "--region", target["region"], "--output", "json"
        ]))

    identity = aws("sts", "get-caller-identity")
    if identity["Account"] != target["container"]:
        raise ValueError("AWS login is for a different account")
    deployment = json.loads(subprocess.check_output([
        "tofu", f"-chdir={args.root / 'infra/envs' / args.env}", "output", "-json", "deployment"
    ]))
    for service, rule in deployment["cron_rules"].items():
        targets = aws("events", "list-targets-by-rule", "--rule", rule)["Targets"]
        if len(targets) != 1:
            raise ValueError("expected exactly one scheduled ECS target")
        scheduled = targets[0]
        parameters = scheduled["EcsParameters"]
        definition = aws("ecs", "describe-task-definition", "--task-definition", parameters["TaskDefinitionArn"])["taskDefinition"]
        container = next(c for c in definition["containerDefinitions"] if c["name"] == service)
        if container["image"] != release["image"]:
            raise ValueError("scheduled definition does not use the committed image")
        network = parameters["NetworkConfiguration"]["awsvpcConfiguration"]
        network = {key[0].lower() + key[1:]: value for key, value in network.items()}
        started = aws(
            "ecs", "run-task", "--cluster", scheduled["Arn"],
            "--task-definition", parameters["TaskDefinitionArn"],
            "--launch-type", parameters["LaunchType"], "--count", "1",
            "--network-configuration", json.dumps({"awsvpcConfiguration": network}),
            "--started-by", "neckbeard-live-probe",
        )
        if started.get("failures") or len(started.get("tasks", [])) != 1:
            raise ValueError(f"cron task did not start: {started.get('failures')}")
        arn = started["tasks"][0]["taskArn"]
        print(json.dumps({"service": service, "task": arn, "status": "started"}), flush=True)
        deadline = time.monotonic() + 600
        while True:
            task = aws("ecs", "describe-tasks", "--cluster", scheduled["Arn"], "--tasks", arn)["tasks"][0]
            if task["lastStatus"] == "STOPPED":
                break
            if time.monotonic() >= deadline:
                raise TimeoutError(f"cron task did not finish: {arn}")
            time.sleep(10)
        actual = next(c for c in task["containers"] if c["name"] == service)
        if actual.get("exitCode") != 0 or actual.get("imageDigest") != release["image"].split("@")[1]:
            raise ValueError(f"cron did not finish successfully with the expected image: {arn}")
        options = container["logConfiguration"]["options"]
        stream = options["awslogs-stream-prefix"] + "/" + service + "/" + arn.rsplit("/", 1)[1]
        events = aws("logs", "get-log-events", "--log-group-name", options["awslogs-group"], "--log-stream-name", stream)["events"]
        reports = [e["message"] for e in events if re.fullmatch(
            rf"bellwether report \(env {args.env}\): [1-9][0-9]* note\(s\), [1-9][0-9]* processed", e["message"]
        )]
        if len(reports) != 1:
            raise ValueError("cron logs did not prove it read the populated application database")
        print(json.dumps({"service": service, "task": arn, "image": actual["imageDigest"], "report": reports[0]}), flush=True)
    if not deployment["cron_rules"]:
        raise ValueError("fixture has no scheduled workload")


if __name__ == "__main__":
    main()
