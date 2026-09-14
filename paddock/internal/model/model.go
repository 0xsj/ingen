package model

// Package model contains the language-neutral graph and finding types shared
// by Paddock's source adapters and rule engine.

type Graph struct {
	ModulePath string     `json:"module_path"`
	Packages   []*Package `json:"packages"`
	Edges      []*Edge    `json:"edges"`
}

type Package struct {
	ImportPath string            `json:"import_path"`
	RelPath    string            `json:"path"`
	Component  string            `json:"component,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type Edge struct {
	FromImportPath string `json:"from"`
	FromPath       string `json:"from_path"`
	ToImportPath   string `json:"to"`
	ToPath         string `json:"to_path,omitempty"`
	Kind           string `json:"kind"`
	TargetKind     string `json:"target_kind"`
	File           string `json:"file"`
	Line           int    `json:"line"`
}

type Finding struct {
	RuleID              string            `json:"rule_id"`
	Kind                string            `json:"kind"`
	Severity            string            `json:"severity"`
	From                string            `json:"from,omitempty"`
	FromComponent       string            `json:"from_component,omitempty"`
	FromLabels          map[string]string `json:"from_labels,omitempty"`
	To                  string            `json:"to,omitempty"`
	ToComponent         string            `json:"to_component,omitempty"`
	ToLabels            map[string]string `json:"to_labels,omitempty"`
	File                string            `json:"file,omitempty"`
	Line                int               `json:"line,omitempty"`
	Message             string            `json:"message"`
	Waived              bool              `json:"waived,omitempty"`
	WaiverReason        string            `json:"waiver_reason,omitempty"`
	WaiverOwner         string            `json:"waiver_owner,omitempty"`
	WaiverExpires       string            `json:"waiver_expires,omitempty"`
	WaiverStatus        string            `json:"waiver_status,omitempty"`
	Baselined           bool              `json:"baselined,omitempty"`
	BaselineFingerprint string            `json:"baseline_fingerprint,omitempty"`
}

type RuleSummary struct {
	ID           string              `json:"id"`
	Kind         string              `json:"kind"`
	Severity     string              `json:"severity"`
	From         []map[string]string `json:"from,omitempty"`
	To           []map[string]string `json:"to,omitempty"`
	Allow        []string            `json:"allow,omitempty"`
	Deny         []string            `json:"deny,omitempty"`
	AllowTo      []string            `json:"allow_to,omitempty"`
	Transitive   bool                `json:"transitive,omitempty"`
	Direction    string              `json:"direction,omitempty"`
	ContextLabel string              `json:"context_label,omitempty"`
	Message      string              `json:"message,omitempty"`
}

type BaselineSummary struct {
	Path    string   `json:"path"`
	Entries int      `json:"entries"`
	Matched int      `json:"matched"`
	Stale   []string `json:"stale,omitempty"`
}

type WaiverSummary struct {
	RuleID  string `json:"rule_id"`
	From    string `json:"from"`
	To      string `json:"to,omitempty"`
	Owner   string `json:"owner"`
	Expires string `json:"expires"`
	Status  string `json:"status"`
}

type Result struct {
	Schema       string           `json:"schema"`
	Policy       string           `json:"policy"`
	Root         string           `json:"root"`
	ModulePath   string           `json:"module_path"`
	PackageCount int              `json:"package_count"`
	EdgeCount    int              `json:"edge_count"`
	SourceUnit   string           `json:"source_unit"`
	Findings     []*Finding       `json:"findings"`
	Rules        []RuleSummary    `json:"rules,omitempty"`
	Baseline     *BaselineSummary `json:"baseline,omitempty"`
	Waivers      []WaiverSummary  `json:"waivers,omitempty"`
}

func (r *Result) OK() bool {
	for _, finding := range r.Findings {
		if finding.Severity == "error" && !finding.Waived && !finding.Baselined {
			return false
		}
	}
	return true
}
