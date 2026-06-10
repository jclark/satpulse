PY_FILES = __main__.py runner.py compare.py cases.py

.PHONY: dev-deps update-deps typecheck

dev-deps:
	uv sync --extra dev --frozen

update-deps:
	uv sync --extra dev --upgrade

typecheck:
	uv run --extra dev mypy $(PY_FILES)
