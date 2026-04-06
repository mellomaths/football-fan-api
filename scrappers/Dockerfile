FROM python:3.12-slim-bookworm

COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV PATH="/app/.venv/bin:$PATH"
ENV UV_COMPILE_BYTECODE=1
ENV UV_LINK_MODE=copy

COPY pyproject.toml uv.lock ./
COPY src ./src

RUN uv sync --frozen --no-dev

CMD ["python", "-m", "football_scrapers"]
