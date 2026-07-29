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
- `roundtrip`: mock compare of source/target export JSON fixtures with normalized drift.

## Work Board

Use `TODO.md` as the lightweight Jira board for MRMO, CX as Code, and shared tasks.

## Current State

Both scanners read local checkouts and emit real manifests:

- Provider: registration + exporter metadata from `terraform-provider-genesyscloud`
- MRMO: registry, topics, hierarchy, handler factories, and integration-test coverage from `mrmo-replicator`

`make scan` joins those into a compatibility matrix (ready / warning / blocked).
