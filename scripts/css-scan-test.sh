#!/bin/bash

# Guards public/tailwind.css against prose, and guards the guard.
#
# Tailwind v4 keeps automatic source detection on even when the stylesheet names
# explicit @source directives, so every non-gitignored file in the checkout is
# read for class names and a bare word in a plan document becomes a real rule.
# index.css answers that with `@source not` exclusions. An exclusion is the half
# of this that fails silently in both directions: too narrow and the next design
# document puts a junk rule back, too broad and a class the application asks for
# disappears with no error and a page simply renders wrong. So neither direction
# is argued here, both are built and measured.
#
# Usage: ./scripts/css-scan-test.sh
# Exit 0 only when every check passes.

set -u
cd "$(dirname "$0")/.." || exit 1

TAILWIND=./node_modules/.bin/tailwindcss
[ -x "$TAILWIND" ] || { echo "missing $TAILWIND, run npm install"; exit 1; }

TMP=$(mktemp -d) || exit 1
PROBES=()
FAILED=0
git status --porcelain > "$TMP/status-before"

cleanup() {
  # Probe files sit inside the checkout because that is the only place Tailwind
  # would read them from, so they must go even when a check aborts partway.
  [ ${#PROBES[@]} -gt 0 ] && rm -f "${PROBES[@]}"
  rm -rf "$TMP"
}
trap cleanup EXIT

pass() { echo "PASS  $1"; }
fail() { echo "FAIL  $1"; FAILED=1; }

# Probe tokens are stored reversed. Tailwind scans this file too, so spelling a
# utility here in the forward direction would author the very rule the probe
# then claims to have caught.
REVERSED_TOKENS=(
  "mottob-noitpac" "enon-pans" "trats-taolf" "trats-raelc"
  "egap-retfa-kaerb" "3-snmuloc" "8-tnedni" "stnetnoc-egnahc-lliw"
  "nekrad-dnelb-xim" "hcterts-tnetnoc-ecalp" "x-taeper-gb" "otua-noitalosi"
  "launam-snehpyh" "otua-llorcs" "elbisiv-ecafkcab" "yvaw-noitaroced"
  "repus-ngila"
)
TOKENS=()
for r in "${REVERSED_TOKENS[@]}"; do TOKENS+=("$(printf '%s' "$r" | rev)"); done

# The set of utility class selectors a built stylesheet emits. Only the
# utilities layer counts: the components layer is this project's own authored
# CSS, present in every build, and would only add noise to a diff.
classes() {
  awk '
    /^@layer utilities \{$/ { inlayer = 1; depth = 1; next }
    inlayer {
      n = gsub(/\{/, "{"); depth += n
      n = gsub(/\}/, "}"); depth -= n
      if (depth <= 0) { inlayer = 0; next }
    }
    inlayer && /^  \./ { sel = $0; sub(/ *\{.*$/, "", sel); sub(/^  /, "", sel); print sel }
  ' "$1" | sort -u
}

emits() { grep -q "^  \.$1 {" "$2"; }

build() { "$TAILWIND" -i "$1" -o "$2" >"$TMP/build.log" 2>&1 || { cat "$TMP/build.log"; return 1; }; }

# ---------------------------------------------------------------------------
# Where prose lives.
#
# One probe per tree rather than per directory: a tree is the granularity an
# `@source not` glob operates at, so it is the granularity at which a hole is
# actionable. Each probe goes beside a real prose file in that tree, because
# "the next design document lands next to the existing ones" is the scenario
# being tested, not "someone drops a stray .md into a Go package root".
#
# The .md trees are derived from git so a tree nobody has thought of yet is
# probed without editing this file. The two entries below are named explicitly
# because their prose is not .md, and a fix shaped only around .md would let
# them back in silently: tasks holds .txt evidence dumps, and an e2e .ts spec is
# prose where it counts, which is how a Playwright selector once reached the
# stylesheet as an arbitrary variant. index.css quotes that selector; this file
# deliberately does not, because quoting it here would author it again.
# ---------------------------------------------------------------------------
LOCATIONS=()
while IFS=$'\t' read -r tree dir; do
  LOCATIONS+=("$tree:$dir:md")
done < <(git ls-files '*.md' | sort | awk -F/ '
  { t = (NF == 1 ? "(root)" : $1); d = $0; sub(/\/[^\/]*$/, "", d); if (NF == 1) d = "."
    if (!(t in seen)) { seen[t] = 1; print t "\t" d } }')
for extra in "tasks:txt" "e2e:ts"; do
  tree=${extra%%:*}; ext=${extra##*:}
  dir=$(git ls-files "$tree/*.$ext" | head -1)
  [ -n "$dir" ] && LOCATIONS+=("$tree($ext):${dir%/*}:$ext")
done

if [ ${#LOCATIONS[@]} -gt ${#TOKENS[@]} ]; then
  fail "probe-token-supply: ${#LOCATIONS[@]} prose locations, only ${#TOKENS[@]} tokens"
  exit 1
fi

echo "== prose locations under test =="
i=0
for loc in "${LOCATIONS[@]}"; do
  printf '  %-12s %-56s %s\n' "${loc%%:*}" "$(echo "$loc" | cut -d: -f2)" "${TOKENS[$i]}"
  i=$((i + 1))
done
echo

# ---------------------------------------------------------------------------
# Baseline. Establishes that every probe token is absent from both the checkout
# and the shipped build before anything is injected, so a token that quietly
# became real cannot turn a leak into a silent pass.
# ---------------------------------------------------------------------------
build ./index.css "$TMP/baseline.css" || { fail "baseline-build"; exit 1; }
classes "$TMP/baseline.css" > "$TMP/baseline.classes"
echo "baseline: $(wc -l < "$TMP/baseline.classes" | tr -d ' ') classes"

dirty=""
for tok in "${TOKENS[@]}"; do
  emits "$tok" "$TMP/baseline.css" && dirty="$dirty $tok(in-stylesheet)"
  [ -n "$(git grep -I -l -F -- "$tok" 2>/dev/null)" ] && dirty="$dirty $tok(in-checkout)"
done
if [ -n "$dirty" ]; then fail "probe-tokens-are-unused:$dirty"; else pass "probe-tokens-are-unused"; fi

# ---------------------------------------------------------------------------
# Positive control. A token Tailwind does not actually emit would make its probe
# useless and every leak it should have caught invisible, so prove each one
# emits from a file that is unambiguously scanned: templates/*.tpl is named by
# an explicit @source and stays scanned even under source(none).
# ---------------------------------------------------------------------------
control=templates/zz-css-scan-control.tpl
PROBES+=("$control")
printf '<div class="%s"></div>\n' "${TOKENS[@]}" > "$control"
build ./index.css "$TMP/control.css" || { fail "control-build"; exit 1; }
missing=""
for tok in "${TOKENS[@]}"; do emits "$tok" "$TMP/control.css" || missing="$missing $tok"; done
rm -f "$control"
if [ -n "$missing" ]; then fail "probe-tokens-are-real: not emitted even when scanned:$missing"; else pass "probe-tokens-are-real"; fi

# ---------------------------------------------------------------------------
# Finding A. Prose must not reach the stylesheet, from anywhere.
# ---------------------------------------------------------------------------
i=0
for loc in "${LOCATIONS[@]}"; do
  dir=$(echo "$loc" | cut -d: -f2); ext=${loc##*:}
  probe="$dir/zz-css-scan-probe.$ext"
  PROBES+=("$probe")
  printf 'the planner assigns each bucket a %s slot in the layout\n' "${TOKENS[$i]}" > "$probe"
  i=$((i + 1))
done
build ./index.css "$TMP/prose.css" || { fail "prose-build"; exit 1; }
leaks=""
i=0
for loc in "${LOCATIONS[@]}"; do
  emits "${TOKENS[$i]}" "$TMP/prose.css" && leaks="$leaks ${loc%%:*}"
  i=$((i + 1))
done
for p in "${PROBES[@]}"; do rm -f "$p"; done
PROBES=()
if [ -n "$leaks" ]; then
  fail "prose-does-not-reach-the-stylesheet: prose leaks from:$leaks"
else
  pass "prose-does-not-reach-the-stylesheet"
fi

# ---------------------------------------------------------------------------
# The other direction. Every class the application genuinely authors must
# survive the exclusions. The reference names only the trees that really write
# class attributes and switches detection off, so it is the set no exclusion is
# ever allowed to cut into.
# ---------------------------------------------------------------------------
cat > ./zz-css-scan-reference.css <<'EOF'
@import "tailwindcss" source(none);
@plugin "@tailwindcss/forms";
@plugin "@tailwindcss/typography";
@source "./templates/**/*.tpl";
@source "./src/**/*";
@source "./**/*.go";
@source "./plugins/**/*.lua";
@source "./server/template_presets/*.json";
@source "./scripts/**/*";
@source "./shortcodes/**/*";
EOF
PROBES+=("./zz-css-scan-reference.css")
build ./zz-css-scan-reference.css "$TMP/reference.css" || { fail "reference-build"; exit 1; }
classes "$TMP/reference.css" > "$TMP/reference.classes"
rm -f ./zz-css-scan-reference.css
PROBES=()
echo "reference: $(wc -l < "$TMP/reference.classes" | tr -d ' ') classes"
comm -23 "$TMP/reference.classes" "$TMP/baseline.classes" > "$TMP/lost.txt"
if [ -s "$TMP/lost.txt" ]; then
  fail "authored-classes-survive: the exclusions removed $(wc -l < "$TMP/lost.txt" | tr -d ' ') authored classes:"
  sed 's/^/        /' "$TMP/lost.txt"
else
  pass "authored-classes-survive"
fi

# ---------------------------------------------------------------------------
# The third direction. A class the published docs hand out has to be a class
# the shipped stylesheet serves.
#
# CustomHeader, CustomSidebar, CustomSummary and CustomAvatar bodies, custom
# block type templates and the HTML a plugin shortcode returns all live in the
# database and render under this stylesheet, and docs-site spells them out as
# copy-paste examples. An operator who follows the documentation is therefore
# authoring class attributes that were never in this build's scan. Nothing
# above sees that: docs-site is excluded on purpose, and the reference above
# names only the trees that write class attributes the application itself
# renders. A class that lives only in an example is invisible to both, and
# losing it looks like nothing until a pasted template renders wrong.
#
# Reading docs-site here is not the leak the exclusion closed. Only class
# attributes inside fenced code blocks count, so a utility name in an ordinary
# sentence still reaches nothing. shortcode-error, which docs-site spells in
# prose and nowhere else, is the negative control that pins that.
#
# The comparison needs no judgement about which tokens are real utilities.
# Writing every candidate into a scanned template and diffing that build
# against the shipped one leaves exactly the documented classes the shipped
# build does not serve, because a bespoke example class like recipe-card emits
# no rule in either build and cancels out.
# ---------------------------------------------------------------------------
documented=$TMP/documented.tsv
git ls-files 'docs-site/docs/*.md' | xargs awk -v q="'" '
  FNR == 1 { inblock = 0 }
  /^[ \t]*```/ { inblock = !inblock; next }
  !inblock { next }
  {
    re = "class[ \t]*=[ \t]*[\"" q "][^\"" q "]*[\"" q "]"
    line = $0
    while (match(line, re)) {
      attr = substr(line, RSTART, RLENGTH)
      line = substr(line, RSTART + RLENGTH)
      sub(/^class[ \t]*=[ \t]*./, "", attr)
      sub(/.$/, "", attr)
      n = split(attr, parts, /[ \t]+/)
      for (i = 1; i <= n; i++)
        if (parts[i] != "" && !(parts[i] in seen)) {
          seen[parts[i]] = 1
          print parts[i] "\t" FILENAME ":" FNR
        }
    }
  }' > "$documented"
echo "documented: $(cut -f1 "$documented" | sort -u | wc -l | tr -d ' ') distinct class tokens in docs-site examples"

# An empty diff is also what a broken extractor produces, so prove the
# extractor before believing it. The sentinel is stored reversed for the reason
# the probe tokens are: this file is scanned too.
sentinel=$(printf '%s' 'lluf-w' | rev)
extract_ok=1
[ -s "$documented" ] || { echo "        no class attributes found at all"; extract_ok=0; }
cut -f1 "$documented" | grep -qxF "$sentinel" || { echo "        missed $sentinel, which docs-site writes inside a fenced example"; extract_ok=0; }
if cut -f1 "$documented" | grep -qxF shortcode-error; then
  echo "        picked up shortcode-error, which docs-site writes only in prose"
  extract_ok=0
fi
if [ "$extract_ok" -eq 1 ]; then pass "documented-tokens-are-extracted"; else fail "documented-tokens-are-extracted"; fi

documented_control=templates/zz-css-scan-documented.tpl
PROBES+=("$documented_control")
cut -f1 "$documented" | sort -u | awk '{ printf "<div class=\"%s\"></div>\n", $0 }' > "$documented_control"
build ./index.css "$TMP/documented.css" || { fail "documented-build"; exit 1; }
classes "$TMP/documented.css" > "$TMP/documented.classes"
rm -f "$documented_control"
comm -13 "$TMP/baseline.classes" "$TMP/documented.classes" > "$TMP/unserved.txt"
if [ -s "$TMP/unserved.txt" ]; then
  fail "documented-classes-survive: the stylesheet no longer serves $(wc -l < "$TMP/unserved.txt" | tr -d ' ') classes docs-site hands out:"
  while read -r sel; do
    tok=$(printf '%s' "$sel" | sed 's/^\.//; s/\\//g')
    where=$(awk -F'\t' -v t="$tok" '$1 == t { print $2; exit }' "$documented")
    printf '        %-24s %s\n' "$sel" "${where:-first documented use not located}"
  done < "$TMP/unserved.txt"
else
  pass "documented-classes-survive"
fi

# ---------------------------------------------------------------------------
# Finding B. index.css tells maintainers that one glob matching every .md file
# "would not do what it reads like", inferring it from the true observation that
# an exclusion overlapping an explicit @source is a no-op. The inference does
# not hold: ./**/*.md overlaps neither explicit @source, and it fires. That
# sentence is the one that would argue a future maintainer out of the glob which
# closes the mrql/idlock/benchmarks/models hole, in a comment whose whole job is
# to say what is and is not safe to add.
#
# The measurement below is the falsification. It is built from the current
# index.css with its exclusions stripped, not from a pinned commit, so it stays
# honest as index.css changes.
# ---------------------------------------------------------------------------
grep -v '^@source not ' ./index.css > "$TMP/noexcl-body.css"
after=$(grep -n '^@source ' "$TMP/noexcl-body.css" | tail -1 | cut -d: -f1)
variant() {
  out=./zz-css-scan-variant.css
  { head -"$after" "$TMP/noexcl-body.css"
    [ -n "$1" ] && echo "@source not \"$1\";"
    tail -n +$((after + 1)) "$TMP/noexcl-body.css"; } > "$out"
  PROBES=("$out")
  build "$out" "$TMP/$2.css" || return 1
  classes "$TMP/$2.css" > "$TMP/$2.classes"
  rm -f "$out"
  PROBES=()
}
for v in ":noexcl" "./**/*.md:mdglob" "./templates/**:tplglob"; do
  variant "${v%:*}" "${v##*:}" || { fail "variant-build"; exit 1; }
done

n_noexcl=$(wc -l < "$TMP/noexcl.classes" | tr -d ' ')
n_md=$(wc -l < "$TMP/mdglob.classes" | tr -d ' ')
n_tpl=$(wc -l < "$TMP/tplglob.classes" | tr -d ' ')
n_ref=$(wc -l < "$TMP/reference.classes" | tr -d ' ')
md_cut=$(comm -23 "$TMP/noexcl.classes" "$TMP/mdglob.classes" | wc -l | tr -d ' ')
md_lost=$(comm -23 "$TMP/reference.classes" "$TMP/mdglob.classes" | wc -l | tr -d ' ')
echo "measured: no exclusions=$n_noexcl  plus ./**/*.md=$n_md (cuts $md_cut, of which authored=$md_lost)  plus ./templates/**=$n_tpl"

claim_ok=1
[ "$n_md" -lt "$n_noexcl" ] || { echo "        the .md glob cut nothing, so the comment's claim would hold"; claim_ok=0; }
[ "$md_lost" -eq 0 ] || { echo "        the .md glob cut $md_lost of the $n_ref authored classes"; claim_ok=0; }
[ "$n_tpl" -eq "$n_noexcl" ] || { echo "        the overlapping ./templates/** glob was not a no-op after all"; claim_ok=0; }

# Prose, so this pins the sentence rather than deriving it. Whitespace is
# normalised first so a reflowed comment still matches.
normalised=$(tr '\n' ' ' < ./index.css | tr -s ' ')
stale=""
for phrase in "would not do what it reads like" "one glob matching every"; do
  case "$normalised" in *"$phrase"*) stale="$stale \"$phrase\"";; esac
done

if [ "$claim_ok" -eq 1 ] && [ -n "$stale" ]; then
  fail "md-glob-claim-is-honest: the build shows ./**/*.md cuts $md_cut classes and none of the $n_ref authored ones, yet index.css still says:$stale"
elif [ "$claim_ok" -eq 0 ]; then
  fail "md-glob-claim-is-honest: could not reproduce the measurement above; see the lines it printed"
else
  pass "md-glob-claim-is-honest"
fi

# ---------------------------------------------------------------------------
# Finding C. An explicit @source has to reach the whole tree it names.
#
# This is the direction none of the checks above can see. Detection is on, so a
# glob that names a slice of a tree still builds the same stylesheet as one that
# names all of it: the files the glob misses are read anyway, by detection, and
# nothing anywhere fails. `@source "./src/**/*.js"` sat in index.css while src
# held 95 .ts files, four of which author 29 classes the application really
# renders, and every check above passed the whole time.
#
# What made it matter is that the two ways of losing detection both hide behind
# that silence. `@source not` cannot override an explicit @source, so an
# exclusion aimed at src would have been a no-op against the .js line and taken
# the .ts files with it; source(none) would have dropped them without a word.
# The glob is the only thing that says which files are named on purpose, and a
# narrow one says the wrong thing quietly.
#
# So build each explicit glob against its own whole tree with detection off, and
# fail on any class only the whole tree emits. The comparison carries no list of
# extensions and no expected number: it asks the tree what it authors. A tree
# that answers with prose is a real finding too, in the other direction, and the
# fix there is to narrow the tree rather than widen the glob.
# ---------------------------------------------------------------------------
tree_probe() {
  out=./zz-css-scan-tree.css
  { echo '@import "tailwindcss" source(none);'
    echo '@plugin "@tailwindcss/forms";'
    echo '@plugin "@tailwindcss/typography";'
    echo "@source \"$1\";"; } > "$out"
  PROBES=("$out")
  build "$out" "$TMP/$2.css" || return 1
  classes "$TMP/$2.css" > "$TMP/$2.classes"
  rm -f "$out"
  PROBES=()
}

n=0
unreached_total=0
while IFS= read -r glob; do
  tree=$(printf '%s' "$glob" | sed 's|^\./||; s|/.*$||')
  if [ ! -d "$tree" ]; then
    fail "explicit-globs-reach-their-trees: @source \"$glob\" names no directory"
    continue
  fi
  n=$((n + 1))
  tree_probe "$glob" "glob$n" || { fail "tree-probe-build"; exit 1; }
  tree_probe "./$tree/**/*" "tree$n" || { fail "tree-probe-build"; exit 1; }
  comm -13 "$TMP/glob$n.classes" "$TMP/tree$n.classes" > "$TMP/unreached$n.txt"
  count=$(wc -l < "$TMP/unreached$n.txt" | tr -d ' ')
  printf '  %-24s reaches %s of the %s classes ./%s/ authors\n' \
    "$glob" "$(wc -l < "$TMP/glob$n.classes" | tr -d ' ')" \
    "$(wc -l < "$TMP/tree$n.classes" | tr -d ' ')" "$tree"
  [ "$count" -eq 0 ] && continue
  unreached_total=$((unreached_total + count))
  fail "explicit-globs-reach-their-trees: @source \"$glob\" misses $count classes ./$tree/ authors:"
  while read -r sel; do
    tok=$(printf '%s' "$sel" | sed 's/^\.//; s/\\//g')
    where=$(grep -rl -F -- "$tok" "$tree" 2>/dev/null | head -1)
    printf '        %-26s %s\n' "$sel" "${where:-first use not located}"
  done < "$TMP/unreached$n.txt"
done < <(grep '^@source "' ./index.css | sed 's/^@source "//; s/";$//')

if [ "$n" -eq 0 ]; then
  fail "explicit-globs-reach-their-trees: index.css names no explicit @source at all"
elif [ "$unreached_total" -eq 0 ]; then
  pass "explicit-globs-reach-their-trees"
fi

# ---------------------------------------------------------------------------
# The probes are written into the checkout, so prove they are all gone again.
# Only what this run added counts: whoever is editing the exclusions runs this
# with a dirty tree, and failing them for their own work in progress would make
# the script unusable exactly when it is wanted.
git status --porcelain > "$TMP/status-after"
if ! diff -q "$TMP/status-before" "$TMP/status-after" >/dev/null; then
  fail "probes-are-cleaned-up: this run left files behind"
  diff "$TMP/status-before" "$TMP/status-after" | grep '^>' | sed 's/^> /        /'
else
  pass "probes-are-cleaned-up"
fi

echo
[ "$FAILED" -eq 0 ] && echo "css-scan-test: all checks passed" || echo "css-scan-test: FAILURES above"
exit "$FAILED"
