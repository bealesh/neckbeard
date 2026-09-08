# Private-by-default network (DESIGN §10.1). Both workload subnets are delegated
# to their platform service; nothing here gets a public IP except the optional NAT
# gateway. Unlike AWS/GCP there are no per-zone subnets: Azure subnets are regional
# (honest per-cloud difference, §3.3), so var.zones does not shape this module.

resource "azurerm_virtual_network" "this" {
  name                = "${var.name_prefix}-vnet"
  location            = var.region
  resource_group_name = var.resource_group_name
  address_space       = ["10.0.0.0/16"]
}

# Container Apps environment subnet. A workload-profiles environment requires the
# subnet to be delegated to Microsoft.App/environments and at least /27; we use /23
# so the environment never runs out of infrastructure addresses as apps scale.
resource "azurerm_subnet" "apps" {
  name                 = "${var.name_prefix}-apps"
  resource_group_name  = var.resource_group_name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = ["10.0.0.0/23"]

  delegation {
    name = "containerapps"
    service_delegation {
      name    = "Microsoft.App/environments"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

# PostgreSQL flexible server subnet: private access requires a subnet delegated to
# the service; the server gets no public endpoint at all.
resource "azurerm_subnet" "db" {
  name                 = "${var.name_prefix}-db"
  resource_group_name  = var.resource_group_name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = ["10.0.2.0/24"]

  delegation {
    name = "postgres"
    service_delegation {
      name    = "Microsoft.DBforPostgreSQL/flexibleServers"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

# Private DNS zone for the flexible server's VNet-injected endpoint: the server's
# FQDN resolves only inside linked VNets — the private-by-default control for the
# database (§10.1).
resource "azurerm_private_dns_zone" "postgres" {
  name                = "${var.name_prefix}.postgres.database.azure.com"
  resource_group_name = var.resource_group_name
}

resource "azurerm_private_dns_zone_virtual_network_link" "postgres" {
  name                  = "${var.name_prefix}-pg-link"
  resource_group_name   = var.resource_group_name
  private_dns_zone_name = azurerm_private_dns_zone.postgres.name
  virtual_network_id    = azurerm_virtual_network.this.id
}

# NAT gateway for deterministic egress from the apps subnet. Honest per-cloud
# difference (§3.3): an Azure NAT gateway attaches to subnets, not zones, and a
# subnet takes at most one — so "single" and "per-zone" are the SAME topology here
# (one gateway, one public IP). The strategy names stay for cross-cloud preset
# symmetry; the cost report prices them identically on Azure.
# With nat_strategy = "none", outbound flows use Container Apps' platform-managed
# egress (non-deterministic source IPs) rather than having no egress at all —
# another difference from the AWS lane, where "none" means no internet path.
locals {
  nat_count = var.nat_strategy == "none" ? 0 : 1
}

resource "azurerm_public_ip" "nat" {
  count               = local.nat_count
  name                = "${var.name_prefix}-nat-ip"
  location            = var.region
  resource_group_name = var.resource_group_name
  allocation_method   = "Static"
  sku                 = "Standard"
}

resource "azurerm_nat_gateway" "this" {
  count               = local.nat_count
  name                = "${var.name_prefix}-nat"
  location            = var.region
  resource_group_name = var.resource_group_name
  sku_name            = "Standard"
}

resource "azurerm_nat_gateway_public_ip_association" "this" {
  count                = local.nat_count
  nat_gateway_id       = azurerm_nat_gateway.this[0].id
  public_ip_address_id = azurerm_public_ip.nat[0].id
}

resource "azurerm_subnet_nat_gateway_association" "apps" {
  count          = local.nat_count
  subnet_id      = azurerm_subnet.apps.id
  nat_gateway_id = azurerm_nat_gateway.this[0].id
}
