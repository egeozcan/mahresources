#!/usr/bin/env bash

# Seed a local Mahresources instance with a deterministic Project Management
# playground, or remove exactly the rows created by this script.
#
# Usage:
#   ./scripts/seed-project-management.sh seed [base-url] [anchor-date]
#   ./scripts/seed-project-management.sh reset [base-url]
#   ./scripts/seed-project-management.sh reseed [base-url] [anchor-date]
#
# The anchor date defaults to today. Given the same anchor, names, relationships,
# workflow states and dates are identical. MR_TOKEN is forwarded as a Bearer
# token when the target has authentication enabled.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACTION="${1:-seed}"
BASE_URL="${2:-${MAHRESOURCES_URL:-http://localhost:8181}}"
ANCHOR_DATE="${3:-$(date +%Y-%m-%d)}"
STATE_FILE="${MAH_PM_DEMO_STATE:-$SCRIPT_DIR/../.pm-demo-state}"

BASE_URL="${BASE_URL%/}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

info() { printf "%b[INFO]%b %s\n" "$BLUE" "$NC" "$*" >&2; }
ok() { printf "%b[OK]%b %s\n" "$GREEN" "$NC" "$*" >&2; }
warn() { printf "%b[WARN]%b %s\n" "$YELLOW" "$NC" "$*" >&2; }
die() { printf "%b[FAIL]%b %s\n" "$RED" "$NC" "$*" >&2; exit 1; }

usage() {
    cat >&2 <<'EOF'
Usage:
  ./scripts/seed-project-management.sh seed [base-url] [anchor-date]
  ./scripts/seed-project-management.sh reset [base-url]
  ./scripts/seed-project-management.sh reseed [base-url] [anchor-date]

Environment:
  MR_TOKEN                  Bearer token for an auth-enabled server
  MAHRESOURCES_URL          Default base URL (when base-url is omitted)
  MAH_PM_DEMO_STATE         Override the reset manifest path
  MAH_PM_DEMO_ALLOW_REMOTE  Set to 1 to permit a non-loopback target
EOF
}

case "$ACTION" in
    seed|reset|reseed) ;;
    -h|--help) usage; exit 0 ;;
    http://*|https://*)
        # Keep the one-argument shape used by the repository's older seeders.
        ANCHOR_DATE="${2:-$(date +%Y-%m-%d)}"
        BASE_URL="${ACTION%/}"
        ACTION="seed"
        ;;
    *) usage; die "unknown action: $ACTION" ;;
esac

for command in curl jq; do
    command -v "$command" >/dev/null 2>&1 || die "$command is required"
done

# The demo script is intentionally local-first. A remote instance may contain
# real data, so reaching one requires a conspicuous opt-in even though reset is
# ID-scoped.
if ! printf '%s\n' "$BASE_URL" | grep -Eq '^https?://(localhost|127\.0\.0\.1|\[::1\])(:[0-9]+)?(/|$)'; then
    [ "${MAH_PM_DEMO_ALLOW_REMOTE:-0}" = "1" ] || die \
        "refusing non-loopback target $BASE_URL (set MAH_PM_DEMO_ALLOW_REMOTE=1 to override)"
fi

CURL_ARGS=(--silent --show-error --connect-timeout 5 --max-time 30)
if [ -n "${MR_TOKEN:-}" ]; then
    CURL_ARGS+=(--header "Authorization: Bearer $MR_TOKEN")
fi

# request METHOD PATH [curl args...]
# Prints only the response body. SQLite's transient writer contention is retried
# in the same bounded fashion as the Playwright API helper.
request() {
    local method="$1" path="$2"
    shift 2
    local attempt tmp status body summary

    attempt=1
    while [ "$attempt" -le 5 ]; do
        tmp="$(mktemp "${TMPDIR:-/tmp}/mah-pm-demo.XXXXXX")"
        if status=$(curl "${CURL_ARGS[@]}" --request "$method" --output "$tmp" \
            --write-out '%{http_code}' "$BASE_URL$path" "$@"); then
            body="$(<"$tmp")"
        else
            rm -f "$tmp"
            [ "$attempt" -lt 5 ] || die "request failed: $method $path"
            sleep "$attempt"
            attempt=$((attempt + 1))
            continue
        fi
        rm -f "$tmp"

        case "$status" in
            2??) printf '%s' "$body"; return 0 ;;
        esac
        case "$body" in
            *"database is locked"*|*"SQLITE_BUSY"*)
                [ "$attempt" -lt 5 ] || die "database remained busy: $method $path"
                sleep "$attempt"
                attempt=$((attempt + 1))
                continue
                ;;
        esac
        summary="$(printf '%s' "$body" | tr '\n' ' ' | cut -c1-300)"
        die "HTTP $status from $method $path: ${summary:-empty response}"
    done
}

date_shift() {
    local offset="$1" modifier
    if date -j -u -f '%Y-%m-%d' "$ANCHOR_DATE" '+%Y-%m-%d' >/dev/null 2>&1; then
        modifier="${offset}d"
        case "$offset" in -*) ;; *) modifier="+${modifier}" ;; esac
        date -j -u -v"$modifier" -f '%Y-%m-%d' "$ANCHOR_DATE" '+%Y-%m-%d'
    elif date -u -d "$ANCHOR_DATE $offset days" '+%Y-%m-%d' >/dev/null 2>&1; then
        date -u -d "$ANCHOR_DATE $offset days" '+%Y-%m-%d'
    else
        die "anchor date must be a real YYYY-MM-DD date: $ANCHOR_DATE"
    fi
}

track_entity() {
    local kind="$1" id="$2" name="$3"
    # Names in this dataset are single-line and tab-free, which keeps the reset
    # manifest human-readable without lossy shell escaping.
    printf 'entity\t%s\t%s\t%s\n' "$kind" "$id" "$name" >> "$STATE_FILE"
}

create_tag() {
    local name="$1" description="$2" body id
    body="$(request POST '/v1/tag' \
        --data-urlencode "name=$name" \
        --data-urlencode "Description=$description")"
    id="$(printf '%s' "$body" | jq -er '.ID')"
    track_entity tag "$id" "$name"
    printf '%s' "$id"
}

create_group() {
    local name="$1" description="$2" category_id="$3" owner_id="$4" meta="$5"
    local args body id
    args=(
        --data-urlencode "name=$name"
        --data-urlencode "Description=$description"
        --data-urlencode "categoryId=$category_id"
        --data-urlencode "Meta=$meta"
    )
    if [ -n "$owner_id" ]; then args+=(--data-urlencode "ownerId=$owner_id"); fi
    body="$(request POST '/v1/group' "${args[@]}")"
    id="$(printf '%s' "$body" | jq -er '.ID')"
    track_entity group "$id" "$name"
    printf '%s' "$id"
}

create_epic() {
    local project_id="$1" name="$2" payload body id
    payload="$(jq -nc --argjson project_id "$project_id" --arg name "$name" \
        '{project_id:$project_id,name:$name}')"
    body="$(request POST '/v1/plugins/project-management/api/epic/create' \
        --header 'Content-Type: application/json' --data "$payload")"
    id="$(printf '%s' "$body" | jq -er '.id')"
    track_entity group "$id" "$name"
    printf '%s' "$id"
}

edit_group_meta() {
    local id="$1" path="$2" json_value="$3"
    request POST "/v1/group/editMeta?id=$id" \
        --data-urlencode "path=$path" \
        --data-urlencode "value=$json_value" >/dev/null
}

create_task() {
    local owner_id="$1" name="$2" status="$3" priority="$4" due="$5" start="$6" description="$7"
    local payload body id
    payload="$(jq -nc \
        --argjson owner_id "$owner_id" \
        --arg name "$name" \
        --arg status "$status" \
        --arg priority "$priority" \
        --arg due "$due" \
        --arg start "$start" \
        --arg description "$description" \
        '{owner_id:$owner_id,name:$name,description:$description}
         + (if $status == "" then {} else {status:$status} end)
         + (if $priority == "" then {} else {priority:$priority} end)
         + (if $due == "" then {} else {due:$due} end)
         + (if $start == "" then {} else {start:$start} end)')"
    body="$(request POST '/v1/plugins/project-management/api/task/create' \
        --header 'Content-Type: application/json' --data "$payload")"
    id="$(printf '%s' "$body" | jq -er '.id')"
    track_entity note "$id" "$name"
    printf '%s' "$id"
}

create_no_status_task() {
    local owner_id="$1" note_type_id="$2" name="$3" description="$4"
    local body id
    # This one deliberately uses the host note endpoint. It represents the
    # documented edge case where a PM Task created outside the plugin has no
    # status: visible in backlog/dashboard, absent from board columns.
    body="$(request POST '/v1/note' \
        --data-urlencode "Name=$name" \
        --data-urlencode "Description=$description" \
        --data-urlencode "ownerId=$owner_id" \
        --data-urlencode "NoteTypeId=$note_type_id" \
        --data-urlencode 'Meta={"priority":"low","order":"n"}')"
    id="$(printf '%s' "$body" | jq -er '.ID')"
    track_entity note "$id" "$name"
    printf '%s' "$id"
}

create_block() {
    local note_id="$1" type="$2" position="$3" content="$4"
    local payload body id
    payload="$(jq -nc \
        --argjson note_id "$note_id" \
        --arg type "$type" \
        --arg position "$position" \
        --argjson content "$content" \
        '{noteId:$note_id,type:$type,position:$position,content:$content}')"
    body="$(request POST '/v1/note/block' \
        --header 'Content-Type: application/json' --data "$payload")"
    id="$(printf '%s' "$body" | jq -er '.id')"
    printf '%s' "$id"
}

add_tags() {
    local entity_plural="$1" entity_id="$2"
    shift 2
    local args tag_id
    args=(--data-urlencode "ID=$entity_id")
    for tag_id in "$@"; do args+=(--data-urlencode "EditedId=$tag_id"); done
    request POST "/v1/$entity_plural/addTags" "${args[@]}" >/dev/null
}

ensure_plugin() {
    local plugins enabled
    plugins="$(request GET '/v1/plugins/manage')"
    printf '%s' "$plugins" | jq -e '.[] | select(.name == "project-management")' >/dev/null \
        || die "project-management plugin is not visible; restart the server with the branch's plugin directory"
    enabled="$(printf '%s' "$plugins" | jq -r '.[] | select(.name == "project-management") | .enabled')"
    if [ "$enabled" != "true" ]; then
        info "Enabling project-management plugin"
        request POST '/v1/plugin/enable' --data-urlencode 'name=project-management' >/dev/null
    fi
}

verify_name_and_delete() {
    local kind="$1" id="$2" expected_name="$3" endpoint delete_path tmp status body actual_name summary
    case "$kind" in
        note) endpoint="/v1/note?id=$id"; delete_path="/v1/note/delete?Id=$id" ;;
        group) endpoint="/v1/group?id=$id"; delete_path="/v1/group/delete?Id=$id" ;;
        tag) endpoint="/v1/tag?id=$id"; delete_path="/v1/tag/delete?Id=$id" ;;
        *) warn "unknown state entry kind '$kind' (id $id)"; return 1 ;;
    esac

    tmp="$(mktemp "${TMPDIR:-/tmp}/mah-pm-demo-reset.XXXXXX")"
    if ! status=$(curl "${CURL_ARGS[@]}" --output "$tmp" --write-out '%{http_code}' "$BASE_URL$endpoint"); then
        rm -f "$tmp"
        warn "could not inspect $kind $id; leaving it untouched"
        return 1
    fi
    body="$(<"$tmp")"
    rm -f "$tmp"
    if [ "$status" = "404" ]; then
        warn "$kind $id is already absent"
        return 0
    fi
    case "$status" in
        2??) ;;
        *)
            summary="$(printf '%s' "$body" | tr '\n' ' ' | cut -c1-200)"
            warn "HTTP $status while inspecting $kind $id: ${summary:-empty response}"
            return 1
            ;;
    esac

    actual_name="$(printf '%s' "$body" | jq -r '.Name // .name // empty')"
    if [ "$actual_name" != "$expected_name" ]; then
        warn "refusing to delete $kind $id: expected '$expected_name', found '$actual_name'"
        return 1
    fi
    request POST "$delete_path" >/dev/null
    return 0
}

reset_demo() {
    local recorded_url failures record kind id name
    [ -f "$STATE_FILE" ] || { ok "No Project Management demo state found; nothing to reset"; return 0; }

    recorded_url="$(awk -F '\t' '$1 == "base_url" { print $2; exit }' "$STATE_FILE")"
    [ -n "$recorded_url" ] || die "state file is missing its base_url: $STATE_FILE"
    [ "$recorded_url" = "$BASE_URL" ] || die \
        "state file belongs to $recorded_url, not $BASE_URL (set MAH_PM_DEMO_STATE to choose another manifest)"

    info "Removing tracked Project Management demo rows from $BASE_URL"
    failures=0
    while IFS=$'\t' read -r record kind id name; do
        [ "$record" = "entity" ] || continue
        if ! verify_name_and_delete "$kind" "$id" "$name"; then
            failures=$((failures + 1))
        fi
    done < <(awk '$1 == "entity" { rows[++n]=$0 } END { for (i=n; i>=1; i--) print rows[i] }' "$STATE_FILE")

    if [ "$failures" -ne 0 ]; then
        die "$failures tracked row(s) could not be safely removed; state kept at $STATE_FILE"
    fi
    rm -f "$STATE_FILE"
    ok "Removed the synthetic projects, epics, tasks and labels; plugin setup/taxonomy was kept"
}

seed_demo() {
    local setup config project_category epic_category task_type
    local past3 past2 past1 today tomorrow next5 next7 later target30 target45 target60
    local marker_tag aurora_tag operations_tag research_tag platform_tag launch_tag unassigned_tag
    local aurora operations empty_project
    local discovery platform launch reliability automation orphaned
    local t1 t2 t3 t4 t5 t6 t7 t8 t9 t10 t11 t12 t13 t14 t15
    local body aurora_stats operations_stats projects epics task_blocks
    local acceptance_block status_update_block

    [ ! -e "$STATE_FILE" ] || die "demo state already exists at $STATE_FILE; run reset or reseed first"
    date_shift 0 >/dev/null

    ensure_plugin
    info "Provisioning Project Management taxonomy"
    setup="$(request POST '/v1/plugins/project-management/api/setup' \
        --header 'Content-Type: application/json' --data '{}')"
    project_category="$(printf '%s' "$setup" | jq -er '.project_category_id')"
    epic_category="$(printf '%s' "$setup" | jq -er '.epic_category_id')"
    task_type="$(printf '%s' "$setup" | jq -er '.task_type_id')"

    # The named fixture is meant to demonstrate the bundled workflow, not make
    # guesses about an operator's custom state machine. Refuse custom identifiers
    # before creating any sample rows; label-only customizations remain fine.
    config="$(request GET '/v1/plugins/project-management/api/config')"
    printf '%s' "$config" | jq -e '
        .default_status == "todo" and .done_status == "done"
        and ([.statuses[].name] | (index("backlog") and index("todo") and index("in_progress") and index("blocked") and index("done")))
        and ([.priorities[].name] | (index("low") and index("medium") and index("high") and index("urgent")))
    ' >/dev/null || die \
        "the demo seeder requires the default status/priority identifiers; restore them or seed a purpose-built fixture"

    umask 077
    printf 'version\t1\nbase_url\t%s\nanchor_date\t%s\n' "$BASE_URL" "$ANCHOR_DATE" > "$STATE_FILE"

    past3="$(date_shift -3)"
    past2="$(date_shift -2)"
    past1="$(date_shift -1)"
    today="$(date_shift 0)"
    tomorrow="$(date_shift 1)"
    next5="$(date_shift 5)"
    next7="$(date_shift 7)"
    later="$(date_shift 21)"
    target30="$(date_shift 30)"
    target45="$(date_shift 45)"
    target60="$(date_shift 60)"

    info "Creating synthetic labels"
    marker_tag="$(create_tag 'pm-demo/synthetic' 'Marker used only by the Project Management synthetic playground')"
    aurora_tag="$(create_tag 'pm-demo/aurora' 'Synthetic Aurora project label')"
    operations_tag="$(create_tag 'pm-demo/operations' 'Synthetic Operations project label')"
    research_tag="$(create_tag 'pm-demo/research' 'Synthetic research work')"
    platform_tag="$(create_tag 'pm-demo/platform' 'Synthetic platform work')"
    launch_tag="$(create_tag 'pm-demo/launch' 'Synthetic launch work')"
    unassigned_tag="$(create_tag 'pm-demo/unassigned' 'Synthetic orphaned-epic work')"

    info "Creating projects and epics"
    body="$(jq -nc --arg target "$target45" '{status:"in_progress",key:"AUR",target_date:$target}')"
    aurora="$(create_group '[PM Demo] Aurora Launch' \
        'Synthetic product-launch project for exercising every Project Management view.' \
        "$project_category" '' "$body")"
    body="$(jq -nc --arg target "$target30" '{status:"todo",key:"OPS",target_date:$target}')"
    operations="$(create_group '[PM Demo] Operations Refresh' \
        'Synthetic internal-operations project with overdue, blocked and completed work.' \
        "$project_category" '' "$body")"
    body="$(jq -nc --arg target "$target60" '{status:"backlog",key:"EMPTY",target_date:$target}')"
    empty_project="$(create_group '[PM Demo] Empty Playground' \
        'Synthetic empty project for checking zero states and first-task flows.' \
        "$project_category" '' "$body")"

    discovery="$(create_epic "$aurora" 'Discovery & design')"
    platform="$(create_epic "$aurora" 'Platform delivery')"
    launch="$(create_epic "$aurora" 'Launch readiness')"
    reliability="$(create_epic "$operations" 'Reliability')"
    automation="$(create_epic "$operations" 'Workflow automation')"

    edit_group_meta "$discovery" status '"in_progress"'
    edit_group_meta "$discovery" target_date "\"$later\""
    edit_group_meta "$platform" status '"blocked"'
    edit_group_meta "$platform" target_date "\"$target30\""
    edit_group_meta "$launch" status '"todo"'
    edit_group_meta "$launch" target_date "\"$target45\""
    edit_group_meta "$reliability" status '"in_progress"'
    edit_group_meta "$automation" status '"backlog"'

    body="$(jq -nc --arg target "$next7" '{status:"blocked",target_date:$target}')"
    orphaned="$(create_group '[PM Demo] Orphaned epic' \
        'Synthetic PM Epic with no project owner; it exercises the Unassigned picker section.' \
        "$epic_category" '' "$body")"

    for body in "$aurora" "$operations" "$empty_project" "$discovery" "$platform" "$launch" "$reliability" "$automation" "$orphaned"; do
        add_tags groups "$body" "$marker_tag"
    done
    add_tags groups "$aurora" "$aurora_tag"
    add_tags groups "$operations" "$operations_tag"
    add_tags groups "$orphaned" "$unassigned_tag"

    info "Creating tasks across statuses, priorities and timeline buckets"
    t1="$(create_task "$discovery" 'Interview pilot customers' 'backlog' 'high' "${later}T10:00" "${past3}T09:00" 'Synthetic discovery task: future due date and research label.')"
    t2="$(create_task "$discovery" 'Validate accessible color system' 'done' 'medium' "${past3}T16:00" "${past3}T09:00" 'Synthetic completed task whose past due date must not count as overdue.')"
    t3="$(create_task "$platform" 'Publish API contract' 'in_progress' 'urgent' "${today}T17:00" "${past2}T09:00" 'Synthetic active task due on the anchor date.')"
    t4="$(create_task "$platform" 'Resolve sandbox provisioning blocker' 'blocked' 'high' "${tomorrow}T12:00" "${past1}T09:00" 'Synthetic blocker due tomorrow.')"
    t5="$(create_task "$platform" 'Migrate telemetry dashboard' 'todo' 'medium' "${next5}T15:00" "${today}T09:00" 'Synthetic task in the next-seven-days timeline bucket.')"
    t6="$(create_task "$launch" 'Draft release announcement' 'backlog' 'low' "${later}T11:00" '' 'Synthetic later-due launch task.')"
    t7="$(create_task "$launch" 'Run launch readiness review' '' 'urgent' "${next7}T14:00" '' 'Synthetic task omitting status to exercise the configured default.')"
    t8="$(create_task "$aurora" 'Decide release codename' 'done' '' '' '' 'Synthetic project-level task with neither epic, priority nor due date.')"
    t9="$(create_no_status_task "$aurora" "$task_type" 'Triage imported feedback' 'Synthetic core-created PM Task exercising the effective default status across native and plugin views.')"

    t10="$(create_task "$reliability" 'Document restore drill' 'in_progress' 'high' "${past2}T10:00" "${past3}T09:00" 'Synthetic unfinished overdue task.')"
    t11="$(create_task "$reliability" 'Rotate on-call shadow' 'todo' 'medium' "${tomorrow}T17:00" "${today}T10:00" 'Synthetic operations task due tomorrow.')"
    t12="$(create_task "$automation" 'Archive obsolete runbooks' 'done' 'low' "${past1}T12:00" "${past3}T09:00" 'Synthetic completed operations task.')"
    t13="$(create_task "$automation" 'Automate weekly triage' 'blocked' 'urgent' '' '' 'Synthetic blocked task with no due date.')"
    t14="$(create_task "$operations" 'Inventory duplicate alerts' 'backlog' '' "${later}T09:00" '' 'Synthetic direct project task with no epic and no priority.')"
    t15="$(create_task "$orphaned" 'Find a new project home' 'todo' 'medium' "${next5}T13:00" '' 'Synthetic task reachable through the orphaned epic view.')"

    info "Adding PM-specific content blocks to the active delivery task"
    body="$(jq -nc '{criteria:"API consumers can complete the documented happy path\nValidation failures use the documented error shape",verification:"Run the contract test suite"}')"
    acceptance_block="$(create_block "$t3" 'plugin:project-management:acceptance-criteria' 'n' "$body")"
    body="$(jq -nc '{summary:"The first contract draft is published for review.",next_step:"Resolve reviewer comments and tag v1.",blocker:"Waiting on the authentication error taxonomy."}')"
    status_update_block="$(create_block "$t3" 'plugin:project-management:status-update' 't' "$body")"

    for body in "$t1" "$t2" "$t3" "$t4" "$t5" "$t6" "$t7" "$t8" "$t9" "$t10" "$t11" "$t12" "$t13" "$t14" "$t15"; do
        add_tags notes "$body" "$marker_tag"
    done
    for body in "$t1" "$t2"; do add_tags notes "$body" "$aurora_tag" "$research_tag"; done
    for body in "$t3" "$t4" "$t5"; do add_tags notes "$body" "$aurora_tag" "$platform_tag"; done
    for body in "$t6" "$t7" "$t8" "$t9"; do add_tags notes "$body" "$aurora_tag" "$launch_tag"; done
    for body in "$t10" "$t11" "$t12" "$t13" "$t14"; do add_tags notes "$body" "$operations_tag"; done
    add_tags notes "$t15" "$unassigned_tag"

    info "Verifying seeded relationships and aggregate states"
    aurora_stats="$(request GET "/v1/plugins/project-management/api/stats?project=$aurora&now=${today}T12:00")"
    printf '%s' "$aurora_stats" | jq -e '.total == 9 and .by_status.done == 2' >/dev/null \
        || die "Aurora stats did not match the 9-task / 2-done fixture"
    operations_stats="$(request GET "/v1/plugins/project-management/api/stats?project=$operations&now=${today}T12:00")"
    printf '%s' "$operations_stats" | jq -e '.total == 5 and .by_status.done == 1 and .overdue == 1' >/dev/null \
        || die "Operations stats did not match the 5-task / 1-done / 1-overdue fixture"
    epics="$(request GET "/v1/plugins/project-management/api/epics?project=$aurora")"
    printf '%s' "$epics" | jq -e '.epics | length == 3' >/dev/null \
        || die "Aurora epic relationship verification failed"
    projects="$(request GET '/v1/plugins/project-management/api/projects')"
    printf '%s' "$projects" | jq -e --argjson id "$orphaned" '.unassigned | any(.id == $id)' >/dev/null \
        || die "orphaned epic was not exposed under Unassigned"
    task_blocks="$(request GET "/v1/note/blocks?noteId=$t3")"
    printf '%s' "$task_blocks" | jq -e \
        --argjson acceptance "$acceptance_block" --argjson update "$status_update_block" \
        'length == 2
         and any(.id == $acceptance and .type == "plugin:project-management:acceptance-criteria")
         and any(.id == $update and .type == "plugin:project-management:status-update")' >/dev/null \
        || die "PM Task block verification failed"

    printf 'complete\ttrue\n' >> "$STATE_FILE"
    ok "Seeded 3 projects, 6 epics, 15 tasks, 2 task blocks and 7 labels (anchor $ANCHOR_DATE)"
    printf '\nOpen the sample views:\n'
    printf '  %s/plugins/project-management/board?project=%s&view=board\n' "$BASE_URL" "$aurora"
    printf '  %s/plugins/project-management/board?project=%s&view=dashboard\n' "$BASE_URL" "$operations"
    printf '  %s/plugins/project-management/board?epic=%s&view=timeline\n' "$BASE_URL" "$orphaned"
    printf 'Reset manifest: %s\n' "$STATE_FILE"
}

# Connectivity is checked before any state change. The site root intentionally
# redirects to /dashboard, while this JSON endpoint is stable and non-mutating.
# A 401/403 is reported with the endpoint's own message; use an admin MR_TOKEN
# for setup on an auth-enabled instance.
request GET '/v1/plugins/manage' >/dev/null

case "$ACTION" in
    seed) seed_demo ;;
    reset) reset_demo ;;
    reseed) reset_demo; seed_demo ;;
esac
