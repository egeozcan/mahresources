#!/bin/bash

# Mahresources Semantic Data Seeder
# Seeds an instance with rich content across all entity types
# Usage: ./seed.sh [base-url]

# Don't exit on error - we handle errors per-entity
# set -e

# Trap to handle interrupts gracefully
trap 'echo "Script interrupted"; exit 1' INT TERM

BASE_URL="${1:-http://localhost:8181}"
API_URL="$BASE_URL/v1"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters (separate variables for bash 3.x compatibility)
TAGS_SUCCESS=0
TAGS_FAIL=0
CATEGORIES_SUCCESS=0
CATEGORIES_FAIL=0
NOTE_TYPES_SUCCESS=0
NOTE_TYPES_FAIL=0
RELATION_TYPES_SUCCESS=0
RELATION_TYPES_FAIL=0
GROUPS_SUCCESS=0
GROUPS_FAIL=0
NOTES_SUCCESS=0
NOTES_FAIL=0
RESOURCES_SUCCESS=0
RESOURCES_FAIL=0
RELATIONS_SUCCESS=0
RELATIONS_FAIL=0
QUERIES_SUCCESS=0
QUERIES_FAIL=0

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check server connectivity
check_server() {
    log_info "Checking server connectivity at $BASE_URL..."
    if curl -s --connect-timeout 5 "$BASE_URL" > /dev/null 2>&1; then
        log_success "Server is reachable"
    else
        log_error "Cannot connect to server at $BASE_URL"
        exit 1
    fi
}

# Generic POST function - returns ID on success, empty on failure
post_entity() {
    local endpoint="$1"
    local data="$2"

    response=$(curl -s -w "\n%{http_code}" -X POST "$API_URL/$endpoint" -d "$data" 2>/dev/null)
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if echo "$http_code" | grep -q "^2"; then
        echo "$body" | grep -o '"ID":[0-9]*' | grep -o '[0-9]*' | head -1
        return 0
    else
        return 1
    fi
}

# POST JSON data
post_json() {
    local endpoint="$1"
    local data="$2"

    response=$(curl -s -w "\n%{http_code}" -X POST "$API_URL/$endpoint" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json" \
        -d "$data" 2>/dev/null)
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if echo "$http_code" | grep -q "^2"; then
        echo "$body" | grep -o '"ID":[0-9]*' | grep -o '[0-9]*' | head -1
        return 0
    else
        return 1
    fi
}

# ============================================================================
# TAG CREATION
# ============================================================================
create_tags() {
    log_info "Creating tags..."

    TAG_IDS=""

    # All tags in one list
    local tags="draft pending in-review approved published archived deprecated active inactive suspended on-hold cancelled critical high-priority medium-priority low-priority backlog urgent important normal deferred someday feature bug enhancement documentation research experiment prototype production maintenance refactor security performance engineering design marketing sales support hr finance legal operations executive product qa verified unverified tested untested stable unstable beta alpha release-candidate gold-master confidential public internal draft-only needs-review outdated evergreen time-sensitive mobile desktop web api frontend backend database infrastructure cloud-native serverless microservice monolith legacy modern"

    for tag in $tags; do
        local desc="Tag for categorizing items as $tag"
        id=$(post_entity "tag" "name=$tag&Description=$desc")
        if [ -n "$id" ]; then
            TAG_IDS="$TAG_IDS $id"
            TAGS_SUCCESS=$((TAGS_SUCCESS + 1))
        else
            TAGS_FAIL=$((TAGS_FAIL + 1))
        fi
    done

    log_success "Created $TAGS_SUCCESS tags"
}

# ============================================================================
# CATEGORY CREATION
# ============================================================================
create_categories() {
    log_info "Creating categories..."

    CATEGORY_IDS=""

    local categories="Department Team Project Initiative Program Portfolio Document Report Presentation Spreadsheet Diagram Mockup Prototype Specification Employee Contractor Vendor Customer Partner Stakeholder Meeting Conference Workshop Training Review Retrospective Planning Demo Software Hardware Service Subscription License SupportPlan AddOn Bundle ImageAsset VideoAsset AudioAsset CodeRepository Dataset Model Template Component ResearchPaper CaseStudy WhitePaper BlogPost Tutorial Guide APIDocumentation ReleaseNotes Changelog Roadmap Sprint Epic UserStory Task BugReport FeatureRequest Milestone Objective KeyResult Strategy Tactic Architecture Design Implementation"

    for cat in $categories; do
        local desc="Category for organizing $cat items"
        id=$(post_entity "category" "name=$cat&Description=$desc")
        if [ -n "$id" ]; then
            CATEGORY_IDS="$CATEGORY_IDS $id"
            CATEGORIES_SUCCESS=$((CATEGORIES_SUCCESS + 1))
        else
            CATEGORIES_FAIL=$((CATEGORIES_FAIL + 1))
        fi
    done

    log_success "Created $CATEGORIES_SUCCESS categories"
}

# ============================================================================
# NOTE TYPE CREATION
# ============================================================================
create_note_types() {
    log_info "Creating note types..."

    NOTE_TYPE_IDS=""

    local note_types="README Changelog APIDoc UserGuide Tutorial FAQ Troubleshooting Reference EmailSummary MeetingNotes Announcement Memo Newsletter StatusUpdate WeeklyReport MonthlyReport QuarterlyReport AnnualReport Assessment Evaluation Audit Benchmark Comparison SurveyResults RoadmapDoc SprintPlan MilestoneDoc ObjectiveDoc KeyResultDoc GoalDoc StrategyDoc TacticDoc ArchitectureDoc DesignDoc RFC ADR Postmortem IncidentReport Runbook SOP CodeReview PRDescription CommitNotes BugAnalysis FeatureSpec Requirements UserResearch InterviewNotes Brainstorm DecisionLog RiskAssessment SWOTAnalysis CompetitorAnalysis MarketResearch CustomerFeedback SupportTicket KnowledgeBase InternalWiki PersonalNotes DraftDoc SpecificationDoc PlanningDoc ReviewDoc"

    for nt in $note_types; do
        local desc="Note type for $nt content"
        id=$(post_entity "note/noteType" "name=$nt&Description=$desc")
        if [ -n "$id" ]; then
            NOTE_TYPE_IDS="$NOTE_TYPE_IDS $id"
            NOTE_TYPES_SUCCESS=$((NOTE_TYPES_SUCCESS + 1))
        else
            NOTE_TYPES_FAIL=$((NOTE_TYPES_FAIL + 1))
        fi
    done

    log_success "Created $NOTE_TYPES_SUCCESS note types"
}

# ============================================================================
# RELATION TYPE CREATION
# ============================================================================
create_relation_types() {
    log_info "Creating relation types..."

    RELATION_TYPE_IDS=""

    local relation_types="parent-of child-of contains part-of belongs-to owns managed-by reports-to depends-on blocks required-by enables triggers follows precedes related-to similar-to alternative-to replaces derived-from based-on inspired-by assigned-to reviewed-by approved-by created-by modified-by archived-by customer-of vendor-for partner-with competes-with acquired-by merged-with funded-by sponsors collaborates-with references cites implements extends overrides inherits-from compatible-with incompatible-with integrates-with connected-to linked-to associated-with correlates-with causes affects influences supports opposes complements duplicates supersedes deprecates validates tests-for documents describes monitors tracks"

    # Don't set category restrictions so relations can work with any groups
    for rt in $relation_types; do
        local desc="Relationship type: $rt"
        id=$(post_entity "relationType" "name=$rt&Description=$desc")
        if [ -n "$id" ]; then
            RELATION_TYPE_IDS="$RELATION_TYPE_IDS $id"
            RELATION_TYPES_SUCCESS=$((RELATION_TYPES_SUCCESS + 1))
        else
            RELATION_TYPES_FAIL=$((RELATION_TYPES_FAIL + 1))
        fi
    done

    log_success "Created $RELATION_TYPES_SUCCESS relation types"
}

# ============================================================================
# GROUP CREATION
# ============================================================================
create_groups() {
    log_info "Creating groups..."

    GROUP_IDS=""

    # Get category IDs as array
    cat_arr=($CATEGORY_IDS)
    tag_arr=($TAG_IDS)

    local groups="Engineering-Backend Engineering-Frontend Engineering-DevOps Engineering-Mobile Engineering-QA Engineering-Security Design-UX Design-UI Design-Brand Design-Research Product-Core Product-Growth Product-Platform Product-Analytics Marketing-Content Marketing-Digital Marketing-Events Marketing-PR Sales-Enterprise Sales-SMB Sales-Partnerships Support-Tier1 Support-Tier2 Support-Enterprise HR-Recruiting HR-PeopleOps HR-Learning Finance-Accounting Finance-FPA Finance-Procurement Legal-Contracts Legal-Compliance Legal-IP Project-Alpha Project-Beta Project-Gamma Project-Delta Project-Epsilon Customer-AcmeCorp Customer-TechStart Customer-GlobalTech Customer-InnovateCo Customer-FutureLabs Vendor-CloudServices Vendor-HardwareInc Vendor-SaaSTools Q1-2024-Planning Q2-2024-Planning Q3-2024-Planning Q4-2024-Planning AnnualReview-2023 AnnualReview-2024 Research-AIML Research-Data Research-UX Archive-2022 Archive-2023 Training-Materials Onboarding-Program Company-Handbook Brand-Assets External-Partners Internal-Tools DevTools"

    for group in $groups; do
        local name=$(echo "$group" | sed 's/-/ /g')
        local desc="Group for $name related items"
        # Get a category (cycle through)
        local cat_idx=$((GROUPS_SUCCESS % ${#cat_arr[@]}))
        local cat_id="${cat_arr[$cat_idx]:-1}"
        # Get tags
        local tag_idx1=$((GROUPS_SUCCESS % ${#tag_arr[@]}))
        local tag_idx2=$(((GROUPS_SUCCESS + 1) % ${#tag_arr[@]}))
        local tag1="${tag_arr[$tag_idx1]:-1}"
        local tag2="${tag_arr[$tag_idx2]:-1}"

        id=$(post_entity "group" "name=$name&Description=$desc&categoryId=$cat_id&tags=$tag1&tags=$tag2")
        if [ -n "$id" ]; then
            GROUP_IDS="$GROUP_IDS $id"
            GROUPS_SUCCESS=$((GROUPS_SUCCESS + 1))
        else
            GROUPS_FAIL=$((GROUPS_FAIL + 1))
        fi
    done

    log_success "Created $GROUPS_SUCCESS groups"
}

# ============================================================================
# NOTE CREATION
# ============================================================================
create_notes() {
    log_info "Creating notes..."

    NOTE_IDS=""

    group_arr=($GROUP_IDS)
    nt_arr=($NOTE_TYPE_IDS)
    tag_arr=($TAG_IDS)

    local notes="Weekly-Engineering-Standup Q1-Product-Roadmap-Review Customer-Feedback-Summary Security-Audit-Findings New-Hire-Onboarding Architecture-Decision Bug-Triage-Sprint-23 Marketing-Campaign-Results Sales-Pipeline-Review Support-Ticket-Analysis Code-Review-Guidelines Database-Migration-Plan API-Deprecation-Notice Design-System-Release-Notes Incident-Postmortem Performance-Optimization-Results User-Research-Findings Competitive-Analysis-Update Budget-Planning-FY2024 Team-Retrospective Release-Notes-v350 Customer-Success-Story Vendor-Evaluation Training-Completion-Report Interview-Feedback Project-Status-Alpha Technical-Debt-Assessment Customer-Churn-Analysis Feature-Specification-Export Legal-Review-Terms Infrastructure-Cost-Report Sprint-Planning-24 User-Acceptance-Testing Partner-Integration-Guide Employee-Survey-Results Accessibility-Audit-Results Data-Privacy-Assessment Quarterly-Business-Review Product-Analytics-Report Board-Update-Meeting-Notes Risk-Register-Update Process-Improvement-Proposal Customer-Interview-Notes Technical-Spec-Auth-v2 Marketing-Content-Calendar Support-Escalation-Procedures Compliance-Training-Summary Feature-Prioritization-Framework Team-Capacity-Planning AB-Test-Results Documentation-Style-Guide Deployment-Runbook Customer-Health-Score-Model API-Rate-Limiting-Strategy Mobile-App-Analytics Knowledge-Transfer-Document Hiring-Pipeline-Status Feature-Flag-Documentation Error-Budget-Report Customer-Communication-Template"

    for note in $notes; do
        local name=$(echo "$note" | sed 's/-/ /g')
        local desc="Detailed content for $name"
        # Get random assignments
        local grp_idx=$((NOTES_SUCCESS % ${#group_arr[@]}))
        local nt_idx=$((NOTES_SUCCESS % ${#nt_arr[@]}))
        local tag_idx=$((NOTES_SUCCESS % ${#tag_arr[@]}))

        local owner_id="${group_arr[$grp_idx]:-1}"
        local note_type_id="${nt_arr[$nt_idx]:-1}"
        local tag_id="${tag_arr[$tag_idx]:-1}"

        id=$(post_entity "note" "Name=$name&Description=$desc&ownerId=$owner_id&noteTypeId=$note_type_id&tags=$tag_id")
        if [ -n "$id" ]; then
            NOTE_IDS="$NOTE_IDS $id"
            NOTES_SUCCESS=$((NOTES_SUCCESS + 1))
        else
            NOTES_FAIL=$((NOTES_FAIL + 1))
        fi
    done

    log_success "Created $NOTES_SUCCESS notes"
}

# ============================================================================
# RESOURCE CREATION
# ============================================================================
create_resources() {
    log_info "Creating resources..."

    RESOURCE_IDS=""

    group_arr=($GROUP_IDS)
    tag_arr=($TAG_IDS)

    local resources="product-roadmap-2024 architecture-diagram-v3 brand-guidelines customer-presentation monthly-report-template team-photo-engineering office-layout-floor1 product-screenshot-dashboard product-screenshot-search product-screenshot-settings logo-primary logo-monochrome logo-icon marketing-banner-q1 social-media-template email-header-template presentation-template invoice-template contract-template nda-template employee-handbook onboarding-checklist benefits-overview expense-policy travel-policy security-guidelines data-classification incident-response-plan disaster-recovery-plan api-documentation integration-guide user-manual quick-start-guide admin-guide release-notes-v3 changelog-2024 migration-guide troubleshooting-guide faq-document training-slides workshop-materials certification-guide video-tutorial-intro video-tutorial-advanced webinar-recording-jan podcast-episode-12 infographic-features infographic-workflow case-study-acme case-study-techstart case-study-globaltech whitepaper-ai whitepaper-security research-report-ux research-report-market competitive-analysis pricing-sheet feature-comparison roi-calculator benchmark-results"

    for res in $resources; do
        local name=$(echo "$res" | sed 's/-/ /g')
        local desc="Resource file: $name"
        # Use placeholder images
        local url="https://picsum.photos/seed/$res/800/600"

        local grp_idx=$((RESOURCES_SUCCESS % ${#group_arr[@]}))
        local tag_idx=$((RESOURCES_SUCCESS % ${#tag_arr[@]}))
        local owner_id="${group_arr[$grp_idx]:-1}"
        local tag_id="${tag_arr[$tag_idx]:-1}"

        id=$(post_entity "resource/remote" "url=$url&name=$name&ownerId=$owner_id&tags=$tag_id")
        if [ -n "$id" ]; then
            RESOURCE_IDS="$RESOURCE_IDS $id"
            RESOURCES_SUCCESS=$((RESOURCES_SUCCESS + 1))
        else
            RESOURCES_FAIL=$((RESOURCES_FAIL + 1))
        fi
    done

    log_success "Created $RESOURCES_SUCCESS resources"
}

# ============================================================================
# RELATION CREATION
# ============================================================================
create_relations() {
    log_info "Creating relations..."
    log_info "Note: Relations require groups with matching categories to relation type constraints."

    RELATION_IDS=""

    group_arr=($GROUP_IDS)

    # Get existing relation types from the server
    existing_rts=$(curl -s "$API_URL/relationTypes" 2>/dev/null | grep -o '"ID":[0-9]*' | grep -o '[0-9]*' | head -5)
    rt_arr=($existing_rts)

    num_groups=${#group_arr[@]}
    num_rts=${#rt_arr[@]}

    if [ "$num_rts" -eq 0 ]; then
        log_warn "No relation types found, skipping relation creation"
        return
    fi

    log_info "Found $num_rts existing relation types, creating relations..."

    # Create 65 relations (some may fail due to category constraints)
    for i in $(seq 1 65); do
        local from_idx=$((i % num_groups))
        local to_idx=$(((i + 3) % num_groups))

        # Avoid self-relations
        if [ $from_idx -eq $to_idx ]; then
            to_idx=$(((to_idx + 1) % num_groups))
        fi

        local from_id="${group_arr[$from_idx]}"
        local to_id="${group_arr[$to_idx]}"
        local rt_idx=$((i % num_rts))
        local rel_type_id="${rt_arr[$rt_idx]:-1}"

        local json="{\"FromGroupId\":$from_id,\"ToGroupId\":$to_id,\"GroupRelationTypeId\":$rel_type_id}"
        id=$(post_json "relation" "$json")
        if [ -n "$id" ]; then
            RELATION_IDS="$RELATION_IDS $id"
            RELATIONS_SUCCESS=$((RELATIONS_SUCCESS + 1))
        else
            RELATIONS_FAIL=$((RELATIONS_FAIL + 1))
        fi
    done

    log_success "Created $RELATIONS_SUCCESS relations"
}

# ============================================================================
# QUERY CREATION
# ============================================================================
create_queries() {
    log_info "Creating queries..."

    QUERY_IDS=""

    # Store queries in a temp file to handle special characters
    cat > /tmp/seed_queries.txt << 'QUERIES_EOF'
Recent-Resources|SELECT * FROM resources ORDER BY created_at DESC LIMIT 20
Recent-Notes|SELECT * FROM notes ORDER BY created_at DESC LIMIT 20
Recent-Groups|SELECT * FROM groups ORDER BY created_at DESC LIMIT 20
Resources-by-Size|SELECT * FROM resources ORDER BY file_size DESC LIMIT 20
Notes-This-Week|SELECT * FROM notes WHERE created_at > datetime('now', '-7 days')
Groups-by-Category|SELECT categories.name, COUNT(*) FROM groups JOIN categories ON groups.category_id = categories.id GROUP BY categories.id
Resources-by-Type|SELECT content_type, COUNT(*) FROM resources GROUP BY content_type
Notes-by-Type|SELECT note_types.name, COUNT(*) FROM notes JOIN note_types ON notes.note_type_id = note_types.id GROUP BY note_types.id
Untagged-Resources|SELECT * FROM resources WHERE id NOT IN (SELECT resource_id FROM resource_tags)
Untagged-Groups|SELECT * FROM groups WHERE id NOT IN (SELECT group_id FROM group_tags)
Untagged-Notes|SELECT * FROM notes WHERE id NOT IN (SELECT note_id FROM note_tags)
Most-Tagged-Resources|SELECT resources.id, COUNT(resource_tags.tag_id) as cnt FROM resources LEFT JOIN resource_tags ON resources.id = resource_tags.resource_id GROUP BY resources.id ORDER BY cnt DESC LIMIT 20
Most-Tagged-Groups|SELECT groups.id, COUNT(group_tags.tag_id) as cnt FROM groups LEFT JOIN group_tags ON groups.id = group_tags.group_id GROUP BY groups.id ORDER BY cnt DESC LIMIT 20
Groups-Without-Owner|SELECT * FROM groups WHERE owner_id IS NULL
Notes-Without-Owner|SELECT * FROM notes WHERE owner_id IS NULL
Resources-Without-Owner|SELECT * FROM resources WHERE owner_id IS NULL
Images-Only|SELECT * FROM resources WHERE content_type LIKE 'image/%'
Documents-Only|SELECT * FROM resources WHERE content_type LIKE 'application/pdf'
Large-Files|SELECT * FROM resources WHERE file_size > 1000000 ORDER BY file_size DESC
Small-Files|SELECT * FROM resources WHERE file_size < 10000 ORDER BY file_size ASC
Active-Tags|SELECT tags.id, COUNT(resource_tags.resource_id) as cnt FROM tags LEFT JOIN resource_tags ON tags.id = resource_tags.tag_id GROUP BY tags.id HAVING cnt > 0
Unused-Tags|SELECT * FROM tags WHERE id NOT IN (SELECT tag_id FROM resource_tags)
Categories-Usage|SELECT categories.name, COUNT(groups.id) FROM categories LEFT JOIN groups ON categories.id = groups.category_id GROUP BY categories.id
Relation-Types-Usage|SELECT group_relation_types.name, COUNT(group_relations.id) FROM group_relation_types LEFT JOIN group_relations ON group_relation_types.id = group_relations.group_relation_type_id GROUP BY group_relation_types.id
Notes-With-Resources|SELECT notes.* FROM notes WHERE id IN (SELECT note_id FROM note_resources)
Notes-Without-Resources|SELECT notes.* FROM notes WHERE id NOT IN (SELECT note_id FROM note_resources)
Resources-With-Notes|SELECT resources.* FROM resources WHERE id IN (SELECT resource_id FROM note_resources)
Resources-Without-Notes|SELECT resources.* FROM resources WHERE id NOT IN (SELECT resource_id FROM note_resources)
Top-Level-Groups|SELECT * FROM groups WHERE owner_id IS NULL
Nested-Groups|SELECT * FROM groups WHERE owner_id IS NOT NULL
Resources-Created-Today|SELECT * FROM resources WHERE date(created_at) = date('now')
Notes-Created-Today|SELECT * FROM notes WHERE date(created_at) = date('now')
Resources-Last-30-Days|SELECT * FROM resources WHERE created_at > datetime('now', '-30 days')
Notes-Last-30-Days|SELECT * FROM notes WHERE created_at > datetime('now', '-30 days')
Groups-Last-30-Days|SELECT * FROM groups WHERE created_at > datetime('now', '-30 days')
Content-Type-Distribution|SELECT content_type, COUNT(*), SUM(file_size) FROM resources GROUP BY content_type
Daily-Resource-Count|SELECT date(created_at), COUNT(*) FROM resources GROUP BY date(created_at) ORDER BY date(created_at) DESC LIMIT 30
Daily-Note-Count|SELECT date(created_at), COUNT(*) FROM notes GROUP BY date(created_at) ORDER BY date(created_at) DESC LIMIT 30
Tag-Cloud-Data|SELECT tags.name, COUNT(resource_tags.resource_id) FROM tags LEFT JOIN resource_tags ON tags.id = resource_tags.tag_id GROUP BY tags.id ORDER BY COUNT(resource_tags.resource_id) DESC LIMIT 50
Resources-Per-Group|SELECT groups.name, COUNT(group_resources.resource_id) FROM groups LEFT JOIN group_resources ON groups.id = group_resources.group_id GROUP BY groups.id
Notes-Per-Group|SELECT groups.name, COUNT(group_notes.note_id) FROM groups LEFT JOIN group_notes ON groups.id = group_notes.group_id GROUP BY groups.id
Storage-Usage|SELECT SUM(file_size), COUNT(*), AVG(file_size) FROM resources
Duplicate-Names-Check|SELECT name, COUNT(*) FROM resources GROUP BY name HAVING COUNT(*) > 1
Recent-Relations|SELECT * FROM group_relations ORDER BY created_at DESC LIMIT 20
Full-Text-Search|SELECT * FROM resources WHERE name LIKE '%search%'
All-Entities-Count|SELECT 'resources', COUNT(*) FROM resources
Notes-Count|SELECT 'notes', COUNT(*) FROM notes
Groups-Count|SELECT 'groups', COUNT(*) FROM groups
Tags-Count|SELECT 'tags', COUNT(*) FROM tags
Categories-Count|SELECT 'categories', COUNT(*) FROM categories
Weekly-Activity|SELECT date(created_at), COUNT(*) FROM resources GROUP BY date(created_at)
Monthly-Activity|SELECT date(created_at), COUNT(*) FROM notes GROUP BY date(created_at)
Empty-Description-Resources|SELECT * FROM resources WHERE description IS NULL OR description = ''
Empty-Description-Notes|SELECT * FROM notes WHERE description IS NULL OR description = ''
Empty-Description-Groups|SELECT * FROM groups WHERE description IS NULL OR description = ''
Resources-Ordered-By-Name|SELECT * FROM resources ORDER BY name ASC
Notes-Ordered-By-Name|SELECT * FROM notes ORDER BY name ASC
Groups-Ordered-By-Name|SELECT * FROM groups ORDER BY name ASC
Tags-Ordered-By-Name|SELECT * FROM tags ORDER BY name ASC
Categories-Ordered-By-Name|SELECT * FROM categories ORDER BY name ASC
QUERIES_EOF

    while IFS='|' read -r name sql; do
        [ -z "$name" ] && continue
        local readable_name=$(echo "$name" | sed 's/-/ /g')

        # Use curl's --data-urlencode for proper encoding
        response=$(curl -s -w "\n%{http_code}" -X POST "$API_URL/query" \
            --data-urlencode "name=$readable_name" \
            --data-urlencode "Text=$sql" 2>/dev/null)
        http_code=$(echo "$response" | tail -n1)
        body=$(echo "$response" | sed '$d')

        if echo "$http_code" | grep -q "^2"; then
            id=$(echo "$body" | grep -o '"ID":[0-9]*' | grep -o '[0-9]*' | head -1)
            if [ -n "$id" ]; then
                QUERY_IDS="$QUERY_IDS $id"
                QUERIES_SUCCESS=$((QUERIES_SUCCESS + 1))
            else
                QUERIES_FAIL=$((QUERIES_FAIL + 1))
            fi
        else
            QUERIES_FAIL=$((QUERIES_FAIL + 1))
        fi
    done < /tmp/seed_queries.txt

    rm -f /tmp/seed_queries.txt
    log_success "Created $QUERIES_SUCCESS queries"
}

# ============================================================================
# VERIFICATION
# ============================================================================
verify_pagination() {
    log_info "Verifying pagination..."

    local endpoints="tags categories note/noteTypes relationTypes groups notes resources queries"
    local names="Tags Categories NoteTypes RelationTypes Groups Notes Resources Queries"

    local endpoint_arr=($endpoints)
    local name_arr=($names)

    for i in $(seq 0 $((${#endpoint_arr[@]} - 1))); do
        local endpoint="${endpoint_arr[$i]}"
        local name="${name_arr[$i]}"

        # Get page 1 count
        page1=$(curl -s "$API_URL/$endpoint?page=1" 2>/dev/null)
        count1=$(echo "$page1" | grep -o '"ID"' | wc -l | tr -d ' ')

        # Get page 2 count
        page2=$(curl -s "$API_URL/$endpoint?page=2" 2>/dev/null)
        count2=$(echo "$page2" | grep -o '"ID"' | wc -l | tr -d ' ')

        if [ "$count1" -ge 50 ] && [ "$count2" -gt 0 ]; then
            log_success "$name: Page 1 has $count1 items, Page 2 has $count2 items - Pagination working!"
        elif [ "$count1" -ge 50 ]; then
            log_warn "$name: Page 1 has $count1 items, but Page 2 is empty"
        else
            log_warn "$name: Only $count1 items on page 1 (need 50+ for pagination)"
        fi
    done
}

# ============================================================================
# SUMMARY
# ============================================================================
print_summary() {
    echo ""
    echo "=============================================="
    echo "           SEEDING COMPLETE"
    echo "=============================================="
    echo ""
    printf "%-20s %10s %10s\n" "Entity Type" "Success" "Failed"
    printf "%-20s %10s %10s\n" "--------------------" "----------" "----------"
    printf "%-20s %10d %10d\n" "Tags" "$TAGS_SUCCESS" "$TAGS_FAIL"
    printf "%-20s %10d %10d\n" "Categories" "$CATEGORIES_SUCCESS" "$CATEGORIES_FAIL"
    printf "%-20s %10d %10d\n" "Note Types" "$NOTE_TYPES_SUCCESS" "$NOTE_TYPES_FAIL"
    printf "%-20s %10d %10d\n" "Relation Types" "$RELATION_TYPES_SUCCESS" "$RELATION_TYPES_FAIL"
    printf "%-20s %10d %10d\n" "Groups" "$GROUPS_SUCCESS" "$GROUPS_FAIL"
    printf "%-20s %10d %10d\n" "Notes" "$NOTES_SUCCESS" "$NOTES_FAIL"
    printf "%-20s %10d %10d\n" "Resources" "$RESOURCES_SUCCESS" "$RESOURCES_FAIL"
    printf "%-20s %10d %10d\n" "Relations" "$RELATIONS_SUCCESS" "$RELATIONS_FAIL"
    printf "%-20s %10d %10d\n" "Queries" "$QUERIES_SUCCESS" "$QUERIES_FAIL"
    echo ""

    total_success=$((TAGS_SUCCESS + CATEGORIES_SUCCESS + NOTE_TYPES_SUCCESS + RELATION_TYPES_SUCCESS + GROUPS_SUCCESS + NOTES_SUCCESS + RESOURCES_SUCCESS + RELATIONS_SUCCESS + QUERIES_SUCCESS))
    total_fail=$((TAGS_FAIL + CATEGORIES_FAIL + NOTE_TYPES_FAIL + RELATION_TYPES_FAIL + GROUPS_FAIL + NOTES_FAIL + RESOURCES_FAIL + RELATIONS_FAIL + QUERIES_FAIL))

    echo "Total: $total_success created, $total_fail failed"
    echo ""
}

# ============================================================================
# MAIN
# ============================================================================
main() {
    echo ""
    echo "=============================================="
    echo "    Mahresources Semantic Data Seeder"
    echo "=============================================="
    echo "Target: $BASE_URL"
    echo ""

    check_server

    create_tags
    create_categories
    create_note_types
    create_relation_types
    create_groups
    create_notes
    create_resources
    create_relations
    create_queries

    verify_pagination
    print_summary
}

main
