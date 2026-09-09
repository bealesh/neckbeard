package pipeline

import (
	"fmt"
	"strings"
)

// RenderGitLab emits .gitlab-ci.yml from the same model as the GitHub renderer.
// OIDC uses id_tokens; cloud credentials are derived per cloud — the AWS SDK's
// web-identity flow, GCP's STS token exchange, or azurerm's native OIDC support —
// never static keys (§10.1). prd apply is a manual, environment-bound job (§11.3).
func RenderGitLab(m Model) []byte {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }

	w("%s", generatedYAMLHeader)
	w("stages: [test, build, infra]")
	w("")
	w("variables:")
	w("  AWS_REGION: %s", m.Region)
	w("")
	w(".on_mr_or_default: &on_mr_or_default")
	w("  rules:")
	w("    - if: $CI_PIPELINE_SOURCE == \"merge_request_event\"")
	w("    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH")
	w("")
	w(".oidc: &oidc")
	w("  id_tokens:")
	w("    NECKBEARD_OIDC_TOKEN:")
	w("      aud: %s", gitlabAudience(m))
	w("")
	w("test:")
	w("  stage: test")
	w("  image: ubuntu:24.04")
	w("  <<: *on_mr_or_default")
	w("  script:")
	w("    - ./.neckbeard/hooks/test.sh")
	w("")
	w("policy:")
	w("  # The policy gate (DESIGN §10.1): checkov over every env root, honoring the")
	w("  # generated .checkov.yaml (skips are curated against this pinned version).")
	w("  stage: test")
	w("  image:")
	w("    name: bridgecrew/checkov:3.3.10")
	w("    entrypoint: [\"\"]")
	w("  rules:")
	w("    - if: $CI_PIPELINE_SOURCE == \"merge_request_event\"")
	w("      changes: [\"infra/**/*\", \".neckbeard/catalog/**/*\", \".checkov.yaml\"]")
	w("    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH")
	w("      changes: [\"infra/**/*\", \".neckbeard/catalog/**/*\", \".checkov.yaml\"]")
	w("  script:")
	for _, env := range m.Envs {
		w("    - checkov -d infra/envs/%s --quiet --compact --framework terraform --config-file .checkov.yaml", env)
		w("    - checkov -d infra/bootstrap/%s --quiet --compact --framework terraform --config-file .checkov.yaml", env)
	}
	w("")
	w("build:")
	w("  # Build once on the default branch; every environment runs this image by digest.")
	w("  stage: build")
	w("  image: docker:27")
	w("  services: [\"docker:27-dind\"]")
	w("  <<: *oidc")
	w("  environment: dev")
	w("  rules:")
	w("    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH")
	w("  variables:")
	w("    REGISTRY: $%s_DEV", VarRegistry)
	w("  script:")
	w("    - docker build -f %s -t \"neckbeard-build:$CI_COMMIT_SHA\" .", shellQuote(m.Dockerfile))
	w("    # Scan gate: HIGH/CRITICAL block (DESIGN §10.1)")
	w("    - docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy image --severity HIGH,CRITICAL --exit-code 1 \"neckbeard-build:$CI_COMMIT_SHA\"")
	w("    - |")
	w("      if [ -z \"$REGISTRY\" ]; then")
	w("        echo \"%s_DEV is not set (bootstrap pending, see docs/bootstrap.md) — build ran, push skipped\"", VarRegistry)
	w("        exit 0")
	w("      fi")
	w("      apk add --no-cache curl jq >/dev/null")
	for _, line := range gitlabRegistryLogin(m) {
		w("      %s", line)
	}
	w("      docker tag \"neckbeard-build:$CI_COMMIT_SHA\" \"$REGISTRY:$CI_COMMIT_SHA\"")
	w("      docker push \"$REGISTRY:$CI_COMMIT_SHA\"")
	w("      echo \"Pushed digest:\"")
	w("      docker inspect --format='{{index .RepoDigests 0}}' \"$REGISTRY:$CI_COMMIT_SHA\"")
	w("")
	for _, env := range m.Envs {
		w("infra-%s:", env)
		w("  stage: infra")
		w("  image:")
		w("    name: ghcr.io/opentofu/opentofu:%s # pinned by the blueprint", m.TofuVersion)
		w("    entrypoint: [\"\"]")
		w("  <<: *oidc")
		w("  environment: %s", env)
		w("  rules:")
		w("    - if: $CI_PIPELINE_SOURCE == \"merge_request_event\"")
		w("      changes: [\"infra/**/*\", \".neckbeard/catalog/**/*\", \".checkov.yaml\"]")
		if env == "prd" {
			w("    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH")
			w("      changes: [\"infra/**/*\", \".neckbeard/catalog/**/*\", \".checkov.yaml\"]")
			w("      when: manual # prd gate: deployment approvals on Premium/Ultimate; protected branch otherwise (§11.3)")
		} else {
			w("    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH")
			w("      changes: [\"infra/**/*\", \".neckbeard/catalog/**/*\", \".checkov.yaml\"]")
		}
		w("  script:")
		w("    - tofu -chdir=infra/envs/%s init -backend=false -input=false", env)
		w("    - tofu -chdir=infra/envs/%s validate", env)
		w("    - |")
		w("      if [ ! -f infra/envs/%s/backend.hcl ] || [ -z \"$%s\" ]; then", env, GateVar(m.Cloud, env))
		w("        echo \"bootstrap pending for %s (backend.hcl or CI variables missing, see docs/bootstrap.md) — validate-only run; plan/apply NOT EXERCISED\"", env)
		w("        exit 0")
		w("      fi")
		if m.Cloud == "azure" {
			w("      if [ \"$CI_PIPELINE_SOURCE\" = \"merge_request_event\" ]; then")
			w("        echo \"azure + gitlab: Entra federated credentials are exact-match (no wildcard subjects), so merge-request pipelines cannot federate — validate-only run; plan NOT EXERCISED (docs/bootstrap.md)\"")
			w("        exit 0")
			w("      fi")
		}
		for _, line := range gitlabInfraAuth(m, env) {
			w("      %s", line)
		}
		w("      tofu -chdir=infra/envs/%s init -reconfigure -input=false -backend-config=backend.hcl", env)
		w("      tofu -chdir=infra/envs/%s plan -input=false -out=tfplan", env)
		w("      if [ \"$CI_COMMIT_BRANCH\" = \"$CI_DEFAULT_BRANCH\" ]; then")
		w("        tofu -chdir=infra/envs/%s apply -input=false tfplan", env)
		w("      fi")
		w("")
	}
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}

func gitlabAudience(m Model) string {
	// Matches what the bootstrap modules configure as the accepted audience.
	return "https://gitlab.com"
}

// gitlabInfraAuth exports the credentials OpenTofu's provider + backend read,
// derived from the job's OIDC token.
func gitlabInfraAuth(m Model, env string) []string {
	envU := strings.ToUpper(env)
	apply := fmt.Sprintf(`[ "$CI_COMMIT_BRANCH" = "$CI_DEFAULT_BRANCH" ]`)
	switch m.Cloud {
	case "gcp":
		return []string{
			fmt.Sprintf(`SA="$%s_%s"; if %s; then SA="$%s_%s"; fi`, VarGCPPlanSA, envU, apply, VarGCPApplySA, envU),
			`echo "$NECKBEARD_OIDC_TOKEN" > /tmp/oidc-token`,
			// external_account credentials: the google provider exchanges the file-
			// sourced GitLab token via STS and impersonates the service account.
			fmt.Sprintf(`printf '{"type":"external_account","audience":"//iam.googleapis.com/%%s","subject_token_type":"urn:ietf:params:oauth:token-type:jwt","token_url":"https://sts.googleapis.com/v1/token","credential_source":{"file":"/tmp/oidc-token"},"service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%%s:generateAccessToken"}' "$%s_%s" "$SA" > /tmp/gcp-credentials.json`, VarGCPProvider, envU),
			`export GOOGLE_APPLICATION_CREDENTIALS=/tmp/gcp-credentials.json`,
		}
	case "azure":
		return []string{
			fmt.Sprintf(`export ARM_CLIENT_ID="$%s_%s"; if %s; then export ARM_CLIENT_ID="$%s_%s"; fi`, VarAzurePlanClient, envU, apply, VarAzureApplyClient, envU),
			fmt.Sprintf(`export ARM_TENANT_ID="$%s" ARM_SUBSCRIPTION_ID="$%s_%s"`, VarAzureTenant, VarAzureSub, envU),
			`export ARM_USE_OIDC=true ARM_OIDC_TOKEN="$NECKBEARD_OIDC_TOKEN"`,
		}
	default: // aws
		return []string{
			fmt.Sprintf(`ROLE="$%s_%s"; if %s; then ROLE="$%s_%s"; fi`, VarPlanRole, envU, apply, VarApplyRole, envU),
			`echo "$NECKBEARD_OIDC_TOKEN" > /tmp/oidc-token`,
			`export AWS_WEB_IDENTITY_TOKEN_FILE=/tmp/oidc-token AWS_ROLE_ARN="$ROLE"`,
		}
	}
}

// gitlabRegistryLogin derives a docker login from the OIDC token without cloud
// CLIs (the dind image carries only curl + jq, installed above).
func gitlabRegistryLogin(m Model) []string {
	switch m.Cloud {
	case "gcp":
		return []string{
			// GitLab token → STS federated token → SA access token → docker login.
			fmt.Sprintf(`STS=$(curl -sf -X POST https://sts.googleapis.com/v1/token -H 'Content-Type: application/json' -d "{\"audience\":\"//iam.googleapis.com/$%s_DEV\",\"grantType\":\"urn:ietf:params:oauth:grant-type:token-exchange\",\"requestedTokenType\":\"urn:ietf:params:oauth:token-type:access_token\",\"scope\":\"https://www.googleapis.com/auth/cloud-platform\",\"subjectTokenType\":\"urn:ietf:params:oauth:token-type:jwt\",\"subjectToken\":\"$NECKBEARD_OIDC_TOKEN\"}" | jq -r .access_token)`, VarGCPProvider),
			fmt.Sprintf(`TOKEN=$(curl -sf -X POST -H "Authorization: Bearer $STS" -H 'Content-Type: application/json' "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/$%s_DEV:generateAccessToken" -d '{"scope":["https://www.googleapis.com/auth/cloud-platform"]}' | jq -r .accessToken)`, VarGCPApplySA),
			`echo "$TOKEN" | docker login -u oauth2accesstoken --password-stdin "https://${REGISTRY%%/*}"`,
		}
	case "azure":
		return []string{
			// GitLab token → Entra client-assertion token → ACR refresh token → login.
			fmt.Sprintf(`AAD=$(curl -sf -X POST "https://login.microsoftonline.com/$%s/oauth2/v2.0/token" --data-urlencode "client_id=$%s_DEV" --data-urlencode "scope=https://management.azure.com/.default" --data-urlencode "client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer" --data-urlencode "client_assertion=$NECKBEARD_OIDC_TOKEN" --data-urlencode "grant_type=client_credentials" | jq -r .access_token)`, VarAzureTenant, VarAzureApplyClient),
			`ACR_HOST="${REGISTRY%%/*}"`,
			`REFRESH=$(curl -sf -X POST "https://${ACR_HOST}/oauth2/exchange" --data-urlencode "grant_type=access_token" --data-urlencode "service=${ACR_HOST}" --data-urlencode "access_token=${AAD}" | jq -r .refresh_token)`,
			`echo "$REFRESH" | docker login -u 00000000-0000-0000-0000-000000000000 --password-stdin "$ACR_HOST"`,
		}
	default: // aws
		return []string{
			`apk add --no-cache aws-cli >/dev/null`,
			`echo "$NECKBEARD_OIDC_TOKEN" > /tmp/oidc-token`,
			fmt.Sprintf(`export AWS_WEB_IDENTITY_TOKEN_FILE=/tmp/oidc-token AWS_ROLE_ARN="$%s_DEV"`, VarApplyRole),
			`aws ecr get-login-password --region "$AWS_REGION" | docker login --username AWS --password-stdin "${REGISTRY%%/*}"`,
		}
	}
}
