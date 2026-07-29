# Compatibility Report

**Schema:** `compatibility-lab/v1` · **Provider:** `PROVIDER_FIXTURE` · **MRMO:** `MRMO_FIXTURE`

| Ready | Warning | Unknown | Blocked | Provider Resources | MRMO Resources |
|:-----:|:-------:|:-------:|:-------:|:------------------:|:--------------:|
| 0 | 1 | 2 | 2 | 5 | 3 |

## Blocked (2)

### `genesyscloud_blocked_only`

- **Score:** 0
- **blocker · `MRMO_REGISTRY_MISSING`** — resource is not registered as MRMO-supported
- **unknown · `PROVIDER_BLOCK_HASH_UNKNOWN`** — no static QuickHashFields call or ResourceMeta.BlockHash assignment was found; hash stability cannot be confirmed

### `genesyscloud_fake_dep`

- **Score:** 0
- **blocker · `PROVIDER_EXPORTER_MISSING`** — resource does not have a provider exporter
- **blocker · `MRMO_REGISTRY_MISSING`** — resource is not registered as MRMO-supported

## Warning (1)

### `genesyscloud_flow`

- **MRMO ref:** `architect-flow`
- **MRMO tier:** 4
- **Score:** 70
- **warning · `MRMO_INTEGRATION_TEST_MISSING`** — resource has no handler integration test coverage
- **unknown · `PROVIDER_BLOCK_HASH_UNKNOWN`** — no static QuickHashFields call or ResourceMeta.BlockHash assignment was found; hash stability cannot be confirmed

## Unknown (2)

### `genesyscloud_auth_division`

- **MRMO ref:** `auth-division`
- **MRMO tier:** 0
- **Score:** 60
- **unknown · `PROVIDER_BLOCK_HASH_UNKNOWN`** — no static QuickHashFields call or ResourceMeta.BlockHash assignment was found; hash stability cannot be confirmed

### `genesyscloud_routing_queue`

- **MRMO ref:** `routing-queue`
- **MRMO tier:** 4
- **Score:** 60
- **unknown · `PROVIDER_BLOCK_HASH_UNKNOWN`** — no static QuickHashFields call or ResourceMeta.BlockHash assignment was found; hash stability cannot be confirmed

