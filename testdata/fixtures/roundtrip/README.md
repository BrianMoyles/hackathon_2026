# Roundtrip Fixtures

Mock CX as Code JSON exports for LAB-8:

- `source.json` — source-org export
- `target.json` — target-org export after a partial / drifted replicate

Expected normalized drift for queues:

- `support`: attribute change (`acw_wrapup_prompt`, `division_id`)
- `sales`: missing in target
- `billing`: extra in target

Volatile `id` / `self_uri` differences are ignored by normalization.
