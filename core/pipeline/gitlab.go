package pipeline

import (
	"fmt"
	"strings"
)

// RenderGitLab emits .gitlab-ci.yml from the same model as the GitHub renderer.
// OIDC uses id_tokens + AWS_WEB_IDENTITY_TOKEN_FILE (the AWS SDK's native web
// identity flow), so no CLI credential dance and no static keys. prd apply is a
// manual job bound to the prd environment: deployment approvals enforce on
// Premium/Ultimate; the universal fallback is the protected default branch
// (DESIGN §11.3).
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
	w("      aud: https://gitlab.com")
	w("")
	w("test:")
	w("  stage: test")
	w("  image: ubuntu:24.04")
	w("  <<: *on_mr_or_default")
	w("  script:")
	w("    - ./.neckbeard/hooks/test.sh")
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
	w("    - docker build -f %s -t \"neckbeard-build:$CI_COMMIT_SHA\" .", m.Dockerfile)
	w("    # Scan gate: HIGH/CRITICAL block (DESIGN §10.1)")
	w("    - docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy image --severity HIGH,CRITICAL --exit-code 1 \"neckbeard-build:$CI_COMMIT_SHA\"")
	w("    - |")
	w("      if [ -z \"$REGISTRY\" ]; then")
	w("        echo \"%s_DEV is not set (bootstrap pending) — build ran, push skipped\"", VarRegistry)
	w("        exit 0")
	w("      fi")
	w("      apk add --no-cache aws-cli >/dev/null")
	w("      echo \"$NECKBEARD_OIDC_TOKEN\" > /tmp/oidc-token")
	w("      export AWS_WEB_IDENTITY_TOKEN_FILE=/tmp/oidc-token")
	w("      export AWS_ROLE_ARN=\"$%s_DEV\"", VarApplyRole)
	w("      aws ecr get-login-password --region \"$AWS_REGION\" | docker login --username AWS --password-stdin \"${REGISTRY%%%%/*}\"")
	w("      docker tag \"neckbeard-build:$CI_COMMIT_SHA\" \"$REGISTRY:$CI_COMMIT_SHA\"")
	w("      docker push \"$REGISTRY:$CI_COMMIT_SHA\"")
	w("      echo \"Pushed digest:\"")
	w("      docker inspect --format='{{index .RepoDigests 0}}' \"$REGISTRY:$CI_COMMIT_SHA\"")
	w("")
	for _, env := range m.Envs {
		envUpper := strings.ToUpper(env)
		w("infra-%s:", env)
		w("  stage: infra")
		w("  image:")
		w("    name: ghcr.io/opentofu/opentofu:%s # pinned by the blueprint", m.TofuVersion)
		w("    entrypoint: [\"\"]")
		w("  <<: *oidc")
		w("  environment: %s", env)
		w("  rules:")
		w("    - if: $CI_PIPELINE_SOURCE == \"merge_request_event\"")
		w("      changes: [\"infra/**/*\"]")
		if env == "prd" {
			w("    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH")
			w("      changes: [\"infra/**/*\"]")
			w("      when: manual # prd gate: deployment approvals on Premium/Ultimate; protected branch otherwise (§11.3)")
		} else {
			w("    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH")
			w("      changes: [\"infra/**/*\"]")
		}
		w("  script:")
		w("    - tofu -chdir=infra/envs/%s init -backend=false -input=false", env)
		w("    - tofu -chdir=infra/envs/%s validate", env)
		w("    - |")
		w("      ROLE=\"$%s_%s\"", VarPlanRole, envUpper)
		w("      if [ \"$CI_COMMIT_BRANCH\" = \"$CI_DEFAULT_BRANCH\" ]; then ROLE=\"$%s_%s\"; fi", VarApplyRole, envUpper)
		w("      if [ ! -f infra/envs/%s/backend.hcl ] || [ -z \"$ROLE\" ]; then", env)
		w("        echo \"bootstrap pending for %s (backend.hcl or CI roles missing) — validate-only run; plan/apply NOT EXERCISED\"", env)
		w("        exit 0")
		w("      fi")
		w("      echo \"$NECKBEARD_OIDC_TOKEN\" > /tmp/oidc-token")
		w("      export AWS_WEB_IDENTITY_TOKEN_FILE=/tmp/oidc-token")
		w("      export AWS_ROLE_ARN=\"$ROLE\"")
		w("      tofu -chdir=infra/envs/%s init -reconfigure -input=false -backend-config=backend.hcl", env)
		w("      tofu -chdir=infra/envs/%s plan -input=false -out=tfplan", env)
		w("      if [ \"$CI_COMMIT_BRANCH\" = \"$CI_DEFAULT_BRANCH\" ]; then")
		w("        tofu -chdir=infra/envs/%s apply -input=false tfplan", env)
		w("      fi")
		w("")
	}
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}
