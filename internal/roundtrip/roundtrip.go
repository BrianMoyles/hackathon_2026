package roundtrip

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// volatileAttributeKeys are stripped during normalization so org-local IDs
// and URIs do not show up as false-positive drift in the mock roundtrip.
var volatileAttributeKeys = map[string]struct{}{
	"id":       {},
	"self_uri": {},
}

// ExportDocument is the subset of a CX as Code / Terraform JSON export that
// the mock roundtrip understands: a top-level "resource" map keyed by
// Terraform type, then block label, then attributes.
type ExportDocument struct {
	Resource map[string]map[string]map[string]any `json:"resource"`
}

// DriftKind classifies a single drift finding.
type DriftKind string

const (
	DriftMissingInTarget DriftKind = "missing_in_target"
	DriftExtraInTarget   DriftKind = "extra_in_target"
	DriftAttributeChange DriftKind = "attribute_change"
)

// Finding is one normalized drift item.
type Finding struct {
	Kind          DriftKind      `json:"kind"`
	ResourceType  string         `json:"resourceType"`
	BlockLabel    string         `json:"blockLabel"`
	Attribute     string         `json:"attribute,omitempty"`
	SourceValue   any            `json:"sourceValue,omitempty"`
	TargetValue   any            `json:"targetValue,omitempty"`
	SourceAttrs   map[string]any `json:"-"`
	TargetAttrs   map[string]any `json:"-"`
}

// Report is the full mock roundtrip result.
type Report struct {
	SourcePath   string    `json:"sourcePath"`
	TargetPath   string    `json:"targetPath"`
	ResourceType string    `json:"resourceType,omitempty"`
	Findings     []Finding `json:"findings"`
	Summary      Summary   `json:"summary"`
}

// Summary counts findings by kind.
type Summary struct {
	MissingInTarget  int `json:"missingInTarget"`
	ExtraInTarget    int `json:"extraInTarget"`
	AttributeChanges int `json:"attributeChanges"`
	Total            int `json:"total"`
}

// CompareFiles loads two export JSON fixtures, normalizes them, and reports drift.
// If resourceType is non-empty, comparison is limited to that Terraform type.
func CompareFiles(sourcePath, targetPath, resourceType string) (Report, error) {
	source, err := loadExport(sourcePath)
	if err != nil {
		return Report{}, fmt.Errorf("source export: %w", err)
	}
	target, err := loadExport(targetPath)
	if err != nil {
		return Report{}, fmt.Errorf("target export: %w", err)
	}

	report := Compare(source, target, resourceType)
	report.SourcePath = sourcePath
	report.TargetPath = targetPath
	return report, nil
}

// Compare diffs two in-memory export documents after normalization.
func Compare(source, target ExportDocument, resourceType string) Report {
	sourceNorm := Normalize(source)
	targetNorm := Normalize(target)

	types := unionKeys(sourceNorm.Resource, targetNorm.Resource)
	if resourceType != "" {
		types = []string{resourceType}
	}

	var findings []Finding
	for _, tfType := range types {
		sourceBlocks := sourceNorm.Resource[tfType]
		targetBlocks := targetNorm.Resource[tfType]
		if sourceBlocks == nil {
			sourceBlocks = map[string]map[string]any{}
		}
		if targetBlocks == nil {
			targetBlocks = map[string]map[string]any{}
		}

		labels := unionKeys(sourceBlocks, targetBlocks)
		for _, label := range labels {
			sourceAttrs, inSource := sourceBlocks[label]
			targetAttrs, inTarget := targetBlocks[label]

			switch {
			case inSource && !inTarget:
				findings = append(findings, Finding{
					Kind:         DriftMissingInTarget,
					ResourceType: tfType,
					BlockLabel:   label,
					SourceAttrs:  sourceAttrs,
				})
			case !inSource && inTarget:
				findings = append(findings, Finding{
					Kind:         DriftExtraInTarget,
					ResourceType: tfType,
					BlockLabel:   label,
					TargetAttrs:  targetAttrs,
				})
			default:
				findings = append(findings, attributeDiffs(tfType, label, sourceAttrs, targetAttrs)...)
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].ResourceType != findings[j].ResourceType {
			return findings[i].ResourceType < findings[j].ResourceType
		}
		if findings[i].BlockLabel != findings[j].BlockLabel {
			return findings[i].BlockLabel < findings[j].BlockLabel
		}
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		return findings[i].Attribute < findings[j].Attribute
	})

	return Report{
		ResourceType: resourceType,
		Findings:     findings,
		Summary:      summarize(findings),
	}
}

// Normalize strips volatile attributes and drops empty resource maps so
// comparisons focus on durable configuration drift.
func Normalize(doc ExportDocument) ExportDocument {
	out := ExportDocument{Resource: map[string]map[string]map[string]any{}}
	if doc.Resource == nil {
		return out
	}

	types := make([]string, 0, len(doc.Resource))
	for tfType := range doc.Resource {
		types = append(types, tfType)
	}
	sort.Strings(types)

	for _, tfType := range types {
		blocks := doc.Resource[tfType]
		normalizedBlocks := map[string]map[string]any{}
		labels := make([]string, 0, len(blocks))
		for label := range blocks {
			labels = append(labels, label)
		}
		sort.Strings(labels)

		for _, label := range labels {
			attrs := normalizeAttrs(blocks[label])
			if len(attrs) == 0 {
				continue
			}
			normalizedBlocks[label] = attrs
		}
		if len(normalizedBlocks) > 0 {
			out.Resource[tfType] = normalizedBlocks
		}
	}
	return out
}

func normalizeAttrs(attrs map[string]any) map[string]any {
	if attrs == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(attrs))
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, volatile := volatileAttributeKeys[key]; volatile {
			continue
		}
		out[key] = normalizeValue(attrs[key])
	}
	return out
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeAttrs(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return value
	}
}

func attributeDiffs(tfType, label string, source, target map[string]any) []Finding {
	keys := unionStringKeys(source, target)
	var findings []Finding
	for _, key := range keys {
		sourceVal, inSource := source[key]
		targetVal, inTarget := target[key]
		if !inSource {
			findings = append(findings, Finding{
				Kind:         DriftAttributeChange,
				ResourceType: tfType,
				BlockLabel:   label,
				Attribute:    key,
				TargetValue:  targetVal,
			})
			continue
		}
		if !inTarget {
			findings = append(findings, Finding{
				Kind:         DriftAttributeChange,
				ResourceType: tfType,
				BlockLabel:   label,
				Attribute:    key,
				SourceValue:  sourceVal,
			})
			continue
		}
		if !valuesEqual(sourceVal, targetVal) {
			findings = append(findings, Finding{
				Kind:         DriftAttributeChange,
				ResourceType: tfType,
				BlockLabel:   label,
				Attribute:    key,
				SourceValue:  sourceVal,
				TargetValue:  targetVal,
			})
		}
	}
	return findings
}

func valuesEqual(a, b any) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	if errLeft != nil || errRight != nil {
		return fmt.Sprint(a) == fmt.Sprint(b)
	}
	return string(left) == string(right)
}

func summarize(findings []Finding) Summary {
	summary := Summary{Total: len(findings)}
	for _, finding := range findings {
		switch finding.Kind {
		case DriftMissingInTarget:
			summary.MissingInTarget++
		case DriftExtraInTarget:
			summary.ExtraInTarget++
		case DriftAttributeChange:
			summary.AttributeChanges++
		}
	}
	return summary
}

func loadExport(path string) (ExportDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExportDocument{}, err
	}
	var doc ExportDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return ExportDocument{}, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if doc.Resource == nil {
		return ExportDocument{}, fmt.Errorf("%s: missing top-level \"resource\" object", filepath.Base(path))
	}
	return doc, nil
}

func unionKeys[V any](a, b map[string]V) []string {
	seen := map[string]struct{}{}
	for key := range a {
		seen[key] = struct{}{}
	}
	for key := range b {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func unionStringKeys(a, b map[string]any) []string {
	seen := map[string]struct{}{}
	for key := range a {
		seen[key] = struct{}{}
	}
	for key := range b {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// FormatTable renders a human-readable drift report.
func FormatTable(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Roundtrip drift (mock)\n")
	fmt.Fprintf(&b, "Source: %s\n", report.SourcePath)
	fmt.Fprintf(&b, "Target: %s\n", report.TargetPath)
	if report.ResourceType != "" {
		fmt.Fprintf(&b, "Resource filter: %s\n", report.ResourceType)
	}
	fmt.Fprintf(
		&b,
		"Summary: missing=%d extra=%d changed=%d total=%d\n",
		report.Summary.MissingInTarget,
		report.Summary.ExtraInTarget,
		report.Summary.AttributeChanges,
		report.Summary.Total,
	)
	if len(report.Findings) == 0 {
		fmt.Fprintf(&b, "No drift detected after normalization.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\nKind                 Type                           Label                Detail\n")
	fmt.Fprintf(&b, "----                 ----                           -----                ------\n")
	for _, finding := range report.Findings {
		detail := ""
		switch finding.Kind {
		case DriftAttributeChange:
			detail = fmt.Sprintf("%s: %v -> %v", finding.Attribute, finding.SourceValue, finding.TargetValue)
		case DriftMissingInTarget:
			detail = "present in source only"
		case DriftExtraInTarget:
			detail = "present in target only"
		}
		fmt.Fprintf(
			&b,
			"%-20s %-30s %-20s %s\n",
			finding.Kind,
			finding.ResourceType,
			finding.BlockLabel,
			detail,
		)
	}
	return b.String()
}
