package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
	"github.com/bealesh/neckbeard/core/ownership"
)

// k8sDelivery renders the clusters/ layer for kubernetes lanes: a kustomize base
// of app manifests, per-env overlays with digest pins, and Flux delivery —
// Kustomization CRs per env, image automation into dev, PR-based digest bumps for
// stg/prd (DESIGN §11.2).
//
// Delivered here: namespaces, deployments, services, cronjobs, ingress, Flux CRs,
// and the controllers layer (aws-load-balancer-controller makes Ingress real;
// external-secrets materializes the app secret from Secrets Manager). Apps depend
// on controllers in Flux, so nothing applies against missing CRDs.
type k8sLaneConfig struct {
	ingressClass string
	// per-service ingress annotations (aws needs healthcheck-path; gce reads the
	// readinessProbe instead — an honest per-cloud difference)
	ingressAnnotations func(s blueprint.Service) []string
	// controllers-layer resource files beyond app-secrets (helm repos + releases)
	controllerFiles func(bp *blueprint.Blueprint, env blueprint.Environment) map[string][]byte
	// ClusterSecretStore for the cloud's secret manager (cluster-scoped, lives in
	// the controllers layer)
	secretStore func(bp *blueprint.Blueprint, env blueprint.Environment) []byte
	// remoteRef key for a logical secret name (aws: prefix/NAME, gcp: prefix-NAME,
	// azure: sanitized NAME)
	secretKey func(prefix, name string) string
	// extra spec appended to the controllers Flux Kustomization (e.g. Azure's
	// postBuild substitution for apply-time values like the workload-identity
	// client id, published by bootstrap into neckbeard-cluster-vars)
	controllersPostBuild string
}

var awsK8s = k8sLaneConfig{
	ingressClass: "alb",
	ingressAnnotations: func(s blueprint.Service) []string {
		return []string{
			"alb.ingress.kubernetes.io/scheme: internet-facing",
			"alb.ingress.kubernetes.io/target-type: ip",
			"alb.ingress.kubernetes.io/healthcheck-path: " + s.HealthPath,
		}
	},
	controllerFiles: awsControllerFiles,
	secretStore:     awsSecretStoreYAML,
	secretKey:       func(prefix, name string) string { return prefix + "/" + name },
}

var gcpK8s = k8sLaneConfig{
	// GKE's built-in ingress: no controller install; healthchecks ride the
	// readinessProbe.
	ingressClass:       "gce",
	ingressAnnotations: func(blueprint.Service) []string { return nil },
	controllerFiles:    gcpControllerFiles,
	secretStore:        gcpSecretStoreYAML,
	secretKey:          func(prefix, name string) string { return prefix + "-" + name },
}

var azureK8s = k8sLaneConfig{
	// AKS app-routing addon: managed nginx, no controller install.
	ingressClass:       "webapprouting.kubernetes.azure.com",
	ingressAnnotations: func(blueprint.Service) []string { return nil },
	controllerFiles:    azureControllerFiles,
	secretStore:        azureSecretStoreYAML,
	secretKey:          func(prefix, name string) string { return strings.ReplaceAll(name, "_", "-") },
	controllersPostBuild: `  postBuild:
    substituteFrom:
      # Bootstrap publishes apply-time values (external-secrets client id) here.
      - kind: ConfigMap
        name: neckbeard-cluster-vars
`,
}

func k8sDeliveryAWS(bp *blueprint.Blueprint) []ownership.File   { return k8sDelivery(bp, awsK8s) }
func k8sDeliveryGCP(bp *blueprint.Blueprint) []ownership.File   { return k8sDelivery(bp, gcpK8s) }
func k8sDeliveryAzure(bp *blueprint.Blueprint) []ownership.File { return k8sDelivery(bp, azureK8s) }

func k8sDelivery(bp *blueprint.Blueprint, cfg k8sLaneConfig) []ownership.File {
	files := []ownership.File{
		{Path: "clusters/base/apps/kustomization.yaml", Content: baseKustomization(bp), Owner: ownership.OwnerGenerated},
		{Path: "clusters/base/apps/namespace.yaml", Content: namespaceYAML(bp), Owner: ownership.OwnerGenerated},
	}
	for _, s := range bp.Services {
		switch s.Kind {
		case "http":
			files = append(files,
				ownership.File{Path: "clusters/base/apps/deployment-" + s.Name + ".yaml", Content: deploymentYAML(bp, s), Owner: ownership.OwnerGenerated},
				ownership.File{Path: "clusters/base/apps/service-" + s.Name + ".yaml", Content: serviceYAML(bp, s), Owner: ownership.OwnerGenerated},
				ownership.File{Path: "clusters/base/apps/ingress-" + s.Name + ".yaml", Content: ingressYAML(bp, s, cfg), Owner: ownership.OwnerGenerated},
			)
		case "worker":
			files = append(files,
				ownership.File{Path: "clusters/base/apps/deployment-" + s.Name + ".yaml", Content: deploymentYAML(bp, s), Owner: ownership.OwnerGenerated},
			)
		case "cron":
			files = append(files,
				ownership.File{Path: "clusters/base/apps/cronjob-" + s.Name + ".yaml", Content: cronJobYAML(bp, s), Owner: ownership.OwnerGenerated},
			)
		}
	}
	for _, env := range bp.Environments {
		dir := "clusters/" + env.Name
		files = append(files,
			ownership.File{Path: dir + "/apps.yaml", Content: fluxKustomizationYAML(bp, env.Name), Owner: ownership.OwnerGenerated},
			ownership.File{Path: dir + "/apps/kustomization.yaml", Content: envKustomization(bp, env.Name), Owner: ownership.OwnerGenerated},
			// The ExternalSecret lives in the APPS layer: it targets the app
			// namespace, which the controllers layer must not depend on (namespace
			// dependency cycle, review finding 2026-09-08). Flux's dependsOn still
			// guarantees the ESO CRD exists first.
			ownership.File{Path: dir + "/apps/external-secret.yaml", Content: externalSecretYAML(bp, env, cfg), Owner: ownership.OwnerGenerated},
			ownership.File{Path: dir + "/controllers.yaml", Content: fluxControllersYAML(bp, env.Name, cfg), Owner: ownership.OwnerGenerated},
			ownership.File{Path: dir + "/controllers/app-secrets.yaml", Content: cfg.secretStore(bp, env), Owner: ownership.OwnerGenerated},
		)
		ctrl := cfg.controllerFiles(bp, env)
		names := []string{"app-secrets.yaml"}
		for name := range ctrl {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if content, ok := ctrl[name]; ok {
				files = append(files, ownership.File{Path: dir + "/controllers/" + name, Content: content, Owner: ownership.OwnerGenerated})
			}
		}
		files = append(files, ownership.File{Path: dir + "/controllers/kustomization.yaml", Content: controllersKustomization(names), Owner: ownership.OwnerGenerated})
		if env.Name == "dev" {
			// Image automation commits fresh digests to dev only; stg/prd move by
			// promotion PR (§11.2).
			files = append(files, ownership.File{Path: "clusters/dev/image-automation.yaml", Content: imageAutomationYAML(bp), Owner: ownership.OwnerGenerated})
		}
	}
	return files
}

const generatedYAMLManifestHeader = "# Generated by neckbeard — do not edit. Regeneration refuses modified files;\n# change neckbeard.yaml or app-profile.yaml and re-run plan + scaffold.\n"

// placeholderImage keeps manifests deployable-shaped before the first release;
// Flux image automation (dev) and promotion PRs (stg/prd) own the real digests.
// The kustomize images transformer matches on the tag-less repo name.
const (
	placeholderRepo  = "public.ecr.aws/docker/library/busybox"
	placeholderImage = placeholderRepo + ":stable"
)

func appNS(bp *blueprint.Blueprint) string { return bp.App }

func namespaceYAML(bp *blueprint.Blueprint) []byte {
	return fmt.Appendf(nil, `%sapiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    managed-by: neckbeard
`, generatedYAMLManifestHeader, appNS(bp))
}

func baseKustomization(bp *blueprint.Blueprint) []byte {
	var b strings.Builder
	b.WriteString(generatedYAMLManifestHeader)
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - namespace.yaml\n")
	for _, s := range bp.Services {
		switch s.Kind {
		case "http":
			fmt.Fprintf(&b, "  - deployment-%s.yaml\n  - service-%s.yaml\n  - ingress-%s.yaml\n", s.Name, s.Name, s.Name)
		case "worker":
			fmt.Fprintf(&b, "  - deployment-%s.yaml\n", s.Name)
		case "cron":
			fmt.Fprintf(&b, "  - cronjob-%s.yaml\n", s.Name)
		}
	}
	return []byte(b.String())
}

func containerYAML(bp *blueprint.Blueprint, s blueprint.Service, indent string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, indent+f+"\n", a...) }
	w("- name: %s", s.Name)
	// No image-automation marker here: the env overlay's images transformer owns
	// the pin, so the setter markers live there (review finding 2026-09-08).
	w("  image: %s", placeholderImage)
	quoted := make([]string, len(s.Args))
	for i, a := range s.Args {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	if len(quoted) > 0 {
		w("  args: [%s]", strings.Join(quoted, ", "))
	}
	w("  securityContext:")
	w("    allowPrivilegeEscalation: false")
	w("    readOnlyRootFilesystem: true")
	w("    runAsNonRoot: true")
	w("    capabilities:")
	w("      drop: [\"ALL\"]")
	w("  resources:")
	w("    requests: { cpu: 250m, memory: 256Mi }")
	w("    limits: { cpu: \"1\", memory: 1Gi }")
	// The secret is materialized by external-secrets (controllers layer, next):
	// until then pods stay Pending — fail-closed by design.
	w("  envFrom:")
	w("    - secretRef:")
	w("        name: %s-secrets", bp.App)
	if s.Kind == "http" {
		w("  ports:")
		w("    - containerPort: %d", s.Port)
		w("  readinessProbe:")
		w("    httpGet: { path: %s, port: %d }", s.HealthPath, s.Port)
		w("  livenessProbe:")
		w("    httpGet: { path: %s, port: %d }", s.HealthPath, s.Port)
		w("    initialDelaySeconds: 10")
	}
	return b.String()
}

func deploymentYAML(bp *blueprint.Blueprint, s blueprint.Service) []byte {
	replicas := 1
	return fmt.Appendf(nil, `%sapiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      # Tolerates AKS Spot pools; a no-op on clusters without that taint.
      tolerations:
        - { key: kubernetes.azure.com/scalesetpriority, operator: Equal, value: spot, effect: NoSchedule }
      containers:
%s`, generatedYAMLManifestHeader, s.Name, appNS(bp), replicas, s.Name, s.Name, containerYAML(bp, s, "        "))
}

func serviceYAML(bp *blueprint.Blueprint, s blueprint.Service) []byte {
	return fmt.Appendf(nil, `%sapiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app: %s
  ports:
    - port: 80
      targetPort: %d
`, generatedYAMLManifestHeader, s.Name, appNS(bp), s.Name, s.Port)
}

func ingressYAML(bp *blueprint.Blueprint, s blueprint.Service, cfg k8sLaneConfig) []byte {
	annotations := ""
	if lines := cfg.ingressAnnotations(s); len(lines) > 0 {
		annotations = "\n  annotations:"
		for _, l := range lines {
			annotations += "\n    " + l
		}
	}
	return fmt.Appendf(nil, `%sapiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  namespace: %s%s
spec:
  ingressClassName: %s
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: %s
                port:
                  number: 80
`, generatedYAMLManifestHeader, s.Name, appNS(bp), annotations, cfg.ingressClass, s.Name)
}

func cronJobYAML(bp *blueprint.Blueprint, s blueprint.Service) []byte {
	return fmt.Appendf(nil, `%sapiVersion: batch/v1
kind: CronJob
metadata:
  name: %s
  namespace: %s
spec:
  schedule: %q
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: Never
          tolerations:
            - { key: kubernetes.azure.com/scalesetpriority, operator: Equal, value: spot, effect: NoSchedule }
          containers:
%s`, generatedYAMLManifestHeader, s.Name, appNS(bp), s.Schedule, containerYAML(bp, s, "            "))
}

func envKustomization(bp *blueprint.Blueprint, env string) []byte {
	// Image-automation setter markers live on THESE lines (dev only): the
	// ImageUpdateAutomation walks ./clusters/dev, so markers anywhere else are
	// invisible to it. stg/prd move only via promotion PRs (DESIGN §11.2).
	tagLine := "    newTag: bootstrap-pending"
	nameLine := "  - name: " + placeholderRepo
	if env == "dev" {
		nameLine += "\n    newName: " + placeholderRepo + " # {\"$imagepolicy\": \"flux-system:" + bp.App + ":name\"}"
		tagLine += " # {\"$imagepolicy\": \"flux-system:" + bp.App + ":tag\"}"
	}
	return fmt.Appendf(nil, `%sapiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base/apps
  - external-secret.yaml
images:
  # The release flow owns this pin: Flux image automation rewrites it on dev;
  # stg/prd move only via promotion PRs bumping the digest (DESIGN §11.2).
%s
%s
`, generatedYAMLManifestHeader, nameLine, tagLine)
}

func fluxKustomizationYAML(bp *blueprint.Blueprint, env string) []byte {
	prune := "true"
	return fmt.Appendf(nil, `%s# Applied by flux bootstrap --path=clusters/%s (bootstrap docs land with M3).
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: %s-apps
  namespace: flux-system
spec:
  interval: 5m
  dependsOn:
    - name: %s-controllers
  path: ./clusters/%s/apps
  prune: %s
  sourceRef:
    kind: GitRepository
    name: flux-system
  timeout: 3m
  wait: true
`, generatedYAMLManifestHeader, env, bp.App, bp.App, env, prune)
}

// fluxControllersYAML is the controllers Kustomization; the apps Kustomization
// depends on it so app manifests never race controller CRDs.
func fluxControllersYAML(bp *blueprint.Blueprint, env string, cfg k8sLaneConfig) []byte {
	return fmt.Appendf(nil, `%sapiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: %s-controllers
  namespace: flux-system
spec:
  interval: 10m
  path: ./clusters/%s/controllers
  prune: true
  sourceRef:
    kind: GitRepository
    name: flux-system
  timeout: 5m
  wait: true
%s`, generatedYAMLManifestHeader, bp.App, env, cfg.controllersPostBuild)
}

func controllersKustomization(names []string) []byte {
	var b strings.Builder
	b.WriteString(generatedYAMLManifestHeader)
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  - %s\n", n)
	}
	return []byte(b.String())
}

func awsControllerFiles(bp *blueprint.Blueprint, env blueprint.Environment) map[string][]byte {
	return map[string][]byte{
		"helm-repositories.yaml": awsHelmRepositoriesYAML(),
		"alb-controller.yaml":    albControllerYAML(bp, env.Name),
		"external-secrets.yaml":  externalSecretsHelmYAML("external-secrets"),
	}
}

func awsHelmRepositoriesYAML() []byte {
	return []byte(generatedYAMLManifestHeader + `apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: eks-charts
  namespace: flux-system
spec:
  interval: 1h
  url: https://aws.github.io/eks-charts
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: external-secrets
  namespace: flux-system
spec:
  interval: 1h
  url: https://charts.external-secrets.io
`)
}

func albControllerYAML(bp *blueprint.Blueprint, env string) []byte {
	clusterName := fmt.Sprintf("%s-%s-%s", bp.Org, bp.App, env)
	return fmt.Appendf(nil, `%s# Makes the ALB-class Ingress real. Identity: EKS Pod Identity association
# created by the runtime-k8s module for kube-system/aws-load-balancer-controller.
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: aws-load-balancer-controller
  namespace: kube-system
spec:
  interval: 30m
  chart:
    spec:
      chart: aws-load-balancer-controller
      # TODO(catalog): pin exact once the release harness exercises upgrades.
      version: ">=1.8.0"
      sourceRef:
        kind: HelmRepository
        name: eks-charts
        namespace: flux-system
  values:
    clusterName: %s
    region: %s
    serviceAccount:
      create: true
      name: aws-load-balancer-controller
`, generatedYAMLManifestHeader, clusterName, bp.Region)
}

func externalSecretsHelmYAML(extraValues string) []byte {
	values := "    installCRDs: true\n"
	if extraValues != "external-secrets" && extraValues != "" {
		values += extraValues
	}
	return fmt.Appendf(nil, `%s# Materializes the app secret from the cloud secrets manager. Identity comes
# from the runtime-k8s module (EKS Pod Identity / GKE Workload Identity).
apiVersion: v1
kind: Namespace
metadata:
  name: external-secrets
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: external-secrets
  namespace: external-secrets
spec:
  interval: 30m
  chart:
    spec:
      chart: external-secrets
      # TODO(catalog): pin exact once the release harness exercises upgrades.
      version: ">=0.10.0"
      sourceRef:
        kind: HelmRepository
        name: external-secrets
        namespace: flux-system
  values:
%s`, generatedYAMLManifestHeader, values)
}

// gcpControllerFiles: GKE ingress is built-in (no ALB controller); only
// external-secrets installs, with its KSA annotated to the Workload Identity GSA
// the runtime-k8s module creates (account id replicated from the module's
// derivation: substr("<prefix>-eso", 0, 30)).
func gcpControllerFiles(bp *blueprint.Blueprint, env blueprint.Environment) map[string][]byte {
	prefix := fmt.Sprintf("%s-%s-%s", bp.Org, bp.App, env.Name)
	gsa := prefix + "-eso"
	if len(gsa) > 30 {
		gsa = gsa[:30]
	}
	extraValues := fmt.Sprintf("    serviceAccount:\n      annotations:\n        iam.gke.io/gcp-service-account: %s@%s.iam.gserviceaccount.com\n", gsa, env.Container)
	return map[string][]byte{
		"helm-repositories.yaml": gcpHelmRepositoriesYAML(),
		"external-secrets.yaml":  externalSecretsHelmYAML(extraValues),
	}
}

func gcpHelmRepositoriesYAML() []byte {
	return []byte(generatedYAMLManifestHeader + `apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: external-secrets
  namespace: flux-system
spec:
  interval: 1h
  url: https://charts.external-secrets.io
`)
}

// gcpSecretStoreYAML: Secret Manager provider (Workload Identity auth).
func gcpSecretStoreYAML(bp *blueprint.Blueprint, env blueprint.Environment) []byte {
	return fmt.Appendf(nil, `%sapiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: %s
spec:
  provider:
    gcpsm:
      projectID: %s
`, generatedYAMLManifestHeader, bp.App, env.Container)
}

func azureControllerFiles(bp *blueprint.Blueprint, env blueprint.Environment) map[string][]byte {
	extraValues := "    serviceAccount:\n      annotations:\n        azure.workload.identity/client-id: ${EXTERNAL_SECRETS_CLIENT_ID}\n    podLabels:\n      azure.workload.identity/use: \"true\"\n"
	return map[string][]byte{
		"helm-repositories.yaml": gcpHelmRepositoriesYAML(), // external-secrets repo only, same as gcp
		"external-secrets.yaml":  externalSecretsHelmYAML(extraValues),
	}
}

// azureVaultName replicates catalog/azure/secrets: trimsuffix(substr(prefix,0,24),"-").
func azureVaultName(prefix string) string {
	if len(prefix) > 24 {
		prefix = prefix[:24]
	}
	return strings.TrimSuffix(prefix, "-")
}

// azureSecretStoreYAML: Key Vault provider over workload identity. Vault secret
// names forbid underscores; the lane's secretKey sanitizes (DATABASE_URL lives as
// DATABASE-URL) while env var names keep the original spelling.
func azureSecretStoreYAML(bp *blueprint.Blueprint, env blueprint.Environment) []byte {
	prefix := fmt.Sprintf("%s-%s-%s", bp.Org, bp.App, env.Name)
	return fmt.Appendf(nil, `%sapiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: %s
spec:
  provider:
    azurekv:
      authType: WorkloadIdentity
      vaultUrl: https://%s.vault.azure.net
      serviceAccountRef:
        name: external-secrets
        namespace: external-secrets
`, generatedYAMLManifestHeader, bp.App, azureVaultName(prefix))
}

// externalSecretYAML materializes <app>-secrets in the app namespace from the
// cloud store. Lives in the APPS layer (the namespace exists there); the ESO CRD
// ordering comes from the apps→controllers dependsOn.
func externalSecretYAML(bp *blueprint.Blueprint, env blueprint.Environment, cfg k8sLaneConfig) []byte {
	prefix := fmt.Sprintf("%s-%s-%s", bp.Org, bp.App, env.Name)
	var b strings.Builder
	fmt.Fprintf(&b, `%sapiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: %s-secrets
  namespace: %s
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: %s
  target:
    name: %s-secrets
  data:
`, generatedYAMLManifestHeader, bp.App, appNS(bp), bp.App, bp.App)
	for _, name := range secretNames(env) {
		fmt.Fprintf(&b, "    - secretKey: %s\n      remoteRef:\n        key: %s\n", name, cfg.secretKey(prefix, name))
	}
	return []byte(b.String())
}

// awsSecretStoreYAML: the cluster-scoped Secrets Manager store. Values stay in
// the cloud store; only names are referenced (DESIGN §8, §10.1). Until operators
// set values, pods stay Pending — fail-closed by design.
func awsSecretStoreYAML(bp *blueprint.Blueprint, env blueprint.Environment) []byte {
	return fmt.Appendf(nil, `%sapiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: %s
spec:
  provider:
    aws:
      service: SecretsManager
      region: %s
`, generatedYAMLManifestHeader, bp.App, bp.Region)
}

// secretNames pulls the derived secret_names input off the env's secrets module.
func secretNames(env blueprint.Environment) []string {
	for _, m := range env.Modules {
		if m.Name != "secrets" {
			continue
		}
		for _, in := range m.Inputs {
			if in.Key != "secret_names" {
				continue
			}
			switch v := in.Value.(type) {
			case []string:
				return v
			case []any:
				out := make([]string, 0, len(v))
				for _, e := range v {
					if s, ok := e.(string); ok {
						out = append(out, s)
					}
				}
				return out
			}
		}
	}
	return nil
}

func imageAutomationYAML(bp *blueprint.Blueprint) []byte {
	return fmt.Appendf(nil, `%s# Image automation: fresh digests land on dev automatically; stg/prd move only
# via promotion PRs (DESIGN §11.2). The registry URL is completed at bootstrap
# (account-specific) — flux bootstrap docs cover it.
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageRepository
metadata:
  name: %s
  namespace: flux-system
spec:
  image: REGISTRY_URL_FROM_BOOTSTRAP/%s
  interval: 2m
---
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImagePolicy
metadata:
  name: %s
  namespace: flux-system
spec:
  imageRepositoryRef:
    name: %s
  filterTags:
    pattern: '^[0-9a-f]{40}$' # immutable commit-SHA tags only
  policy:
    alphabetical:
      order: asc
---
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageUpdateAutomation
metadata:
  name: %s
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: flux-system
  git:
    checkout:
      ref:
        branch: main
    commit:
      author:
        name: neckbeard-flux
        email: flux@neckbeard.local
      messageTemplate: "dev: image update by Flux image automation"
    push:
      branch: main
  update:
    path: ./clusters/dev
    strategy: Setters
`, generatedYAMLManifestHeader, bp.App, bp.App, bp.App, bp.App, bp.App)
}
