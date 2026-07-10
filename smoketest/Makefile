PY_FILES = run.py system.py ntpsock.py ntpshm.py common.py \
	program_api.py program_satpulsed.py program_satpulsewb.py scenarios

.PHONY: dev-deps update-deps typecheck

dev-deps:
	uv sync --extra dev --frozen

update-deps:
	uv sync --extra dev --upgrade

typecheck:
	uv run --extra dev mypy $(PY_FILES)
