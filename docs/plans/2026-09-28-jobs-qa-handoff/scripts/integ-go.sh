#!/bin/bash
SP=/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad
WT=$SP/lanes/${2:-b1-integration}; L=$SP/integ-logs; TAG=${1:-r1}
cd $WT || exit 1
npx vitest run > $L/vitest-$TAG.log 2>&1; echo "EXIT=$?" >> $L/vitest-$TAG.log
go test --tags 'json1 fts5' ./... > $L/go-$TAG.log 2>&1; echo "EXIT=$?" >> $L/go-$TAG.log
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... ./application_context/... ./jobs/... -count=1 > $L/go-pg-$TAG.log 2>&1; echo "EXIT=$?" >> $L/go-pg-$TAG.log
