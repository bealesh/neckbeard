#!/usr/bin/env python3
"""Turns the per-run metrics.json files into the markdown tables in results/.
Medians with min–max ranges; counts where the metric is a count. Usage:
aggregate.py <runs-dir> [raw-copy-dir]"""
import glob
import json
import os
import shutil
import statistics
import sys

CELLS = [("onboard", "bellwether"), ("onboard", "umami"), ("onboard", "python"), ("needs", "bellwether"), ("needs", "umami"), ("static-site", "none")]


def load(runs):
    rows = []
    for p in sorted(glob.glob(os.path.join(runs, "*", "*", "*", "*", "metrics.json"))):
        m = json.load(open(p))
        m["_path"] = p
        rows.append(m)
    return rows


def med(vals, fmt="{:.0f}"):
    vals = [v for v in vals if v is not None]
    if not vals:
        return "—"
    if len(vals) == 1:
        return fmt.format(vals[0])
    return f"{fmt.format(statistics.median(vals))} ({fmt.format(min(vals))}–{fmt.format(max(vals))})"


def frac(vals):
    vals = [v for v in vals if v is not None]
    return f"{sum(1 for v in vals if v)}/{len(vals)}" if vals else "—"


def table(rows, task, app):
    cell = [r for r in rows if r["run"]["task"] == task and r["run"]["app"] == app]
    if not cell:
        return ""
    arms = {"plugin": [r for r in cell if r["run"]["arm"] == "plugin"], "bare": [r for r in cell if r["run"]["arm"] == "bare"]}
    lines = [f"| metric | with neckbeard (n={len(arms['plugin'])}) | without (n={len(arms['bare'])}) |", "|---|---|---|"]

    def row(name, fn, fmt="{:.0f}"):
        a, b = fn(arms["plugin"]), fn(arms["bare"])
        if a == "—" and b == "—":
            return  # nothing measured on either side (Infracost without a login, say): no row, the limits say why
        lines.append(f"| {name} | {a} | {b} |")

    row("cost, USD", lambda a: med([r["agent"]["cost_usd"] for r in a], "{:.2f}"))
    row("wall time, minutes", lambda a: med([r["run"]["wall_seconds"] / 60 for r in a], "{:.1f}"))
    row("turns", lambda a: med([r["agent"]["turns"] for r in a]))
    row("output tokens", lambda a: med([r["agent"]["output_tokens"] for r in a]))
    row("input tokens incl. cache reads, millions", lambda a: med([((r["agent"]["input_tokens"] or 0) + (r["agent"]["cache_read_tokens"] or 0) + (r["agent"]["cache_write_tokens"] or 0)) / 1e6 for r in a], "{:.1f}"))
    row("finished without error", lambda a: frac([not r["agent"]["is_error"] and r["run"]["exit"] == 0 for r in a]))
    row("tool calls", lambda a: med([r["tools"]["tool_calls"] for r in a]))
    row("runs that invoked the neckbeard CLI", lambda a: frac([r["tools"]["neckbeard_cli_calls"] > 0 for r in a]))
    if task in ("onboard", "static-site"):
        row("produced OpenTofu", lambda a: frac([r["sizes"]["has_tf"] for r in a]))
        row("passes V0 (tofu validate on every root, checkov 0 failed, actionlint clean)", lambda a: frac([r["v0"]["passes"] for r in a]))
        row("tofu validate on every root", lambda a: frac([r["v0"]["tofu_valid_all"] for r in a]))
        row("tofu fmt clean", lambda a: frac([r["v0"]["fmt_clean_all"] for r in a]))
        row("checkov failed checks, after any documented skips", lambda a: med([r["v0"]["checkov_failed_total"] for r in a if r["sizes"]["has_tf"]]))
        row("checkov failed checks, no skips honoured", lambda a: med([r["v0"].get("checkov_raw_failed_total") for r in a if r["sizes"]["has_tf"]]))
        row("actionlint issues", lambda a: med([len(r["v0"]["actionlint"]["issues"] or []) for r in a if r["v0"]["actionlint"].get("workflows")]))
        row("IAM `Action: *`", lambda a: med([r["scan"]["wildcards"]["action_star"] for r in a]))
        row("IAM `service:*`", lambda a: med([r["scan"]["wildcards"]["service_star"] for r in a]))
        row("IAM `Resource: *`", lambda a: med([r["scan"]["wildcards"]["resource_star"] for r in a]))
        row("plaintext secret hits", lambda a: med([r["scan"]["secrets_count"] for r in a]))
        row("lines of HCL to review", lambda a: med([r["sizes"]["tf_lines"] for r in a]))
        row("lines of workflow YAML", lambda a: med([r["sizes"]["workflow_lines"] for r in a]))
        row("Infracost, dev environment, USD/month", lambda a: med([(r.get("infracost") or {}).get("dev_monthly_usd") for r in a], "{:.0f}"))
        if task == "static-site":
            row("declined the task by name", lambda a: frac([r["declined"] for r in a]))
    if task == "needs":
        row("facts found (ground truth list)", lambda a: med([r["facts"]["ratio"] * 100 for r in a if r.get("facts")], "{:.0f}%"))
        row("citations with line numbers", lambda a: med([r["facts"]["citations_with_lines"] for r in a if r.get("facts")]))
        row("answer length, lines", lambda a: med([r["facts"]["doc_lines"] for r in a if r.get("facts")]))
    return "\n".join(lines)


def main():
    runs = sys.argv[1]
    rows = load(runs)
    out = []
    for task, app in CELLS:
        t = table(rows, task, app)
        if t:
            out.append(f"### {task} · {app}\n\n{t}\n")
    print("\n".join(out))
    if len(sys.argv) > 2:
        raw = sys.argv[2]
        os.makedirs(raw, exist_ok=True)
        for r in rows:
            dst = os.path.join(raw, "{task}-{app}-{arm}-{n}.json".format(**r["run"]))
            shutil.copy(r["_path"], dst)
            md = os.path.join(os.path.dirname(r["_path"]), "result.md")
            if os.path.exists(md):
                shutil.copy(md, dst[:-5] + ".summary.md")


if __name__ == "__main__":
    main()
