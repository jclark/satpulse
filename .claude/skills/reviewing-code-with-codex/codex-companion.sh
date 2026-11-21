#!/bin/sh
# Run the companion script of the newest installed version of the Codex
# plugin for Claude Code (openai/codex-plugin-cc) with the given arguments.
s=$(ls -d "$HOME"/.claude/plugins/cache/openai-codex/codex/*/scripts/codex-companion.mjs 2>/dev/null | sort -V | tail -1)
if [ -z "$s" ]; then
	echo "the Codex plugin is not installed: run /plugin install codex@openai-codex, then /codex:setup" >&2
	exit 1
fi
exec node "$s" "$@"
