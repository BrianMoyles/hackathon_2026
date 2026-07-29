// Package providerdiff compares two provider manifests and emits a
// risk-graded list of findings. It is the analytical core behind the
// `diff-provider-pr` command: given a base and a head snapshot of the
// Terraform provider (typically two git refs), it flags the changes that
// would break MRMO downstream.
//
// LAB-7 acceptance criteria drive the risk model:
//
//   - Removed exporters on MRMO-supported resources → `high`.
//   - Removed or type-changed RefAttrs / EncodedRefAttrs on MRMO-supported
//     resources → `high`.
//   - Removed resources that MRMO relies on → `high`.
//   - Structural flips (singleton toggled, ExportID changed) → `medium`.
//   - Additions and informational metadata flips → `low`.
//
// The output is deterministic: findings are sorted by (TerraformType,
// Kind, Attribute, BeforeValue) so consumer diffs and golden tests stay
// stable.
//
// JSON contract:
//
//   - SchemaVersion is "compatibility-lab-diff/v1"; bump on any
//     field-name / type / vocabulary change.
//   - `kind` values: RESOURCE_ADDED, RESOURCE_REMOVED, EXPORTER_ADDED,
//     EXPORTER_REMOVED, REFATTR_ADDED, REFATTR_REMOVED, REFATTR_CHANGED,
//     ENCODED_REFATTR_ADDED, ENCODED_REFATTR_REMOVED,
//     ENCODED_REFATTR_CHANGED, SINGLETON_FLIPPED, EXPORT_ID_CHANGED.
//   - `risk` values: "high" | "medium" | "low".
package providerdiff

import (
	"sort"

	"compatibility-lab/internal/model"
)

// SchemaVersion is the JSON contract version for DiffReport. It is
// separate from matrix.SchemaVersion because a compatibility scan and a
// provider-PR diff are different documents; a consumer that only reads
// one of them should not have to track versioning on the other.
const SchemaVersion = "compatibility-lab-diff/v1"

// Kind enumerates the categories of change we can detect between two
// provider snapshots. The values are exported string constants so JSON
// consumers can match on stable identifiers instead of numeric enums.
type Kind string

const (
	KindResourceAdded         Kind = "RESOURCE_ADDED"
	KindResourceRemoved       Kind = "RESOURCE_REMOVED"
	KindExporterAdded         Kind = "EXPORTER_ADDED"
	KindExporterRemoved       Kind = "EXPORTER_REMOVED"
	KindRefAttrAdded          Kind = "REFATTR_ADDED"
	KindRefAttrRemoved        Kind = "REFATTR_REMOVED"
	KindRefAttrChanged        Kind = "REFATTR_CHANGED"
	KindEncodedRefAttrAdded   Kind = "ENCODED_REFATTR_ADDED"
	KindEncodedRefAttrRemoved Kind = "ENCODED_REFATTR_REMOVED"
	KindEncodedRefAttrChanged Kind = "ENCODED_REFATTR_CHANGED"
	KindSingletonFlipped      Kind = "SINGLETON_FLIPPED"
	KindExportIDChanged       Kind = "EXPORT_ID_CHANGED"
)

// Risk is a coarse severity grading for a Finding. It is deliberately
// smaller than the matrix Issue vocabulary because a diff answers a
// narrower question ("should a human review this PR carefully?") than a
// full compatibility scan.
type Risk string

const (
	RiskHigh   Risk = "high"
	RiskMedium Risk = "medium"
	RiskLow    Risk = "low"
)

// DiffReport is the top-level document produced by Diff. See the package
// comment for the JSON contract.
type DiffReport struct {
	SchemaVersion string      `json:"schemaVersion"`
	Inputs        DiffInputs  `json:"inputs"`
	Summary       DiffSummary `json:"summary"`
	Findings      []Finding   `json:"findings"`
}

// DiffInputs records what was compared. It is populated by CLI callers
// (git refs, repo paths) and is JSON-optional so pure-Go callers can
// leave it zeroed without polluting the output.
type DiffInputs struct {
	ProviderRepo string `json:"providerRepo,omitempty"`
	MRMORepo     string `json:"mrmoRepo,omitempty"`
	BaseRef      string `json:"baseRef,omitempty"`
	HeadRef      string `json:"headRef,omitempty"`
}

// DiffSummary is a fixed-shape tally so CI can render "N high-risk
// findings" without walking the findings slice.
type DiffSummary struct {
	TotalFindings   int `json:"totalFindings"`
	HighRiskCount   int `json:"highRiskCount"`
	MediumRiskCount int `json:"mediumRiskCount"`
	LowRiskCount    int `json:"lowRiskCount"`
}

// Finding is one atomic change between the two snapshots. `Attribute`,
// `BeforeValue`, and `AfterValue` are optional because different Kinds
// carry different amounts of context (e.g. RESOURCE_ADDED has none of
// them; REFATTR_CHANGED has all three).
type Finding struct {
	TerraformType string `json:"terraformType"`
	Kind          Kind   `json:"kind"`
	Risk          Risk   `json:"risk"`
	MRMOSupported bool   `json:"mrmoSupported"`
	Attribute     string `json:"attribute,omitempty"`
	BeforeValue   string `json:"beforeValue,omitempty"`
	AfterValue    string `json:"afterValue,omitempty"`
	Message       string `json:"message"`
}

// Diff compares two provider manifests and returns a graded list of
// findings. The optional mrmoManifest is used only for risk
// classification — pass a zero-valued MRMOManifest when no MRMO context
// is available and every finding will default to `mrmoSupported: false`.
//
// Determinism: the returned Findings slice is sorted by
// (TerraformType, Kind, Attribute, BeforeValue) so callers can pipe it
// through golden tests without an extra sort step.
func Diff(base, head model.ProviderManifest, mrmoManifest model.MRMOManifest) DiffReport {
	baseByType := indexProviderResources(base.Resources)
	headByType := indexProviderResources(head.Resources)
	mrmoSupported := indexMRMOSupported(mrmoManifest.Resources)

	var findings []Finding

	// Resources removed / mutated.
	for terraformType, baseResource := range baseByType {
		headResource, ok := headByType[terraformType]
		if !ok {
			findings = append(findings, resourceRemovedFinding(baseResource, mrmoSupported[terraformType]))
			continue
		}
		findings = append(findings, compareResources(baseResource, headResource, mrmoSupported[terraformType])...)
	}
	// Resources newly added on head.
	for terraformType, headResource := range headByType {
		if _, existed := baseByType[terraformType]; existed {
			continue
		}
		findings = append(findings, resourceAddedFinding(headResource, mrmoSupported[terraformType]))
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].TerraformType != findings[j].TerraformType {
			return findings[i].TerraformType < findings[j].TerraformType
		}
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].Attribute != findings[j].Attribute {
			return findings[i].Attribute < findings[j].Attribute
		}
		return findings[i].BeforeValue < findings[j].BeforeValue
	})
	if findings == nil {
		// Match the matrix contract: emit `findings: []` rather than
		// `findings: null` on a no-change diff so consumers can rely on
		// the field being present.
		findings = []Finding{}
	}

	summary := DiffSummary{TotalFindings: len(findings)}
	for _, finding := range findings {
		switch finding.Risk {
		case RiskHigh:
			summary.HighRiskCount++
		case RiskMedium:
			summary.MediumRiskCount++
		case RiskLow:
			summary.LowRiskCount++
		}
	}

	return DiffReport{
		SchemaVersion: SchemaVersion,
		Inputs: DiffInputs{
			ProviderRepo: head.RepoPath,
		},
		Summary:  summary,
		Findings: findings,
	}
}

// compareResources produces the per-attribute findings for a resource
// that exists in both base and head. The exporter-removed check is the
// specific "high risk on MRMO-supported" case LAB-7 requires; the
// exporter-added check is the mirror image and is always low risk.
func compareResources(base, head model.ProviderResource, mrmoSupported bool) []Finding {
	var findings []Finding

	if base.HasExporter && !head.HasExporter {
		risk := RiskLow
		if mrmoSupported {
			risk = RiskHigh
		}
		findings = append(findings, Finding{
			TerraformType: head.TerraformType,
			Kind:          KindExporterRemoved,
			Risk:          risk,
			MRMOSupported: mrmoSupported,
			Message:       "resource exporter was removed",
		})
	} else if !base.HasExporter && head.HasExporter {
		findings = append(findings, Finding{
			TerraformType: head.TerraformType,
			Kind:          KindExporterAdded,
			Risk:          RiskLow,
			MRMOSupported: mrmoSupported,
			Message:       "resource exporter was added",
		})
	}

	findings = append(findings, diffRefAttrs(base, head, mrmoSupported)...)
	findings = append(findings, diffEncodedRefAttrs(base, head, mrmoSupported)...)

	if base.IsSingleton != head.IsSingleton {
		findings = append(findings, Finding{
			TerraformType: head.TerraformType,
			Kind:          KindSingletonFlipped,
			Risk:          RiskMedium,
			MRMOSupported: mrmoSupported,
			BeforeValue:   boolString(base.IsSingleton),
			AfterValue:    boolString(head.IsSingleton),
			Message:       "isSingleton flipped between snapshots",
		})
	}
	if base.ExportID != head.ExportID {
		findings = append(findings, Finding{
			TerraformType: head.TerraformType,
			Kind:          KindExportIDChanged,
			Risk:          RiskMedium,
			MRMOSupported: mrmoSupported,
			BeforeValue:   base.ExportID,
			AfterValue:    head.ExportID,
			Message:       "exportId changed between snapshots",
		})
	}
	return findings
}

func diffRefAttrs(base, head model.ProviderResource, mrmoSupported bool) []Finding {
	var findings []Finding
	baseAttrs := indexRefAttrsByAttribute(base.RefAttrs)
	headAttrs := indexRefAttrsByAttribute(head.RefAttrs)

	for name, baseAttr := range baseAttrs {
		headAttr, present := headAttrs[name]
		if !present {
			risk := RiskLow
			if mrmoSupported {
				risk = RiskHigh
			}
			findings = append(findings, Finding{
				TerraformType: head.TerraformType,
				Kind:          KindRefAttrRemoved,
				Risk:          risk,
				MRMOSupported: mrmoSupported,
				Attribute:     name,
				BeforeValue:   baseAttr.RefType,
				Message:       "refAttr removed",
			})
			continue
		}
		if baseAttr.RefType != headAttr.RefType {
			risk := RiskLow
			if mrmoSupported {
				risk = RiskHigh
			}
			findings = append(findings, Finding{
				TerraformType: head.TerraformType,
				Kind:          KindRefAttrChanged,
				Risk:          risk,
				MRMOSupported: mrmoSupported,
				Attribute:     name,
				BeforeValue:   baseAttr.RefType,
				AfterValue:    headAttr.RefType,
				Message:       "refAttr refType changed",
			})
		}
	}
	for name, headAttr := range headAttrs {
		if _, existed := baseAttrs[name]; existed {
			continue
		}
		findings = append(findings, Finding{
			TerraformType: head.TerraformType,
			Kind:          KindRefAttrAdded,
			Risk:          RiskLow,
			MRMOSupported: mrmoSupported,
			Attribute:     name,
			AfterValue:    headAttr.RefType,
			Message:       "refAttr added",
		})
	}
	return findings
}

func diffEncodedRefAttrs(base, head model.ProviderResource, mrmoSupported bool) []Finding {
	var findings []Finding
	baseAttrs := indexEncodedRefAttrs(base.EncodedRefAttrs)
	headAttrs := indexEncodedRefAttrs(head.EncodedRefAttrs)

	for key, baseAttr := range baseAttrs {
		headAttr, present := headAttrs[key]
		if !present {
			risk := RiskLow
			if mrmoSupported {
				risk = RiskHigh
			}
			findings = append(findings, Finding{
				TerraformType: head.TerraformType,
				Kind:          KindEncodedRefAttrRemoved,
				Risk:          risk,
				MRMOSupported: mrmoSupported,
				Attribute:     key,
				BeforeValue:   baseAttr.RefType,
				Message:       "encodedRefAttr removed",
			})
			continue
		}
		if baseAttr.RefType != headAttr.RefType {
			risk := RiskLow
			if mrmoSupported {
				risk = RiskHigh
			}
			findings = append(findings, Finding{
				TerraformType: head.TerraformType,
				Kind:          KindEncodedRefAttrChanged,
				Risk:          risk,
				MRMOSupported: mrmoSupported,
				Attribute:     key,
				BeforeValue:   baseAttr.RefType,
				AfterValue:    headAttr.RefType,
				Message:       "encodedRefAttr refType changed",
			})
		}
	}
	for key, headAttr := range headAttrs {
		if _, existed := baseAttrs[key]; existed {
			continue
		}
		findings = append(findings, Finding{
			TerraformType: head.TerraformType,
			Kind:          KindEncodedRefAttrAdded,
			Risk:          RiskLow,
			MRMOSupported: mrmoSupported,
			Attribute:     key,
			AfterValue:    headAttr.RefType,
			Message:       "encodedRefAttr added",
		})
	}
	return findings
}

func resourceRemovedFinding(base model.ProviderResource, mrmoSupported bool) Finding {
	risk := RiskLow
	if mrmoSupported {
		risk = RiskHigh
	}
	return Finding{
		TerraformType: base.TerraformType,
		Kind:          KindResourceRemoved,
		Risk:          risk,
		MRMOSupported: mrmoSupported,
		Message:       "resource removed from provider",
	}
}

func resourceAddedFinding(head model.ProviderResource, mrmoSupported bool) Finding {
	return Finding{
		TerraformType: head.TerraformType,
		Kind:          KindResourceAdded,
		Risk:          RiskLow,
		MRMOSupported: mrmoSupported,
		Message:       "resource added to provider",
	}
}

func indexProviderResources(resources []model.ProviderResource) map[string]model.ProviderResource {
	out := make(map[string]model.ProviderResource, len(resources))
	for _, r := range resources {
		out[r.TerraformType] = r
	}
	return out
}

// indexMRMOSupported precomputes the "is this Terraform type registered
// in MRMO?" lookup that the risk classifier hammers on every finding.
// Only resources whose TerraformType is non-empty count; MRMO entries
// with only a ResourceTypeRef are not linkable to a provider diff.
func indexMRMOSupported(resources []model.MRMOResource) map[string]bool {
	out := make(map[string]bool, len(resources))
	for _, r := range resources {
		if r.TerraformType == "" {
			continue
		}
		out[r.TerraformType] = true
	}
	return out
}

func indexRefAttrsByAttribute(attrs []model.RefAttr) map[string]model.RefAttr {
	out := make(map[string]model.RefAttr, len(attrs))
	for _, a := range attrs {
		out[a.Attribute] = a
	}
	return out
}

// indexEncodedRefAttrs keys on "container.nested" because an
// EncodedRefAttr identity is the pair of paths; the same nested
// attribute inside two different containers is a different reference.
func indexEncodedRefAttrs(attrs []model.EncodedRefAttr) map[string]model.EncodedRefAttr {
	out := make(map[string]model.EncodedRefAttr, len(attrs))
	for _, a := range attrs {
		out[a.ContainerAttribute+"."+a.NestedAttribute] = a
	}
	return out
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
