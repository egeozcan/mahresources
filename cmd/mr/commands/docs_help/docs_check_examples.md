---
exitCodes: 0 if every non-skipped doctest passes its declared expectation; 1 otherwise
relatedCmds: docs lint, docs dump
---

# Long

Walks the command tree, extracts every example tagged `# mr-doctest:`, and
evaluates each block against the connected server. Per-example metadata on the
label line controls behavior: `expect-exit=N`, `tolerate=/regex/`,
`skip-on=ephemeral`, `timeout=Ns`, and `stdin=<fixture>`.

The runner pipes each block through `bash -e -o pipefail -c`, with cwd set to
`cmd/mr/` so examples can reference `./testdata/*` fixtures. Requires
`MAHRESOURCES_URL`, `bash`, and `jq` on PATH.

`--files` switches the source from the command tree to markdown outside it:
files, globs, or directories, repeatable, and each `.md` file's fenced
`bash`/`sh`/`shell` blocks become one doctest apiece. The opt-in is inverted
there, because such a file is examples rather than prose that contains some: a
block runs unless it opens with `# mr-doctest: skip, <reason>`. The same
per-example metadata is accepted on that directive line. This is what keeps the
installable agent skill under `skills/` executable rather than merely plausible.

Those blocks run in a temporary directory, so an example that writes a file
(`mrql export -o out.csv`) cannot dirty the working tree that CI diffs
afterwards. A relative path in such a block therefore resolves inside that
scratch directory; only `stdin=<fixture>` still resolves against
`cmd/mr/testdata`. A listed file with no runnable block is an error rather than
a silent pass, since zero examples look exactly like success.

# Example

  # Run against a local ephemeral server
  mr docs check-examples --server http://localhost:8181 --environment=ephemeral

  # Inherit server URL from the environment
  MAHRESOURCES_URL=http://localhost:8181 mr docs check-examples --environment=ephemeral

  # Run the agent skill's markdown examples instead of the command tree's
  mr docs check-examples --files skills/mahresources-mrql/SKILL.md --environment=ephemeral
