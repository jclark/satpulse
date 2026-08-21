#!/bin/sh
# Deprecated: use make instead. This shim remains until nothing refers to it.
exec make -f Makefile.unix "$@"
