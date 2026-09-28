#!/bin/bash
# Try a tiny pi request every 15 minutes until the Codex model answers; exit 0 when it does.
cd /private/tmp || exit 1
for i in $(seq 1 64); do
  out=$(pi -p --no-session -xt edit,write,bash --model openai-codex/gpt-6-sol:high "Reply with exactly the word READY." < /dev/null 2>&1)
  if echo "$out" | grep -q 'READY'; then echo "pi available at $(date +%H:%M) (attempt $i)"; exit 0; fi
  echo "$(date +%H:%M) attempt $i: $(echo "$out" | grep -iE 'limit|error' | head -1)"
  sleep 900
done
echo "gave up after 16 h"
