package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Repository is the flattened view of one Nexus repository, used for CSV
// and table rendering. The JSON export path does NOT go through this struct
// (see ListRepositoriesRaw), so unknown attributes are never lost there.
type Repository struct {
	Name       string
	Format     string
	Type       string
	URL        string
	Version    string
	Attributes map[string]map[string]any
}

// repositoryDTO mirrors the NXRM3 GET /v1/repositories response item. It is
// unexported: it exists only to decode into Repository.
type repositoryDTO struct {
	Name       string                                `json:"name"`
	Format     string                                `json:"format"`
	Type       string                                `json:"type"`
	URL        string                                `json:"url"`
	Version    string                                `json:"version"`
	Attributes map[string]map[string]json.RawMessage `json:"attributes"`
}

// ListRepositories fetches all repositories, decodes them into Repository
// values, and returns them sorted by name for deterministic exports.
func (c *Client) ListRepositories(ctx context.Context) ([]Repository, error) {
	body, err := c.get(ctx, pathRepositories)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}

	var dtos []repositoryDTO
	if err := json.Unmarshal(body, &dtos); err != nil {
		return nil, fmt.Errorf("parse repositories response: %w", err)
	}

	repos := make([]Repository, 0, len(dtos))
	for _, dto := range dtos {
		attrs := make(map[string]map[string]any, len(dto.Attributes))
		for section, fields := range dto.Attributes {
			parsed := make(map[string]any, len(fields))
			for key, raw := range fields {
				var v any
				if err := json.Unmarshal(raw, &v); err != nil {
					return nil, fmt.Errorf("parse attribute %s.%s.%s: %w",
						dto.Name, section, key, err)
				}

				parsed[key] = v
			}

			attrs[section] = parsed
		}

		repos = append(repos, Repository{
			Name:       dto.Name,
			Format:     dto.Format,
			Type:       dto.Type,
			URL:        dto.URL,
			Version:    dto.Version,
			Attributes: attrs,
		})
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})

	return repos, nil
}

// attributeValue renders one attribute field for CSV output. Missing fields
// become empty strings; booleans render as true/false; arrays are joined
// with ";" in their JSON order.
func attributeValue(repo Repository, section, key string) string {
	fields, ok := repo.Attributes[section]
	if !ok {
		return ""
	}

	v, ok := fields[key]
	if !ok || v == nil {
		return ""
	}

	return formatAttributeValue(v)
}

// formatAttributeValue converts a decoded JSON value to its CSV text form.
func formatAttributeValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, formatAttributeValue(item))
		}

		return strings.Join(parts, ";")
	case map[string]any:
		// Nested maps are rendered as key=value pairs joined by ";" with
		// keys sorted for determinism.
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+formatAttributeValue(val[k]))
		}

		return strings.Join(parts, ";")
	default:
		return fmt.Sprintf("%v", v)
	}
}
