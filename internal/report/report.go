package report

import (
	"fmt"
	"io"
	"strings"

	"compatibility-lab/internal/matrix"
	"compatibility-lab/internal/model"
	"compatibility-lab/internal/providerdiff"
)

// diffRiskOrder controls both the section order in WriteDiffMarkdown and
// the row-group order in WriteDiffTable. It mirrors LAB-7's severity
// model: reviewers should see the high-risk findings before anything
// else, then descend into medium and low.
var diffRiskOrder = []providerdiff.Risk{providerdiff.RiskHigh, providerdiff.RiskMedium, providerdiff.RiskLow}

var diffRiskTitle = map[providerdiff.Risk]string{
	providerdiff.RiskHigh:   "High Risk",
	providerdiff.RiskMedium: "Medium Risk",
	providerdiff.RiskLow:    "Low Risk",
}

// markdownStatusOrder is the fixed section order for WriteMarkdown. It
// mirrors the resource-status precedence defined in the matrix package
// (blocked > warning > unknown > ready) so the most actionable findings
// show first in a PR comment.
var markdownStatusOrder = []string{"blocked", "warning", "unknown", "ready"}

// markdownSectionTitle is the human-facing heading for each status
// bucket. Kept in one place so the markdown golden and the table golden
// stay in sync when we tweak wording.
var markdownSectionTitle = map[string]string{
	"blocked": "Blocked",
	"warning": "Warning",
	"unknown": "Unknown",
	"ready":   "Ready",
}

func WriteTable(w io.Writer, report matrix.CompatibilityReport) error {
	if _, err := fmt.Fprintf(w, "Status   Score  Terraform Type                MRMO Ref\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "------   -----  --------------                --------\n"); err != nil {
		return err
	}
	for _, resource := range report.Resources {
		if _, err := fmt.Fprintf(
			w,
			"%-8s %-6d %-29s %s\n",
			resource.Status,
			resource.Score,
			resource.TerraformType,
			resource.ResourceTypeRef,
		); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(
		w,
		"\nSummary: %d ready, %d warning, %d unknown, %d blocked (of %d provider resources / %d MRMO resources)\n",
		report.Summary.ReadyCount,
		report.Summary.WarningCount,
		report.Summary.UnknownCount,
		report.Summary.BlockedCount,
		report.Summary.ProviderResourceCount,
		report.Summary.MRMOResourceCount,
	); err != nil {
		return err
	}
	return nil
}

func WriteResource(w io.Writer, resource matrix.ResourceReadiness) error {
	if _, err := fmt.Fprintf(w, "%s\n", resource.TerraformType); err != nil {
		return err
	}
	if resource.ResourceTypeRef != "" {
		if _, err := fmt.Fprintf(w, "MRMO ref: %s\n", resource.ResourceTypeRef); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Status: %s\nScore: %d\n", resource.Status, resource.Score); err != nil {
		return err
	}

	if resource.Provider != nil {
		if _, err := fmt.Fprintf(w, "Provider exporter: %t\n", resource.Provider.HasExporter); err != nil {
			return err
		}
		if err := writeFileOutput(w, resource.Provider); err != nil {
			return err
		}
		if err := writeBlockHash(w, resource.Provider); err != nil {
			return err
		}
	}
	if resource.MRMO != nil {
		if _, err := fmt.Fprintf(w, "MRMO tier: %d\n", resource.MRMO.Tier); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Reconciliation eligible: %t\n", resource.MRMO.ReconciliationEligible); err != nil {
			return err
		}
		if resource.MRMO.IntegrationTestStatus != "" {
			if _, err := fmt.Fprintf(w, "Integration tests: %s\n", resource.MRMO.IntegrationTestStatus); err != nil {
				return err
			}
		}
		for _, topic := range resource.MRMO.Topics {
			if _, err := fmt.Fprintf(w, "Topic: %s -> %s\n", topic.Topic, topic.Handler); err != nil {
				return err
			}
		}
	}
	for _, issue := range resource.Issues {
		if _, err := fmt.Fprintf(w, "%s: %s - %s\n", issue.Severity, issue.Code, issue.Message); err != nil {
			return err
		}
	}
	return nil
}

// writeFileOutput surfaces CX-5's file-output metadata in explain output.
// The block is printed whenever the exporter writes files or declares
// third-party file references, so flow / user-prompt / script-style
// resources show their output behavior explicitly instead of being
// silently indistinguishable from a plain resource.
func writeFileOutput(w io.Writer, provider *model.ProviderResource) error {
	if !provider.WritesFiles && len(provider.ThirdPartyRefAttrs) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "Writes files: %t\n", provider.WritesFiles); err != nil {
		return err
	}
	if provider.CustomFileDirectory != "" {
		if _, err := fmt.Fprintf(w, "Output sub-directory: %s\n", provider.CustomFileDirectory); err != nil {
			return err
		}
	}
	if len(provider.ThirdPartyRefAttrs) > 0 {
		if _, err := fmt.Fprintf(w, "Third-party ref attributes: %s\n", strings.Join(provider.ThirdPartyRefAttrs, ", ")); err != nil {
			return err
		}
	}
	return nil
}

// writeBlockHash surfaces CX-6's BlockHash observation. The status is
// spelled out either way — "observed" or "unknown (no static
// QuickHashFields call)" — so a missing hash is loud rather than hidden.
// The line is only printed when the resource has an exporter; without one
// the field is not meaningful.
func writeBlockHash(w io.Writer, provider *model.ProviderResource) error {
	if !provider.HasExporter {
		return nil
	}
	status := "unknown (no static QuickHashFields call or ResourceMeta.BlockHash assignment found)"
	if provider.BlockHashObserved {
		status = "observed"
	}
	_, err := fmt.Fprintf(w, "Block hash: %s\n", status)
	return err
}

// WriteMarkdown renders a PR-friendly markdown view of the compatibility
// report. The layout is intentionally opinionated:
//
//   - A single top-level H1 makes the report copy-pasteable into GitHub PR
//     comments without colliding with a repo's markdown heading levels.
//   - A summary table gives reviewers the shape at a glance.
//   - Sections come in the status precedence order defined by the matrix
//     package (blocked > warning > unknown > ready) so the most actionable
//     items sit at the top of the reviewer's scroll.
//   - Blocked / Warning / Unknown resources are printed as verbose blocks
//     with issue bullets, because the reader needs the "why".
//   - Ready resources collapse into a `<details>` element with a compact
//     table, because there can be a lot of them and they don't need
//     inline attention.
//
// The output is deterministic: it consumes the already-sorted
// CompatibilityReport.Resources slice and never re-sorts it, so LAB-3's
// alphabetical guarantee flows straight through into markdown.
func WriteMarkdown(w io.Writer, report matrix.CompatibilityReport) error {
	buckets := groupByStatus(report.Resources)

	if _, err := fmt.Fprintln(w, "# Compatibility Report"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := writeMarkdownHeader(w, report); err != nil {
		return err
	}
	if err := writeMarkdownSummaryTable(w, report.Summary); err != nil {
		return err
	}

	for _, status := range markdownStatusOrder {
		resources := buckets[status]
		if len(resources) == 0 {
			continue
		}
		if status == "ready" {
			if err := writeMarkdownReadyBlock(w, resources); err != nil {
				return err
			}
			continue
		}
		if err := writeMarkdownIssueSection(w, status, resources); err != nil {
			return err
		}
	}

	return nil
}

// groupByStatus keeps the existing ordering within each bucket (Build
// already sorted by TerraformType), so the caller cannot re-order behind
// our backs.
func groupByStatus(resources []matrix.ResourceReadiness) map[string][]matrix.ResourceReadiness {
	buckets := make(map[string][]matrix.ResourceReadiness, len(markdownStatusOrder))
	for _, r := range resources {
		buckets[r.Status] = append(buckets[r.Status], r)
	}
	return buckets
}

// writeMarkdownHeader prints the schema + inputs line. Both paths are
// rendered even when empty so the shape of the document is fixed; the
// LAB-3 golden fixtures set RepoPath = "PROVIDER_FIXTURE" / "MRMO_FIXTURE"
// for exactly this reason.
func writeMarkdownHeader(w io.Writer, report matrix.CompatibilityReport) error {
	_, err := fmt.Fprintf(
		w,
		"**Schema:** `%s` · **Provider:** `%s` · **MRMO:** `%s`\n\n",
		report.SchemaVersion,
		nonEmpty(report.Inputs.ProviderRepo),
		nonEmpty(report.Inputs.MRMORepo),
	)
	return err
}

func writeMarkdownSummaryTable(w io.Writer, summary matrix.Summary) error {
	if _, err := fmt.Fprintln(w, "| Ready | Warning | Unknown | Blocked | Provider Resources | MRMO Resources |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|:-----:|:-------:|:-------:|:-------:|:------------------:|:--------------:|"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		w,
		"| %d | %d | %d | %d | %d | %d |\n\n",
		summary.ReadyCount,
		summary.WarningCount,
		summary.UnknownCount,
		summary.BlockedCount,
		summary.ProviderResourceCount,
		summary.MRMOResourceCount,
	); err != nil {
		return err
	}
	return nil
}

// writeMarkdownIssueSection prints the verbose per-resource block used
// for the Blocked / Warning / Unknown buckets. Each resource gets:
//
//   - an H3 with the Terraform type (backticked so long names don't
//     glitch GitHub's markdown parser),
//   - a bullet list of context (MRMO ref, tier, score),
//   - a bullet list of issues with severity, code, and message,
//   - a MRMO-orphan line when the provider side is missing entirely,
//     which is the specific case CX-3 / CX-4 flag as blocked.
func writeMarkdownIssueSection(w io.Writer, status string, resources []matrix.ResourceReadiness) error {
	title := markdownSectionTitle[status]
	if _, err := fmt.Fprintf(w, "## %s (%d)\n\n", title, len(resources)); err != nil {
		return err
	}
	for _, resource := range resources {
		if _, err := fmt.Fprintf(w, "### `%s`\n\n", resource.TerraformType); err != nil {
			return err
		}
		if resource.ResourceTypeRef != "" {
			if _, err := fmt.Fprintf(w, "- **MRMO ref:** `%s`\n", resource.ResourceTypeRef); err != nil {
				return err
			}
		}
		if resource.MRMO != nil {
			if _, err := fmt.Fprintf(w, "- **MRMO tier:** %d\n", resource.MRMO.Tier); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "- **Score:** %d\n", resource.Score); err != nil {
			return err
		}
		if resource.Provider == nil && resource.MRMO != nil {
			if _, err := fmt.Fprintln(w, "- **Provider-side:** _no matching provider resource_"); err != nil {
				return err
			}
		}
		for _, issue := range resource.Issues {
			if _, err := fmt.Fprintf(
				w,
				"- **%s · `%s`** — %s\n",
				issue.Severity,
				issue.Code,
				issue.Message,
			); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

// writeMarkdownReadyBlock renders the Ready bucket as a collapsible
// details block with a compact table. Reviewers see the count in the
// summary and can expand only when they want to audit which resources
// passed.
func writeMarkdownReadyBlock(w io.Writer, resources []matrix.ResourceReadiness) error {
	if _, err := fmt.Fprintf(w, "<details>\n<summary><strong>Ready (%d)</strong></summary>\n\n", len(resources)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| Terraform Type | MRMO Ref | Tier | Score |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---|:---:|:---:|"); err != nil {
		return err
	}
	for _, resource := range resources {
		tier := "-"
		if resource.MRMO != nil {
			tier = fmt.Sprintf("%d", resource.MRMO.Tier)
		}
		ref := resource.ResourceTypeRef
		if ref == "" {
			ref = "-"
		}
		if _, err := fmt.Fprintf(
			w,
			"| `%s` | `%s` | %s | %d |\n",
			resource.TerraformType,
			ref,
			tier,
			resource.Score,
		); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\n</details>"); err != nil {
		return err
	}
	return nil
}

// nonEmpty replaces empty repo paths with a placeholder so the markdown
// header keeps its shape when a scan was run against manifests that
// didn't record a path (e.g. golden fixtures).
func nonEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// WriteDiffTable renders a LAB-7 provider-PR diff as a fixed-column
// text table. Findings are grouped by risk (high → medium → low)
// because that is the order a reviewer wants to walk the report; within
// a group the ordering is whatever providerdiff.Diff produced, which is
// already deterministic (sorted by TerraformType + Kind + Attribute).
func WriteDiffTable(w io.Writer, report providerdiff.DiffReport) error {
	if _, err := fmt.Fprintf(
		w,
		"Provider PR Diff  base=%q head=%q\n\nSummary: %d high, %d medium, %d low (of %d findings)\n\n",
		report.Inputs.BaseRef,
		report.Inputs.HeadRef,
		report.Summary.HighRiskCount,
		report.Summary.MediumRiskCount,
		report.Summary.LowRiskCount,
		report.Summary.TotalFindings,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Risk    MRMO  Kind                          Terraform Type                 Attribute            Before → After\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "----    ----  ----                          --------------                 ---------            --------------\n"); err != nil {
		return err
	}
	for _, risk := range diffRiskOrder {
		for _, finding := range report.Findings {
			if finding.Risk != risk {
				continue
			}
			mrmo := "no"
			if finding.MRMOSupported {
				mrmo = "yes"
			}
			transition := diffTransitionText(finding)
			if _, err := fmt.Fprintf(
				w,
				"%-6s  %-4s  %-29s %-30s %-20s %s\n",
				finding.Risk,
				mrmo,
				finding.Kind,
				finding.TerraformType,
				truncate(finding.Attribute, 20),
				transition,
			); err != nil {
				return err
			}
		}
	}
	if report.Summary.TotalFindings == 0 {
		if _, err := fmt.Fprintln(w, "(no findings)"); err != nil {
			return err
		}
	}
	return nil
}

// WriteDiffMarkdown renders the LAB-7 diff report as a PR-comment-ready
// markdown document. The layout mirrors WriteMarkdown for compatibility
// reports (title, summary table, then sections in severity order) so
// consumers get one visual language across both commands.
func WriteDiffMarkdown(w io.Writer, report providerdiff.DiffReport) error {
	buckets := groupFindingsByRisk(report.Findings)

	if _, err := fmt.Fprintln(w, "# Provider PR Diff"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		w,
		"**Schema:** `%s` · **Base:** `%s` · **Head:** `%s`\n\n",
		report.SchemaVersion,
		nonEmpty(report.Inputs.BaseRef),
		nonEmpty(report.Inputs.HeadRef),
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| High | Medium | Low | Total |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|:----:|:------:|:---:|:-----:|"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		w,
		"| %d | %d | %d | %d |\n\n",
		report.Summary.HighRiskCount,
		report.Summary.MediumRiskCount,
		report.Summary.LowRiskCount,
		report.Summary.TotalFindings,
	); err != nil {
		return err
	}

	if report.Summary.TotalFindings == 0 {
		if _, err := fmt.Fprintln(w, "_No differences detected between the two snapshots._"); err != nil {
			return err
		}
		return nil
	}

	for _, risk := range diffRiskOrder {
		findings := buckets[risk]
		if len(findings) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "## %s (%d)\n\n", diffRiskTitle[risk], len(findings)); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "| Terraform Type | Kind | Attribute | Before → After | MRMO |"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "|---|---|---|---|:---:|"); err != nil {
			return err
		}
		for _, finding := range findings {
			mrmo := "no"
			if finding.MRMOSupported {
				mrmo = "**yes**"
			}
			if _, err := fmt.Fprintf(
				w,
				"| `%s` | `%s` | %s | %s | %s |\n",
				finding.TerraformType,
				finding.Kind,
				markdownAttribute(finding.Attribute),
				markdownTransition(finding),
				mrmo,
			); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func groupFindingsByRisk(findings []providerdiff.Finding) map[providerdiff.Risk][]providerdiff.Finding {
	buckets := make(map[providerdiff.Risk][]providerdiff.Finding, len(diffRiskOrder))
	for _, f := range findings {
		buckets[f.Risk] = append(buckets[f.Risk], f)
	}
	return buckets
}

// diffTransitionText renders the before/after pair for the table
// column. It intentionally uses a plain ASCII arrow ("->") to keep the
// terminal output width-stable in monospace fonts; the markdown formatter
// uses "→" for legibility since GitHub renders unicode fine.
func diffTransitionText(f providerdiff.Finding) string {
	switch {
	case f.BeforeValue != "" && f.AfterValue != "":
		return fmt.Sprintf("%s -> %s", f.BeforeValue, f.AfterValue)
	case f.BeforeValue != "":
		return fmt.Sprintf("%s -> (removed)", f.BeforeValue)
	case f.AfterValue != "":
		return fmt.Sprintf("(added) -> %s", f.AfterValue)
	default:
		return ""
	}
}

func markdownAttribute(attribute string) string {
	if attribute == "" {
		return "-"
	}
	return "`" + attribute + "`"
}

func markdownTransition(f providerdiff.Finding) string {
	switch {
	case f.BeforeValue != "" && f.AfterValue != "":
		return fmt.Sprintf("`%s` → `%s`", f.BeforeValue, f.AfterValue)
	case f.BeforeValue != "":
		return fmt.Sprintf("`%s` → _removed_", f.BeforeValue)
	case f.AfterValue != "":
		return fmt.Sprintf("_added_ → `%s`", f.AfterValue)
	default:
		return "-"
	}
}

// truncate keeps table columns width-stable. The three-dot ellipsis is
// ASCII rather than the "…" character so the column width in a terminal
// matches the header column width, which uses ASCII dashes.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func WriteDependencies(w io.Writer, dependencies []matrix.DependencyReadiness) error {
	if len(dependencies) == 0 {
		_, err := fmt.Fprintln(w, "No dependencies found.")
		return err
	}
	for _, dependency := range dependencies {
		if _, err := fmt.Fprintf(
			w,
			"%s from %s: providerExportable=%t mrmoSupported=%t status=%s\n",
			dependency.TerraformType,
			dependency.Source,
			dependency.ProviderExportable,
			dependency.MRMOSupported,
			dependency.Status,
		); err != nil {
			return err
		}
	}
	return nil
}
