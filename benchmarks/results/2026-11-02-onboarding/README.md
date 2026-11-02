# The same agent, with and without neckbeard

2 November 2026. Claude Code 2.1.312 in print mode, model `claude-fable-5-1`
with `claude-haiku-5-5` for its side tasks, the account's configured default
for both arms. Three tasks, three apps, two arms, three runs per cell: 36 runs,
six agent-hours, 143.85 USD. No run errored, hit its turn cap, or attempted a
cloud CLI. The plugin was this repository at `717bfc7` with the CLI built from
the same commit; two umami runs picked up the released `v0.4.0-beta.1` CLI
from the operator's home directory instead, see limits. Method and scorer are
described in [the benchmarks README](../../README.md); the full tables are in
[tables.md](tables.md) and every run's metrics and closing summary are under
[raw/](raw/).

## What happened

The argument this answers is "just ask the agent, it works fine." It does. In
all nine bare onboarding runs the agent produced OpenTofu that initialised and
validated on every root, formatted clean, with workflows actionlint had no
complaints about and no plaintext secret anywhere. If the question is whether
a capable agent can write you AWS infrastructure from a prompt, the answer is
yes, three apps out of three, nine times out of nine. Nobody needs a plugin
for that.

What the plugin changed is what came back and how long it took to arrive.

| onboarding, medians over 9 runs each | with neckbeard | without |
|---|---|---|
| wall time | 12 min | 21 min |
| cost | 5.32 USD | 6.47 USD |
| output tokens | 39k | 83k |
| turns | 43 | 17 |
| lines of HCL a reviewer reads | 1,111 | 2,133 |
| passes V0 | 9 of 9 | 0 of 9 |
| checkov findings, after documented skips | 0 | 42 |
| checkov findings, no skips honoured | 38 to 44 | 16 to 52 |
| IAM `service:*` statements | 1 | 1 to 14 |

The plugin arm finished in about half the wall time on every app and wrote
under half the HCL, because the CLI did the writing: it spent three times as
many turns reading and running `neckbeard` and produced less than half the
output tokens. Cost followed tokens, 4 to 35 percent lower depending on the
app. It passed V0 in every run. The bare arm passed in none, and not because
its Terraform was wrong, but because V0 includes checkov with zero failures and
hand-written infrastructure arrives with forty-odd findings. Whether those are
worth fixing is a separate question, and the next paragraph is about it.

## The checkov number, honestly

The plugin's zero is a documented-skips zero. Run the same scan with the skip
list switched off and the catalog carries 38 findings on bellwether and
python and 44 on umami, the same families the bare arm trips over: RDS
without deletion protection or enhanced monitoring, ALBs without access logs
or a WAF, ECS tasks without read-only root filesystems, a registry without
image scanning. The catalog does not have fewer findings than a good agent
writes. It has the same ones, written down once, each with a reason, in a
file the reviewer reads instead of rediscovering them per run.

Two things in the table are real, though. The plugin's raw count was the same
number every run; the bare arm's ranged from 16 to 52 for an identical prompt,
which is the difference between a reviewed list and a dice roll. And the
findings on the catalog are the catalog's, fixable once for every user. Both
numbers are in every table so nobody has to take the zero on faith. Along the
way the scorer also showed that neckbeard's own validator was not evaluating
the catalog modules at all when it ran checkov, which is now
[#58](https://github.com/bealesh/neckbeard/issues/58).

## IAM

The bare arm's CI apply role is where the wildcards live. On umami the median
run granted eleven `service:*` statements, `ec2:*`, `ecs:*`, `rds:*` and the
rest, on `Resource: *`, in the GitHub OIDC role. The catalog's apply role has
one `iam:*` scoped to prefixed role and policy ARNs and one read-only
`iam:Get*/List*` on `*`. That reads as a win and partly is, with a caveat the
scanner cannot see: the catalog's apply role also attaches AWS's managed
`PowerUserAccess`, which is broader in effect than any list of service
wildcards. The wildcard count measures what is written in the HCL, not the
blast radius of the role. Both arms gave CI a role that can do most things to
the account.

## Secrets

Zero plaintext secrets in either arm, 36 runs. Both reached for Secrets
Manager and `random_password`. The scanner's only hits were `format()`
strings for connection URLs, which it now ignores. Not a differentiator, and
good news about the agent.

## Where the plugin did not help

**needs**, the analysis task, was a wash. Both arms found every fact on the
ground-truth lists for both apps, cited them with line numbers at similar
density, and cost the same. In five of six plugin runs the agent did not even
invoke the CLI; it read the code. The plugin's analyzer exists to feed the
planner, not to beat a model at reading a repository, and the table says so.

**static-site**, S3 behind CloudFront with a certificate and DNS, was
designed to be outside the catalog, and the plugin lost it the way it should
have lost it, with one wrinkle. In two of three runs the agent ran `neckbeard
analyze`, was told there was no containerised service, said plainly that
neckbeard was not usable for this, and wrote roughly 500 lines of Terraform by
hand, landing within noise of the bare arm on every metric. The wrinkle is
that the skill says the agent never hand-writes infrastructure. That rule did
not survive a task the catalog cannot serve, and the right fix is to make the
skill say what the agent actually does, which is
[#59](https://github.com/bealesh/neckbeard/issues/59). Neither arm declined
the task.

## The estimate

Both arms produced a monthly estimate every run. The plugin's came from
`neckbeard estimate`, priced through Infracost 0.10 with the cost model's
usage assumptions, provenance line included: 315 to 446 USD a month for three
environments depending on the app and the agent's override choices. The bare
arm wrote a costs file by hand from list prices every time.

The tables carry a third number so the two layouts can be compared on one
footing: a raw Infracost breakdown of each arm's dev root, no usage file, only
what is provisioned. It does not flatter the plugin. The catalog's dev
environment prices at 82 to 125 USD a month against 40 to 49 for the
hand-written one, on every app. Half of the gap is a NAT gateway: the solo
preset uses VPC interface endpoints instead, and the agent overrode that in
the plugin runs because the endpoints cost more at this size, which is a
defensible call it stated in its summary. The other half is task size: the
catalog's preset runs half-vCPU Fargate tasks and the bare agent chose
quarter-vCPU ones, at nine dollars apiece instead of eighteen. Whether a
quarter-vCPU task is the right floor for a Go service is an opinion; the
catalog's is the more conservative one, and it costs twice as much. Three bare
runs put all three environments in one root with workspaces and have no dev
figure in that column.

## Limits

Small numbers: three runs per cell, one model, one day, one account. Treat
anything inside the min to max range as noise. The apps are two fixtures and
one real product; a monorepo or a Java service might go differently. The
fact lists for the analysis task are nine and ten items written by the person
who wrote the plugin. V0 is syntax, schema and policy; nothing here deployed,
and a passing `tofu validate` has never once meant a thing deploys.

Two umami plugin runs used the released `v0.4.0-beta.1` CLI with catalog
0.4.1 rather than the head build, because the agent prepended the operator's
`~/.local/bin` to its own `PATH`. Their numbers are indistinguishable from the
third run's, and the estimate totals match to the cent, so they stay in; the
harness should pin the binary harder next time.

Infracost's 2.x CLI could not price anything on this machine because its
dashboard login had expired. The estimator and the scorer both prefer the 0.10
line, which worked, so the cost column is filled; a machine with only 2.x and
no login would show it empty.

The bare arm's prompt said "do not ask me questions" and so did the plugin's,
because print mode cannot answer them. The plugin's normal flow pauses for
confirmation twice. A benchmark that let it would be slower and probably
better, and this one did not.
