package report

import (
	"fmt"
	"io"
	"strings"

	"compatibility-lab/internal/matrix"
	"compatibility-lab/internal/model"
)

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
