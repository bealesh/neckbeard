# Kubernetes runtime on AKS: workload identity + OIDC issuer, the app-routing
# addon for managed ingress, Azure CNI overlay with network policy, and a
# federated identity for external-secrets scoped to the app's Key Vault.
# Delivery (Flux, manifests) is rendered by the clusters layer.
#
# Honest notes (DESIGN §3.3, §10):
# - The API endpoint is public at M2, same documented posture as EKS/GKE.
# - Spot: only user pools can be Spot on AKS. With spot=true the default pool is a
#   1-node system pool and a tainted Spot user pool carries workloads; the
#   delivery layer's Spot toleration is what lets pods schedule there.
# - The external-secrets client id is only known after apply: the bootstrap step
#   publishes it into the neckbeard-cluster-vars ConfigMap that Flux substitutes
#   into the controllers layer.

locals {
  vm_size = {
    small  = "Standard_D2as_v5"
    medium = "Standard_D4as_v5"
  }[var.node_shape]
  # dns_prefix: letters/digits/dashes, must start+end alphanumeric.
  dns_prefix = trimsuffix(substr(var.name_prefix, 0, 42), "-")
}

data "azurerm_client_config" "current" {}

# Baseline observability is cloud-native (DESIGN §16): control-plane and container
# logs land in a per-env Log Analytics workspace, 30-day retention (a named log
# cost in the estimate).
resource "azurerm_log_analytics_workspace" "this" {
  name                = "${var.name_prefix}-logs"
  location            = var.region
  resource_group_name = var.resource_group_name
  sku                 = "PerGB2018"
  retention_in_days   = 30
}

resource "azurerm_kubernetes_cluster" "this" {
  name                = var.name_prefix
  location            = var.region
  resource_group_name = var.resource_group_name
  dns_prefix          = local.dns_prefix

  oidc_issuer_enabled       = true
  workload_identity_enabled = true

  # SLA-backed control plane, consistent with the EKS/GKE fee; the Free tier is a
  # possible solo-tier cost lever once the harness proves the downgrade path.
  sku_tier = "Standard"

  # Patch-level auto-upgrades ride the chosen channel; minors stay deliberate.
  automatic_upgrade_channel = "patch"

  # kubectl access is Entra-only: no local admin credentials to leak.
  local_account_disabled = true
  azure_active_directory_role_based_access_control {
    azure_rbac_enabled = true
    tenant_id          = data.azurerm_client_config.current.tenant_id
  }

  oms_agent {
    log_analytics_workspace_id = azurerm_log_analytics_workspace.this.id
  }

  identity {
    type = "SystemAssigned"
  }

  default_node_pool {
    name                 = "system"
    vm_size              = local.vm_size
    vnet_subnet_id       = var.subnet_id
    max_pods             = 50
    auto_scaling_enabled = !var.spot
    # Spot mode: this pool shrinks to a fixed single system node; the Spot user
    # pool below carries the workloads.
    node_count = var.spot ? 1 : null
    min_count  = var.spot ? null : var.min_nodes
    max_count  = var.spot ? null : var.max_nodes

    upgrade_settings {
      max_surge = "10%"
    }
  }

  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_policy      = "azure"
  }

  # Managed nginx ingress (app routing addon); the delivery layer's Ingress uses
  # its class. Custom DNS zones attach post-M2.
  web_app_routing {
    dns_zone_ids = []
  }
}

resource "azurerm_kubernetes_cluster_node_pool" "spot" {
  count                 = var.spot ? 1 : 0
  name                  = "spot"
  kubernetes_cluster_id = azurerm_kubernetes_cluster.this.id
  vm_size               = local.vm_size
  vnet_subnet_id        = var.subnet_id
  mode                  = "User"

  priority        = "Spot"
  eviction_policy = "Delete"
  spot_max_price  = -1
  max_pods        = 50

  auto_scaling_enabled = true
  min_count            = var.min_nodes
  max_count            = var.max_nodes

  # AKS taints Spot pools automatically; the delivery layer tolerates it.
  node_taints = ["kubernetes.azure.com/scalesetpriority=spot:NoSchedule"]
}

# --- external-secrets identity (workload identity federation) ---

resource "azurerm_user_assigned_identity" "external_secrets" {
  name                = "${var.name_prefix}-eso"
  location            = var.region
  resource_group_name = var.resource_group_name
}

resource "azurerm_federated_identity_credential" "external_secrets" {
  name                = "external-secrets"
  resource_group_name = var.resource_group_name
  parent_id           = azurerm_user_assigned_identity.external_secrets.id
  issuer              = azurerm_kubernetes_cluster.this.oidc_issuer_url
  audience            = ["api://AzureADTokenExchange"]
  subject             = "system:serviceaccount:external-secrets:external-secrets"
}

resource "azurerm_role_assignment" "external_secrets_kv" {
  count                = var.key_vault_id == "" ? 0 : 1
  scope                = var.key_vault_id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.external_secrets.principal_id
}
