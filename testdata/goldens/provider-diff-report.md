# Provider PR Diff

**Schema:** `compatibility-lab-diff/v1` · **Base:** `main` · **Head:** `pr/example`

| High | Medium | Low | Total |
|:----:|:------:|:---:|:-----:|
| 3 | 1 | 2 | 6 |

## High Risk (3)

| Terraform Type | Kind | Attribute | Before → After | MRMO |
|---|---|---|---|:---:|
| `genesyscloud_auth_division` | `RESOURCE_REMOVED` | - | - | **yes** |
| `genesyscloud_routing_queue` | `EXPORTER_REMOVED` | - | - | **yes** |
| `genesyscloud_routing_queue` | `REFATTR_CHANGED` | `division_id` | `genesyscloud_auth_division` → `genesyscloud_renamed_target` | **yes** |

## Medium Risk (1)

| Terraform Type | Kind | Attribute | Before → After | MRMO |
|---|---|---|---|:---:|
| `genesyscloud_flow` | `EXPORT_ID_CHANGED` | - | _added_ → `renamed_export_id` | **yes** |

## Low Risk (2)

| Terraform Type | Kind | Attribute | Before → After | MRMO |
|---|---|---|---|:---:|
| `genesyscloud_new_thing` | `RESOURCE_ADDED` | - | - | no |
| `genesyscloud_routing_queue` | `REFATTR_ADDED` | `new_field_id` | _added_ → `genesyscloud_new_ref` | **yes** |

