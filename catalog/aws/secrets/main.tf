# Secret containers only: neckbeard provisions the named secrets so IAM can scope
# access to the ${name_prefix}/ path. Values are set out-of-band by operators and
# never pass through neckbeard or its state (DESIGN §8, §10.1).

resource "aws_secretsmanager_secret" "this" {
  for_each = toset(var.secret_names)
  name     = "${var.name_prefix}/${each.value}"
}
