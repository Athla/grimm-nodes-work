package discovery

import (
	"github.com/guilherme-grimm/graph-go/internal/adapters"
)

// SourceYAML identifies services originating from YAML configuration.
const SourceYAML = "yaml"

// YAMLEntry holds the fields MergeWithYAML needs from a config entry.
type YAMLEntry struct {
	Name   string
	Type   string
	Config adapters.ConnectionConfig
}

// MergeWithYAML applies YAML overrides to discovered services. A YAML entry
// whose Name matches a discovered service overrides that service's Type and
// Config (preserving Source/Nodes/Edges/Health/Metadata). YAML entries with
// no matching discovered service are appended with Source="yaml".
func MergeWithYAML(discovered []ServiceInfo, yamlEntries []YAMLEntry) []ServiceInfo {
	yamlByName := make(map[string]YAMLEntry, len(yamlEntries))
	for _, e := range yamlEntries {
		yamlByName[e.Name] = e
	}

	matched := make(map[string]bool)
	result := make([]ServiceInfo, 0, len(discovered)+len(yamlEntries))

	for _, svc := range discovered {
		if e, ok := yamlByName[svc.Name]; ok {
			matched[svc.Name] = true
			result = append(result, ServiceInfo{
				Name:     e.Name,
				Type:     e.Type,
				Source:   svc.Source,
				Config:   e.Config,
				Nodes:    svc.Nodes,
				Edges:    svc.Edges,
				Health:   svc.Health,
				Metadata: svc.Metadata,
			})
			continue
		}
		result = append(result, svc)
	}

	for _, e := range yamlEntries {
		if matched[e.Name] {
			continue
		}
		result = append(result, ServiceInfo{
			Name:   e.Name,
			Type:   e.Type,
			Source: SourceYAML,
			Config: e.Config,
		})
	}

	return result
}
