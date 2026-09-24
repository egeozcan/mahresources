# Mahresources

Mahresources organizes personal information and provides MRQL for querying resources, notes, groups, and their relationships.

## MRQL Language

**MRQL Query**:
A request that selects, orders, groups, or summarizes Mahresources entities.
_Avoid_: SQL query

**MRQL Entity Result**:
A resource, note, or group returned by an MRQL Query, either directly or within a result bucket. Aggregate rows summarize entities and are not themselves MRQL Entity Results.

**Effective MRQL Query**:
An MRQL Query after parameter binding, defaults, safety bounds, and the requesting principal's authorization scope have been applied.
_Avoid_: Parsed query, raw query

**MRQL Explanation**:
A non-executing diagnostic description of an Effective MRQL Query.
_Avoid_: Query plan

## Entity Interaction

**Entity Selection**:
A set of Mahresources entities of one entity type chosen for an action. Resource, note, and group selections remain independent even when presented in the same query results.

**Bulk Action**:
An operation offered for an Entity Selection according to its entity type and the action's requirements. The same action is available in ordinary entity lists and MRQL entity results.

**Mass Edit**:
An operation that applies several edits together to selected entities or a query-defined set of entities of one type. Its available edits depend on that entity type.

## Background Work

**Job**:
A durable execution of accepted background work. A Job exists while delayed or queued, remains inspectable after it finishes, and exposes only the controls its kind supports.
_Avoid_: Download, action, task

**Job Center**:
The unified place where a person inspects and controls the Jobs visible to them. It includes all user- or operator-facing Job kinds, while internal housekeeping remains outside it.
_Avoid_: Downloads, Action Center

**Job Kind**:
A family of Jobs that shares input semantics, phases, outputs, and supported commands while using its own specialized executor. Kind is not lifecycle state or origin.
_Avoid_: Source, job type

**Job State**:
A Job's normalized lifecycle classification: scheduled, queued, running, paused, blocked, succeeded, failed, cancelled, or interrupted. A kind-specific Phase may describe finer progress without redefining the Job State.
_Avoid_: Phase, status detail

**Active Job**:
A Job that is scheduled, queued, running, or paused: work that is still expected to proceed without anyone intervening. A blocked Job is not active; it Needs Attention.
_Avoid_: Pending job, in-flight job

**Finished Job**:
A Job in a terminal Job State: succeeded, failed, cancelled, or interrupted. Finishing says nothing about success, so a failed or interrupted Finished Job can also Need Attention.
_Avoid_: Completed job, done job

**Job Retry**:
A request to rerun an unsuccessful finished Job's unchanged input under current authorization and policy. It creates a new Job owned by the requester and linked to the finished Job; it never changes or replaces the original Job's outcome.
_Avoid_: Attempt, restart

**Job Repeat**:
A safe rerun of a successful Job's unchanged input. It creates an independent linked Job and may branch from earlier repeats, subject to the Job Kind's duplicate and concurrency policy.
_Avoid_: Retry, restart

**Job Lineage**:
The durable links between related Jobs, including retries, repeats, and parent-child work. Every Job keeps its own identity and outcome within the lineage.
_Avoid_: Attempt history, merged job

**Job Command**:
A control a Job currently offers, such as cancel, pause, resume, retry, inspect output, download an artifact, or dismiss its record. Availability comes from the Job rather than being inferred by the interface from its kind or state.
_Avoid_: Action

**Job Event**:
A durable, significant fact in a Job's timeline, such as acceptance, a phase change, a control decision, a warning, an outcome, or an artifact. Routine progress ticks are snapshots rather than Job Events.
_Avoid_: Log line, progress update

**Job Output**:
A typed result published by a Job, such as an entity, expiring artifact, report, structured summary, safe external link, or log reference. Its availability and retention are independent of the Job's outcome and retention.
_Avoid_: Result path, return value

**Needs Attention**:
The derived view of visible Jobs that are blocked, failed, or interrupted and have not been dismissed. It is operational state, not an unread-notification state.
_Avoid_: Inbox, unread jobs

**Schedule**:
A durable definition of recurring work, not a Job. Each occurrence becomes its own Job only when materialized for dispatch; one-off deferred work is already a scheduled Job.
_Avoid_: Scheduled job, recurring job

## Metadata Indexing

**Indexed Metadata Key**:
A category/type-selected metadata path and comparison kind whose MRQL expression
is maintained as a database index. Declarations live on resource categories, group
categories, and note types. A background reconciler shares physical indexes across
carriers of the same entity type and removes one only after its last declaration
is removed. This is a performance choice, not a metadata validation requirement.

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

## Resource Versions

**Resource Version**:
One stored content state of a Resource: its bytes, content type and pixel dimensions as they stood at one point in that Resource's history. Restoring an earlier state records a new Resource Version rather than reinstating the old one, so two Resource Versions may hold the same bytes.
_Avoid_: revision, snapshot, copy

**Current Version**:
The Resource Version whose content the Resource serves now. Every other attribute of the Resource — its name, tags, owner and related entities — belongs to the Resource, never to a Resource Version.
_Avoid_: head, tip, live version

**Historical Version**:
A Resource Version that is not the Current Version.
_Avoid_: previous version, old version, earlier version. Version numbers are not upload order once a merge has transferred versions between Resources, so "previous" names no particular Resource Version.

**Displayed Version**:
The Resource Version a viewer is showing for a Resource. It is the Current Version unless a reader has chosen otherwise, and choosing otherwise changes what is on screen and nothing about the Resource.
_Avoid_: active version, open version

## Plugin Commands

**Command Declaration**:
A plugin's fixed argv template that the operator consents to run as the server service account, together with its timeout, sensitive parameter names and declared input file names.
_Avoid_: command, server command, shell command

**Declared Input File**:
A file name listed by a Command Declaration whose contents the plugin supplies for one run; the host writes it into that run's Exchange Folder before the program starts, and the contents are never stored in any record.
_Avoid_: input, input parameter, argument, config file

**Run Parameter**:
A value the plugin supplies for one run to fill a single whole-element `{{placeholder}}` in a Command Declaration's argv; a sensitive parameter's value is redacted from every record.
_Avoid_: input, argument, variable

**Command Import**:
A file a command produced that the plugin has asked to enter the library.
_Avoid_: input file, upload

**Exchange Folder**:
The private per-run working directory a command runs in: the host creates it, the process reads and writes files there, and the plugin reaches them afterwards through `mah.fs`.
_Avoid_: exchange dir, staging folder, working directory
