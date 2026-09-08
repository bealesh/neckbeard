# Private-by-default network (DESIGN §10.1): workloads live in private subnets;
# public subnets exist only for the load balancer and NAT gateways.

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  az_names  = slice(data.aws_availability_zones.available.names, 0, var.zones)
  nat_count = var.nat_strategy == "none" ? 0 : (var.nat_strategy == "single" ? 1 : var.zones)
}

resource "aws_vpc" "this" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags                 = { Name = "${var.name_prefix}-vpc" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name_prefix}-igw" }
}

resource "aws_subnet" "public" {
  count                   = var.zones
  vpc_id                  = aws_vpc.this.id
  cidr_block              = cidrsubnet(aws_vpc.this.cidr_block, 8, count.index)
  availability_zone       = local.az_names[count.index]
  map_public_ip_on_launch = false
  tags                    = { Name = "${var.name_prefix}-public-${local.az_names[count.index]}" }
}

resource "aws_subnet" "private" {
  count             = var.zones
  vpc_id            = aws_vpc.this.id
  cidr_block        = cidrsubnet(aws_vpc.this.cidr_block, 8, count.index + 100)
  availability_zone = local.az_names[count.index]
  tags              = { Name = "${var.name_prefix}-private-${local.az_names[count.index]}" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name_prefix}-public" }
}

resource "aws_route" "public_internet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.this.id
}

resource "aws_route_table_association" "public" {
  count          = var.zones
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_eip" "nat" {
  count  = local.nat_count
  domain = "vpc"
  tags   = { Name = "${var.name_prefix}-nat-${count.index}" }
}

resource "aws_nat_gateway" "this" {
  count         = local.nat_count
  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id
  tags          = { Name = "${var.name_prefix}-nat-${count.index}" }
  depends_on    = [aws_internet_gateway.this]
}

resource "aws_route_table" "private" {
  count  = var.zones
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name_prefix}-private-${local.az_names[count.index]}" }
}

resource "aws_route" "private_nat" {
  count                  = var.nat_strategy == "none" ? 0 : var.zones
  route_table_id         = aws_route_table.private[count.index].id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.this[var.nat_strategy == "single" ? 0 : count.index].id
}

resource "aws_route_table_association" "private" {
  count          = var.zones
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}

# With nat_strategy = "none" (solo tier) private subnets have no internet path, so
# the AWS services Fargate needs — image pull, logs, secrets — are reached through
# VPC endpoints instead. Interface endpoints bill hourly; at 1 AZ that lands close
# to a NAT gateway, and the cost report shows both so the tradeoff is visible.
locals {
  interface_endpoints = var.nat_strategy == "none" ? toset(["ecr.api", "ecr.dkr", "logs", "secretsmanager"]) : toset([])
}

data "aws_region" "current" {}

resource "aws_security_group" "vpc_endpoints" {
  count       = var.nat_strategy == "none" ? 1 : 0
  name_prefix = "${var.name_prefix}-vpce-"
  description = "HTTPS from the VPC to interface endpoints"
  vpc_id      = aws_vpc.this.id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "vpc_endpoints_https" {
  count             = var.nat_strategy == "none" ? 1 : 0
  security_group_id = aws_security_group.vpc_endpoints[0].id
  cidr_ipv4         = aws_vpc.this.cidr_block
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_vpc_endpoint" "interface" {
  for_each            = local.interface_endpoints
  vpc_id              = aws_vpc.this.id
  service_name        = "com.amazonaws.${data.aws_region.current.region}.${each.value}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = aws_subnet.private[*].id
  security_group_ids  = [aws_security_group.vpc_endpoints[0].id]
  private_dns_enabled = true
}

# S3 gateway endpoint (free): ECR image layers download via S3.
resource "aws_vpc_endpoint" "s3" {
  count             = var.nat_strategy == "none" ? 1 : 0
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${data.aws_region.current.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = aws_route_table.private[*].id
}
