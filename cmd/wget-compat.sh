#!/bin/busybox sh
set -eu

quiet=0
spider=0
output=""
url=""

while [ $# -gt 0 ]; do
    case "$1" in
        --quiet|-q)
            quiet=1
            ;;
        --spider)
            spider=1
            ;;
        --tries=*|--tries)
            if [ "$1" = "--tries" ] && [ $# -gt 1 ]; then
                shift
            fi
            ;;
        --output-document=*)
            output="${1#*=}"
            ;;
        --output-document|-O)
            if [ $# -gt 1 ]; then
                shift
                output="$1"
            fi
            ;;
        -*)
            ;;
        *)
            url="$1"
            ;;
    esac
    shift
done

if [ -z "$url" ]; then
    echo "WGETCOMPAT-RUN-MISSINGURL" >&2
    exit 1
fi

set --
if [ "$quiet" -eq 1 ]; then
    set -- "$@" -q
fi
if [ "$spider" -eq 1 ]; then
    set -- "$@" --spider
fi
if [ -n "$output" ]; then
    set -- "$@" -O "$output"
fi

exec /bin/busybox wget "$@" "$url"
