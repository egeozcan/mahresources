#!/bin/bash

# Edge-case seeder for the 2026-07-29 UI bug hunt (docs/todo.md, Phase 0).
#
# .claude/skills/seed-data/seed.sh gives ~60 of each entity, but none of the
# shapes the hunt's findings actually need: its groups are flat, its resources
# come from picsum.photos, and every name is ASCII. Run this afterwards, against
# the same ephemeral instance.
#
# Usage: ./scripts/seed-edge-cases.sh [base-url]
# Requires: e2e/test-assets/bughunt/ (go run scripts/gen-bughunt-fixtures.go)

BASE_URL="${1:-http://localhost:8181}"
API="$BASE_URL/v1"
ASSETS="e2e/test-assets/bughunt"

GREEN='\033[0;32m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
ok()   { echo -e "${GREEN}[OK]${NC} $1"; }
info() { echo -e "${BLUE}[INFO]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; }

if [ ! -d "$ASSETS" ]; then
    fail "$ASSETS missing — run: go run scripts/gen-bughunt-fixtures.go"
    exit 1
fi

if ! curl -s --connect-timeout 5 "$BASE_URL" > /dev/null 2>&1; then
    fail "no server at $BASE_URL"
    exit 1
fi

# Extract the ID from a create response, or empty on failure. Responses are
# inconsistent about case ("ID" on entities, "id"/"versionNumber" on versions).
id_of() { echo "$1" | grep -oE '"[Ii][Dd]":[0-9]+' | head -1 | grep -o '[0-9]*'; }

# The background hash and thumbnail workers contend with the seeder for the
# SQLite write lock, so a create can fail with "database is locked" purely from
# timing. e2e/helpers/api-client.ts retries the same way.
retry() {
    local attempt=1 out
    while [ $attempt -le 5 ]; do
        out=$("$@")
        case "$out" in
            *"database is locked"*|*"SQLITE_BUSY"*) sleep "$attempt"; attempt=$((attempt + 1)); continue ;;
        esac
        echo "$out"; return 0
    done
    echo "$out"; return 1
}

# upload <file> <name> [extra curl -F args...]
upload_once() {
    local file="$1" name="$2"; shift 2
    curl -s -X POST "$API/resource" -F "resource=@$file" -F "Name=$name" -F "Meta={}" "$@"
}
upload() { retry upload_once "$@"; }

post_json_once() { curl -s -X POST "$API/$1" -H 'Content-Type: application/json' -d "$2"; }
post_json() { retry post_json_once "$1" "$2"; }

# create <endpoint> <json> -> echoes the new ID, or empty.
#
# Deliberately two statements rather than id_of "$(post_json ...)": nesting a
# double-quoted JSON body inside "$( ... )" makes the shell collapse the \"
# escapes, and the server receives a malformed body.
create() {
    local resp
    resp=$(post_json "$1" "$2")
    id_of "$resp"
}

echo "Seeding edge cases into $BASE_URL"
echo

# ---------------------------------------------------------------------------
# Media edge cases — extreme aspect ratios, alpha, vector, non-raster, empty.
# Findings 10, 11, 12, 67, 69, 72, 73, 75, 86.
# ---------------------------------------------------------------------------
info "Uploading media edge cases..."
declare -a MEDIA=(
    "panorama-2400x400.png|bughunt panorama 2400x400"
    "tall-400x1600.png|bughunt tall 400x1600"
    "icon-32x32.png|bughunt tiny icon 32x32"
    "alpha-transparent.png|bughunt alpha transparency"
    "plain-400x300.png|bughunt plain raster 400x300"
    "edge.svg|bughunt vector logo (SVG)"
    "notes.md|bughunt markdown file"
    "payload.json|bughunt json file"
    "table.csv|bughunt csv file"
    "plain.txt|bughunt plain text file"
    "empty.bin|bughunt zero-byte file"
)
MEDIA_OK=0
SVG_ID=""; ALPHA_ID=""; PLAIN_ID=""
for entry in "${MEDIA[@]}"; do
    f="${entry%%|*}"; n="${entry##*|}"
    resp=$(upload "$ASSETS/$f" "$n")
    rid=$(id_of "$resp")
    if [ -n "$rid" ]; then
        MEDIA_OK=$((MEDIA_OK + 1))
        case "$f" in
            edge.svg)              SVG_ID="$rid" ;;
            alpha-transparent.png) ALPHA_ID="$rid" ;;
            plain-400x300.png)     PLAIN_ID="$rid" ;;
        esac
    else
        fail "upload $f: $(echo "$resp" | head -c 200)"
    fi
done
ok "Uploaded $MEDIA_OK/${#MEDIA[@]} media edge cases (svg=$SVG_ID alpha=$ALPHA_ID plain=$PLAIN_ID)"

# ---------------------------------------------------------------------------
# Names: astral plane (finding 84) and 170 characters (findings 67, 89, 141).
# ---------------------------------------------------------------------------
info "Uploading awkward names..."
UNICODE_NAME='Ünïcødé Ñame 测试 🎨'
LONG_NAME='A resource with a really extremely long descriptive name that ought to test how the interface truncates things in cards, tables, breadcrumbs and the browser title bar'
u_resp=$(upload "$ASSETS/version-2.png" "$UNICODE_NAME")
l_resp=$(upload "$ASSETS/version-3.png" "$LONG_NAME")
UNICODE_ID=$(id_of "$u_resp"); LONG_ID=$(id_of "$l_resp")
[ -n "$UNICODE_ID" ] && ok "Unicode-named resource: $UNICODE_ID (${#LONG_NAME} chars in the long one: $LONG_ID)" \
    || fail "unicode upload: $(echo "$u_resp" | head -c 200)"

# ---------------------------------------------------------------------------
# A resource with 3 versions (findings 85, 140). Version uploads use the form
# field `file`, not `resource` — see server/api_handlers/version_api_handlers.go.
# ---------------------------------------------------------------------------
info "Building a multi-version resource..."
# Uploads are deduped by content hash, so this stacks versions onto the plain
# raster already uploaded above rather than re-posting the same bytes.
VERSIONED_ID="$PLAIN_ID"
if [ -n "$VERSIONED_ID" ]; then
    for v in version-2 version-3; do
        curl -s -X POST "$API/resource/versions?resourceId=$VERSIONED_ID" \
            -F "file=@$ASSETS/$v.png" -F "comment=bughunt $v" > /dev/null
    done
    VCOUNT=$(curl -s "$API/resource/versions?resourceId=$VERSIONED_ID" | grep -o '"versionNumber":[0-9]*' | wc -l | tr -d ' ')
    ok "Resource $VERSIONED_ID has $VCOUNT versions"
else
    fail "no plain raster resource to version"
fi

# ---------------------------------------------------------------------------
# A 6-level group chain (findings 8, 43 need depth >= 5). seed.sh creates only
# flat groups: create_groups() never sets ownerId.
# ---------------------------------------------------------------------------
info "Building a 6-level group hierarchy..."
ROOT_RESP=$(post_json group '{"Name":"BugHunt Photography","Description":"Depth-test root"}')
PARENT_ID=$(id_of "$ROOT_RESP")
DEEPEST_ID="$PARENT_ID"
if [ -n "$PARENT_ID" ]; then
    DEPTH=1
    for level in "Landscapes" "Sub-level 1" "Sub-level 2" "Sub-level 3" "Sub-level 4"; do
        resp=$(post_json group "{\"Name\":\"BugHunt $level\",\"OwnerId\":$PARENT_ID}")
        child=$(id_of "$resp")
        if [ -z "$child" ]; then
            fail "level '$level': $(echo "$resp" | head -c 200)"
            break
        fi
        PARENT_ID="$child"; DEEPEST_ID="$child"; DEPTH=$((DEPTH + 1))
    done
    ok "Group chain $DEPTH levels deep, deepest id=$DEEPEST_ID (try /group/tree?containing=$DEEPEST_ID)"
else
    fail "root group: $(echo "$ROOT_RESP" | head -c 200)"
fi

# ---------------------------------------------------------------------------
# A category carrying both a CustomHeader template and a MetaSchema
# (findings 17, 29, 93), plus one with a deliberately broken schema.
# ---------------------------------------------------------------------------
info "Creating templated categories..."
SCHEMA='{\"type\":\"object\",\"properties\":{\"camera\":{\"type\":\"string\"},\"iso\":{\"type\":\"number\"},\"shot_at\":{\"type\":\"string\",\"format\":\"date\"}}}'
HEADER='<div class=\"p-2 bg-blue-50 rounded\">[property name] — [property description default=\"no description\"]</div>'
cat_resp=$(post_json category "{\"Name\":\"BugHunt Templated\",\"Description\":\"Has a CustomHeader and a MetaSchema\",\"MetaSchema\":\"$SCHEMA\",\"CustomHeader\":\"$HEADER\"}")
CAT_ID=$(id_of "$cat_resp")
[ -n "$CAT_ID" ] && ok "Templated category: $CAT_ID" || fail "category: $(echo "$cat_resp" | head -c 200)"

# An empty category, so the live preview has nothing to render against (finding 29).
empty_resp=$(post_json category '{"Name":"BugHunt Empty Category","Description":"No groups — live preview has nothing to bind to"}')
EMPTY_CAT_ID=$(id_of "$empty_resp")
[ -n "$EMPTY_CAT_ID" ] && ok "Empty category: $EMPTY_CAT_ID"

# A group in the templated category, so the header template has something to render.
if [ -n "$CAT_ID" ]; then
    post_json group "{\"Name\":\"BugHunt Templated Group\",\"CategoryId\":$CAT_ID,\"Meta\":\"{\\\"camera\\\":\\\"Leica M6\\\",\\\"iso\\\":400}\"}" > /dev/null
    ok "Group created in category $CAT_ID"
fi

# ---------------------------------------------------------------------------
# Push categories and note types past 50, so the cap-at-50/51 truncation in the
# 'Copy from existing' picker and the tree root list is reachable
# (findings 28, 44, 52).
# ---------------------------------------------------------------------------
info "Padding categories and note types past the 50-row cap..."
CAT_TOTAL=$(curl -s "$API/categories?page=1" | grep -o '"ID":[0-9]*' | wc -l | tr -d ' ')
NT_TOTAL=$(curl -s "$API/note/noteTypes?page=1" | grep -o '"ID":[0-9]*' | wc -l | tr -d ' ')
for i in $(seq 1 25); do
    post_json category "{\"Name\":\"BugHunt Zeta Category $i\"}" > /dev/null
    post_json noteType "{\"Name\":\"BugHunt Zeta NoteType $i\"}" > /dev/null
done
ok "Added 25 categories and 25 note types (page-1 counts before: $CAT_TOTAL / $NT_TOTAL)"

# Root groups sorting late in the alphabet, to expose the tree picker's cap.
for name in "Travel 2026" "Work Projects" "Zulu Archive"; do
    parent=$(create group "{\"Name\":\"BugHunt $name\"}")
    [ -n "$parent" ] && post_json group "{\"Name\":\"BugHunt $name child\",\"OwnerId\":$parent}" > /dev/null
done
ok "Added late-alphabet root groups with children"

# ---------------------------------------------------------------------------
# A note carrying one block of every type (findings 6, 47, 48, 49, 50, 126).
# ---------------------------------------------------------------------------
info "Creating a note with every block type..."
note_resp=$(post_json note '{"Name":"BugHunt block playground","Description":"One block of each type"}')
NOTE_ID=$(id_of "$note_resp")
if [ -n "$NOTE_ID" ]; then
    add_block() {
        curl -s -X POST "$API/note/block" -H 'Content-Type: application/json' \
            -d "{\"noteId\":$NOTE_ID,\"type\":\"$1\",\"content\":$2}" -o /dev/null -w '%{http_code}'
    }
    BLOCKS_OK=0
    for spec in \
        'heading|{"text":"BugHunt heading","level":2}' \
        'text|{"text":"Some body text for the bug hunt."}' \
        'divider|{}' \
        'todos|{"items":[{"id":"a","label":"Dual-write enabled","done":false},{"id":"b","label":"Shadow-read comparison green","done":false}]}' \
        'todos|{"items":[]}' \
        'table|{"columns":[{"id":"c1","label":"Metric"},{"id":"c2","label":"Value"}],"rows":[{"c1":"latency","c2":"12ms"}]}' \
        'gallery|{"resourceIds":[]}' \
        'references|{"groupIds":[]}' \
        'calendar|{"events":[]}' ; do
        t="${spec%%|*}"; c="${spec#*|}"
        code=$(add_block "$t" "$c")
        if [ "$code" = "201" ]; then BLOCKS_OK=$((BLOCKS_OK + 1)); else fail "block $t -> HTTP $code"; fi
    done
    ok "Note $NOTE_ID has $BLOCKS_OK blocks (including an empty todos block for finding 126)"
else
    fail "note: $(echo "$note_resp" | head -c 200)"
fi

# ---------------------------------------------------------------------------
# Relations (findings 57, 59/64, 60/65, 137, 138).
#
# seed.sh creates zero relation types and zero relations: it posts only
# name+Description, while the endpoint requires FromCategory/ToCategory, so all
# 65 attempts 400 with "fromCategory and toCategory are required". Its SKILL.md
# blames an FTS trigger bug; it is simply missing required fields.
# ---------------------------------------------------------------------------
info "Creating relation types and relations..."

# Category names are unique, so a re-run against the same instance must reuse
# the existing row rather than treat the 400 as a failure.
category_named() {
    local name="$1" resp existing
    resp=$(post_json category "{\"Name\":\"$name\"}")
    existing=$(id_of "$resp")
    if [ -z "$existing" ]; then
        existing=$(id_of "$(curl -s --get "$API/categories" --data-urlencode "Name=$name")")
    fi
    echo "$existing"
}

PERSON_CAT=$(category_named "BugHunt Person")
LOCATION_CAT=$(category_named "BugHunt Location")
BUSINESS_CAT=$(category_named "BugHunt Business")

if [ -n "$PERSON_CAT" ] && [ -n "$LOCATION_CAT" ]; then
    RT_PAYLOAD="{\"Name\":\"BugHunt Address\",\"Description\":\"A lives at B\",\"FromCategory\":$PERSON_CAT,\"ToCategory\":$LOCATION_CAT}"
    RT_ID=$(create relationType "$RT_PAYLOAD")

    PERSON_G=$(create group "{\"Name\":\"BugHunt Anna Lindqvist\",\"CategoryId\":$PERSON_CAT}")
    LOCATION_G=$(create group "{\"Name\":\"BugHunt Reykjavik Studio\",\"CategoryId\":$LOCATION_CAT}")
    BUSINESS_G=$(create group "{\"Name\":\"BugHunt Northwind Labs\",\"CategoryId\":$BUSINESS_CAT}")

    if [ -n "$RT_ID" ] && [ -n "$PERSON_G" ] && [ -n "$LOCATION_G" ]; then
        # A named relation, and an unnamed one — finding 59/64 is an empty <h1>
        # on relations with no Name, which the create form does not require.
        post_json relation "{\"FromGroupId\":$PERSON_G,\"ToGroupId\":$LOCATION_G,\"GroupRelationTypeId\":$RT_ID,\"Name\":\"BugHunt named relation\"}" > /dev/null
        post_json relation "{\"FromGroupId\":$PERSON_G,\"ToGroupId\":$LOCATION_G,\"GroupRelationTypeId\":$RT_ID}" > /dev/null
        ok "Relation type $RT_ID with 2 relations (one unnamed, for the empty-h1 finding)"
        echo "  Category-mismatch repro: /relation/new?FromGroupId=$BUSINESS_G&ToGroupId=$LOCATION_G&GroupRelationTypeId=$RT_ID"
    else
        fail "relation type or groups (rt=$RT_ID person=$PERSON_G location=$LOCATION_G)"
    fi
else
    fail "relation categories (person=$PERSON_CAT location=$LOCATION_CAT)"
fi

# A relation type with no categories must still be rejected — keep one attempt
# so the seeder notices if that validation ever changes.
RT_BAD=$(post_json relationType '{"Name":"BugHunt Invalid RT"}')
echo "$RT_BAD" | grep -q 'required' && ok "Relation type still requires both categories" \
    || fail "relation type validation changed: $(echo "$RT_BAD" | head -c 160)"

# ---------------------------------------------------------------------------
# A saved SQL query whose text contains the characters smartypants mangles
# (finding 82), and one that fails at runtime (finding 100).
# ---------------------------------------------------------------------------
info "Creating SQL queries with typography-sensitive text..."
curl -s -X POST "$API/query" \
    --data-urlencode "Name=BugHunt typography test" \
    --data-urlencode "Text=SELECT * FROM tags WHERE name != 'x' AND id > 1 -- range 1--5 and dots..." > /dev/null
curl -s -X POST "$API/query" \
    --data-urlencode "Name=BugHunt broken sql" \
    --data-urlencode "Text=SELECT * FROM nonexistent_table_xyz" > /dev/null
curl -s -X POST "$API/query" \
    --data-urlencode "Name=BugHunt column order" \
    --data-urlencode "Text=SELECT name AS zebra, id AS apple, description AS mango FROM tags ORDER BY id LIMIT 3" > /dev/null
ok "Created 3 SQL queries"

echo
info "Edge-case seeding complete."
echo "  SVG resource:        $SVG_ID   (rotate / recalculate / preview)"
echo "  Alpha PNG:           $ALPHA_ID (rotate format preservation)"
echo "  Plain PNG:           $PLAIN_ID"
echo "  Versioned resource:  $VERSIONED_ID"
echo "  Unicode name:        $UNICODE_ID"
echo "  Long name:           $LONG_ID"
echo "  Deepest group:       $DEEPEST_ID"
echo "  Templated category:  $CAT_ID"
echo "  Empty category:      $EMPTY_CAT_ID"
echo "  Block playground:    $NOTE_ID"
