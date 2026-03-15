package discovery

import (
	"binary/internal/adapters"
)

// YAMLEntry holds the fields MergeServices needs from a config entry.
type YAMLEntry struct {
	Name   string
	Type   string
	Config adapters.ConnectionConfig
}

// MergeServices merges Docker-discovered services with YAML config entries.
// YAML entries override Docker-discovered entries when matched by name.
// YAML-only entries (not found in Docker) are appended.
func MergeServices(discovered []DiscoveredService, yamlEntries []YAMLEntry) []DiscoveredService {
	// Build lookup of YAML entries by name
	yamlByName := make(map[string]YAMLEntry, len(yamlEntries))
	for _, entry := range yamlEntries {
		yamlByName[entry.Name] = entry
	}

	// Track which YAML entries have been matched
	matched := make(map[string]bool)

	// Apply YAML overrides to Docker-discovered services
	result := make([]DiscoveredService, 0, len(discovered)+len(yamlEntries))
	for _, svc := range discovered {
		if yamlEntry, ok := yamlByName[svc.Name]; ok {
			// YAML overrides Docker discovery
			matched[svc.Name] = true
			result = append(result, DiscoveredService{
				Name:        yamlEntry.Name,
				Type:        ServiceType(yamlEntry.Type),
				Config:      yamlEntry.Config,
				IPAddress:   svc.IPAddress,
				ContainerID: svc.ContainerID,
			})
		} else {
			result = append(result, svc)
		}
	}

	// Append YAML-only entries (not found in Docker)
	for _, entry := range yamlEntries {
		if !matched[entry.Name] {
			result = append(result, DiscoveredService{
				Name:   entry.Name,
				Type:   ServiceType(entry.Type),
				Config: entry.Config,
			})
		}
	}

	return result
}
