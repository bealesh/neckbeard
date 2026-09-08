.PHONY: build test golden-update fmt vet

build:
	go build -o bin/neckbeard ./cmd/neckbeard

test:
	go test ./...

golden-update:
	go test ./core/planner -run TestGoldenBlueprint -update

fmt:
	gofmt -w .

vet:
	go vet ./...
