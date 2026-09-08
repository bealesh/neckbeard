variable "name_prefix" {
  description = "Resource name prefix: {org}-{app}-{env}."
  type        = string
}

variable "region" {
  description = "Region of the Cloud Run services behind the serverless NEGs."
  type        = string
}

variable "http_services" {
  description = "HTTP services to expose. The first (sorted by the planner) is the default route; the rest get path rules /<name>/*. health_path is unused here — serverless NEG backends have no LB health checks (Cloud Run manages health), an honest per-cloud difference (DESIGN §3.3)."
  type = list(object({
    name        = string
    port        = number
    health_path = string
  }))
}

variable "service_names" {
  description = "Map of http service logical name to Cloud Run service name (wired from the runtime module)."
  type        = map(string)
}

variable "waf_enabled" {
  description = "Attach a Cloud Armor policy (preconfigured SQLi/XSS rules) to the backends."
  type        = bool
}
