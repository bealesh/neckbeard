# Managed PostgreSQL (RDS). The master password is generated and stored by RDS in
# Secrets Manager (manage_master_user_password): no credential passes through
# neckbeard or version control. PITR is a function of automated backups: retention
# > 0 enables it, enforced below when the preset demands pitr.

locals {
  rds_instance_class = {
    smallest = "db.t4g.micro"
    small    = "db.t4g.small"
    medium   = "db.t4g.medium"
  }[var.instance_class]
}

resource "aws_db_subnet_group" "this" {
  name       = "${var.name_prefix}-pg"
  subnet_ids = var.private_subnet_ids
}

resource "aws_security_group" "db" {
  name_prefix = "${var.name_prefix}-pg-"
  description = "Postgres access for ${var.name_prefix}"
  vpc_id      = var.vpc_id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "from_runtime" {
  count                        = length(var.allowed_security_group_ids)
  security_group_id            = aws_security_group.db.id
  description                  = "Postgres from the runtime service security group"
  referenced_security_group_id = var.allowed_security_group_ids[count.index]
  from_port                    = 5432
  to_port                      = 5432
  ip_protocol                  = "tcp"
}

resource "aws_db_instance" "this" {
  identifier                  = "${var.name_prefix}-pg"
  engine                      = "postgres"
  engine_version              = "16"
  auto_minor_version_upgrade  = true
  instance_class              = local.rds_instance_class
  allocated_storage           = 20
  max_allocated_storage       = 100
  storage_encrypted           = true
  db_name                     = "app"
  username                    = "app"
  manage_master_user_password = true
  multi_az                    = var.multi_zone
  backup_retention_period     = var.backup_retention_days
  deletion_protection         = var.deletion_protection
  db_subnet_group_name        = aws_db_subnet_group.this.name
  vpc_security_group_ids      = [aws_security_group.db.id]
  publicly_accessible         = false
  # Postgres logs to CloudWatch; performance insights where the instance class
  # supports it (not on the smallest shared-core class).
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  performance_insights_enabled    = var.instance_class != "smallest"
  # Non-protected environments (dev/stg presets) tear down cleanly for the
  # deploy→verify→teardown release matrix; prd keeps a final snapshot.
  skip_final_snapshot       = !var.deletion_protection
  final_snapshot_identifier = var.deletion_protection ? "${var.name_prefix}-pg-final" : null

  lifecycle {
    precondition {
      condition     = !var.pitr || var.backup_retention_days > 0
      error_message = "pitr requires backup_retention_days > 0 (RDS PITR rides on automated backups)."
    }
  }
}

resource "aws_db_instance" "replica" {
  count                      = var.read_replicas
  identifier                 = "${var.name_prefix}-pg-ro-${count.index}"
  replicate_source_db        = aws_db_instance.this.identifier
  instance_class             = local.rds_instance_class
  auto_minor_version_upgrade = true
  storage_encrypted          = true
  vpc_security_group_ids     = [aws_security_group.db.id]
  publicly_accessible        = false
  skip_final_snapshot        = true
}
