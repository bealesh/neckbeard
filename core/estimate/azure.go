package estimate

import (
	"fmt"
	"strings"

	"github.com/bealesh/neckbeard/core/blueprint"
)

func correctAzureDatabase(d *breakdown, bp *blueprint.Blueprint, env blueprint.Environment, lookup productLookup) ([]string, error) {
	if bp.Cloud != "azure" {
		return nil, nil
	}
	var primary *pricedResource
	existingReplicas := 0
	for _, p := range d.Projects {
		for _, r := range p.Breakdown.Resources {
			if r.ResourceType == "azurerm_federated_identity_credential" {
				r.CostComponents = nil
				r.Subresources = nil
			}
			if r.Name == "module.postgres.azurerm_postgresql_flexible_server.this" {
				primary = r
			}
			if strings.HasPrefix(r.Name, "module.postgres.azurerm_postgresql_flexible_server.replica[") {
				existingReplicas++
			}
		}
	}
	if primary == nil {
		return nil, nil
	}
	inputs := moduleInputs(env, "postgres")
	replicas, err := inputNumber(inputs, "read_replicas")
	if err != nil {
		return nil, err
	}
	if float64(existingReplicas) > replicas {
		return nil, fmt.Errorf("Postgres replica count differs from blueprint")
	}
	rate, err := azurePrice(lookup, bp.Region, "Azure Database for PostgreSQL", "Storage Data Stored", "1 GB/Month", priceAttribute{Key: "productName", Value: "Azure Database for PostgreSQL Flex Server Storage"}, priceAttribute{Key: "skuName", Value: "Storage"})
	if err != nil {
		return nil, err
	}
	ha, _ := inputs["multi_zone"].(bool)
	copies := 1.0
	if ha {
		copies = 2
	}
	var cpuRate float64
	found := false
	for _, c := range primary.CostComponents {
		if strings.HasPrefix(c.Name, "Compute (") {
			if c.Price == nil || c.PriceNotFound {
				return nil, fmt.Errorf("missing Azure Postgres compute price")
			}
			cpuRate, err = number(*c.Price, "Postgres compute price")
			if err != nil {
				return nil, err
			}
			found = true
			if err := setQuantity(c, copies*hoursPerMonth); err != nil {
				return nil, err
			}
		}
		if c.Name == "Storage" {
			c.Price = decimalString(rate)
			c.PriceNotFound = false
			if err := setQuantity(c, 32*copies); err != nil {
				return nil, err
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("missing Azure Postgres compute component")
	}
	for pi, p := range d.Projects {
		kept := []*pricedResource{}
		for _, r := range p.Breakdown.Resources {
			if !strings.HasPrefix(r.Name, "module.postgres.azurerm_postgresql_flexible_server.replica[") {
				kept = append(kept, r)
			}
		}
		d.Projects[pi].Breakdown.Resources = kept
	}
	for i := 0; i < int(replicas); i++ {
		if err := addResource(d, &pricedResource{Name: fmt.Sprintf("neckbeard.postgres_replica[%d]", i), ResourceType: "azurerm_postgresql_flexible_server", CostComponents: []*costComponent{component("Read replica compute", "hours", cpuRate, hoursPerMonth), component("Read replica storage", "GB months", rate, 32)}}); err != nil {
			return nil, err
		}
	}
	modeledInstances(d, "azurerm_postgresql_flexible_server", int(replicas)-existingReplicas)
	return []string{"Azure Postgres uses the current storage SKU and 32 GiB per copy. HA bills primary and standby compute/storage; read replicas inherit the primary SKU and storage, with no extra replica backup allocation. Federated workload identities do not allocate Entra P1/P2 user licenses."}, nil
}
