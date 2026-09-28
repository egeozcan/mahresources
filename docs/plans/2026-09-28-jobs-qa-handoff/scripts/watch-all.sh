#!/bin/bash
# Watch every listed lane; exit with a line naming the first lane idle for LIMIT seconds.
# A lane is busy when a process has its cwd inside the lane worktree, or its HEAD, worktree
# status or tmp dir changed since the last sample.
SP=/private/tmp/claude-501/-Users-egecan-Code-mahresources/e6836352-7b72-44a7-9489-ddcee80f38af/scratchpad
LIMIT=$1; shift
ST=$SP/.watch-state; mkdir -p $ST
sig() { { git -C $SP/lanes/$1 rev-parse HEAD; git -C $SP/lanes/$1 status --porcelain | grep -v '^??'; ls -lT $SP/lanes/$1-tmp; } 2>/dev/null | md5; }
busy() { lsof -d cwd -Fn 2>/dev/null | grep -c "^n$SP/lanes/$1\(/\|$\)"; }
for l in "$@"; do sig $l > $ST/$l.sig; echo 0 > $ST/$l.q; done
for i in $(seq 1 1440); do
  sleep 30
  for l in "$@"; do
    now=$(sig $l); b=$(busy $l); q=$(cat $ST/$l.q)
    if [ "$now" = "$(cat $ST/$l.sig)" ] && [ "$b" = "0" ]; then q=$((q+30)); else q=0; echo "$now" > $ST/$l.sig; fi
    echo $q > $ST/$l.q
    if [ $q -ge $LIMIT ]; then hit=1; fi
  done
  if [ -n "$hit" ]; then
    echo "idle check at $(date +%H:%M) (limit $((LIMIT/60)) min):"
    for l in "$@"; do echo "  $l quiet $(( $(cat $ST/$l.q) / 60 )) min, HEAD $(git -C $SP/lanes/$l rev-parse --short HEAD)"; done
    exit 0
  fi
done
echo "watch-all ended"
