MRMO / CX as Code Compatibility Lab
===================================

Hackathon CLI for checking whether a CX as Code resource is ready for MRMO
replication and reconciliation.

The tool joins metadata from:

- MRMO: resource registry, Kafka topics, handlers, reconciliation hierarchy, tests.
- CX as Code: provider resources, exporters, RefAttrs, singleton metadata, file-output behavior.

## Quick Start

```bash
make scan
make explain
make deps
```

Or run directly:

```bash
go run ./cmd/compatibility-lab scan \
  --provider-repo /Users/BMOYLES/genesys_src/repos/terraform-provider-genesyscloud \
  --mrmo-repo /Users/BMOYLES/genesys_src/repos/mrmo-replicator
```

## Commands

- `scan`: build the compatibility matrix.
- `explain <resourceTypeOrRef>`: show why a resource is ready, risky, or blocked.
- `dependency-closure <resourceTypeOrRef>`: show exporter dependencies and MRMO support.
- `diff-provider-pr`: planned provider metadata diff command.
- `roundtrip`: planned export/apply/export drift test.

## Work Board

Use `TODO.md` as the lightweight Jira board for MRMO, CX as Code, and shared tasks.

## Current State

This is a scaffold. The scanners currently return a small `routing-queue` sample so the
CLI can be run while the MRMO and CX scanner implementations are filled in.
