package explain

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ingen/paddock/internal/model"
)

const schema = "paddock.explanation/v1"

type Document struct {
	Schema       string                 `json:"schema"`
	SourceSchema string                 `json:"source_schema"`
	Status       string                 `json:"status"`
	Root         string                 `json:"root"`
	ModulePath   string                 `json:"module_path"`
	PackageCount int                    `json:"package_count"`
	EdgeCount    int                    `json:"edge_count"`
	Baseline     *model.BaselineSummary `json:"baseline,omitempty"`
	Waivers      []model.WaiverSummary  `json:"waivers,omitempty"`
	Findings     []FindingExplanation   `json:"findings"`
}

type FindingExplanation struct {
	Finding          *model.Finding     `json:"finding"`
	Rule             *model.RuleSummary `json:"rule,omitempty"`
	Status           string             `json:"status"`
	Observation      string             `json:"observation"`
	Why              string             `json:"why"`
	SuggestedActions []string           `json:"suggested_actions"`
}

func Explain(result *model.Result) Document {
	rules := make(map[string]*model.RuleSummary, len(result.Rules))
	for index := range result.Rules {
		rule := result.Rules[index]
		rules[rule.ID] = &rule
	}
	document := Document{
		Schema:       schema,
		SourceSchema: result.Schema,
		Status:       resultStatus(result),
		Root:         result.Root,
		ModulePath:   result.ModulePath,
		PackageCount: result.PackageCount,
		EdgeCount:    result.EdgeCount,
		Baseline:     result.Baseline,
		Waivers:      result.Waivers,
		Findings:     make([]FindingExplanation, 0, len(result.Findings)),
	}
	for _, finding := range result.Findings {
		rule := rules[finding.RuleID]
		document.Findings = append(document.Findings, FindingExplanation{
			Finding:          finding,
			Rule:             rule,
			Status:           findingStatus(finding),
			Observation:      observation(finding),
			Why:              finding.Message,
			SuggestedActions: suggestedActions(finding, rule),
		})
	}
	return document
}

func Text(w io.Writer, document Document) error {
	if _, err := fmt.Fprintf(w, "EXPLAIN %s %s (%d findings)\n", document.Status, document.Root, len(document.Findings)); err != nil {
		return err
	}
	for _, waiver := range document.Waivers {
		if waiver.Status == "unused" || waiver.Status == "unused-expired" {
			if _, err := fmt.Fprintf(w, "WARNING: unused waiver %s from %s\n", waiver.RuleID, waiver.From); err != nil {
				return err
			}
		}
	}
	for index, finding := range document.Findings {
		rule := finding.Finding.RuleID
		kind := finding.Finding.Kind
		if finding.Rule != nil && finding.Rule.Kind != "" {
			kind = finding.Rule.Kind
		}
		if _, err := fmt.Fprintf(w, "%d. [%s] %s (%s)\n", index+1, finding.Status, rule, kind); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "   Observed: %s\n", finding.Observation); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "   Why: %s\n", finding.Why); err != nil {
			return err
		}
		for _, action := range finding.SuggestedActions {
			if _, err := fmt.Fprintf(w, "   Next: %s\n", action); err != nil {
				return err
			}
		}
	}
	return nil
}

func JSON(w io.Writer, document Document) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func resultStatus(result *model.Result) string {
	if result.OK() {
		return "PASS"
	}
	return "FAIL"
}

func findingStatus(finding *model.Finding) string {
	switch {
	case finding.WaiverStatus == "expired":
		return "expired-waiver"
	case finding.Waived:
		return "waived"
	case finding.Baselined:
		return "baselined"
	default:
		return "blocking"
	}
}

func observation(finding *model.Finding) string {
	from := finding.From
	if finding.FromComponent != "" {
		from += " [" + finding.FromComponent + "]"
	}
	to := finding.To
	if finding.ToComponent != "" {
		to += " [" + finding.ToComponent + "]"
	}
	if finding.File != "" {
		location := finding.File
		if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, finding.Line)
		}
		if to != "" {
			return fmt.Sprintf("%s imports %s", location, to)
		}
		return location
	}
	if to != "" {
		return fmt.Sprintf("%s depends on %s", from, to)
	}
	if from != "" {
		return from
	}
	return "the dependency graph contains a policy violation"
}

func suggestedActions(finding *model.Finding, rule *model.RuleSummary) []string {
	if rule == nil {
		return []string{"Inspect the policy rule and move the dependency behind an approved architectural boundary."}
	}
	switch rule.Kind {
	case "allow-dependencies":
		if len(rule.Allow) > 0 {
			return []string{"Move the dependency behind an approved boundary or use one of the allowed targets: " + strings.Join(rule.Allow, ", ")}
		}
		return []string{"Move the dependency behind an approved architectural boundary."}
	case "deny-dependencies":
		return []string{"Remove the dependency or introduce a boundary that avoids the denied target."}
	case "layer-direction":
		return []string{"Reverse the dependency so it points " + directionText(rule.Direction) + "."}
	case "no-cross-context":
		return []string{"Keep the dependency within the same context or call the other context through its public boundary."}
	case "mediated-dependency":
		if len(rule.AllowTo) > 0 {
			return []string{"Route the dependency through one of the permitted boundaries: " + strings.Join(rule.AllowTo, ", ")}
		}
		return []string{"Route the dependency through an explicit public boundary or shared contract."}
	case "public-api-only":
		return []string{"Import the other context's public API instead of its internal implementation."}
	case "coverage":
		return []string{"Add or adjust a component selector so this source unit has exactly one architectural owner."}
	case "required-dependency":
		if len(rule.Allow) > 0 {
			return []string{"Add a dependency to one of the required targets: " + strings.Join(rule.Allow, ", ")}
		}
		return []string{"Add the required architectural boundary dependency."}
	case "unresolved":
		return []string{"Fix the import path, add the missing source file, or configure the project alias in tsconfig.json."}
	case "no-cycles":
		return []string{"Break the import cycle by extracting a stable boundary, moving shared types, or inverting the dependency."}
	default:
		return []string{"Move the dependency behind an approved architectural boundary."}
	}
}

func directionText(direction string) string {
	switch direction {
	case "toward-lower-layer":
		return "toward a lower layer"
	case "toward-higher-layer":
		return "toward a higher layer"
	default:
		return "in the direction required by the policy"
	}
}
