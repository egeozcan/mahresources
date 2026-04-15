---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource, resources list, groups list
---

# Long

A Series is an ordered collection of Resources, typically used for content
that has an intrinsic sequence: a volume of a manga, a photo shoot, the
chapters of a scanned document. A Resource may belong to at most one
Series via its `SeriesId` reference, and removing that reference detaches
the Resource from the Series without deleting either.

Use the `series` subcommands to manage a series by ID: fetch it, create
a new one, rename or fully edit it, delete it, remove a resource from
its series, or list series matching filters. Series membership is
assigned on the Resource side (see `resource edit --series-id`), so to
attach a resource to a series edit the resource.
