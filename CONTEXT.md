# Mahresources

Mahresources organizes personal information and provides MRQL for querying resources, notes, groups, and their relationships.

## MRQL Language

**MRQL Query**:
A request that selects, orders, groups, or summarizes Mahresources entities.
_Avoid_: SQL query

**Effective MRQL Query**:
An MRQL Query after parameter binding, defaults, safety bounds, and the requesting principal's authorization scope have been applied.
_Avoid_: Parsed query, raw query

**MRQL Explanation**:
A non-executing diagnostic description of an Effective MRQL Query.
_Avoid_: Query plan
