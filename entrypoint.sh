#!/bin/bash
set -e

case "$IMGPROXY_MALLOC" in
  malloc) ;;
  jemalloc) export LD_PRELOAD="$LD_PRELOAD:/usr/local/lib/libjemalloc.so" ;;
  tcmalloc) export LD_PRELOAD="$LD_PRELOAD:/usr/local/lib/libtcmalloc_minimal.so" ;;
esac

PLUS_PORT=${PLUS_HTTP_PORT:-8080}
IMGPROXY_PORT=$(echo ${IMGPROXY_BIND:-:8081} | sed 's/^.*://')

export IMGPROXY_BIND=":${IMGPROXY_PORT}"
imgproxy &
IMGPROXY_PID=$!

exec imgproxy_plus
