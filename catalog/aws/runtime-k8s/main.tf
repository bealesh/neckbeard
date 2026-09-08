# Kubernetes runtime on EKS: private nodes, managed node group, core addons, and
# EKS Pod Identity for workload IAM. Delivery (Flux, in-cluster ingress via
# aws-load-balancer-controller, app manifests under clusters/) is rendered by the
# clusters layer — this module is the platform only.
#
# Honest notes (DESIGN §3.3, §10):
# - The API endpoint is public at M1/M2 (private access is also on); origin
#   lockdown / allowlisting is a hardening roadmap item and the topology doc
#   says so rather than implying a private cluster.
# - A dedicated control plane bills while idle in every environment, dev included
#   — the estimate shows it (§5 of the design resolved against shared clusters).

locals {
  instance_types = {
    small  = ["t3a.medium"]
    medium = ["m6a.large"]
  }[var.node_shape]
  # Pinned like every other version in the catalog; bumped deliberately.
  kubernetes_version = "1.33"
}

data "aws_iam_policy_document" "cluster_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "cluster" {
  name               = "${var.name_prefix}-eks-cluster"
  assume_role_policy = data.aws_iam_policy_document.cluster_assume.json
}

resource "aws_iam_role_policy_attachment" "cluster" {
  role       = aws_iam_role.cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

resource "aws_eks_cluster" "this" {
  name     = var.name_prefix
  version  = local.kubernetes_version
  role_arn = aws_iam_role.cluster.arn

  vpc_config {
    subnet_ids              = var.private_subnet_ids
    endpoint_private_access = true
    endpoint_public_access  = true
  }

  access_config {
    authentication_mode                         = "API"
    bootstrap_cluster_creator_admin_permissions = true
  }

  # Control-plane audit trail (log volume is a named usage assumption in the
  # estimate, not a surprise).
  enabled_cluster_log_types = ["api", "audit"]

  depends_on = [aws_iam_role_policy_attachment.cluster]
}

data "aws_iam_policy_document" "node_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "nodes" {
  name               = "${var.name_prefix}-eks-nodes"
  assume_role_policy = data.aws_iam_policy_document.node_assume.json
}

resource "aws_iam_role_policy_attachment" "nodes" {
  for_each = toset([
    "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
    "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
    "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
  ])
  role       = aws_iam_role.nodes.name
  policy_arn = each.value
}

resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "${var.name_prefix}-default"
  node_role_arn   = aws_iam_role.nodes.arn
  subnet_ids      = var.private_subnet_ids
  instance_types  = local.instance_types
  capacity_type   = var.spot ? "SPOT" : "ON_DEMAND"

  scaling_config {
    min_size     = var.min_nodes
    desired_size = var.min_nodes
    max_size     = var.max_nodes
  }

  update_config {
    max_unavailable = 1
  }

  lifecycle {
    # The cluster autoscaler owns desired_size after bootstrap.
    ignore_changes = [scaling_config[0].desired_size]
  }

  depends_on = [aws_iam_role_policy_attachment.nodes]
}

# Core addons ride the cluster version's defaults; catalog majors may pin exact
# addon versions once the release matrix exercises upgrades.
resource "aws_eks_addon" "core" {
  for_each     = toset(["vpc-cni", "coredns", "kube-proxy", "eks-pod-identity-agent"])
  cluster_name = aws_eks_cluster.this.name
  addon_name   = each.value

  depends_on = [aws_eks_node_group.this]
}
