package report

import (
	"fmt"
	"io"

	"compatibility-lab/internal/matrix"
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
