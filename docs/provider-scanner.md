# Provider Scanner Work Plan

Read these inputs from `/Users/BMOYLES/genesys_src/repos/terraform-provider-genesyscloud`:

- `genesyscloud/provider_registrar/provider_registrar.go`
- `genesyscloud/resource_exporter/resource_exporter.go`
- per-resource `SetRegistrar` and exporter functions
- `public/data/resource_permissions-latest.json`
- dependency tree artifacts when present

Emit one `model.ProviderResource` per provider resource.

Required checks:

- Provider resource exists.
- Exporter exists.
- RefAttrs and dependency targets are captured.
- Singleton `ExportID` is captured and validated.
- Excluded attributes are captured.
- Third-party file refs and custom file writers are captured.
- Custom resolver presence is captured.
- BlockHash hints are captured where static scanning can find them.
