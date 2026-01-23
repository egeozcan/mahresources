---
name: seed-data
description: Seeds a mahresources instance with semantic content across all entity types, creating enough items to trigger pagination (>50 per type)
disable-model-invocation: true
argument-hint: [base-url]
allowed-tools: Bash, Read
---

# Seed Data Skill

Seeds a mahresources instance with rich semantic content covering all entity types. Creates 60+ items of each type to ensure pagination is triggered.

## Usage

```
/seed-data http://localhost:8181
```

If no URL is provided, defaults to `http://localhost:8181`.

## Instructions

When this skill is invoked, run the seed script to populate the mahresources instance:

```bash
.claude/skills/seed-data/seed.sh $ARGUMENTS
```

If `$ARGUMENTS` is empty, the script defaults to `http://localhost:8181`.

The script will:
1. Check server connectivity
2. Create 60+ entities of each type in the correct dependency order
3. Verify pagination is working
4. Print a summary of created entities

## Known Limitations

- **Relation Types**: May fail to persist due to an FTS trigger bug in the application. The script reports success but entities may not be saved.
- **Relations**: Require groups with matching categories to the relation type's category constraints. Most will fail due to category mismatches with the default relation types.

Despite these limitations, the script successfully creates pagination-triggering content for:
- Tags (78+ items)
- Categories (68+ items)
- Note Types (63+ items)
- Groups (64+ items)
- Notes (60+ items)
- Resources (60+ items)
- Queries (60+ items)

### Execution Order (Dependencies)

Create entities in this order to satisfy foreign key relationships:

1. **Tags** (no dependencies)
2. **Categories** (no dependencies)
3. **NoteTypes** (no dependencies)
4. **RelationTypes** (depends on Categories)
5. **Groups** (depends on Categories, Tags)
6. **Notes** (depends on Groups, Tags, NoteTypes)
7. **Resources** (depends on Groups, Tags, Notes)
8. **Relations** (depends on Groups, RelationTypes)
9. **Queries** (no dependencies, but useful after data exists)

### Content Themes

Use these semantic themes to create meaningful, interconnected data:

#### Theme 1: Software Development Company
- Categories: Departments, Projects, Employees, Meetings, Documents
- Tags: urgent, in-progress, completed, review-needed, approved, archived
- Groups: Engineering Team, Design Team, Marketing Team, HR Department, Finance
- Notes: Meeting minutes, project specs, code reviews, performance reviews
- Resources: Design mockups, architecture diagrams, reports, presentations

#### Theme 2: Research Library
- Categories: Books, Journals, Authors, Publishers, Research Topics
- Tags: peer-reviewed, open-access, citation-needed, primary-source, archived
- Groups: Computer Science, Biology, Physics, Mathematics, Philosophy
- Notes: Research summaries, literature reviews, annotations, bibliographies
- Resources: PDFs, images, datasets, supplementary materials

#### Theme 3: Media Production
- Categories: Films, TV Shows, Actors, Directors, Studios
- Tags: released, in-production, pre-production, cancelled, award-winning
- Groups: Action Movies, Documentaries, Animated Films, TV Dramas, Shorts
- Notes: Script notes, production logs, reviews, cast lists
- Resources: Posters, trailers, stills, behind-the-scenes

#### Theme 4: E-commerce Inventory
- Categories: Electronics, Clothing, Home & Garden, Sports, Books
- Tags: in-stock, out-of-stock, on-sale, new-arrival, bestseller, clearance
- Groups: Smartphones, Laptops, Men's Wear, Women's Wear, Kitchen, Outdoor
- Notes: Product descriptions, customer reviews, shipping info, returns
- Resources: Product images, manuals, size charts, videos

### API Endpoints Reference

Base path: `/v1/`

#### Creating Entities

Use `curl` or equivalent to POST form data:

```bash
# Tag
curl -X POST "$BASE_URL/v1/tag" -d "name=TagName&Description=Tag description"

# Category
curl -X POST "$BASE_URL/v1/category" -d "name=CategoryName&Description=Description"

# NoteType
curl -X POST "$BASE_URL/v1/note/noteType" -d "name=NoteTypeName&Description=Description"

# RelationType
curl -X POST "$BASE_URL/v1/relationType" -d "name=RelationName&Description=Description&fromCategory=1&toCategory=2"

# Group (with category and tags)
curl -X POST "$BASE_URL/v1/group" -d "name=GroupName&Description=Description&categoryId=1&tags=1&tags=2"

# Note (with owner, type, tags)
curl -X POST "$BASE_URL/v1/note" -d "Name=NoteName&Description=Content&ownerId=1&noteTypeId=1&tags=1"

# Resource from remote URL
curl -X POST "$BASE_URL/v1/resource/remote" -d "url=https://example.com/image.jpg&name=ResourceName&ownerId=1&tags=1"

# Relation
curl -X POST "$BASE_URL/v1/relation" -H "Content-Type: application/json" -d '{"fromGroupId":1,"toGroupId":2,"groupRelationTypeId":1}'

# Query
curl -X POST "$BASE_URL/v1/query" -d "name=QueryName&Text=SELECT * FROM resources LIMIT 10"
```

### Required Counts (for pagination)

Create at least these quantities to trigger pagination (threshold is 50):

| Entity Type | Minimum Count |
|-------------|---------------|
| Tags | 60 |
| Categories | 60 |
| NoteTypes | 60 |
| RelationTypes | 60 |
| Groups | 60 |
| Notes | 60 |
| Resources | 60 |
| Relations | 60 |
| Queries | 60 |

### Semantic Data Lists

#### Tags (60+ items)

Create tags across these categories:
- Status: draft, pending, in-review, approved, published, archived, deprecated, active, inactive, suspended
- Priority: critical, high, medium, low, backlog, urgent, important, normal, deferred
- Type: feature, bug, enhancement, documentation, research, experiment, prototype, production
- Department: engineering, design, marketing, sales, support, hr, finance, legal, operations
- Quality: verified, unverified, tested, untested, stable, unstable, beta, alpha, release-candidate
- Timeline: q1-2024, q2-2024, q3-2024, q4-2024, planned, in-progress, completed, overdue

#### Categories (60+ items)

Create categories for:
- Organization: department, team, project, initiative, program, portfolio, division, unit
- Content: document, report, presentation, spreadsheet, diagram, mockup, prototype, specification
- People: employee, contractor, vendor, customer, partner, stakeholder, advisor, board-member
- Events: meeting, conference, workshop, training, review, retrospective, planning, demo
- Products: software, hardware, service, subscription, license, support-plan, add-on, bundle
- Assets: image, video, audio, code, data, model, template, component

#### NoteTypes (60+ items)

Create note types for:
- Documentation: readme, changelog, api-doc, user-guide, tutorial, faq, troubleshooting, reference
- Communication: email-summary, meeting-notes, announcement, memo, newsletter, status-update
- Analysis: report, review, assessment, evaluation, audit, benchmark, comparison, survey
- Planning: roadmap, sprint-plan, milestone, objective, key-result, goal, strategy, tactic
- Technical: architecture, design-doc, rfc, adr, postmortem, incident-report, runbook, sop

#### RelationTypes (60+ items)

Create relation types like:
- Hierarchy: parent-of, child-of, contains, part-of, belongs-to, owns, managed-by
- Dependencies: depends-on, blocks, required-by, enables, triggers, follows, precedes
- Association: related-to, similar-to, alternative-to, replaces, derived-from, based-on
- Workflow: assigned-to, reviewed-by, approved-by, created-by, modified-by, archived-by
- Business: customer-of, vendor-for, partner-with, competes-with, acquired-by, merged-with

#### Groups (60+ items)

Create hierarchical groups using the categories above, with meaningful names like:
- "Engineering - Backend Team", "Engineering - Frontend Team", "Engineering - DevOps"
- "Q1 2024 Roadmap", "Q2 2024 Roadmap", "Product Launch Initiative"
- "Customer: Acme Corp", "Customer: TechStart Inc", "Vendor: Cloud Services Ltd"

#### Notes (60+ items)

Create notes with realistic content:
- Meeting notes with dates, attendees, action items
- Technical specifications with requirements
- Project status updates with metrics
- Research findings with citations
- Review feedback with recommendations

#### Resources (60+ items)

Create resources using public domain images and content:
- Use URLs from picsum.photos for placeholder images: `https://picsum.photos/seed/{name}/800/600`
- Use httpbin.org for test files: `https://httpbin.org/image/png`
- Use placeholder.com for labeled images: `https://via.placeholder.com/800x600.png?text=ResourceName`

#### Queries (60+ items)

Create useful saved queries:
- "Recent Resources": `SELECT * FROM resources ORDER BY created_at DESC LIMIT 20`
- "Untagged Groups": `SELECT * FROM groups WHERE id NOT IN (SELECT group_id FROM group_tags)`
- "Notes by Type": `SELECT note_types.name, COUNT(*) FROM notes JOIN note_types ON notes.note_type_id = note_types.id GROUP BY note_types.id`
- Various filtered views and reports

### Execution Strategy

1. **Batch creation**: Create entities in batches of 10-20 using parallel curl commands
2. **Track IDs**: Store created entity IDs for use in relationships
3. **Verify counts**: After creation, query each endpoint to verify pagination works
4. **Report results**: Output a summary of created entities

### Verification

After seeding, verify pagination by checking:
```bash
curl "$BASE_URL/v1/tags?page=1" | jq '.length'  # Should be 50
curl "$BASE_URL/v1/tags?page=2" | jq '.length'  # Should be 10+
```

### Error Handling

- If the server is not reachable, report the error and stop
- If an entity creation fails, log the error and continue with the next
- At the end, report success count vs failure count for each entity type
