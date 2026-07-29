# Provider Fixtures

Trimmed provider tree used by golden tests (`go test ./...`):

- `genesyscloud/routing_queue` — resource + data source + exporter with RefAttrs
- `genesyscloud/auth_division` — resource + exporter
- `genesyscloud/architect_flow` — file-output exporter (`genesyscloud_flow`)
- `genesyscloud/blocked_only` — provider-only resource (no MRMO registry entry)
- `genesyscloud/fake_dep` — RefAttr dependency target

Import paths are synthetic (`example.com/...`); the scanner only needs AST shape.
