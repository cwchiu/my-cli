package falconcis

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// API path constants for the Falcon container-compliance endpoints.
const (
	pathFrameworks  = "/container-compliance/aggregates/compliance-by-framework/v2"
	pathRules       = "/container-compliance/aggregates/rules/v2"
	pathRuleDetails = "/container-compliance/combined/rule-details-by-rule-ids/v1"
	pathFindings    = "/container-compliance/combined/findings-by-nodes/v2"
	paramFilter     = "filter"
	paramIDs        = "ids"
	paramLimit      = "limit"
	paramOffset     = "offset"
	rulePageSize    = 500
	ruleIDBatchSize = 100
)

// FrameworkSummary is one entry of the compliance-by-framework aggregate.
type FrameworkSummary struct {
	FrameworkName    string `json:"framework_name"`
	FrameworkVersion string `json:"framework_version"`
	PassedCount      int    `json:"passed_count"`
	FailedCount      int    `json:"failed_count"`
}

type ruleStatus struct {
	ID                   string `json:"id"`
	FrameworkNameVersion string `json:"framework_name_version"`
	FrameworkName        string `json:"framework_name"`
	FrameworkVersion     string `json:"framework_version"`
	Name                 string `json:"name"`
	RecommendationID     string `json:"recommendation_id"`
	Severity             int    `json:"severity"`
	AssetType            string `json:"asset_type"`
	Status               string `json:"status"`
}

type frameworkBucket struct {
	FrameworkNameVersion    string       `json:"framework_name_version"`
	FrameworkName           string       `json:"framework_name"`
	FrameworkVersion        string       `json:"framework_version"`
	FailedRulesCount        int          `json:"failed_rules_count"`
	PassedRulesCount        int          `json:"passed_rules_count"`
	TotalRulesCount         int          `json:"total_rules_count"`
	PercentageOfPassedRules float64      `json:"percentage_of_passed_rules"`
	RuleStatusList          []ruleStatus `json:"rule_status_list"`
}

// frameworksResponse is the payload of compliance-by-framework/v2.
type frameworksResponse struct {
	Resources []struct {
		Name    string            `json:"name"`
		Buckets []frameworkBucket `json:"buckets"`
	} `json:"resources"`
}

// RuleCompliance is one rule assessment from /container-compliance/aggregates/rules/v2.
type RuleCompliance struct {
	ID                            string  `json:"id"`
	FrameworkNameVersion          string  `json:"framework_name_version"`
	FrameworkName                 string  `json:"framework_name"`
	FrameworkVersion              string  `json:"framework_version"`
	Name                          string  `json:"name"`
	RecommendationID              string  `json:"recommendation_id"`
	Severity                      int     `json:"severity"`
	AssetType                     string  `json:"asset_type"`
	PassedAssessmentCount         int     `json:"passed_assessment_count"`
	FailedAssessmentCount         int     `json:"failed_assessment_count"`
	TotalAssessmentCount          int     `json:"total_assessment_count"`
	PercentageOfPassedAssessments float64 `json:"percentage_of_passed_assessments"`
}

// FailedRule is an alias for backwards compatibility.
type FailedRule = RuleCompliance

// ProbeRule is an alias for backwards compatibility.
type ProbeRule = RuleCompliance

// rulesResponse is the payload of aggregates/rules/v2.
type rulesResponse struct {
	Meta struct {
		Pagination struct {
			Offset int `json:"offset"`
			Limit  int `json:"limit"`
			Total  int `json:"total"`
		} `json:"pagination"`
	} `json:"meta"`
	Resources []struct {
		Name    string           `json:"name"`
		Buckets []RuleCompliance `json:"buckets"`
	} `json:"resources"`
}

// RuleDetail is the metadata of one rule from rule-details-by-rule-ids/v1.
type RuleDetail struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Audit       string `json:"audit"`
	Remediation string `json:"remediation"`
}

// ruleDetailsResponse is the payload of rule-details-by-rule-ids/v1.
type ruleDetailsResponse struct {
	Resources []RuleDetail `json:"resources"`
}

// ListFrameworks returns the assessed compliance frameworks, sorted by name
// and version. This API exposes a bucket per framework/version with already
// aggregated pass/fail counts.
func (c *Client) ListFrameworks(ctx context.Context) ([]FrameworkSummary, error) {
	var resp frameworksResponse
	if err := c.getJSON(ctx, pathFrameworks, nil, &resp); err != nil {
		return nil, fmt.Errorf("list frameworks: %w", err)
	}

	summaries := make([]FrameworkSummary, 0)

	for _, resource := range resp.Resources {
		for _, bucket := range resource.Buckets {
			summaries = append(summaries, FrameworkSummary{
				FrameworkName:    bucket.FrameworkName,
				FrameworkVersion: bucket.FrameworkVersion,
				PassedCount:      bucket.PassedRulesCount,
				FailedCount:      bucket.FailedRulesCount,
			})
		}
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].FrameworkName != summaries[j].FrameworkName {
			return summaries[i].FrameworkName < summaries[j].FrameworkName
		}

		return summaries[i].FrameworkVersion < summaries[j].FrameworkVersion
	})

	return summaries, nil
}

// ListRules returns all compliance rule assessments from aggregates/rules/v2,
// paginating through all available pages and optionally filtering by framework name.
func (c *Client) ListRules(ctx context.Context, frameworkFilter string) ([]RuleCompliance, error) {
	var allRules []RuleCompliance

	offset := 0

	for {
		query := url.Values{}
		query.Set(paramLimit, strconv.Itoa(rulePageSize))
		query.Set(paramOffset, strconv.Itoa(offset))

		var resp rulesResponse
		if err := c.getJSON(ctx, pathRules, query, &resp); err != nil {
			return nil, fmt.Errorf("list rules: %w", err)
		}

		if len(resp.Resources) == 0 {
			break
		}

		fetched := 0

		for _, resource := range resp.Resources {
			for _, bucket := range resource.Buckets {
				fetched++

				if frameworkFilter != "" &&
					!strings.Contains(strings.ToLower(bucket.FrameworkName), strings.ToLower(frameworkFilter)) &&
					!strings.Contains(strings.ToLower(bucket.FrameworkNameVersion), strings.ToLower(frameworkFilter)) {
					continue
				}

				allRules = append(allRules, bucket)
			}
		}

		if fetched == 0 {
			break
		}

		offset += fetched
		if resp.Meta.Pagination.Total > 0 && offset >= resp.Meta.Pagination.Total {
			break
		}
	}

	sort.Slice(allRules, func(i, j int) bool {
		return allRules[i].ID < allRules[j].ID
	})

	return allRules, nil
}

// ListFailedRules returns the failed checks from aggregates/rules/v2.
func (c *Client) ListFailedRules(ctx context.Context, frameworkFilter string) ([]RuleCompliance, error) {
	rules, err := c.ListRules(ctx, frameworkFilter)
	if err != nil {
		return nil, err
	}

	var failed []RuleCompliance

	for _, rule := range rules {
		if rule.FailedAssessmentCount > 0 {
			failed = append(failed, rule)
		}
	}

	return failed, nil
}

// failedRulesFilter builds the FQL used for the container-compliance API when a
// framework restriction is required. The successful validation showed no filter
// is required for the working endpoint, so this remains best-effort.
func failedRulesFilter(frameworkFilter string) string {
	if frameworkFilter == "" {
		return ""
	}

	return fmt.Sprintf("framework_name:'%s'", frameworkFilter)
}

// GetRuleDetails returns the metadata for the given rule IDs, split into
// batches to keep request URLs within sane limits.
func (c *Client) GetRuleDetails(ctx context.Context, ruleIDs []string) ([]RuleDetail, error) {
	details := make([]RuleDetail, 0, len(ruleIDs))

	for start := 0; start < len(ruleIDs); start += ruleIDBatchSize {
		end := min(start+ruleIDBatchSize, len(ruleIDs))

		query := url.Values{}
		query.Set(paramIDs, strings.Join(ruleIDs[start:end], ","))

		var resp ruleDetailsResponse
		if err := c.getJSON(ctx, pathRuleDetails, query, &resp); err != nil {
			return nil, fmt.Errorf("get rule details: %w", err)
		}

		details = append(details, resp.Resources...)
	}

	return details, nil
}

// ProbeResult is the outcome of a probe run, containing assessed frameworks and rules.
type ProbeResult struct {
	Frameworks []FrameworkSummary `json:"frameworks"`
	Rules      []RuleCompliance   `json:"rules"`
}

// Probe validates the API end to end: it lists frameworks and
// returns the assessed rules from aggregates/rules/v2.
func (c *Client) Probe(ctx context.Context, frameworkFilter string) (*ProbeResult, error) {
	frameworks, err := c.ListFrameworks(ctx)
	if err != nil {
		return nil, err
	}

	rules, err := c.ListRules(ctx, frameworkFilter)
	if err != nil {
		return nil, err
	}

	return &ProbeResult{Frameworks: frameworks, Rules: rules}, nil
}
