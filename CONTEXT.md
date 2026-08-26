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

## Resource Reduction

**Resource Reduction**:
A named, durable proposal to merge Clusters, accumulated and reviewed over time and applied in whole or in part.
_Avoid_: sweep, batch, cleanup, job

**Cluster**:
A set of Resources proposed as holding the same content, exactly one of which is the Winner.
_Avoid_: group, duplicate set, batch

**Winner**:
The Resource in a Cluster that survives the merge, absorbing every Loser's tags, notes and related groups.
_Avoid_: keeper, primary, original

**Loser**:
A Resource in a Cluster that is merged into the Winner and then deleted.
_Avoid_: duplicate, discard

**Winner Rule**:
The ordered list of criteria that selects a Cluster's Winner, each criterion breaking the ties left by the one before it.
_Avoid_: sort order, ranking, scoring

**Identical Resources**:
Resources sharing a content hash, and therefore the same bytes.
_Avoid_: duplicate, exact duplicate

**Near-Identical Resources**:
Resources within the perceptual distance threshold of one another. Only ever images, since perceptual hashes exist for no other content type.
_Avoid_: duplicate, similar

**Extent**:
The set of Resources a Resource Reduction considers: an explicit set of Resources, or a set of Groups expanded through their descendants.
_Avoid_: scope, selection, range

**Reviewed Cluster**:
A Cluster that has been explicitly acted on, and is therefore frozen against re-clustering when the Extent grows.
_Avoid_: seen, confirmed, done

**Stale Cluster**:
A Cluster whose Winner or a Loser no longer exists, or whose Winner-to-Loser pair no longer holds, as discovered when a Resource Reduction is applied.
_Avoid_: invalid, broken, expired
