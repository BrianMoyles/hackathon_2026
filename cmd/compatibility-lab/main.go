package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"compatibility-lab/internal/matrix"
	"compatibility-lab/internal/model"
	"compatibility-lab/internal/providerdiff"
	"compatibility-lab/internal/report"
	"compatibility-lab/internal/roundtrip"
	"compatibility-lab/internal/scanner/mrmo"
	"compatibility-lab/internal/scanner/provider"
)

const defaultProviderRepo = "/Users/BMOYLES/genesys_src/repos/terraform-provider-genesyscloud"
const defaultMRMORepo = "/Users/BMOYLES/genesys_src/repos/mrmo-replicator"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "scan":
		err = runScan(os.Args[2:])
	case "explain":
		err = runExplain(os.Args[2:])
	case "dependency-closure":
		err = runDependencyClosure(os.Args[2:])
	case "diff-provider-pr":
		err = runDiffProviderPR(os.Args[2:])
	case "roundtrip":
		err = runRoundtrip(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	providerRepo := fs.String("provider-repo", defaultProviderRepo, "path to terraform-provider-genesyscloud")
	mrmoRepo := fs.String("mrmo-repo", defaultMRMORepo, "path to mrmo-replicator")
	format := fs.String("format", "table", "output format: table, json, or markdown")
	strict := fs.Bool("strict", false, "exit non-zero when any resource is blocked (warnings and unknowns stay report-only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	providerManifest, err := provider.Scan(*providerRepo)
	if err != nil {
		return err
	}
	mrmoManifest, err := mrmo.Scan(*mrmoRepo)
	if err != nil {
		return err
	}

	compatibilityReport := matrix.Build(providerManifest, mrmoManifest)
	if err := writeReport(compatibilityReport, *format); err != nil {
		return err
	}

	// LAB-2: --strict flips a green run into a red one when any resource is
	// blocked. Warnings and unknowns still surface in the report but do not
	// fail the exit code, so CI can be strict about blockers without
	// getting drowned in noise from static-analysis "unknowns".
	if *strict && compatibilityReport.HasStrictFailures() {
		return fmt.Errorf("strict mode: %d blocked resource(s) found", compatibilityReport.Summary.BlockedCount)
	}
	return nil
}

func runExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ExitOnError)
	providerRepo := fs.String("provider-repo", defaultProviderRepo, "path to terraform-provider-genesyscloud")
	mrmoRepo := fs.String("mrmo-repo", defaultMRMORepo, "path to mrmo-replicator")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: compatibility-lab explain <resourceTypeOrRef>")
	}

	providerManifest, err := provider.Scan(*providerRepo)
	if err != nil {
		return err
	}
	mrmoManifest, err := mrmo.Scan(*mrmoRepo)
	if err != nil {
		return err
	}

	resourceReport := matrix.Explain(providerManifest, mrmoManifest, fs.Arg(0))
	return report.WriteResource(os.Stdout, resourceReport)
}

func runDependencyClosure(args []string) error {
	fs := flag.NewFlagSet("dependency-closure", flag.ExitOnError)
	providerRepo := fs.String("provider-repo", defaultProviderRepo, "path to terraform-provider-genesyscloud")
	mrmoRepo := fs.String("mrmo-repo", defaultMRMORepo, "path to mrmo-replicator")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: compatibility-lab dependency-closure <resourceTypeOrRef>")
	}

	providerManifest, err := provider.Scan(*providerRepo)
	if err != nil {
		return err
	}
	mrmoManifest, err := mrmo.Scan(*mrmoRepo)
	if err != nil {
		return err
	}

	closure := matrix.DependencyClosure(providerManifest, mrmoManifest, fs.Arg(0))
	return report.WriteDependencies(os.Stdout, closure)
}

// runDiffProviderPR is LAB-7's CLI surface. It compares two snapshots of
// the terraform-provider-genesyscloud repo and surfaces the changes that
// would break MRMO downstream. Two operational modes are supported:
//
//   - Git-ref mode (default): --provider-repo + --base + --head materialize
//     the two refs into temporary worktrees, scan each, and diff. This is
//     the "point at a PR branch and go" path.
//   - Manifest-file mode: --base-manifest + --head-manifest read two
//     pre-scanned JSON manifests. This exists so CI pipelines that already
//     run `scan --format json` on the base branch can persist that artifact
//     and diff head against it later, without a second git checkout.
//
// --strict flips the exit code non-zero when any finding is high risk,
// mirroring the --strict flag on `scan`. That gives CI a symmetric knob:
// one for "resource is out of sync" (scan) and one for "provider PR breaks
// MRMO" (diff).
func runDiffProviderPR(args []string) error {
	fs := flag.NewFlagSet("diff-provider-pr", flag.ExitOnError)
	providerRepo := fs.String("provider-repo", defaultProviderRepo, "path to terraform-provider-genesyscloud (git-ref mode)")
	baseRef := fs.String("base", "main", "base git ref (git-ref mode)")
	headRef := fs.String("head", "HEAD", "head git ref (git-ref mode)")
	baseManifestFile := fs.String("base-manifest", "", "pre-scanned base provider manifest JSON (manifest-file mode)")
	headManifestFile := fs.String("head-manifest", "", "pre-scanned head provider manifest JSON (manifest-file mode)")
	mrmoRepo := fs.String("mrmo-repo", defaultMRMORepo, "path to mrmo-replicator (optional; enables MRMO-aware risk grading)")
	format := fs.String("format", "table", "output format: table, json, or markdown")
	strict := fs.Bool("strict", false, "exit non-zero when any finding is high risk")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Mode disambiguation: manifest files take precedence when set, so a
	// CI job passing both --provider-repo (from environment) and explicit
	// manifest files does not accidentally trigger a git checkout.
	usingManifestFiles := *baseManifestFile != "" || *headManifestFile != ""
	if usingManifestFiles && (*baseManifestFile == "" || *headManifestFile == "") {
		return fmt.Errorf("both --base-manifest and --head-manifest are required in manifest-file mode")
	}

	var baseManifest, headManifest model.ProviderManifest
	var cleanup func()
	var err error
	if usingManifestFiles {
		baseManifest, err = readProviderManifestFile(*baseManifestFile)
		if err != nil {
			return fmt.Errorf("read base manifest: %w", err)
		}
		headManifest, err = readProviderManifestFile(*headManifestFile)
		if err != nil {
			return fmt.Errorf("read head manifest: %w", err)
		}
	} else {
		baseManifest, headManifest, cleanup, err = providerdiff.CompareRefs(*providerRepo, *baseRef, *headRef)
		defer cleanup()
		if err != nil {
			return err
		}
	}

	var mrmoManifest model.MRMOManifest
	if *mrmoRepo != "" {
		if scanned, scanErr := mrmo.Scan(*mrmoRepo); scanErr == nil {
			mrmoManifest = scanned
		}
		// Missing MRMO repo is not fatal: every finding just defaults
		// to `mrmoSupported: false` and the risk model degrades to
		// "provider-only" grading. That is intentional for CI jobs
		// that only have the provider checkout available.
	}

	diffReport := providerdiff.Diff(baseManifest, headManifest, mrmoManifest)
	diffReport.Inputs.ProviderRepo = *providerRepo
	diffReport.Inputs.MRMORepo = *mrmoRepo
	if !usingManifestFiles {
		diffReport.Inputs.BaseRef = *baseRef
		diffReport.Inputs.HeadRef = *headRef
	} else {
		diffReport.Inputs.BaseRef = *baseManifestFile
		diffReport.Inputs.HeadRef = *headManifestFile
	}

	if err := writeDiffReport(diffReport, *format); err != nil {
		return err
	}

	if *strict && diffReport.Summary.HighRiskCount > 0 {
		return fmt.Errorf("strict mode: %d high-risk finding(s)", diffReport.Summary.HighRiskCount)
	}
	return nil
}

func writeDiffReport(diffReport providerdiff.DiffReport, format string) error {
	switch format {
	case "table":
		return report.WriteDiffTable(os.Stdout, diffReport)
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(diffReport)
	case "markdown":
		return report.WriteDiffMarkdown(os.Stdout, diffReport)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func readProviderManifestFile(path string) (model.ProviderManifest, error) {
	var manifest model.ProviderManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, fmt.Errorf("parse %s: %w", path, err)
	}
	return manifest, nil
}

func runRoundtrip(args []string) error {
	fs := flag.NewFlagSet("roundtrip", flag.ExitOnError)
	source := fs.String("source", "testdata/fixtures/roundtrip/source.json", "source export JSON fixture")
	target := fs.String("target", "testdata/fixtures/roundtrip/target.json", "target export JSON fixture")
	resourceType := fs.String("resource", "", "optional Terraform resource type filter")
	format := fs.String("format", "table", "output format: table or json")
	mode := fs.String("mode", "mock", "roundtrip mode (only mock is implemented)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *mode != "mock" {
		return fmt.Errorf("unsupported roundtrip mode %q (only mock is implemented)", *mode)
	}

	driftReport, err := roundtrip.CompareFiles(*source, *target, *resourceType)
	if err != nil {
		return err
	}

	switch *format {
	case "table":
		fmt.Print(roundtrip.FormatTable(driftReport))
		return nil
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(driftReport)
	default:
		return fmt.Errorf("unsupported format %q", *format)
	}
}

func writeReport(compatibilityReport matrix.CompatibilityReport, format string) error {
	switch format {
	case "table":
		return report.WriteTable(os.Stdout, compatibilityReport)
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(compatibilityReport)
	case "markdown":
		return report.WriteMarkdown(os.Stdout, compatibilityReport)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func notImplemented(command, purpose string) error {
	fmt.Printf("%s is planned but not implemented yet: %s.\n", command, purpose)
	return nil
}

func usage() {
	fmt.Print(`MRMO / CX as Code Compatibility Lab

Usage:
  compatibility-lab scan [--provider-repo path] [--mrmo-repo path] [--format table|json|markdown] [--strict]
  compatibility-lab explain <resourceTypeOrRef>
  compatibility-lab dependency-closure <resourceTypeOrRef>
  compatibility-lab diff-provider-pr [--provider-repo path] [--base ref] [--head ref] [--mrmo-repo path] [--format table|json|markdown] [--strict]
  compatibility-lab diff-provider-pr --base-manifest path --head-manifest path [--mrmo-repo path] [--format table|json|markdown]
  compatibility-lab roundtrip [--mode mock] [--source path] [--target path] [--resource type] [--format table|json]
`)
}
