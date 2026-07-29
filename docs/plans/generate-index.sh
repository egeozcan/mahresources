#!/usr/bin/env bash
# Regenerate docs/plans/README.md from the contents of docs/plans/.
# Run after adding, moving, or archiving a plan:  ./docs/plans/generate-index.sh
set -euo pipefail
cd "$(dirname "$0")/../.."   # repo root

area_of() { # $1 = filename stem (no date, no suffix)
  case "$1" in
    mrql*|*-mrql-*|*-mrql)              echo "MRQL" ;;
    *selector*|*autocompleter*|*picker*|*dropdown*|*chip-input*|*quick-slot*|*multi-tag*)
                                        echo "Selectors & pickers" ;;
    *category-template*|*shortcode*|*category-section*|*template-partial*)
                                        echo "Category templates & shortcodes" ;;
    *schema*|*metadata*|*display-renderers*|*labeled-enum*|*meta-subpath*|*free-field*)
                                        echo "Schema & metadata display" ;;
    *plugin*|*lua*|*data-views*)        echo "Plugins" ;;
    *export*|*import*|*entity-guid*|*group-*|*merge*)
                                        echo "Groups, export & import" ;;
    *auth*|*user*|*admin*|*rbac*|*session*|*share*)
                                        echo "Auth, admin & sharing" ;;
    *cli*)                              echo "CLI (mr)" ;;
    *tag*|tier*)                        echo "Tagging" ;;
    *note*|*block*|*calendar*|*at-mentions*|*timeline*)
                                        echo "Notes, blocks & timeline" ;;
    *resource*|*version*|*thumbnail*|*image*|*similarity*|*media*|*crop*|*trim*|*paste-upload*|*gallery*|*lightbox*|*office*)
                                        echo "Resources & media" ;;
    *bug-backlog*|*bughunt*|*audit*|*adversarial*|*review*|*kan-*)
                                        echo "Bug backlogs & audits" ;;
    *docs*|*e2e*|*postgres*|*test*|*benchmark*|*perf*)
                                        echo "Docs, testing & performance" ;;
    *)                                  echo "Other" ;;
  esac
}

TMP=$(mktemp)
for f in docs/plans/archive/*.md; do
  b=$(basename "$f")
  stem=$(echo "$b" | sed -E 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}-//; s/-(design|impl)\.md$//; s/\.md$//')
  date=$(echo "$b" | grep -oE '^[0-9]{4}-[0-9]{2}-[0-9]{2}' || true)
  # filenames without a date prefix fall back to their last commit date
  if [ -z "$date" ]; then
    date=$(git log -1 --follow --format=%ad --date=format:%Y-%m-%d -- "$f" 2>/dev/null || true)
    [ -z "$date" ] && date="undated"
  fi
  printf '%s\t%s\t%s\t%s\n' "$(area_of "$stem")" "$stem" "$date" "$b" >> "$TMP"
done

OUT=docs/plans/README.md
{
  echo "# Plans & design docs"
  echo
  echo "Design and implementation notes, one pair per feature. They are a historical"
  echo "record of intent, not living documentation — when a plan and the code disagree,"
  echo "the code wins. For how the system is actually put together see"
  echo "[../architecture/](../architecture/) and the root [CLAUDE.md](../../CLAUDE.md)."
  echo
  echo "## Conventions"
  echo
  echo '- `YYYY-MM-DD-<slug>-design.md` — what we decided to build and why.'
  echo '- `YYYY-MM-DD-<slug>-impl.md` — how it was built, usually with a task checklist.'
  echo '- A slug with no suffix is a standalone plan with no separate design doc.'
  echo '- The date is when the work started, not when it shipped.'
  echo
  echo "## Active"
  echo
  for f in docs/plans/*.md; do
    b=$(basename "$f"); [ "$b" = "README.md" ] && continue
    echo "- [$b]($b)"
  done
  echo
  echo "## Archive"
  echo
  echo "Shipped or abandoned work, kept for the reasoning. $(ls docs/plans/archive/*.md | wc -l | tr -d ' ') documents."
  echo
  # stable, meaningful section order
  for area in "MRQL" "Selectors & pickers" "Schema & metadata display" \
              "Category templates & shortcodes" "Resources & media" \
              "Notes, blocks & timeline" "Groups, export & import" \
              "Auth, admin & sharing" "Plugins" "CLI (mr)" "Tagging" \
              "Bug backlogs & audits" "Docs, testing & performance" "Other"; do
    rows=$(awk -F'\t' -v a="$area" '$1==a' "$TMP" | sort -t$'\t' -k3,3 || true)
    [ -z "$rows" ] && continue
    n=$(echo "$rows" | grep -c . )
    echo "### $area ($n)"
    echo
    # group by stem so design+impl appear on one line
    echo "$rows" | awk -F'\t' '{print $2}' | sort -u | while read -r stem; do
      links=""
      first_date=""
      while IFS=$'\t' read -r _a _s d b; do
        [ "$_s" = "$stem" ] || continue
        [ -z "$first_date" ] && first_date="$d"
        case "$b" in
          *-design.md) label="design" ;;
          *-impl.md)   label="impl" ;;
          *)           label="plan" ;;
        esac
        links="$links [$label](archive/$b)"
      done <<< "$rows"
      echo "- \`$first_date\` **$stem** —$links"
    done
    echo
  done
} > "$OUT"

rm -f "$TMP"
echo "wrote $OUT ($(wc -l < "$OUT" | tr -d ' ') lines)"
