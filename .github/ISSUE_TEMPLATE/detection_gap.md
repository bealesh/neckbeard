---
name: Analyzer detection gap
about: "`neckbeard analyze` missed or misread something in a real repository"
labels: detection-gap
---

**Repository** (link if public, or describe the layout)

**What the draft got wrong** — missing datastore/service, wrong port or health
path, a dev/base image treated as a service, a secret misclassified…

**The evidence the analyzer should have seen** (file:line)

Detection is deterministic on purpose: fixes must key on mechanical evidence
(an accessor pattern, a template file, a literal), never a guess. If the only
signal is framework convention, say which framework — dependency-manifest
heuristics are on the roadmap.
