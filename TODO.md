# Compatibility Lab Jira Board

Use this as the hackathon board until the work is moved into Jira. Keep each card small enough that one person can finish it without blocking the other stream.

## Sprint Goal

Deliver an offline CLI that scans both repos, joins provider exporter metadata with MRMO readiness metadata, and produces a clear compatibility report for at least one ready resource, one warning resource, and one blocked resource.

## MRMO Developer Board

<!-- ### MRMO-1: Parse MRMO Resource Registry

- Owner: `MRMO`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/scanner/mrmo/scanner.go`, MRMO `internal/resourcetypes/registry.go`
- Goal: Extract `resourceTypeRef`, Terraform type, domain, and tier source.
- Acceptance: Scanner emits all MRMO registry entries with stable refs and Terraform types. -->

<!-- ### MRMO-2: Parse Topics YAML

- Owner: `MRMO`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/scanner/mrmo/scanner.go`, MRMO `config/topics.yaml`
- Goal: Extract topic, handler, handlerMap entries, Avro schema path, supported types, validation, and `resourceTypeRef`.
- Acceptance: Scanner maps each resource ref to its topic and handler wiring. -->

<!-- ### MRMO-3: Parse Resource Hierarchy

- Owner: `MRMO`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/scanner/mrmo/scanner.go`, MRMO `config/resource-hierarchy.yml`
- Goal: Extract reconciliation tier for each Terraform resource type.
- Acceptance: Missing hierarchy tier becomes `MRMO_HIERARCHY_TIER_MISSING`. -->

### MRMO-4: Detect Handler Factory Coverage

- Owner: `MRMO`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/scanner/mrmo/scanner.go`, MRMO `internal/handlers/`
- Goal: Scan `RegisterHandlerFactory` calls and map handler names to files.
- Acceptance: Topic handler names without a factory become `MRMO_HANDLER_FACTORY_MISSING`.

### MRMO-5: Detect Integration Test Coverage

- Owner: `MRMO`
- Priority: `P1`
- Status: `Todo`
- Files: `internal/scanner/mrmo/scanner.go`, MRMO `internal/integration/tests/handlers_test.go`
- Goal: Identify whether each MRMO-supported resource has handler pipeline test coverage.
- Acceptance: Missing or uncertain coverage appears as a warning, not a blocker.

### MRMO-6: Reconciliation Eligibility Check

- Owner: `MRMO`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/scanner/mrmo/scanner.go`
- Goal: Mark a resource eligible only when it is topic-wired and present in the hierarchy.
- Acceptance: `explain <resource>` shows whether reconciliation will include the resource.

## CX as Code Developer Board

### CX-1: Provider Resource Catalog

- Owner: `CX`
- Priority: `P0`
- Status: `Done`
- Files: `internal/scanner/provider/scanner.go`, provider `provider_registrar.go`
- Goal: Extract provider resource, data source, and exporter registration status.
- Acceptance: Scanner distinguishes `hasResource`, `hasDataSource`, and `hasExporter`.
- Notes: Static AST walk of `<repo>/genesyscloud/**/*.go` (test files skipped). Collects `RegisterResource` / `RegisterDataSource` / `RegisterExporter` call sites; first argument resolved from string literal or package-local `const`/`var`. Cross-package qualified selectors are skipped for now (safe default until CX-2 needs them). Unit-tested via a synthetic mini-provider tree so behavior is locked without a live checkout.

### CX-2: Exporter Metadata Snapshot

- Owner: `CX`
- Priority: `P0`
- Status: `Done`
- Files: `internal/scanner/provider/scanner.go`, provider `resource_exporter.go`
- Goal: Emit `RefAttrs`, `ExcludedAttributes`, singleton flags, `ExportId`, file-output fields, and custom resolver presence.
- Acceptance: `genesyscloud_routing_queue` and `genesyscloud_routing_utilization` show meaningful exporter metadata.
- Notes: Extended the scanner to walk each `RegisterExporter(RSType, FooExporter())` call site to its exporter function, locate the `&resourceExporter.ResourceExporter{...}` composite literal in the return statement, and pull the CX-2 fields off it. Cross-package `alias.ResourceType` selectors on `RefType` are resolved via each file's import table, which cleanly covers 23 of the 24 `routing_queue` RefAttrs on the live provider (the one unresolved attribute is a `{}` TODO placeholder in the provider source itself, not a scanner bug). Verified against the real repo: `routing_queue` reports 24 RefAttrs + `HasCustomResolvers: true`; `routing_utilization` reports `IsSingleton: true, ExportID: "genesyscloud_routing_utilization"`; `architect_user_prompt` reports `CustomFileDirectory: "audio_prompts"` + two `ThirdPartyRefAttrs`. `EncodedRefAttrs` and `BlockHash` intentionally deferred to CX-3 / CX-6.

### CX-3: RefAttr Dependency Graph

- Owner: `CX`
- Priority: `P0`
- Status: `Done`
- Files: `internal/scanner/provider/scanner.go`, `internal/matrix/matrix.go`, `internal/model/model.go`
- Goal: Convert provider `RefAttrs` and `EncodedRefAttrs` into dependency edges.
- Acceptance: `dependency-closure genesyscloud_routing_queue` shows direct dependencies and their readiness.
- Notes: Added `EncodedRefAttr` type + `EncodedRefAttrs` field to `model.ProviderResource`. Scanner now walks the `map[*JsonEncodeRefAttr]*RefAttrSettings{...}` composite literal (Go auto-addresses struct-literal keys, so both `&Y.JsonEncodeRefAttr{...}` and bare `{Attr: ..., NestedAttr: ...}` spellings are handled). Matrix `buildDependencies` was reworked to (a) always emit dep edges — including for blocked resources so operators can see the graph — and (b) compute real `ProviderExportable` / `MRMOSupported` / `Status` per edge by cross-referencing the provider and MRMO manifests instead of the old hardcoded `warning`. Edge classification: `ready` = exportable + MRMO-supported, `warning` = exportable + not MRMO-supported, `blocked` = not exportable, `unknown` = RefType could not be resolved statically. Verified on the live provider: `dependency-closure genesyscloud_routing_queue` prints all 24 direct RefAttr edges with per-edge readiness; `dependency-closure genesyscloud_integration` prints one `RefAttrs.*` edge plus six `EncodedRefAttrs.config.properties.*` edges.

### CX-4: Singleton Safety Validation

- Owner: `CX`
- Priority: `P0`
- Status: `Done`
- Files: `internal/scanner/provider/scanner.go`, `internal/matrix/matrix.go`
- Goal: Check `IsSingleton` resources always have an `ExportId`.
- Acceptance: Missing singleton export IDs become `PROVIDER_SINGLETON_EXPORT_ID_MISSING`.
- Notes: The `IsSingleton` and `ExportID` fields are already populated by the CX-2 scanner, so CX-4 just needed a matrix blocker that pairs them. Added `PROVIDER_SINGLETON_EXPORT_ID_MISSING` alongside the other provider-side blockers in `buildResourceReadiness`, guarded by `providerResource != nil && IsSingleton && ExportID == ""` so it doesn't double-fire when the resource block itself is missing. Locked with `TestBuild_SingletonExportIDMissing`, which walks Build with three fixtures (broken singleton, healthy singleton, non-singleton) and asserts the blocker fires exactly on the broken one. Verified on the live provider: all 14 singletons (`genesyscloud_idp_*`, `routing_utilization`, `routing_settings`, `organization_authentication_settings`, `outbound_settings`, `outbound_wrapupcodemappings`, `conversations_settings`, `conversations_messaging_supportedcontent_default`) already carry an `ExportID`, so the new blocker stays quiet and will only surface if a future singleton is landed without one.

### CX-5: File Output Metadata

- Owner: `CX`
- Priority: `P1`
- Status: `Done`
- Files: `internal/scanner/provider/scanner.go`, `internal/model/model.go`, `internal/report/report.go`
- Goal: Detect `ThirdPartyRefAttrs` and `CustomFileWriter.SubDirectory` for resources that write files.
- Acceptance: Flow/user-prompt style resources show output-file behavior in `explain`.
- Notes: `ThirdPartyRefAttrs` and `CustomFileWriter.SubDirectory` were already lifted off the `ResourceExporter` composite literal by CX-2. CX-5 adds `WritesFiles bool` on `ProviderResource`: true whenever the `CustomFileWriter{}` literal declares any field (writer func or SubDirectory), matching the runtime nil-check tfexporter uses. `report.WriteResource` now prints a dedicated file-output block (`Writes files`, `Output sub-directory`, `Third-party ref attributes`) whenever either signal is present. Also extended `findResourceExporterLiteral` to follow `return <ident>` back to a `<ident> := &resourceExporter.ResourceExporter{...}` assignment so architect_flow's variable-first return pattern is no longer silently skipped — this alone lit up `genesyscloud_flow`'s ThirdPartyRefAttrs and (via CX-6) its BlockHash observation. Verified on live provider: 7 resources report `WritesFiles: true` — `architect_user_prompt` (audio_prompts), `script` (scripts), `greeting`/`group_greeting` (greeting_audio), `outbound_contact_list` (contacts), `responsemanagement_responseasset` (response_assets), `architect_grammar_language` (language_files). Locked with `TestScan_FileOutputMetadata` covering the writer-func-only edge case (WritesFiles=true, CustomFileDirectory=""), plus a WritesFiles assertion added to the existing CX-2 fake_file_writer test.

### CX-6: BlockHash And BlockLabel Hints

- Owner: `CX`
- Priority: `P2`
- Status: `Done`
- Files: `internal/scanner/provider/scanner.go`, `internal/report/report.go`
- Goal: Static-scan `QuickHashFields` and ResourceMeta label creation where practical.
- Acceptance: Unknown BlockHash remains explicit instead of hidden.
- Notes: `QuickHashFields` lives inside each exporter's `GetResourcesFunc` body, not on the `ResourceExporter` composite literal, so the scanner now extracts the wrapped function name from `GetResourcesFunc: provider.GetAllWithPooledClient(<funcName>)` (and the bare-ident form) and walks that function's body for either a `util.QuickHashFields(...)` call OR a `ResourceMeta{BlockHash: ...}` composite literal. Either match sets `BlockHashObserved = true`; anything else stays false. `report.WriteResource` prints the state explicitly as `Block hash: observed` or `Block hash: unknown (no static QuickHashFields call or ResourceMeta.BlockHash assignment found)` for every exporter, so the "unknown" case is loud instead of silent. Verified on live provider: 9 resources report `BlockHashObserved: true` — `user`, `integration`, `did_pool`, `flow`, `integration_action`, `externalcontacts_contact`, `knowledge_document`, `responsemanagement_response`, `responsemanagement_responseasset` — matching every direct `util.QuickHashFields` caller in the repo. Locked with `TestScan_BlockHashObserved` covering all three paths (util call, ResourceMeta assignment, and the "no hash" case that must remain explicit).

## Shared Board

### LAB-1: Replace Sample Data With Real Scanner Output

- Owner: `Shared`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/scanner/mrmo/scanner.go`, `internal/scanner/provider/scanner.go`
- Goal: Remove the hardcoded `routing-queue` sample once both scanners return real manifests.
- Acceptance: `make scan` reports actual local repo data.

### LAB-2: Improve Matrix Scoring

- Owner: `Shared`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/matrix/matrix.go`
- Goal: Convert provider and MRMO signals into `ready`, `warning`, `blocked`, or `unknown`.
- Acceptance: Blockers fail `--strict`; warnings remain report-only.

### LAB-3: JSON Report Contract

- Owner: `Shared`
- Priority: `P0`
- Status: `Todo`
- Files: `internal/model/model.go`, `internal/matrix/matrix.go`
- Goal: Finalize the compatibility report schema for CI and PR comments.
- Acceptance: JSON output is stable enough for golden tests.

### LAB-4: Markdown Report Output

- Owner: `Shared`
- Priority: `P1`
- Status: `Todo`
- Files: `internal/report/report.go`
- Goal: Add markdown output grouped by blockers, warnings, and ready resources.
- Acceptance: `scan --format markdown` produces PR-friendly output.

### LAB-5: Golden Fixture Tests

- Owner: `Shared`
- Priority: `P1`
- Status: `Todo`
- Files: `testdata/fixtures/`, scanner tests, matrix tests
- Goal: Add small fixture snapshots for MRMO and provider metadata.
- Acceptance: `go test ./...` catches scanner and report regressions.

### LAB-6: Demo Scenarios

- Owner: `Shared`
- Priority: `P0`
- Status: `Todo`
- Files: `README.md`, `testdata/fixtures/`
- Goal: Prepare three demo resources: ready, warning, and blocked.
- Acceptance: Demo can be run offline from a clean checkout.

### LAB-7: Provider PR Diff

- Owner: `Shared`
- Priority: `P2`
- Status: `Todo`
- Files: `cmd/compatibility-lab/main.go`, `internal/matrix/`
- Goal: Compare provider metadata snapshots across git refs.
- Acceptance: Removed exporter or changed `RefAttrs` on an MRMO-supported resource is flagged as high risk.

### LAB-8: Roundtrip Prototype

- Owner: `Shared`
- Priority: `P2`
- Status: `Todo`
- Files: new `internal/roundtrip/`
- Goal: Prototype mocked export/source-target drift comparison.
- Acceptance: Roundtrip can compare two exported JSON fixtures and show normalized drift.

## Suggested First Pulls

- MRMO dev starts with `MRMO-1`, `MRMO-2`, and `MRMO-4`.
- CX dev starts with `CX-1`, `CX-2`, and `CX-3`.
- Shared pairing starts after both scanners can emit at least one real resource.

## Done For Scaffold

- `cmd/compatibility-lab/main.go` exists with planned commands.
- `internal/model` contains initial report structs.
- `internal/scanner/mrmo` and `internal/scanner/provider` exist as implementation slots.
- `internal/matrix` joins sample scanner output.
- `internal/report` prints table, explain, and dependency output.
