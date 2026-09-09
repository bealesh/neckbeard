# Serverless-containers runtime on ECS Fargate. One task definition per service;
# http services sit behind the ingress ALB, workers run detached, cron services run
# as EventBridge-scheduled tasks. Meaningful provider difference (DESIGN §3.3):
# unlike Cloud Run / Container Apps, ECS services do not scale to zero — http
# services keep a floor of one task, and the estimate shows that idle cost.

locals {
  cpu_units = {
    "0.5" = 512
    "1"   = 1024
    "2"   = 2048
  }[var.cpu]
  memory_mb = floor(var.memory_gb * 1024)

  http_services   = { for s in var.services : s.name => s if s.kind == "http" }
  worker_services = { for s in var.services : s.name => s if s.kind == "worker" }
  cron_services   = { for s in var.services : s.name => s if s.kind == "cron" }
  lb_services     = merge(local.http_services, local.worker_services)

  container_secrets = [for name, arn in var.secret_arns : { name = name, valueFrom = arn }]

  # 5-field cron → EventBridge cron(min hour dom month dow year). M1 supports
  # schedules with day-of-week "*" only; numeric day-of-week mapping differs between
  # standard cron (0-6) and AWS (1-7) and is refused rather than mistranslated.
  cron_fields = { for name, s in local.cron_services : name => split(" ", s.schedule) }
  cron_exprs = { for name, f in local.cron_fields :
    name => "cron(${f[0]} ${f[1]} ${f[2]} ${f[3]} ${f[2] == "*" ? "?" : "*"} *)"
  }
}

resource "aws_ecs_cluster" "this" {
  name = var.name_prefix

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.name_prefix}"
  retention_in_days = 30
}

data "aws_iam_policy_document" "task_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# Execution role: image pull, log delivery, and reading the container secrets.
resource "aws_iam_role" "execution" {
  name               = "${var.name_prefix}-ecs-exec"
  assume_role_policy = data.aws_iam_policy_document.task_assume.json
}

resource "aws_iam_role_policy_attachment" "execution_base" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "execution_secrets" {
  count = length(var.secret_arns) > 0 ? 1 : 0
  statement {
    actions   = ["secretsmanager:GetSecretValue"]
    resources = values(var.secret_arns)
  }
}

resource "aws_iam_role_policy" "execution_secrets" {
  count  = length(var.secret_arns) > 0 ? 1 : 0
  name   = "read-container-secrets"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution_secrets[0].json
}

# Task role: the app's runtime identity. Starts empty; app-scoped grants (e.g. the
# storage bucket) wire in at the env root.
resource "aws_iam_role" "task" {
  name               = "${var.name_prefix}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.task_assume.json
}

resource "aws_security_group" "service" {
  name_prefix = "${var.name_prefix}-svc-"
  description = "Fargate tasks for ${var.name_prefix}"
  vpc_id      = var.vpc_id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "from_alb" {
  # Unconditional: a for_each/count on an apply-time module output cannot plan on
  # a fresh environment (same class as the Azure ACR finding, review 2026-09-08).
  for_each                     = local.http_services
  security_group_id            = aws_security_group.service.id
  description                  = "service port from the ingress ALB"
  referenced_security_group_id = var.alb_security_group_id
  from_port                    = each.value.port
  to_port                      = each.value.port
  ip_protocol                  = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.service.id
  description       = "task egress (images, logs, secrets, external APIs)"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

resource "aws_ecs_task_definition" "service" {
  for_each                 = { for s in var.services : s.name => s }
  family                   = "${var.name_prefix}-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = local.cpu_units
  memory                   = local.memory_mb
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([merge({
    name      = each.key
    image     = var.image
    essential = true
    portMappings = each.value.kind == "http" ? [{
      containerPort = each.value.port
      protocol      = "tcp"
    }] : []
    secrets = local.container_secrets
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.this.name
        awslogs-region        = var.region
        awslogs-stream-prefix = each.key
      }
    }
  }, length(each.value.args) > 0 ? { command = each.value.args } : {})])

  lifecycle {
    precondition {
      condition = (
        (local.cpu_units == 512 && local.memory_mb >= 1024 && local.memory_mb <= 4096) ||
        (local.cpu_units == 1024 && local.memory_mb >= 2048 && local.memory_mb <= 8192) ||
        (local.cpu_units == 2048 && local.memory_mb >= 4096 && local.memory_mb <= 16384)
      )
      error_message = "cpu/memory is not a valid Fargate combination."
    }
    precondition {
      condition = alltrue([for name, f in local.cron_fields :
        length(f) == 5 && f[4] == "*"
      ])
      error_message = "cron schedules must be 5-field with day-of-week '*' (numeric day-of-week is refused rather than mistranslated to AWS's 1-7 convention)."
    }
  }
}

resource "aws_ecs_service" "this" {
  for_each        = local.lb_services
  name            = each.key
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.service[each.key].arn
  launch_type     = "FARGATE"
  # http services keep a floor of one task (ECS has no scale-to-zero); workers may
  # genuinely idle at zero.
  desired_count = each.value.kind == "http" ? max(var.min_instances, 1) : var.min_instances

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [aws_security_group.service.id]
    assign_public_ip = false
  }

  dynamic "load_balancer" {
    for_each = each.value.kind == "http" && contains(keys(var.target_group_arns), each.key) ? [1] : []
    content {
      target_group_arn = var.target_group_arns[each.key]
      container_name   = each.key
      container_port   = each.value.port
    }
  }

  lifecycle {
    # The release flow updates the task definition (new image digest); day-to-day
    # applies must not fight it.
    ignore_changes = [task_definition]
  }
}

resource "aws_appautoscaling_target" "service" {
  for_each           = local.http_services
  max_capacity       = var.max_instances
  min_capacity       = max(var.min_instances, 1)
  resource_id        = "service/${aws_ecs_cluster.this.name}/${aws_ecs_service.this[each.key].name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "cpu" {
  for_each           = local.http_services
  name               = "${var.name_prefix}-${each.key}-cpu"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.service[each.key].resource_id
  scalable_dimension = aws_appautoscaling_target.service[each.key].scalable_dimension
  service_namespace  = aws_appautoscaling_target.service[each.key].service_namespace

  target_tracking_scaling_policy_configuration {
    target_value = 70
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
  }
}

# Cron services: EventBridge-scheduled Fargate tasks.
data "aws_iam_policy_document" "events_assume" {
  count = length(local.cron_services) > 0 ? 1 : 0
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "events_run_task" {
  count = length(local.cron_services) > 0 ? 1 : 0
  statement {
    actions   = ["ecs:RunTask"]
    resources = [for name, td in aws_ecs_task_definition.service : td.arn]
  }
  statement {
    actions   = ["iam:PassRole"]
    resources = [aws_iam_role.execution.arn, aws_iam_role.task.arn]
  }
}

resource "aws_iam_role" "events" {
  count              = length(local.cron_services) > 0 ? 1 : 0
  name               = "${var.name_prefix}-events"
  assume_role_policy = data.aws_iam_policy_document.events_assume[0].json
}

resource "aws_iam_role_policy" "events" {
  count  = length(local.cron_services) > 0 ? 1 : 0
  name   = "run-scheduled-tasks"
  role   = aws_iam_role.events[0].id
  policy = data.aws_iam_policy_document.events_run_task[0].json
}

resource "aws_cloudwatch_event_rule" "cron" {
  for_each            = local.cron_services
  name                = "${var.name_prefix}-${each.key}"
  schedule_expression = local.cron_exprs[each.key]
}

resource "aws_cloudwatch_event_target" "cron" {
  for_each = local.cron_services
  rule     = aws_cloudwatch_event_rule.cron[each.key].name
  arn      = aws_ecs_cluster.this.arn
  role_arn = aws_iam_role.events[0].arn

  ecs_target {
    task_definition_arn = aws_ecs_task_definition.service[each.key].arn
    task_count          = 1
    launch_type         = "FARGATE"

    network_configuration {
      subnets          = var.private_subnet_ids
      security_groups  = [aws_security_group.service.id]
      assign_public_ip = false
    }
  }
}
