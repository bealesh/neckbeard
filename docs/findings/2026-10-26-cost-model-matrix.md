# Six-lane cost model reconciliation — 2026-10-26

Model v3 repairs AWS/Azure serverless and all three Kubernetes lanes, extending
[the GCP correction](2026-10-22-cost-estimator-postmortem.md). The price-only
matrix returned complete component coverage in all 48 evaluations: six lanes,
two currencies, four scenarios. No cloud resources were created or changed.
This does not establish deployment success or a measured monthly invoice.

## Reference workload

One **development** environment, solo tier, based on the checked-in Bellwether
configuration and basic planner profile: one HTTP service, one always-on worker,
one daily scheduled job, Postgres, object storage, registry and three secrets.
Each lane uses its own catalog sizes, so these are not equal-capacity benchmarks.
Regions: AWS us-east-1, Azure eastus, GCP us-central1. Dev uses Spot nodes where
configured; snapshots cannot guarantee later Spot rates or availability.

Expected usage: 730 aggregate active instance-hours per HTTP service, minimum
nodes with zero additional node-hours, 30 job executions of one minute each,
2 million requests, 20 GB internet transfer, 2 GB ingress, zero cross-zone
transfer, 5 GB log ingestion, zero query GB, 50 AWS ECS custom metric series,
10 GB registry images, 10 GB extra backups, 2 GB/month object growth over
12 months, 10,000 object reads and 1,000 writes. Storage, workers and minimum
compute remain at baseline. Low/high scale variable usage by 0.5/2; maximum
replicas/nodes still bound compute. No free allowances, discounts or taxes.
Optional builds, direct bucket egress and archive/export features are explicitly
assumed unused. See the [model reference](../../skills/neckbeard/references/cost-model.md)
for capacity assumptions and limits.

## Recorded results

Amounts per month, one environment, using currency-specific pricing API replies
on October 26, 2026. EUR prices come from the API rather than relabeled USD.

### USD

| Lane | Baseline | Low | Expected | High |
|---|---:|---:|---:|---:|
| AWS Fargate | 149.35 | 152.63 | 155.91 | 180.50 |
| AWS EKS | 183.20 | 186.47 | 189.75 | 196.31 |
| Azure Container Apps | 82.91 | 110.44 | 137.97 | 193.04 |
| Azure AKS | 235.59 | 243.00 | 250.41 | 265.23 |
| GCP Cloud Run | 61.69 | 91.17 | 120.66 | 179.63 |
| GCP GKE | 126.47 | 129.66 | 132.85 | 139.22 |

### EUR

| Lane | Baseline | Low | Expected | High |
|---|---:|---:|---:|---:|
| AWS Fargate | 133.42 | 136.36 | 139.29 | 161.26 |
| AWS EKS | 163.67 | 166.60 | 169.52 | 175.38 |
| Azure Container Apps | 74.07 | 98.67 | 123.27 | 172.46 |
| Azure AKS | 210.48 | 217.10 | 223.72 | 236.95 |
| GCP Cloud Run | 55.11 | 81.46 | 107.80 | 160.48 |
| GCP GKE | 112.99 | 115.84 | 118.68 | 124.38 |

## What changed

AWS now prices running Fargate floors, active hours and scheduled tasks, ALB
capacity/public IPv4, internet traffic, Container Insights and EKS control-plane
logs. Azure separates active/idle/request charges and jobs, includes managed
networking, corrects Postgres storage/HA/replicas, and removes paid-user license
charges from federated identities. Kubernetes uses total minimum nodes and
explicit additional node-hours, correct boot disks, actual AKS Spot prices plus
the fixed system node, and controller-created ingress. Optional-feature zeros
are named assumptions. Unknown custom billable resources remain partial.

API selection checks vendor, product, region, purchase option, currency and unit;
ambiguous or missing prices fail. Authenticated plans with changed CPU, memory,
scaling, disk or supported billing settings fail when they differ from the
blueprint. Earlier ceiling approvals are invalid under model v3.

## Repeat the check

`go test ./...` replays twelve sanitized expected-scenario adapter breakdowns
and recorded unit-price facts without network access. Separate arithmetic tests
cover scaling bounds, short jobs, persistent worker floors, node disks and HA.
The live matrix evaluates all four scenarios against fresh adapter output.

With Infracost installed and authenticated, from the repository root:

```sh
NECKBEARD_COST_EVIDENCE_ROOT=/tmp/neckbeard-price-check go test ./core/estimate -run '^TestLiveCostMatrix$' -count=1 -v
```

The folder must be empty or contain prior generated reference lanes. The test
renders local fixtures with synthetic cloud identifiers and calls the pricing
API; it does not authenticate to cloud providers or deploy. To refresh the
checked-in snapshots, additionally set `NECKBEARD_WRITE_COST_FIXTURES=1` and
review all changed quotes and totals. If a refresh changes a total, include the
before/after totals and their cause in the PR body so reviewers can assess the
change without searching the JSON snapshots. Credentials and Terraform metadata are
excluded from recorded fixtures.

This is component and pricing reconciliation, not independent invoice matching.
Provider defaults, unmodeled workload behavior, newly introduced resources and
controller behavior still need deployment/billing evidence. The original live
GCP Umami trial remains distinct from this larger reference workload.
