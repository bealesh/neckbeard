# Cost model

`neckbeard estimate` prices a fresh render plus user-owned Terraform extensions
and auto tfvars from `-root` (default `.`). It never uploads an authenticated
plan to Neckbeard; Infracost reads `-plan <local.json> -env dev` locally and
looks up unit prices. Do not commit plans or sensitive variable files.

## GCP corrections

The classic Infracost adapter does not correctly interpret several resources
in the catalog. Model v3 corrects these quantities using live unit prices from
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
Model v3 adds AWS, Azure, EKS, AKS and GKE numerical reconciliation. The
[October 26 matrix](../../../docs/findings/2026-10-26-cost-model-matrix.md)
records twelve live-price snapshots (six lanes, USD and EUR), each evaluated in
four scenarios. Offline tests replay the twelve expected-scenario breakdowns;
focused arithmetic tests cover floors, bounds, short jobs, node scaling, HA and
replicas. This is pricing evidence, not deployment or invoice evidence.

## AWS, Azure and Kubernetes

- ECS Fargate uses configured CPU/memory, Linux x86 list rates and aggregate
  `active_instance_hours` per HTTP service, bounded by running-task minimum and
  maximum. Workers retain their always-on floor. Scheduled tasks use execution
  count and duration, rounded to seconds with a one-minute minimum.
- Azure Container Apps Consumption separates active and idle replica seconds
  and HTTP requests (the request SKU is per million). Workers remain active.
  Jobs use execution seconds, with a one-second minimum. Managed load balancer
  and public-IP charges for the catalog's external VNet environment are included.
- Kubernetes retains minimum nodes and the control plane at idle.
  `additional_node_hours` means aggregate node-hours above the total minimum,
  bounded by the configured maximum; it is not multiplied by the node count.
  GKE uses total node bounds and 100 GiB pd-balanced disks; EKS uses 20 GiB gp3
  AL2023 root volumes. AKS Spot mode retains its fixed on-demand system node
  and separately prices the Spot user pool and its disks.
- Controller-created application ingress is priced even though it is absent
  from the Terraform resource list. AWS includes ALB hours, public IPs and LCU
  usage; GKE includes the catalog's HTTP forwarding rules; AKS includes its
  standard load balancer and public IPs.
- ALB capacity assumes one new connection per request and uses the greater of
  connection and byte dimensions. Long-lived connections, large rule sets or
  mutual TLS need a workload-specific capacity assumption before relying on
  that estimate. Internet transfer uses the highest list tier without free
  allowances. `cross_zone_gb` is total billed traffic across both directions;
  Azure inter-zone transfer has no charge under the current policy.
- Azure Postgres HA bills primary and standby compute/storage. Read replicas
  add their own inherited compute/storage rather than inheriting the primary's
  backup allocation. Federated workload identities do not add paid Entra user
  licenses.
- AWS ECS Container Insights uses explicit `custom_metrics` (preset 50 series),
  retained at idle. EKS control-plane logs include ingestion, retained storage
  and queries. `log_query_gb`, `object_read_requests`, and
  `object_write_requests` are editable workload inputs.
- Optional S3 Select, direct object-bucket egress, blob index/list extras, ACR
  builds and Basic/archive/export log features are explicitly assumed unused.
  Reports list this assumption; enabling those features needs additional usage
  modeling. Storage, active secrets and registry images remain at idle.

Prices are selected by vendor, region, product, purchase option and unit. An
ambiguous product, unavailable currency or mismatched unit fails explicitly.
Spot prices are snapshots. Authentication-plan sizing must match the blueprint;
unsupported sizing changes fail instead of applying the wrong correction.
Model v3 invalidates earlier ceiling approvals. Complete coverage still depends
on the adapter discovering the resources; custom unsupported resources remain
partial rather than being erased by catalog corrections.

Price rules: [load balancing](https://cloud.google.com/load-balancing/pricing),
[Cloud Run](https://cloud.google.com/run/pricing),
[logging](https://cloud.google.com/stackdriver/pricing), and the
[pricing API](https://www.infracost.io/docs/supported_resources/cloud_pricing_api/).

Additional rules: [Fargate](https://aws.amazon.com/fargate/pricing/),
[Container Apps billing](https://learn.microsoft.com/en-us/azure/container-apps/billing),
[Container Apps VNet infrastructure](https://learn.microsoft.com/en-us/azure/container-apps/custom-virtual-networks),
[Postgres HA](https://learn.microsoft.com/azure/reliability/reliability-postgresql-flexible-server),
[GKE boot disks](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/custom-boot-disks),
and [AL2023 EKS defaults](https://github.com/awslabs/amazon-eks-ami/blob/main/templates/al2023/variables-default.json).
