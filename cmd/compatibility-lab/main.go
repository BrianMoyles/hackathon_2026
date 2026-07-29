package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"compatibility-lab/internal/matrix"
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
		err = notImplemented("diff-provider-pr", "compare provider exporter metadata between two git refs")
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
	format := fs.String("format", "table", "output format: table or json")
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
	return writeReport(compatibilityReport, *format)
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
  compatibility-lab scan [--provider-repo path] [--mrmo-repo path] [--format table|json]
  compatibility-lab explain <resourceTypeOrRef>
  compatibility-lab dependency-closure <resourceTypeOrRef>
  compatibility-lab diff-provider-pr
  compatibility-lab roundtrip [--mode mock] [--source path] [--target path] [--resource type] [--format table|json]
`)
}
