### onboard · bellwether

| metric | with neckbeard (n=3) | without (n=3) |
|---|---|---|
| cost, USD | 4.96 (4.90–5.77) | 7.62 (7.59–7.86) |
| wall time, minutes | 11.9 (9.8–12.6) | 24.2 (20.6–25.5) |
| turns | 43 (35–45) | 22 (17–23) |
| output tokens | 35480 (29433–41286) | 87130 (84301–88770) |
| input tokens incl. cache reads, millions | 1.9 (1.6–2.4) | 2.4 (2.0–2.5) |
| finished without error | 3/3 | 3/3 |
| tool calls | 41 (33–43) | 21 (16–22) |
| runs that invoked the neckbeard CLI | 3/3 | 0/3 |
| produced OpenTofu | 3/3 | 3/3 |
| passes V0 (tofu validate on every root, checkov 0 failed, actionlint clean) | 3/3 | 0/3 |
| tofu validate on every root | 3/3 | 3/3 |
| tofu fmt clean | 3/3 | 3/3 |
| checkov failed checks, after any documented skips | 0 (0–0) | 42 (41–44) |
| checkov failed checks, no skips honoured | 38 (38–38) | 42 (41–44) |
| actionlint issues | 0 (0–0) | 0 (0–0) |
| IAM `Action: *` | 0 (0–0) | 0 (0–0) |
| IAM `service:*` | 1 (1–1) | 1 (1–1) |
| IAM `Resource: *` | 1 (1–1) | 1 (0–3) |
| plaintext secret hits | 0 (0–0) | 0 (0–0) |
| lines of HCL to review | 976 (976–1111) | 2426 (2133–2487) |
| lines of workflow YAML | 395 (395–395) | 349 (319–381) |
| Infracost, dev environment, USD/month | 100 (100–125) | 49 (49–49) |

### onboard · umami

| metric | with neckbeard (n=3) | without (n=3) |
|---|---|---|
| cost, USD | 6.19 (5.57–7.37) | 6.47 (6.27–7.13) |
| wall time, minutes | 12.8 (11.8–16.3) | 22.5 (18.6–25.2) |
| turns | 47 (44–60) | 15 (15–17) |
| output tokens | 44072 (39046–55003) | 82516 (74955–83068) |
| input tokens incl. cache reads, millions | 2.9 (2.0–3.3) | 1.6 (1.5–1.9) |
| finished without error | 3/3 | 3/3 |
| tool calls | 45 (42–58) | 14 (14–16) |
| runs that invoked the neckbeard CLI | 3/3 | 0/3 |
| produced OpenTofu | 3/3 | 3/3 |
| passes V0 (tofu validate on every root, checkov 0 failed, actionlint clean) | 3/3 | 0/3 |
| tofu validate on every root | 3/3 | 3/3 |
| tofu fmt clean | 3/3 | 3/3 |
| checkov failed checks, after any documented skips | 0 (0–0) | 42 (20–52) |
| checkov failed checks, no skips honoured | 44 (44–44) | 42 (20–52) |
| actionlint issues | 0 (0–0) | 0 (0–0) |
| IAM `Action: *` | 0 (0–0) | 0 (0–0) |
| IAM `service:*` | 1 (1–1) | 11 (1–14) |
| IAM `Resource: *` | 1 (1–1) | 3 (2–6) |
| plaintext secret hits | 0 (0–0) | 0 (0–0) |
| lines of HCL to review | 928 (928–1201) | 2059 (1974–2250) |
| lines of workflow YAML | 395 (395–395) | 416 (376–430) |
| Infracost, dev environment, USD/month | 82 (82–82) | 45 (41–49) |

### onboard · python

| metric | with neckbeard (n=3) | without (n=3) |
|---|---|---|
| cost, USD | 5.23 (5.16–5.32) | 5.48 (5.43–5.82) |
| wall time, minutes | 12.1 (11.5–13.8) | 17.3 (16.8–20.6) |
| turns | 40 (40–41) | 13 (13–20) |
| output tokens | 38263 (36129–38962) | 66904 (66858–75464) |
| input tokens incl. cache reads, millions | 2.0 (1.8–2.3) | 1.2 (1.1–1.9) |
| finished without error | 3/3 | 3/3 |
| tool calls | 38 (38–39) | 12 (12–19) |
| runs that invoked the neckbeard CLI | 3/3 | 0/3 |
| produced OpenTofu | 3/3 | 3/3 |
| passes V0 (tofu validate on every root, checkov 0 failed, actionlint clean) | 3/3 | 0/3 |
| tofu validate on every root | 3/3 | 3/3 |
| tofu fmt clean | 3/3 | 3/3 |
| checkov failed checks, after any documented skips | 0 (0–0) | 18 (16–45) |
| checkov failed checks, no skips honoured | 38 (38–38) | 18 (16–45) |
| actionlint issues | 0 (0–0) | 0 (0–0) |
| IAM `Action: *` | 0 (0–0) | 0 (0–0) |
| IAM `service:*` | 1 (1–1) | 2 (1–13) |
| IAM `Resource: *` | 1 (1–1) | 1 (1–6) |
| plaintext secret hits | 0 (0–0) | 0 (0–0) |
| lines of HCL to review | 1207 (1195–1237) | 1985 (1511–2156) |
| lines of workflow YAML | 395 (395–395) | 304 (292–323) |
| Infracost, dev environment, USD/month | 82 (82–107) | 40 |

### needs · bellwether

| metric | with neckbeard (n=3) | without (n=3) |
|---|---|---|
| cost, USD | 1.13 (1.06–1.57) | 1.13 (1.04–1.26) |
| wall time, minutes | 1.9 (1.7–2.9) | 1.8 (1.7–2.3) |
| turns | 7 (6–15) | 6 (4–8) |
| output tokens | 7831 (6967–11857) | 7986 (7502–9639) |
| input tokens incl. cache reads, millions | 0.4 (0.3–0.8) | 0.3 (0.2–0.4) |
| finished without error | 3/3 | 3/3 |
| tool calls | 6 (5–12) | 5 (3–7) |
| runs that invoked the neckbeard CLI | 1/3 | 0/3 |
| facts found (ground truth list) | 100% (100%–100%) | 100% (100%–100%) |
| citations with line numbers | 88 (77–98) | 92 (82–110) |
| answer length, lines | 139 (121–173) | 118 (106–168) |

### needs · umami

| metric | with neckbeard (n=3) | without (n=3) |
|---|---|---|
| cost, USD | 2.83 (2.55–2.87) | 2.64 (2.62–3.00) |
| wall time, minutes | 5.0 (4.2–5.1) | 4.5 (3.9–5.3) |
| turns | 13 (13–16) | 15 (15–27) |
| output tokens | 23715 (19337–24278) | 21023 (18409–24413) |
| input tokens incl. cache reads, millions | 1.1 (1.1–1.4) | 1.3 (1.1–1.4) |
| finished without error | 3/3 | 3/3 |
| tool calls | 12 (12–15) | 14 (14–26) |
| runs that invoked the neckbeard CLI | 0/3 | 0/3 |
| facts found (ground truth list) | 100% (100%–100%) | 100% (100%–100%) |
| citations with line numbers | 172 (157–215) | 184 (162–205) |
| answer length, lines | 210 (204–225) | 208 (185–211) |

### static-site · none

| metric | with neckbeard (n=3) | without (n=3) |
|---|---|---|
| cost, USD | 1.76 (1.69–1.89) | 1.59 (1.41–1.64) |
| wall time, minutes | 4.1 (3.9–4.4) | 3.9 (3.5–4.0) |
| turns | 12 (11–12) | 6 (6–7) |
| output tokens | 16875 (16585–18405) | 17010 (15405–17625) |
| input tokens incl. cache reads, millions | 0.5 (0.5–0.6) | 0.3 (0.3–0.4) |
| finished without error | 3/3 | 3/3 |
| tool calls | 10 (9–10) | 5 (5–6) |
| runs that invoked the neckbeard CLI | 2/3 | 0/3 |
| produced OpenTofu | 3/3 | 3/3 |
| passes V0 (tofu validate on every root, checkov 0 failed, actionlint clean) | 1/3 | 0/3 |
| tofu validate on every root | 3/3 | 3/3 |
| tofu fmt clean | 3/3 | 3/3 |
| checkov failed checks, after any documented skips | 4 (0–4) | 4 (4–4) |
| checkov failed checks, no skips honoured | 4 (0–4) | 4 (4–4) |
| actionlint issues | 0 (0–0) | 0 (0–0) |
| IAM `Action: *` | 0 (0–0) | 0 (0–0) |
| IAM `service:*` | 1 (1–1) | 1 (0–1) |
| IAM `Resource: *` | 0 (0–0) | 0 (0–0) |
| plaintext secret hits | 0 (0–0) | 0 (0–0) |
| lines of HCL to review | 498 (471–526) | 503 (491–531) |
| lines of workflow YAML | 71 (67–73) | 75 (68–87) |
| Infracost, dev environment, USD/month | 0 (0–0) | 0 (0–0) |
| declined the task by name | 0/3 | 0/3 |

