# Plugin command inputs are declared names, not arbitrary paths

A command-bearing plugin often has content that only makes sense as a file — a
yt-dlp cookie jar above all — and `mah.commands` gave it no way to put one into
the folder the program runs in. The host writes plugin-supplied contents into the
run's exchange folder before the process starts, but only for names the command
declaration lists, with a leading dot refused outright and a bounded size. The
alternative — a path the plugin chooses, as `argv` parameters already can — would
have let one run hand a program a file at any location the service account can
write, with no name for an operator to consent to and no record of what was
supplied.

## Considered options

- **Arbitrary paths, or a host-templated `{{exchange_dir}}/name`.** A plugin
  already controls one whole argv element, so a path input looks consistent — but
  the exchange folder is the only place the host can bound and account for the
  bytes, and a path would make inputs a filesystem-write capability wearing a
  command's clothes.
- **Stdin or an environment variable.** Both are unbounded in a different
  direction, both invent a second data channel with its own escaping rules, and
  neither fits the motivating case: yt-dlp's `--cookies` wants a file it also
  rewrites in place.
- **Undeclared names, reported after the fact.** Consent would then be to "may be
  given files" rather than to a list, and the operator's review would lose the one
  artifact that makes this power legible.
- **A manifest-carried static file instead of per-run contents.** Narrower, but it
  cannot express a user's cookie jar, and per-run contents subsume it.

## Consequences

A manifest that supplies a name its declaration does not list is refused before
anything durable exists, and every refusal names the file and the command. Input
names are part of manifest identity and of the stored consent record, so adding or
renaming one requires the operator to re-enable the plugin. The host cannot verify
that a declared input is referenced by the program's argv, so that stays a
convention the consent panel shows. Supplied contents linger in the staging root
under `-plugin-command-exchange-retention` unless the plugin discards them, because
reading a program's rewritten file back is a first-class use case. A leading dot is
refused as hygiene rather than as a boundary: the files tools read without being
asked are the operator's mental model, not a complete denylist.
