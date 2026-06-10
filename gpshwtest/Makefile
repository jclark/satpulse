PY_FILES = $(wildcard *.py)

.PHONY: dev-deps update-deps typecheck

dev-deps:
	uv sync --extra dev --frozen

update-deps:
	uv sync --extra dev --upgrade

typecheck:
	test -z "$(PY_FILES)" || uv run --extra dev mypy $(PY_FILES)
