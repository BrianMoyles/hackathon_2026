# MRMO Scanner Work Plan

Read these inputs from `/Users/BMOYLES/genesys_src/repos/mrmo-replicator`:

- `internal/resourcetypes/registry.go`
- `config/topics.yaml`
- `config/resource-hierarchy.yml`
- `internal/handlers/`
- `internal/integration/tests/handlers_test.go`

Emit one `model.MRMOResource` per MRMO-supported resource.

Required checks:

- Registry entry exists.
- Topic wiring exists.
- Handler factory is registered.
- Hierarchy tier exists.
- Resource is reconciliation-eligible.
- Integration test coverage is present or missing.
