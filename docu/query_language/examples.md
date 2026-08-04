# Query Language Examples

This guide shows concrete query and ABAC filter inputs together with the
expected result. For the expression grammar and SQL translation details, see
the [query language architecture guide](README.md).

## Supported query endpoints

| Endpoint | Main field roots | Typical content |
| --- | --- | --- |
| `POST /query/shells` | `$aas` | Asset Administration Shells |
| `POST /query/submodels` | `$sm`, `$sme` | Submodels and their elements |
| `POST /query/shell-descriptors` | `$aasdesc`, nested `$smdesc` | AAS Descriptors and nested Submodel Descriptors |
| `POST /query/submodel-descriptors` | `$smdesc` | Submodel Descriptors |

Every request requires a top-level `$condition`. It selects the parent objects
in `result`. Optional `$filters` shape fragments inside those objects. Available
field paths depend on the endpoint's model and reader.

`$match` in a request and `MATCH` in an ABAC rule are not separate predicates.
They only change whether their enclosing fragment condition is evaluated at
parent scope or against the current fragment row.

Each example shows the complete expected response for the displayed starting
object, including `paging_metadata`, the parent object, unchanged displayed
fields, and the filtered fragments. The starting objects intentionally contain
only the fields needed to demonstrate the behavior.

## 1. Select parent resources

Starting data:

```json
[
  { "id": "urn:sm:motor", "idShort": "Motor" },
  { "id": "urn:sm:pump", "idShort": "Pump" }
]
```

Request:

```http
POST /query/submodels
Content-Type: application/json
```

```json
{
  "$condition": {
    "$eq": [
      { "$field": "$sm#idShort" },
      { "$strVal": "Motor" }
    ]
  }
}
```

Expected result:

```json
{
  "paging_metadata": {},
  "result": [
    { "id": "urn:sm:motor", "idShort": "Motor" }
  ]
}
```

The top-level condition removes the `Pump` parent from `result`. It does not
mask individual fragments inside `Motor`.

## 2. Keep a complete fragment with existential matching

Starting Submodel:

```json
{
  "id": "urn:sm:motor",
  "idShort": "Motor",
  "supplementalSemanticIds": [
    {
      "type": "ExternalReference",
      "keys": [{ "type": "GlobalReference", "value": "VISIBLE" }]
    },
    {
      "type": "ExternalReference",
      "keys": [{ "type": "GlobalReference", "value": "INTERNAL" }]
    }
  ]
}
```

Request with `$match` omitted:

```json
{
  "$condition": {
    "$eq": [
      { "$field": "$sm#id" },
      { "$strVal": "urn:sm:motor" }
    ]
  },
  "$filters": [
    {
      "$fragment": "$sm#supplementalSemanticIds[]",
      "$condition": {
        "$eq": [
          { "$field": "$sm#supplementalSemanticIds[].keys[].value" },
          { "$strVal": "VISIBLE" }
        ]
      }
    }
  ]
}
```

Expected result:

```json
{
  "paging_metadata": {},
  "result": [
    {
      "id": "urn:sm:motor",
      "idShort": "Motor",
      "supplementalSemanticIds": [
        {
          "type": "ExternalReference",
          "keys": [{ "type": "GlobalReference", "value": "VISIBLE" }]
        },
        {
          "type": "ExternalReference",
          "keys": [{ "type": "GlobalReference", "value": "INTERNAL" }]
        }
      ]
    }
  ]
}
```

The condition is evaluated at parent scope. Because one reference contains
`VISIBLE`, the complete fragment is preserved. Setting `$match` to `false` has
the same effect.

## 3. Return only matching fragment rows

Use the same starting data as example 2, but explicitly set `$match`:

```json
{
  "$condition": {
    "$eq": [
      { "$field": "$sm#id" },
      { "$strVal": "urn:sm:motor" }
    ]
  },
  "$filters": [
    {
      "$fragment": "$sm#supplementalSemanticIds[]",
      "$match": true,
      "$condition": {
        "$eq": [
          { "$field": "$sm#supplementalSemanticIds[].keys[].value" },
          { "$strVal": "VISIBLE" }
        ]
      }
    }
  ]
}
```

Expected result:

```json
{
  "paging_metadata": {},
  "result": [
    {
      "id": "urn:sm:motor",
      "idShort": "Motor",
      "supplementalSemanticIds": [
        {
          "type": "ExternalReference",
          "keys": [{ "type": "GlobalReference", "value": "VISIBLE" }]
        }
      ]
    }
  ]
}
```

`$match: true` binds the condition to the reference currently being
reconstructed, so the `INTERNAL` sibling is removed.

## 4. Match nested Submodel Element Lists

Starting structure, shortened to the relevant fields:

```json
{
  "id": "urn:sm:nested",
  "submodelElements": [
    {
      "idShort": "a",
      "modelType": "SubmodelElementList",
      "value": [
        {
          "modelType": "SubmodelElementCollection",
          "value": [
            {
              "idShort": "b",
              "modelType": "SubmodelElementList",
              "value": [
                { "modelType": "Property", "value": "a0-b0" },
                { "modelType": "Property", "value": "a0-b1" }
              ]
            }
          ]
        },
        {
          "modelType": "SubmodelElementCollection",
          "value": [
            {
              "idShort": "b",
              "modelType": "SubmodelElementList",
              "value": [
                { "modelType": "Property", "value": "a1-b0" },
                { "modelType": "Property", "value": "a1-b1" }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

Request:

```json
{
  "$condition": {
    "$eq": [
      { "$field": "$sm#id" },
      { "$strVal": "urn:sm:nested" }
    ]
  },
  "$filters": [
    {
      "$fragment": "$sme.a[].b[]",
      "$match": true,
      "$condition": {
        "$ends-with": [
          { "$field": "$sme.a[].b[]#value" },
          { "$strVal": "-b0" }
        ]
      }
    }
  ]
}
```

Expected result:

```json
{
  "paging_metadata": {},
  "result": [
    {
      "id": "urn:sm:nested",
      "submodelElements": [
        {
          "idShort": "a",
          "modelType": "SubmodelElementList",
          "value": [
            {
              "modelType": "SubmodelElementCollection",
              "value": [
                {
                  "idShort": "b",
                  "modelType": "SubmodelElementList",
                  "value": [
                    { "modelType": "Property", "value": "a0-b0" }
                  ]
                }
              ]
            },
            {
              "modelType": "SubmodelElementCollection",
              "value": [
                {
                  "idShort": "b",
                  "modelType": "SubmodelElementList",
                  "value": [
                    { "modelType": "Property", "value": "a1-b0" }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

The outer `a[]` structure is unchanged. Inside `a[0]`, `a0-b0` remains and
`a0-b1` is removed. Inside `a[1]`, `a1-b0` remains and `a1-b1` is removed.
Each `[]` is bound to the current numeric list index, so values from `a[0]`
cannot satisfy the condition for `a[1]`.

### Indexed variant

An indexed fragment can target only one branch. Full request:

```json
{
  "$condition": {
    "$eq": [
      { "$field": "$sm#id" },
      { "$strVal": "urn:sm:nested" }
    ]
  },
  "$filters": [
    {
      "$fragment": "$sme.a[1].b[]",
      "$match": true,
      "$condition": {
        "$ends-with": [
          { "$field": "$sme.a[].b[]#value" },
          { "$strVal": "-b0" }
        ]
      }
    }
  ]
}
```

Expected result:

```json
{
  "paging_metadata": {},
  "result": [
    {
      "id": "urn:sm:nested",
      "submodelElements": [
        {
          "idShort": "a",
          "modelType": "SubmodelElementList",
          "value": [
            {
              "modelType": "SubmodelElementCollection",
              "value": [
                {
                  "idShort": "b",
                  "modelType": "SubmodelElementList",
                  "value": [
                    { "modelType": "Property", "value": "a0-b0" },
                    { "modelType": "Property", "value": "a0-b1" }
                  ]
                }
              ]
            },
            {
              "modelType": "SubmodelElementCollection",
              "value": [
                {
                  "idShort": "b",
                  "modelType": "SubmodelElementList",
                  "value": [
                    { "modelType": "Property", "value": "a1-b0" }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

`a[0].b` remains complete, while only `a[1].b` is filtered.

## 5. Filter multiple fragments independently

A Submodel Descriptor can filter endpoints and supplemental semantic IDs in the
same request. Starting descriptor:

```json
{
  "id": "urn:smdesc:motor",
  "endpoints": [
    {
      "interface": "SUBMODEL-3.0",
      "protocolInformation": { "href": "https://public.example/motor" }
    },
    {
      "interface": "SUBMODEL-2.0",
      "protocolInformation": { "href": "https://legacy.example/motor" }
    }
  ],
  "supplementalSemanticIds": [
    {
      "type": "ExternalReference",
      "keys": [{ "type": "GlobalReference", "value": "PUBLIC" }]
    },
    {
      "type": "ExternalReference",
      "keys": [{ "type": "GlobalReference", "value": "INTERNAL" }]
    }
  ]
}
```

Request:

```json
{
  "$condition": {
    "$eq": [
      { "$field": "$smdesc#id" },
      { "$strVal": "urn:smdesc:motor" }
    ]
  },
  "$filters": [
    {
      "$fragment": "$smdesc#endpoints[]",
      "$match": true,
      "$condition": {
        "$eq": [
          { "$field": "$smdesc#endpoints[].interface" },
          { "$strVal": "SUBMODEL-3.0" }
        ]
      }
    },
    {
      "$fragment": "$smdesc#supplementalSemanticIds[]",
      "$match": true,
      "$condition": {
        "$eq": [
          { "$field": "$smdesc#supplementalSemanticIds[].keys[].value" },
          { "$strVal": "PUBLIC" }
        ]
      }
    }
  ]
}
```

Expected result:

```json
{
  "paging_metadata": {},
  "result": [
    {
      "id": "urn:smdesc:motor",
      "endpoints": [
        {
          "interface": "SUBMODEL-3.0",
          "protocolInformation": { "href": "https://public.example/motor" }
        }
      ],
      "supplementalSemanticIds": [
        {
          "type": "ExternalReference",
          "keys": [{ "type": "GlobalReference", "value": "PUBLIC" }]
        }
      ]
    }
  ]
}
```

Result explanation:

- Only endpoints with interface `SUBMODEL-3.0` remain.
- Only supplemental semantic references containing `PUBLIC` remain.
- Other descriptor fields are unchanged.
- The two fragment conditions do not have to be satisfied by the same database
  row because they target different fragments.

## 6. Combine an ABAC filter with a request filter

Assume a descriptor contains:

```json
{
  "id": "urn:aasdesc:motor",
  "specificAssetIds": [
    { "name": "customerPartId", "value": "P-100" },
    { "name": "customerPartId", "value": "P-200" },
    { "name": "manufacturerAssetId", "value": "M-9" }
  ]
}
```

The relevant part of an allowing ABAC rule requires customer part IDs:

```json
{
  "USEACL": "reader",
  "USEOBJECTS": ["query_routes"],
  "USEFORMULA": "always_true",
  "FILTER": {
    "FRAGMENT": "$aasdesc#specificAssetIds[]",
    "MATCH": true,
    "CONDITION": {
      "$eq": [
        { "$field": "$aasdesc#specificAssetIds[].name" },
        { "$strVal": "customerPartId" }
      ]
    }
  }
}
```

The caller further narrows the same fragment:

```json
{
  "$condition": { "$boolean": true },
  "$filters": [
    {
      "$fragment": "$aasdesc#specificAssetIds[]",
      "$match": true,
      "$condition": {
        "$eq": [
          { "$field": "$aasdesc#specificAssetIds[].value" },
          { "$strVal": "P-100" }
        ]
      }
    }
  ]
}
```

Effective fragment condition:

```text
name = "customerPartId" AND value = "P-100"
```

Expected result:

```json
{
  "paging_metadata": {},
  "result": [
    {
      "id": "urn:aasdesc:motor",
      "specificAssetIds": [
        { "name": "customerPartId", "value": "P-100" }
      ]
    }
  ]
}
```

The request cannot expose `manufacturerAssetId`, because the ABAC filter is
mandatory. It also cannot recover `P-200`, because the request itself narrowed
the policy-visible rows to `P-100`.

## What can be combined

- `$and`, `$or`, and `$not` can combine logical expressions.
- Comparison operators such as `$eq`, `$ne`, `$gt`, `$ge`, `$lt`, and `$le`
  compare fields and values.
- String operators such as `$contains`, `$starts-with`, `$ends-with`, and
  `$regex` can be used in conditions.
- Multiple request filters can target the same fragment; they are combined with
  `AND`.
- Filters for different fragments are evaluated independently.
- Multiple permitting ABAC rules form `OR` alternatives. A request query is
  applied after those alternatives and cannot widen them.
- Each individual predicate retains its own `$match` or `MATCH` mode when these
  expressions are combined.

For exact value types, casts, field identifier syntax, and fragment guards, see
the [main query language guide](README.md).
