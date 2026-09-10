package release

import "embed"

// Sources are shipped with generated CI so releases use the same tested code as
// the installed CLI, without downloading an unpinned neckbeard binary.
//
//go:embed release.go cli.go aws.go cloudrun.go azure.go kubernetes.go health.go registry.go ci.go store.go foundation.go database.go infra.go
var Sources embed.FS
