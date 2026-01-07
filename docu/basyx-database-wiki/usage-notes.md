# Usage Notes

- All submodel elements are stored in a tree structure using `parent_sme_id` and `path_ltree`.
- Specialized element tables (e.g., `property_element`, `range_element`) extend `submodel_element` by 1:1 relationship.
- Use the provided indexes for efficient queries, especially for text and hierarchical data.
- References and semantic keys are normalized for reuse and fast lookup.
- Qualifiers can be attached to any submodel element.
- See [Entity Reference](./entities.md) and [Relationships & Diagrams](./relationships.md) for more details.
