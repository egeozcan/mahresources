# mr CLI testdata

Fixtures used by `# mr-doctest:` example blocks in the mr CLI help files.

Doctest blocks run with `cwd` set to `cmd/mr/`, so examples reference
files here as `./testdata/sample.jpg`, `./testdata/sample.pdf`, etc.

Files are intentionally tiny (a few KB combined) because the doctest
runner uploads them to an ephemeral server on every CI run.
