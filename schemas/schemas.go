// Package schemas embeds the language-neutral JSON Schema contracts and validates
// YAML documents against them. The .schema.json files are the source of truth for
// every durable artifact (DESIGN §9).
package schemas

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed neckbeard.schema.json
var neckbeardSchema []byte

//go:embed app-profile.schema.json
var appProfileSchema []byte

//go:embed blueprint.schema.json
var blueprintSchema []byte

//go:embed environment-manifest.schema.json
var environmentManifestSchema []byte

var raw = map[string][]byte{
	"neckbeard":            neckbeardSchema,
	"app-profile":          appProfileSchema,
	"blueprint":            blueprintSchema,
	"environment-manifest": environmentManifestSchema,
}

// Read exposes the exact schema used by this binary to agents and installers.
func Read(name string) ([]byte, error) {
	data, ok := raw[name]
	if !ok {
		return nil, fmt.Errorf("unknown schema %q (neckbeard, app-profile, blueprint, environment-manifest)", name)
	}
	return bytes.Clone(data), nil
}

func compile(name string) (*jsonschema.Schema, error) {
	data, ok := raw[name]
	if !ok {
		return nil, fmt.Errorf("unknown schema %q", name)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parsing schema %q: %w", name, err)
	}
	c := jsonschema.NewCompiler()
	url := name + ".schema.json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("adding schema %q: %w", name, err)
	}
	return c.Compile(url)
}

// ValidateYAML checks a YAML document against the named embedded schema.
func ValidateYAML(name string, yamlBytes []byte) error {
	sch, err := compile(name)
	if err != nil {
		return err
	}
	var v any
	if err := yaml.Unmarshal(yamlBytes, &v); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}
	jb, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("converting YAML to JSON: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(jb))
	if err != nil {
		return fmt.Errorf("decoding instance: %w", err)
	}
	if err := sch.Validate(inst); err != nil {
		return fmt.Errorf("schema %q: %w", name, err)
	}
	return nil
}
