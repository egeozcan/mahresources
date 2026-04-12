---
name: mahresources-category-designer
description: Design Mahresources Categories, Resource Categories, and Note Types from a data model. Use when asked to create or update MetaSchema JSON Schema, SectionConfig, or CustomHeader/CustomSidebar/CustomSummary/CustomAvatar/CustomMRQLResult layouts, or when you need to understand built-in shortcodes, built-in plugin shortcodes, category template slots, or x-display behavior.
---

# Mahresources Category Designer

Use this skill when the task is to turn a domain model into a Mahresources content definition and UI package:

- `Category` for Groups
- `ResourceCategory` for Resources
- `NoteType` for Notes

That package usually includes some or all of:

- `MetaSchema`
- `SectionConfig`
- `CustomHeader`
- `CustomSidebar`
- `CustomSummary`
- `CustomAvatar`
- `CustomMRQLResult`

## Canonical Behavior

Prefer the current code paths and the reference files in this skill over older prose docs when they disagree.

Especially important:

- Category templates are stored as raw HTML strings.
- The current implementation expands shortcodes inside those strings, then writes the resulting HTML.
- Do **not** assume nested Pongo2 expressions like `{{ group.Name }}` inside a stored `CustomHeader` are re-evaluated as a second template pass.
- For `CustomHeader`, `CustomSidebar`, `CustomSummary`, and the rendered avatar slots, the surrounding page template already provides Alpine `x-data` with `entity` JSON, so stored markup can use Alpine directives such as `x-text="entity.Name"`.
- `CustomMRQLResult` is different: it is processed by the shortcode engine, not by Pongo2, and it is not automatically wrapped in an Alpine `entity` context. Build it with static HTML plus shortcodes.
- Detail-page descriptions also process shortcodes. Truncated list previews do not.
- Group `CustomAvatar` exists in the model and forms, but the default group list card template does not currently render it. Do not rely on it unless you also change templates.
- Plugin injection slots such as `page_bottom` or `group_detail_before` are a separate mechanism from category custom fields. Category templates do not register into those slots; plugins do.

If you need the exact current slot wiring, read:

- `references/schema-layouts.md`

If you need the shortcode catalog, read:

- `references/shortcodes.md`

## Working Style

Default to a full package unless the user asked for only one piece.

When a user gives you a data model:

1. Identify the target entity kind.
2. Normalize the model into stable fields, optional fields, repeated collections, derived display fields, and edit-heavy fields.
3. Produce a `MetaSchema` that fits the app's supported behavior rather than generic JSON Schema maximalism.
4. Use `SectionConfig` to remove redundant native sections and let the custom layout breathe.
5. Use category templates for identity, hierarchy, summaries, and bespoke presentation.
6. Use schema-driven display for the long tail of metadata instead of hand-rendering every field.
7. Add plugin shortcodes only when they materially improve the result and call out the plugin dependency.

## Schema Rules

Prefer a conservative, app-friendly schema subset.

Good defaults:

- Root schema should usually be `type: "object"`.
- Give user-facing fields `title` and usually `description`.
- Use `required` only for truly mandatory data.
- Prefer `additionalProperties: false` for well-defined models.
- Allow `additionalProperties` only when the data model is intentionally open-ended.
- Use labeled enums with `oneOf` + `const` + `title` for statuses, priorities, phases, and other user-facing choices.
- Use `format` for dates, datetimes, emails, URIs, and similar typed strings.
- Use `x-display` when an object should render as one widget instead of flattening into multiple fields.
- For conditional branches that swap nested object shapes, set `additionalProperties: false` on branch-specific nested objects so stale keys get cleaned up.

Do not overfit the schema:

- Avoid deeply clever `oneOf` trees when a labeled enum or simple object shape will do.
- Avoid schema features the UI cannot present clearly unless the user explicitly needs them.
- Do not store presentational-only duplication in `MetaSchema` if it can be derived by templates.

## Layout Rules

Use the category facilities intentionally:

- Let `MetaSchema` drive validation, forms, search filters, and the structured metadata display.
- Use `CustomHeader` for identity, status, hero metrics, and "above the fold" context.
- Use `CustomSidebar` for supporting stats, editors, mini dashboards, recent activity, and secondary context.
- Use `CustomSummary` for compact list-card signals only.
- Use `CustomAvatar` only for small visual markers or badges.
- Use `CustomMRQLResult` when this entity should render in a distinctive way inside `[mrql]` results.

Prefer shortcodes first, Alpine second:

- Use `[meta]` when the data already lives in `Meta` and you want schema-aware rendering.
- Use `[property]` for built-in entity fields like `Name`, `Description`, `CreatedAt`, `UpdatedAt`, `FileSize`, and similar case-sensitive struct fields.
- Use `[mrql]` for related collections, rollups, and embedded result sets.
- Use plugin shortcodes for charts, badges, inline editors, and dashboards when needed.
- Use Alpine markup only when shortcodes are not expressive enough.

Good visual strategy:

- Put 2-5 important fields in `CustomHeader`.
- Leave broad metadata coverage to the schema display panel.
- Keep `CustomSummary` dense and fast to scan.
- Hide native sections that your custom layout has already made redundant.

## Output Contract

When the user asks for a full design package, return:

- The target definition type: `Category`, `ResourceCategory`, or `NoteType`
- A ready-to-paste `MetaSchema` JSON object
- A ready-to-paste `SectionConfig` JSON object when useful
- The relevant custom template fields as HTML/shortcode strings
- Required plugin enablement, if any
- A short mapping note from source fields to schema paths

Only include fields that add value. It is fine to omit blank custom fields.

## Final Checks

Before you finish:

- Make sure every `[meta path="..."]` path exists in the schema you wrote.
- Make sure every plugin shortcode you used comes from an enabled plugin and fits the current slot.
- Do not rely on nested Pongo2 inside stored category templates.
- Do not rely on Alpine `entity` inside `CustomMRQLResult`.
- For group categories, do not promise `CustomAvatar` output unless templates are also changed.
- Use `SectionConfig` to reduce duplication with the native detail page.

## References

- `references/schema-layouts.md` -- current slot wiring, template behavior, section config shapes, schema rules, and generation patterns
- `references/shortcodes.md` -- built-in shortcodes and built-in plugin shortcode catalog
