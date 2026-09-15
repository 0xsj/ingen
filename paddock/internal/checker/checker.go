package checker

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/paddock/internal/graph"
	"ingen/paddock/internal/model"
	"ingen/paddock/internal/policy"
)

func Check(root, policyPath string) (*model.Result, error) {
	config, err := policy.Load(policyPath)
	if err != nil {
		return nil, err
	}
	return CheckPolicy(root, policyPath, config)
}

func CheckPolicy(root, policyPath string, config policy.Policy) (*model.Result, error) {
	dependencyGraph, err := graph.LoadWithRequest(graph.LoadRequest{
		Root:  root,
		Unit:  config.Source.Unit,
		Roots: append([]string(nil), config.Source.Roots...),
	}, config.Source.Language)
	if err != nil {
		return nil, err
	}
	return CheckGraph(root, policyPath, config, dependencyGraph)
}

func CheckGraph(root, policyPath string, config policy.Policy, dependencyGraph *model.Graph) (*model.Result, error) {
	if dependencyGraph == nil {
		return nil, fmt.Errorf("checker graph is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source root: %w", err)
	}
	result := &model.Result{
		Schema:       "paddock.report/v1",
		Policy:       policyPath,
		Root:         root,
		ModulePath:   dependencyGraph.ModulePath,
		PackageCount: len(dependencyGraph.Packages),
		EdgeCount:    len(dependencyGraph.Edges),
		SourceUnit:   config.Source.Unit,
		Rules:        summarizeRules(config.Rules),
	}

	byImport := make(map[string]*model.Package, len(dependencyGraph.Packages))
	for _, pkg := range dependencyGraph.Packages {
		byImport[pkg.ImportPath] = pkg
	}
	result.Findings = append(result.Findings, Classify(dependencyGraph.Packages, config)...)

	for _, rule := range config.Rules {
		switch rule.Kind {
		case "coverage":
			checkCoverage(dependencyGraph.Packages, config, rule, result)
		case "no-cycles":
			checkCycles(dependencyGraph, config, rule, result)
		case "required-dependency":
			checkRequiredDependencies(dependencyGraph, byImport, config, rule, result)
		default:
			checkEdges(dependencyGraph, byImport, config, rule, result)
		}
	}
	applyWaivers(config.Waivers, result)

	sort.Slice(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i], result.Findings[j]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.To < b.To
	})
	return result, nil
}

func applyWaivers(waivers []policy.Waiver, result *model.Result) {
	today := time.Now().UTC().Format("2006-01-02")
	used := make([]string, len(waivers))
	for _, finding := range result.Findings {
		for index, waiver := range waivers {
			if waiver.Rule != finding.RuleID || !waiverMatches(waiver, finding) {
				continue
			}

			status := "applied"
			if waiver.Expires < today {
				status = "expired"
			}
			used[index] = status
			finding.WaiverReason = waiver.Reason
			finding.WaiverOwner = waiver.Owner
			finding.WaiverExpires = waiver.Expires
			if status == "expired" {
				finding.WaiverStatus = "expired"
			} else {
				finding.Waived = true
				finding.WaiverStatus = "applied"
			}
			break
		}
	}
	if len(waivers) == 0 {
		return
	}
	result.Waivers = make([]model.WaiverSummary, 0, len(waivers))
	for index, waiver := range waivers {
		status := used[index]
		if status == "" {
			status = "unused"
			if waiver.Expires < today {
				status = "unused-expired"
			}
		}
		result.Waivers = append(result.Waivers, model.WaiverSummary{
			RuleID:  waiver.Rule,
			From:    waiver.From,
			To:      waiver.To,
			Owner:   waiver.Owner,
			Expires: waiver.Expires,
			Status:  status,
		})
	}
}

func waiverMatches(waiver policy.Waiver, finding *model.Finding) bool {
	if !matchesAnyPath(waiver.From, finding.File, finding.From) {
		return false
	}
	return waiver.To == "" || matchesAnyPath(waiver.To, finding.To)
}

func matchesAnyPath(pattern string, values ...string) bool {
	for _, value := range values {
		if value == pattern {
			return true
		}
		if value != "" {
			if _, ok := matchPattern(pattern, value); ok {
				return true
			}
		}
	}
	return false
}

// Classify assigns policy components and returns findings for overlapping
// component matches. Unmatched packages remain unclassified so callers that
// only need graph summaries can still inspect the complete source graph.
func Classify(packages []*model.Package, config policy.Policy) []*model.Finding {
	findings := make([]*model.Finding, 0)
	for _, pkg := range packages {
		matches := make([]componentMatch, 0, 1)
		for name, component := range config.Components {
			for _, pattern := range component.Match {
				if captures, ok := matchPattern(pattern, pkg.RelPath); ok {
					labels := make(map[string]string, len(component.Labels))
					for key, value := range component.Labels {
						labels[key] = policy.Substitute(fmt.Sprint(value), captures)
					}
					matches = append(matches, componentMatch{name: name, labels: labels})
					break
				}
			}
		}
		if len(matches) == 1 {
			pkg.Component = matches[0].name
			pkg.Labels = matches[0].labels
		}
		if len(matches) > 1 {
			findings = append(findings, &model.Finding{
				RuleID:   "coverage",
				Kind:     "coverage",
				Severity: "error",
				From:     pkg.RelPath,
				Message:  fmt.Sprintf("source unit matches multiple components: %s", componentNames(matches)),
			})
		}
	}
	return findings
}

type componentMatch struct {
	name   string
	labels map[string]string
}

func componentNames(matches []componentMatch) string {
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match.name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func summarizeRules(rules []policy.Rule) []model.RuleSummary {
	result := make([]model.RuleSummary, 0, len(rules))
	for _, rule := range rules {
		result = append(result, model.RuleSummary{
			ID:           rule.ID,
			Kind:         rule.Kind,
			Severity:     rule.Severity,
			From:         summarizeSelectors(rule.From),
			To:           summarizeSelectors(rule.To),
			Allow:        summarizeTargets(rule.Allow),
			Deny:         summarizeTargets(rule.Deny),
			AllowTo:      summarizeTargets(rule.AllowTo),
			Transitive:   rule.Transitive,
			Direction:    rule.Direction,
			ContextLabel: rule.ContextLabel,
			Message:      rule.Message,
		})
	}
	return result
}

func summarizeSelectors(selectors policy.Selectors) []map[string]string {
	result := make([]map[string]string, 0, len(selectors))
	for _, selector := range selectors {
		copy := make(map[string]string, len(selector))
		for key, value := range selector {
			copy[key] = value
		}
		result = append(result, copy)
	}
	return result
}

func summarizeTargets(targets policy.Targets) []string {
	result := make([]string, 0, len(targets))
	for _, target := range targets {
		result = append(result, targetDescription(target))
	}
	return result
}

func targetDescription(target policy.Target) string {
	if target.Literal != "" {
		return target.Literal
	}
	if target.Kind != "" {
		if target.Value == "" {
			return target.Kind
		}
		return target.Kind + ":" + target.Value
	}
	keys := make([]string, 0, len(target.Labels))
	for key := range target.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+target.Labels[key])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func checkCoverage(packages []*model.Package, config policy.Policy, rule policy.Rule, result *model.Result) {
	for _, pkg := range packages {
		if !underRoots(pkg.RelPath, config.Source.Roots) {
			continue
		}
		if pkg.Component == "" {
			message := rule.Message
			if message == "" {
				message = "every source unit must match exactly one component"
			}
			result.Findings = append(result.Findings, &model.Finding{
				RuleID:   rule.ID,
				Kind:     rule.Kind,
				Severity: rule.Severity,
				From:     pkg.RelPath,
				Message:  message,
			})
		}
	}
}

func checkEdges(g *model.Graph, byImport map[string]*model.Package, config policy.Policy, rule policy.Rule, result *model.Result) {
	for _, edge := range g.Edges {
		from := byImport[edge.FromImportPath]
		if from == nil || !underRoots(from.RelPath, config.Source.Roots) {
			continue
		}
		to := byImport[edge.ToImportPath]

		var bad bool
		switch rule.Kind {
		case "allow-dependencies":
			if selectorsMatch(rule.From, from) && len(rule.Allow) > 0 {
				bad = !anyTargetMatches(rule.Allow, edge, from, to)
			}
		case "deny-dependencies":
			if selectorsMatch(rule.From, from) {
				bad = anyTargetMatches(rule.Deny, edge, from, to)
			}
		case "layer-direction":
			if selectorsMatch(rule.From, from) && selectorsMatch(rule.To, to) {
				bad = violatesLayerDirection(rule, from, to)
			}
		case "no-cross-context":
			bad = crossesContext(rule, from, to)
		case "mediated-dependency":
			bad = crossesContext(rule, from, to) && !anyTargetMatches(rule.AllowTo, edge, from, to)
		case "public-api-only":
			bad = crossesContext(rule, from, to) && selectorsMatch(rule.To, to)
		case "unresolved":
			bad = edge.TargetKind == "unresolved" && selectorsMatch(rule.From, from)
		}

		if bad {
			message := rule.Message
			if message == "" {
				message = "dependency violates architecture policy"
			}
			finding := &model.Finding{
				RuleID:   rule.ID,
				Kind:     rule.Kind,
				Severity: rule.Severity,
				From:     edge.FromPath,
				To:       edgeDisplayPath(edge),
				File:     edge.File,
				Line:     edge.Line,
				Message:  message,
			}
			decorateFinding(finding, from, to)
			result.Findings = append(result.Findings, finding)
		}
	}
}

func checkRequiredDependencies(g *model.Graph, byImport map[string]*model.Package, config policy.Policy, rule policy.Rule, result *model.Result) {
	edgesByFrom := make(map[string][]*model.Edge)
	for _, edge := range g.Edges {
		edgesByFrom[edge.FromImportPath] = append(edgesByFrom[edge.FromImportPath], edge)
	}
	for _, pkg := range g.Packages {
		if !underRoots(pkg.RelPath, config.Source.Roots) || !selectorsMatch(rule.From, pkg) {
			continue
		}
		matched := requiredDependencyMatches(pkg, edgesByFrom, byImport, rule)
		if matched {
			continue
		}
		message := rule.Message
		if message == "" {
			message = "source unit must depend on a required boundary"
		}
		result.Findings = append(result.Findings, &model.Finding{
			RuleID:   rule.ID,
			Kind:     rule.Kind,
			Severity: rule.Severity,
			From:     pkg.RelPath,
			Message:  message,
		})
	}
}

func requiredDependencyMatches(start *model.Package, edgesByFrom map[string][]*model.Edge, byImport map[string]*model.Package, rule policy.Rule) bool {
	if !rule.Transitive {
		for _, edge := range edgesByFrom[start.ImportPath] {
			to := byImport[edge.ToImportPath]
			if anyTargetMatches(rule.Allow, edge, start, to) {
				return true
			}
		}
		return false
	}

	queue := []*model.Package{start}
	visited := map[string]bool{start.ImportPath: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range edgesByFrom[current.ImportPath] {
			to := byImport[edge.ToImportPath]
			if anyTargetMatches(rule.Allow, edge, current, to) {
				return true
			}
			if edge.TargetKind != "internal" || to == nil || visited[to.ImportPath] {
				continue
			}
			visited[to.ImportPath] = true
			queue = append(queue, to)
		}
	}
	return false
}

func decorateFinding(finding *model.Finding, from, to *model.Package) {
	if from != nil {
		finding.FromComponent = from.Component
		finding.FromLabels = copyLabels(from.Labels)
	}
	if to != nil {
		finding.ToComponent = to.Component
		finding.ToLabels = copyLabels(to.Labels)
	}
}

func copyLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	copy := make(map[string]string, len(labels))
	for key, value := range labels {
		copy[key] = value
	}
	return copy
}

func selectorsMatch(selectors policy.Selectors, pkg *model.Package) bool {
	if len(selectors) == 0 {
		return true
	}
	if pkg == nil {
		return false
	}
	for _, selector := range selectors {
		matches := true
		for key, expected := range selector {
			if expected == "same" || expected == "different" {
				continue
			}
			if pkg.Labels[key] != expected {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func anyTargetMatches(targets policy.Targets, edge *model.Edge, from, to *model.Package) bool {
	for _, target := range targets {
		if targetMatches(target, edge, from, to) {
			return true
		}
	}
	return false
}

func targetMatches(target policy.Target, edge *model.Edge, from, to *model.Package) bool {
	if target.Literal != "" {
		if strings.HasPrefix(target.Literal, "standard-library:") {
			return edge.TargetKind == "standard-library" && edge.ToImportPath == strings.TrimPrefix(target.Literal, "standard-library:")
		}
		if strings.HasPrefix(target.Literal, "external:") {
			return edge.TargetKind == "external"
		}
		return edge.ToImportPath == target.Literal || edge.ToPath == target.Literal
	}
	if target.Kind != "" {
		if edge.TargetKind != target.Kind {
			return false
		}
		return target.Value == "" || target.Value == "approved" || edge.ToImportPath == target.Value
	}
	if to == nil {
		return false
	}
	for key, expected := range target.Labels {
		actual := to.Labels[key]
		switch expected {
		case "same":
			if actual != from.Labels[key] {
				return false
			}
		case "different":
			if actual == "" || actual == from.Labels[key] {
				return false
			}
		default:
			if actual != expected {
				return false
			}
		}
	}
	return true
}

func crossesContext(rule policy.Rule, from, to *model.Package) bool {
	if from == nil || to == nil {
		return false
	}
	if !selectorsMatch(rule.From, from) || !selectorsMatch(rule.To, to) {
		return false
	}
	label := rule.ContextLabel
	if label == "" {
		label = "context"
	}
	left, right := from.Labels[label], to.Labels[label]
	return left != "" && right != "" && left != right
}

func violatesLayerDirection(rule policy.Rule, from, to *model.Package) bool {
	fromLayer, fromOK := layer(from)
	toLayer, toOK := layer(to)
	if !fromOK || !toOK {
		return false
	}
	switch rule.Direction {
	case "toward-lower-layer":
		return toLayer > fromLayer
	case "toward-higher-layer":
		return toLayer < fromLayer
	default:
		return false
	}
}

func layer(pkg *model.Package) (int, bool) {
	if pkg == nil {
		return 0, false
	}
	return strconvAtoi(pkg.Labels["layer"])
}

func strconvAtoi(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	var result int
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
		result = result*10 + int(char-'0')
	}
	return result, true
}

func checkCycles(g *model.Graph, config policy.Policy, rule policy.Rule, result *model.Result) {
	adjacency := make(map[string][]string)
	edges := make(map[string]*model.Edge)
	byImport := make(map[string]*model.Package, len(g.Packages))
	for _, pkg := range g.Packages {
		if !underRoots(pkg.RelPath, config.Source.Roots) || !selectorsMatch(rule.From, pkg) {
			continue
		}
		adjacency[pkg.ImportPath] = nil
		byImport[pkg.ImportPath] = pkg
	}
	for _, edge := range g.Edges {
		if edge.TargetKind != "internal" || !containsPackage(byImport, edge.FromImportPath) || !containsPackage(byImport, edge.ToImportPath) {
			continue
		}
		adjacency[edge.FromImportPath] = append(adjacency[edge.FromImportPath], edge.ToImportPath)
		edges[edge.FromImportPath+"\x00"+edge.ToImportPath] = edge
	}
	for key := range adjacency {
		sort.Strings(adjacency[key])
	}

	index := 0
	indices := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	stack := []string{}
	var visit func(string)
	visit = func(node string) {
		indices[node] = index
		low[node] = index
		index++
		stack = append(stack, node)
		onStack[node] = true
		for _, next := range adjacency[node] {
			if _, seen := indices[next]; !seen {
				visit(next)
				if low[next] < low[node] {
					low[node] = low[next]
				}
			} else if onStack[next] && indices[next] < low[node] {
				low[node] = indices[next]
			}
		}
		if low[node] != indices[node] {
			return
		}

		component := []string{}
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == node {
				break
			}
		}
		selfCycle := len(component) == 1 && contains(adjacency[component[0]], component[0])
		if len(component) < 2 && !selfCycle {
			return
		}
		sort.Strings(component)
		edge := firstCycleEdge(component, edges)
		message := rule.Message
		if message == "" {
			message = "import cycle: " + strings.Join(component, " -> ")
		}
		finding := &model.Finding{RuleID: rule.ID, Kind: rule.Kind, Severity: rule.Severity, Message: message}
		if edge != nil {
			finding.From = edge.FromPath
			finding.To = edgeDisplayPath(edge)
			finding.File = edge.File
			finding.Line = edge.Line
			decorateFinding(finding, byImport[edge.FromImportPath], byImport[edge.ToImportPath])
		}
		result.Findings = append(result.Findings, finding)
	}
	keys := make([]string, 0, len(adjacency))
	for key := range adjacency {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, seen := indices[key]; !seen {
			visit(key)
		}
	}
}

func containsPackage(packages map[string]*model.Package, importPath string) bool {
	_, ok := packages[importPath]
	return ok
}

func firstCycleEdge(component []string, edges map[string]*model.Edge) *model.Edge {
	set := make(map[string]bool, len(component))
	for _, item := range component {
		set[item] = true
	}
	var best *model.Edge
	for _, edge := range edges {
		if !set[edge.FromImportPath] || !set[edge.ToImportPath] {
			continue
		}
		if best == nil || edge.File < best.File || (edge.File == best.File && edge.Line < best.Line) {
			best = edge
		}
	}
	return best
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func edgeDisplayPath(edge *model.Edge) string {
	if edge.ToPath != "" {
		return edge.ToPath
	}
	return edge.ToImportPath
}

func underRoots(path string, roots []string) bool {
	path = strings.Trim(path, "/")
	for _, root := range roots {
		root = strings.Trim(strings.TrimSpace(root), "/")
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func matchPattern(pattern, value string) (map[string]string, bool) {
	patternParts := splitPattern(pattern)
	valueParts := splitPattern(value)
	captures := map[string]string{}
	if matchPatternParts(patternParts, valueParts, captures) {
		return captures, true
	}
	return nil, false
}

func splitPattern(value string) []string {
	value = strings.Trim(value, "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func matchPatternParts(pattern, value []string, captures map[string]string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		if matchPatternParts(pattern[1:], value, captures) {
			return true
		}
		for index := 0; index < len(value); index++ {
			if matchPatternParts(pattern[1:], value[index+1:], captures) {
				return true
			}
		}
		return false
	}
	if len(value) == 0 {
		return false
	}
	segment := pattern[0]
	if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") && len(segment) > 2 {
		key := segment[1 : len(segment)-1]
		previous, hadPrevious := captures[key]
		captures[key] = value[0]
		if matchPatternParts(pattern[1:], value[1:], captures) {
			return true
		}
		if hadPrevious {
			captures[key] = previous
		} else {
			delete(captures, key)
		}
		return false
	}
	if segment != "*" && segment != value[0] {
		return false
	}
	return matchPatternParts(pattern[1:], value[1:], captures)
}
