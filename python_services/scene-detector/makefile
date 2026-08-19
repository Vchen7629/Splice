test_all: integration unit

unit:
	uv run pytest tests/unit/

integration:
	uv run pytest tests/integration/

coverage:
	uv run pytest --cov=src --cov-report= tests/unit/
	uv run pytest --cov=src --cov-report=xml:coverage.xml --cov-append tests/integration/