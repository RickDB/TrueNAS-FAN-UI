#!/bin/sh
set -eu

if [ "${WAIT_FOR_IT87:-0}" = "1" ]; then
    timeout="${IT87_WAIT_TIMEOUT:-90}"
    elapsed=0
    printf '[fan-ui] waiting up to %ss for host it87 module\n' "$timeout"

    while ! grep -q '^it87 ' /proc/modules 2>/dev/null; do
        if [ "$elapsed" -ge "$timeout" ]; then
            printf '[fan-ui] WARNING: it87 was not loaded within %ss; starting anyway\n' "$timeout"
            break
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done

    if grep -q '^it87 ' /proc/modules 2>/dev/null; then
        printf '[fan-ui] it87 module detected; starting web service\n'
    fi
fi

exec /usr/local/bin/truenas-fan-ui
