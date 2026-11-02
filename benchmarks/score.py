#!/usr/bin/env python3
"""Scores one benchmark run directory with the same checks for both arms.

Everything here is mechanical and runs on the files the agent left behind:
OpenTofu init/validate/fmt per root, checkov, actionlint, a scan for IAM
wildcards and plaintext secrets, line counts, Infracost when it can price the
output, and, for the analysis task, a ground-truth fact checklist. The agent's
own cost, tokens, turns and wall time come from Claude Code's JSON result.
"""
import glob
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SKIP_DIRS = {".git", ".terraform", "node_modules", ".work", ".neckbeard"}  # .neckbeard is the plugin's vendored catalog cache


def sh(args, cwd=None, timeout=600):
    try:
        p = subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=timeout)
        return p.returncode, p.stdout, p.stderr
    except FileNotFoundError:
        return 127, "", f"{args[0]} not installed"
    except subprocess.TimeoutExpired:
        return 124, "", "timed out"


def walk(root, exts=None, include_terraform=False):
    for dirpath, dirnames, filenames in os.walk(root):
        skip = SKIP_DIRS - ({".terraform"} if include_terraform else set())
        dirnames[:] = [d for d in dirnames if d not in skip]
        for f in filenames:
            if exts is None or os.path.splitext(f)[1] in exts:
                yield os.path.join(dirpath, f)


def changed_files(app):
    """Only what the agent added or changed is scored. The fixture is committed
    before the run and everything is staged after it, so the staged diff is the
    agent's work; the plugin's vendored catalog cache and lock files are not."""
    code, out, _ = sh(["git", "diff", "--cached", "--name-only"], cwd=app)
    files = [f for f in out.split("\n") if f.strip()] if code == 0 else []
    return [f for f in files if not f.startswith(".neckbeard/") and "/.terraform/" not in f and not f.endswith(".terraform.lock.hcl")]


def tf_roots(app):
    """Directories holding changed .tf files with a provider or terraform block
    are roots; when the agent wrote plain module trees without one, every
    changed .tf directory counts."""
    dirs = sorted({os.path.dirname(os.path.join(app, f)) for f in changed_files(app) if f.endswith(".tf")})
    roots = []
    for d in dirs:
        text = "".join(open(p, errors="replace").read() for p in glob.glob(os.path.join(d, "*.tf")))
        if re.search(r'^\s*(provider\s+"|terraform\s*\{)', text, re.M):
            roots.append(d)
    roots = roots or dirs
    # a directory another root consumes as a module is not a root of its own;
    # checkov and validate see it through its callers
    consumed = set()
    for root in roots:
        manifest = os.path.join(root, ".terraform", "modules", "modules.json")
        if os.path.exists(manifest):
            try:
                for m in json.load(open(manifest)).get("Modules", []):
                    if m.get("Key"):
                        consumed.add(os.path.normpath(os.path.join(root, m.get("Dir", "."))))
            except Exception:
                pass
    return [r for r in roots if os.path.normpath(r) not in consumed] or roots


def events(run_dir):
    """Claude Code's stream-json output: one JSON event per line."""
    out = []
    try:
        for line in open(os.path.join(run_dir, "events.jsonl"), errors="replace"):
            line = line.strip()
            if line.startswith("{"):
                try:
                    out.append(json.loads(line))
                except Exception:
                    pass
    except FileNotFoundError:
        pass
    return out


def tool_metrics(evs):
    """What the agent actually did: tool calls by name, shell commands, and
    whether the neckbeard CLI ran. The last one is the plugin arm's proof of
    use and the bare arm's leak check."""
    by_name, commands = {}, []
    for e in evs:
        if e.get("type") != "assistant":
            continue
        for c in e.get("message", {}).get("content", []):
            if c.get("type") != "tool_use":
                continue
            by_name[c["name"]] = by_name.get(c["name"], 0) + 1
            if c["name"] == "Bash":
                commands.append(str(c.get("input", {}).get("command", "")))
            if c["name"] == "Skill":
                commands.append("Skill:" + str(c.get("input", {}).get("skill", "")))
    # the binary in command position followed by one of its subcommands, so a
    # heredoc or comment that merely mentions the word does not count
    sub = r'(analyze|plan|estimate|scaffold|validate|doctor|release|version|schema|presets|configuration|catalog|cost|sync|help|-h|--help)'
    direct = re.compile(r'(^|[;&|(]\s*|\n\s*)(?:[\w./-]*/)?neckbeard\s+' + sub + r'\b')

    def invokes(c):
        if direct.search(c):
            return True
        # `NB=/path/to/neckbeard; $NB plan`: a variable bound to the binary
        for var in re.findall(r'\b([A-Za-z_]\w*)=[\w./-]*/neckbeard\b', c):
            if re.search(r'\$\{?' + re.escape(var) + r'\}?\s+' + sub + r'\b', c):
                return True
        return False
    neck = [c for c in commands if invokes(c)]
    cloud = [c for c in commands if re.match(r'\s*(aws|az|gcloud)\s', c)]
    skill = any(c.startswith("Skill:") and "neckbeard" in c for c in commands)
    return {"tool_calls": sum(by_name.values()), "by_name": by_name, "shell_commands": len([c for c in commands if not c.startswith("Skill:")]),
            "neckbeard_cli_calls": len(neck), "neckbeard_subcommands": sorted({m.group(1) for c in neck for m in re.finditer(r'(?:neckbeard|\$\{?[A-Za-z_]\w*\}?)\s+' + sub + r'\b', c)}),
            "neckbeard_skill_loaded": skill, "cloud_cli_attempts": len(cloud)}


def agent_metrics(run_dir):
    out = {"cost_usd": None, "turns": None, "duration_ms": None, "input_tokens": None, "output_tokens": None,
           "cache_read_tokens": None, "cache_write_tokens": None, "model": None, "is_error": None, "result": ""}
    evs = events(run_dir)
    data = next((d for d in reversed(evs) if d.get("type") == "result"), None)
    if data is None:  # the run died before Claude Code wrote a result
        out["is_error"] = "no result event"
        return out
    u = data.get("usage", {})
    out.update(cost_usd=data.get("total_cost_usd"), turns=data.get("num_turns"), duration_ms=data.get("duration_ms"),
               input_tokens=u.get("input_tokens"), output_tokens=u.get("output_tokens"),
               cache_read_tokens=u.get("cache_read_input_tokens"), cache_write_tokens=u.get("cache_creation_input_tokens"),
               is_error=data.get("is_error"), result=data.get("result", "") or "")
    mu = data.get("modelUsage") or {}
    out["model"] = ",".join(sorted(mu)) if mu else None
    return out


def checkov_skips(path):
    """The check ids under skip-check: in a checkov config, read without a
    YAML library: one `- CKV_...` per line, comments ignored."""
    ids, active = set(), False
    for line in open(path, errors="replace"):
        stripped = line.split("#", 1)[0].rstrip()
        if not stripped.strip():
            continue
        if re.match(r'^skip-check\s*:', stripped):
            active = True
            continue
        if active and re.match(r'^\s+-\s*\S+', stripped):
            ids.add(stripped.split("-", 1)[1].strip().strip("'\""))
            continue
        if not stripped.startswith((" ", "\t")):
            active = False
    return ids


def v0(app):
    roots = tf_roots(app)
    per_root = []
    for root in roots:
        rel = os.path.relpath(root, app)
        r = {"root": rel}
        # <work>/tofu-plugin-cache, shared with the driver; run dirs are <work>/runs/<task>/<app>/<arm>/<n>
        work = os.path.abspath(os.path.join(os.path.dirname(app), "..", "..", "..", "..", ".."))
        cache = os.environ.setdefault("TF_PLUGIN_CACHE_DIR", os.path.join(work, "tofu-plugin-cache"))
        os.makedirs(cache, exist_ok=True)
        code, _, err = sh(["tofu", "init", "-backend=false", "-input=false", "-no-color"], cwd=root, timeout=900)
        r["init"] = code == 0
        if code != 0:
            r["init_error"] = err.strip()[-400:]
        code, out, err = sh(["tofu", "validate", "-json", "-no-color"], cwd=root)
        try:
            v = json.loads(out)
            r["validate"] = bool(v.get("valid"))
            r["validate_errors"] = [d.get("summary") for d in v.get("diagnostics", []) if d.get("severity") == "error"][:5]
        except Exception:
            r["validate"] = False
            r["validate_errors"] = [(err or out).strip()[-300:]]
        code, _, _ = sh(["tofu", "fmt", "-check", "-recursive"], cwd=root)
        r["fmt"] = code == 0
        # checkov is run from inside the root with `-d .`: scanned that way it
        # evaluates the modules the root calls, including ones that live
        # outside the directory, which it does not do when handed a path from
        # elsewhere. One pass with no config gives the findings before anyone's
        # documented skips; the "after skips" view filters that list against
        # the skip-check entries of a .checkov.yaml in the root or at the top
        # of the tree. Same two numbers as running checkov twice, in half the time.
        skips = set()
        for cfg in (os.path.join(root, ".checkov.yaml"), os.path.join(app, ".checkov.yaml")):
            if os.path.exists(cfg):
                skips = checkov_skips(cfg)
                break
        args = ["checkov", "-d", ".", "-o", "json", "--quiet", "--compact", "--download-external-modules", "false", "--framework", "terraform"]
        code, out, err = sh(args, cwd=root, timeout=900)
        try:
            c = json.loads(out)
            if isinstance(c, list):
                c = next((x for x in c if x.get("check_type") == "terraform"), c[0] if c else {})
            summary = c.get("summary", {})
            failed = c.get("results", {}).get("failed_checks", [])
            kept = [f for f in failed if f["check_id"] not in skips]
            r["checkov_raw"] = {"passed": summary.get("passed", 0), "failed": len(failed), "skipped": summary.get("skipped", 0),
                                "failed_ids": sorted({f["check_id"] for f in failed})[:40]}
            r["checkov"] = {"passed": summary.get("passed", 0) + len(failed) - len(kept), "failed": len(kept), "skipped": len(failed) - len(kept),
                            "failed_ids": sorted({f["check_id"] for f in kept})[:40]}
        except Exception:
            r["checkov"] = r["checkov_raw"] = {"passed": None, "failed": None, "skipped": None, "error": (err or out).strip()[-300:]}
        per_root.append(r)
    workflows = sorted(os.path.join(app, f) for f in changed_files(app) if f.startswith(".github/workflows/") and f.endswith((".yml", ".yaml")))
    al = {"workflows": len(workflows), "issues": None, "excluded_queue_max": 0}
    if workflows:
        code, out, err = sh(["actionlint", "-format", "{{json .}}"] + workflows)
        try:
            issues = json.loads(out) if out.strip() else []
        except Exception:
            issues = [{"message": (err or out).strip()[-300:]}]
        kept = []
        for i in issues:
            # the same exclusion neckbeard's own validator applies: actionlint
            # does not know GitHub's `queue:` concurrency key yet
            if i.get("snippet", "").strip().startswith("queue: max") or "\"queue\"" in i.get("message", ""):
                al["excluded_queue_max"] += 1
                continue
            kept.append({"file": os.path.relpath(i.get("filepath", ""), app), "line": i.get("line"), "message": i.get("message", "")[:200]})
        al["issues"] = kept
    checkov_failed = sum((r["checkov"].get("failed") or 0) for r in per_root)
    checkov_raw_failed = sum((r["checkov_raw"].get("failed") or 0) for r in per_root)
    passes = bool(per_root) and all(r["init"] and r["validate"] for r in per_root) and (al["issues"] == [] or al["workflows"] == 0) and checkov_failed == 0
    return {"roots": per_root, "actionlint": al, "checkov_failed_total": checkov_failed, "checkov_raw_failed_total": checkov_raw_failed,
            "checkov_config_present": os.path.exists(os.path.join(app, ".checkov.yaml")),
            "tofu_valid_all": bool(per_root) and all(r["init"] and r["validate"] for r in per_root),
            "fmt_clean_all": bool(per_root) and all(r["fmt"] for r in per_root), "passes": passes}


WILDCARDS = {
    "action_star": [r'"Action"\s*:\s*"\*"', r'"Action"\s*:\s*\[\s*"\*"', r'\bactions?\s*=\s*\[\s*"\*"', r'\bactions?\s*=\s*"\*"'],
    "service_star": [r'"[a-z0-9-]+:\*"'],
    "resource_star": [r'"Resource"\s*:\s*"\*"', r'"Resource"\s*:\s*\[\s*"\*"', r'\bresources?\s*=\s*\[\s*"\*"', r'\bresources?\s*=\s*"\*"'],
}
SECRETS = [
    ("credential_literal", r'(?i)\b(password|passwd|secret|token|api[_-]?key|private[_-]?key)\b["\']?\s*[=:]\s*["\']([^"\'\s$<{]{8,})["\']'),
    ("aws_access_key", r'\bAKIA[0-9A-Z]{16}\b'),
    ("connection_string_with_password", r'(postgres(ql)?|mysql|redis|amqp)://[^:/\s"\']+:[^@\s"\']+@'),
    ("private_key_block", r'-----BEGIN [A-Z ]*PRIVATE KEY-----'),
]
PLACEHOLDERS = re.compile(r'(?i)change[_-]?me|replace|example|placeholder|xxx+|your[_-]|dummy|redacted|<[^>]+>|random_password|generated|sensitive')


def scan(app):
    """Wildcards are counted in what the agent wrote plus the modules its roots
    pull in on init (.terraform/modules), so the plugin's catalog gets the same
    scrutiny as hand-written HCL. Secrets are looked for in the agent's own
    files only; vendored modules carry no values and the fixture is not the
    agent's doing."""
    wild = {k: 0 for k in WILDCARDS}
    wild_hits = []
    secrets = []
    paths = [os.path.join(app, f) for f in changed_files(app)]
    for root in tf_roots(app):
        # every module the root resolved on init, wherever tofu put or found it
        # (git modules land in .terraform/modules; local-path modules, such as
        # the plugin's catalog cache, are referenced in place)
        manifest = os.path.join(root, ".terraform", "modules", "modules.json")
        if not os.path.exists(manifest):
            continue
        try:
            for m in json.load(open(manifest)).get("Modules", []):
                d = os.path.normpath(os.path.join(root, m.get("Dir", ".")))
                if d != os.path.normpath(root) and os.path.isdir(d):
                    paths += [p for p in walk(d, {".tf", ".json"}, include_terraform=True) if "/.terraform/" not in p[len(d):]]
        except Exception:
            pass
    seen = set()
    for path in paths:
        if path in seen or not os.path.isfile(path) or not path.endswith((".tf", ".json", ".tfvars", ".yml", ".yaml", ".env", ".hcl", ".md", ".sh")):
            continue
        seen.add(path)
        try:
            text = open(path, errors="replace").read()
        except Exception:
            continue
        rel = os.path.relpath(path, app)
        if path.endswith((".tf", ".json", ".hcl")):
            for kind, pats in WILDCARDS.items():
                for pat in pats:
                    for m in re.finditer(pat, text):
                        wild[kind] += 1
                        if len(wild_hits) < 60:
                            wild_hits.append({"kind": kind, "file": rel, "line": text.count("\n", 0, m.start()) + 1})
        if "/.terraform/" in path or rel.endswith(".md"):
            continue  # vendored modules are scanned for wildcards, not for the agent's own secrets; prose is not config
        for kind, pat in SECRETS:
            for m in re.finditer(pat, text):
                snippet = m.group(0)
                if kind == "credential_literal" and PLACEHOLDERS.search(snippet):
                    continue
                if "${" in snippet or "{{" in snippet or "%s" in snippet or "%v" in snippet:
                    continue  # an interpolation or format string, not a value
                secrets.append({"kind": kind, "file": rel, "line": text.count("\n", 0, m.start()) + 1})
    return {"wildcards": wild, "wildcard_hits": wild_hits, "secrets": secrets, "secrets_count": len(secrets)}


def sizes(app):
    files = changed_files(app)

    def lines(sel):
        return sum(sum(1 for _ in open(os.path.join(app, f), errors="replace")) for f in files if sel(f) and os.path.isfile(os.path.join(app, f)))
    tf_lines = lines(lambda f: f.endswith(".tf"))
    wf_lines = lines(lambda f: f.startswith(".github/workflows/") and f.endswith((".yml", ".yaml")))
    return {"tf_lines": tf_lines, "workflow_lines": wf_lines, "files_changed": len(files), "has_tf": tf_lines > 0}


def infracost(app, roots):
    """Prices every root Infracost can read. Unavailable (no login) is reported,
    not hidden: the column stays empty until the operator fills it in."""
    result = {"available": None, "roots": [], "total_monthly_usd": None, "dev_monthly_usd": None}
    # the same preference as neckbeard's estimator: the 0.10 line first, since
    # the 2.x CLI routes breakdowns through a dashboard login the 0.10 API key
    # does not need
    binary = None
    for cand in ("infracost-0.10", os.path.expanduser("~/.local/bin/infracost-0.10"), "infracost"):
        code, out, err = sh([cand, "--version"])
        if code == 0:
            binary, result["binary"] = cand, out.strip().split("\n")[0][:60]
            break
    if binary is None:
        result["available"] = False
        result["reason"] = "infracost not installed"
        return result
    total = 0.0
    priced = 0
    for root in roots:
        env_backup = os.environ.get("INFRACOST_SKIP_UPDATE_CHECK")
        os.environ["INFRACOST_SKIP_UPDATE_CHECK"] = "true"
        code, out, err = sh([binary, "breakdown", "--path", ".", "--format", "json"], cwd=root, timeout=900)
        if env_backup is None:
            os.environ.pop("INFRACOST_SKIP_UPDATE_CHECK", None)
        i = out.find("{")
        try:
            d = json.loads(out[i:])
            m = float(d.get("totalMonthlyCost") or 0)
            rel = os.path.relpath(root, app)
            result["roots"].append({"root": rel, "monthly_usd": m, "unsupported": len(d.get("summary", {}).get("unsupportedResourceCounts", {}) or {})})
            total += m
            priced += 1
            if re.search(r'(^|/)dev(/|$)', rel) or len(roots) == 1:
                result["dev_monthly_usd"] = (result["dev_monthly_usd"] or 0) + m
        except Exception:
            result["available"] = False
            lines = [l for l in (err + "\n" + out).split("\n") if l.strip() and "deprecated" not in l]
            errs = [l for l in lines if "ERR" in l] or [l for l in lines if "rror" in l]
            result["reason"] = (errs or lines or ["no output"])[0].strip()[:200]
            return result
    result["available"] = priced > 0
    result["total_monthly_usd"] = total if priced else None
    return result


def facts(app, app_name):
    path = os.path.join(HERE, "facts", f"{app_name}.json")
    if not os.path.exists(path):
        return None
    spec = json.load(open(path))
    doc = ""
    for cand in ("PRODUCTION_NEEDS.md",):
        p = os.path.join(app, cand)
        if os.path.exists(p):
            doc = open(p, errors="replace").read()
    found, missing = [], []
    for f in spec["facts"]:
        (found if any(re.search(pat, doc, re.I) for pat in f["any"]) else missing).append(f["id"])
    cites = len(re.findall(r'[\w./-]+\.(?:go|py|ts|tsx|js|json|prisma|sh|yaml|yml|toml|Dockerfile)[:#]L?\d+|Dockerfile[:#]L?\d+', doc))
    return {"doc_present": bool(doc), "found": found, "missing": missing, "score": f"{len(found)}/{len(spec['facts'])}",
            "ratio": len(found) / len(spec["facts"]), "citations_with_lines": cites, "doc_lines": doc.count("\n")}


def declined(result_text, has_tf):
    t = result_text.lower()
    words = ("not supported", "outside", "doesn't support", "does not support", "cannot help", "not something neckbeard", "unsupported", "declin", "refus", "not a fit")
    return (not has_tf) and any(w in t for w in words)


def main():
    run_dir = sys.argv[1]
    app = os.path.join(run_dir, "app")
    run = json.load(open(os.path.join(run_dir, "run.json")))
    m = {"run": run, "agent": agent_metrics(run_dir), "tools": tool_metrics(events(run_dir))}
    m["sizes"] = sizes(app)
    m["v0"] = v0(app) if m["sizes"]["has_tf"] else {"roots": [], "actionlint": {"workflows": 0, "issues": None}, "checkov_failed_total": 0, "checkov_raw_failed_total": 0, "checkov_config_present": False, "tofu_valid_all": False, "fmt_clean_all": False, "passes": False}
    m["scan"] = scan(app)
    if run["task"] in ("onboard", "static-site"):
        m["infracost"] = infracost(app, tf_roots(app)) if m["sizes"]["has_tf"] else {"available": None, "reason": "no terraform"}
    if run["task"] == "needs":
        m["facts"] = facts(app, run["app"])
    m["declined"] = declined(m["agent"]["result"], m["sizes"]["has_tf"])
    with open(os.path.join(run_dir, "result.md"), "w") as f:
        f.write(m["agent"]["result"])
    m["agent"].pop("result")
    print(json.dumps(m, indent=2))


if __name__ == "__main__":
    main()
