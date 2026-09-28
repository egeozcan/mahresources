#!/bin/bash
SP=/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad
WT=$SP/lanes/${2:-b1-integration}; L=$SP/integ-logs; TAG=${1:-r1}
cd $WT/e2e || exit 1
npm run test:with-server:all > $L/e2e-$TAG.log 2>&1; echo "EXIT=$?" >> $L/e2e-$TAG.log
cp test-results/.last-run.json $L/e2e-$TAG.last-run.json 2>/dev/null
npm run test:with-server:postgres > $L/e2e-pg-$TAG.log 2>&1; echo "EXIT=$?" >> $L/e2e-pg-$TAG.log
cp test-results/.last-run.json $L/e2e-pg-$TAG.last-run.json 2>/dev/null
