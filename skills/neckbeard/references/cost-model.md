# Cost model

`neckbeard estimate` prices a fresh render plus user-owned Terraform extensions
and auto tfvars from `-root` (default `.`). It never uploads an authenticated
plan to Neckbeard; Infracost reads `-plan <local.json> -env dev` locally and
looks up unit prices. Do not commit plans or sensitive variable files.

## GCP corrections

The classic Infracost adapter does not correctly interpret several resources
in the catalog. Model v2 corrects these quantities using live unit prices from
the configured pricing API, in the requested currency:

- Cloud SQL's empty replica configuration does not add a replica. Actual replica
  resources remain priced separately.
- An IP attached to the forwarding rule has no separate unused-IP charge.
- Global forwarding rules share a project bracket: the first five cost
  $0.025/hour combined, subsequent rules $0.01/hour each at the verified USD
  snapshot. Environments sharing a project share this bracket.
  `existing_global_forwarding_rules` specifies other project rules.
- HTTP/HTTPS redirect rules do not each incur the full load-balancer minimum.
  Data processing applies once to inbound plus outbound traffic.
- Cloud Run's catalog configuration uses instance-based billing. Compute is
  CPU/memory seconds from aggregate `active_instance_hours` **per HTTP service**,
  bounded by minimum and maximum instances times 730 hours. There is no separate
  request charge. The preset assumes one active instance all month; edit this
  to represent the workload. It is not inferred from request count.
- Worker pools bill continuously at their manual instance count, including idle.
- Jobs use `job_executions_per_month` (preset 30 per job; override to match the
  schedule) and `job_duration_minutes` (preset one). Duration has the one-minute
  billing minimum and does not scale again when execution count scales.
- Registry storage and active secret versions remain present at idle; access
  operations, backups, log ingestion and traffic have explicit usage inputs.
- Internet transfer currently models US source regions to Americas destinations,
  at the highest published tier with no free allowance. Other source regions
  fail explicitly until a destination-specific model is supplied. Logging also
  excludes free allowances. These choices are conservative, not silent zeros.

Prices come from the pricing API, including for non-USD currencies; they are
never converted by changing the report label. No tax, negotiated discount,
commitment discount, free-tier sharing or future price guarantee is modeled.

## Scenarios and approvals

Baseline zeroes traffic and job executions, retaining stored images, secret
versions and minimum/always-on compute. Low, expected and high use 0.5×, 1× and
2× traffic/active hours; configured maximum instances still bound compute.
High is not a buffer for omitted charges.

Malformed or negative/nonfinite numeric prices are errors. Missing prices or
quantities and unsupported resources remain named, and mark a report PARTIAL.
No separate-charge control-plane objects are listed separately from missing
billable components. A complete report means all returned billable components
have prices and quantities; it does not prove the provider adapter discovered
every possible charge. Unmapped assumptions remain listed for review.

The JSON sidecar records model version, blueprint hash, effective-input digest,
coverage, scope and timestamp. A pre-scaffold ceiling requires a complete report
for the whole blueprint, not just one environment. Changing Terraform extensions
or auto tfvars invalidates approval. Generated files must match their ownership
record; Terraform override files that replace generated settings are refused—
put those settings in supported `neckbeard.yaml` overrides and re-plan.
This is an estimate policy check, not an apply-time or cloud spending cap.

## Evidence and validation scope

On October 23, 2026, the Umami trial in `us-central1` was priced independently
from effective HCL and the saved authenticated plan. Both gave:

| USD/month, one development environment | Baseline | Low | Expected | High |
|---|---|---|---|---|
| Umami, private Cloud SQL, HTTPS | 28.79 | 58.02 | 87.24 | 93.13 |

Expected includes 730 active instance-hours (one CPU, one GiB), 20 GiB outbound,
2 GiB inbound, 5 GiB logs, 10 GiB registry storage and 10 GiB backups, plus the
configured secrets. The deployment allows at most one instance. This reconciles
the model's inputs and unit prices; no full-month invoice is available yet.

`go test ./...` replays the original bad adapter output, tests the first-five
forwarding-rule bracket, attached IP, fake replica, instance floors/ceilings,
always-on worker pools, minimum job duration, idle stock, strict numeric
parsing, currency-specific API responses and changed-input approvals.
AWS/Azure reports also name the known missing internet-transfer and scheduled
compute models and cannot approve a ceiling while those charges are missing.
AWS, Azure and Kubernetes retain the shared strict parser and input-binding
fixes, but have not received this numerical reconciliation. The old comparison
table is not used as evidence for those lanes.

Price rules: [load balancing](https://cloud.google.com/load-balancing/pricing),
[Cloud Run](https://cloud.google.com/run/pricing),
[logging](https://cloud.google.com/stackdriver/pricing), and the
[pricing API](https://www.infracost.io/docs/supported_resources/cloud_pricing_api/).
