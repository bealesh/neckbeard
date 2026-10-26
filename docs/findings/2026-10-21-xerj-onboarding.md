# Xerj onboarding boundary trial — 2026-10-21

## Setup and result

Inspected [xerj-org/xerj at `fb0f352`](https://github.com/xerj-org/xerj/tree/fb0f352bc5fcde4b364f2a9a7432384a52e24251),
a Rust search engine with application-owned persistent data. Ran the actual
Neckbeard CLI built from `00423eb313c2708fcbf01da0ecbde079d49c7f1b`;
its source tree is identical to main's squash merge
[`2792cce`](https://github.com/bealesh/neckbeard/commit/2792cce1e1798a05d537c03a4348c6d332ac7bc2).
The binary reported `neckbeard 0.3.1-0.20261021195936-00423eb313c2`, not a
published beta version. No existing app files were edited.

Both root and `engine/` analysis returned exit zero and wrote drafts. The
maintainer inspection stopped before cloud configuration: the app's persistent
filesystem is not modeled by the catalog. This is an unsupported-workload
finding, not successful onboarding or an independent-user trial.

Reproduce against the pinned Xerj checkout with a CLI from the source above:

```sh
neckbeard doctor -for plan
neckbeard analyze -dir /path/to/xerj -out /tmp/xerj-root-profile.yaml
neckbeard analyze -dir /path/to/xerj/engine -out /tmp/xerj-engine-profile.yaml
```

Doctor reported the bundled catalog ready. Root analysis produced one service,
three needs, 28 secret names, 32 facts, five assumptions, and one unsupported
finding. Engine analysis produced one service, three needs, 13 secret names,
18 facts, five assumptions, and one unsupported finding. These counts describe
these drafts only; secret values were not read or supplied.

## Friction observed

| Draft or pipeline behavior | Repository evidence and implication |
|---|---|
| Root draft uses `engine/Dockerfile`, while generated pipelines use repository-root build context. | [`engine/Dockerfile:8`](https://github.com/xerj-org/xerj/blob/fb0f352bc5fcde4b364f2a9a7432384a52e24251/engine/Dockerfile#L8) uses `COPY . .` and runs Cargo in `/build`; [`engine/docker-compose.yml:5`](https://github.com/xerj-org/xerj/blob/fb0f352bc5fcde4b364f2a9a7432384a52e24251/engine/docker-compose.yml#L5) sets `build: .` relative to `engine/`. Choosing `engine/` as the application root matches that declared context; a repository-root build is not equivalent. No image build was attempted. |
| Both drafts select port 8080 and `/health`. | [`engine/Dockerfile:18`](https://github.com/xerj-org/xerj/blob/fb0f352bc5fcde4b364f2a9a7432384a52e24251/engine/Dockerfile#L18) exposes three ports. Its declared readiness probe at lines 35–36 is `http://127.0.0.1:9200/health/ready`. The draft does not preserve this probe/listener pair. Runtime behavior was not tested. |
| Both drafts infer Postgres from a quoted `DATABASE_URL`. | [`engine/crates/xerj-autoindex/src/extract/code.rs:3028`](https://github.com/xerj-org/xerj/blob/fb0f352bc5fcde4b364f2a9a7432384a52e24251/engine/crates/xerj-autoindex/src/extract/code.rs#L3028) begins a unit test for parsing Python constants. The cited literal at line 3041 is an assertion in that test, not evidence the deployed server needs Postgres. |
| Both drafts infer object storage from `BUCKET_COUNT`. | [`engine/tests/battle_sla.sh:455`](https://github.com/xerj-org/xerj/blob/fb0f352bc5fcde4b364f2a9a7432384a52e24251/engine/tests/battle_sla.sh#L455) counts aggregation-result buckets. This is not an object-storage dependency. |
| Elasticsearch is reported as an unsupported dependency. | The cited evidence is `engine/tests/battle_real.sh`, a comparison/test script. A comparison target is not by itself a production dependency. Review the evidence rather than provisioning or requiring an external Elasticsearch service. |
| No persistent-data finding appears in either draft. | [`engine/docker-compose.yml:12`](https://github.com/xerj-org/xerj/blob/fb0f352bc5fcde4b364f2a9a7432384a52e24251/engine/docker-compose.yml#L12) mounts `xerj-data:/data`; [`engine/Dockerfile:39`](https://github.com/xerj-org/xerj/blob/fb0f352bc5fcde4b364f2a9a7432384a52e24251/engine/Dockerfile#L39) starts the engine with `--data-dir /data`. Treating this as a disposable HTTP container would omit the app's data contract. |

## Change made and limits

The shipped onboarding reference now requires build-context, listener/probe,
and persistent-data review before cloud questions, and traces detected datastore
needs back to production configuration. It explicitly stops an application-owned
persistent-filesystem workload rather than disguising it as a managed database
or an external dependency. These are agent review steps; the deterministic
analyzer still has the detection gaps above. No analyzer precision claim is made.

Plan, estimate, scaffold, V0 validation, Xerj installation/runtime, image build,
cloud deployment, and recovery were NOT EXERCISED. No cloud resources were
created, no credentials were supplied, and no upstream report was posted.
A supported application that consumes an independently operated Xerj endpoint
would be a different trial; it would not establish support for hosting Xerj.
