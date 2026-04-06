FROM python:3.12-slim-bookworm

RUN useradd --create-home --uid 10001 appuser

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

WORKDIR /app
RUN chown appuser:appuser /app

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV PATH="/app/.venv/bin:$PATH"
ENV UV_COMPILE_BYTECODE=1
ENV UV_LINK_MODE=copy

COPY --chown=appuser:appuser pyproject.toml uv.lock ./
COPY --chown=appuser:appuser src ./src

USER appuser
RUN uv sync --frozen --no-dev

HEALTHCHECK --interval=60s --timeout=5s --start-period=15s --retries=3 \
  CMD python -c "import football_scrapers" || exit 1

CMD ["python", "-m", "football_scrapers"]
