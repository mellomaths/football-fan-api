"""PostgreSQL schema for application tables (see api migrations)."""

from psycopg import sql

APP_SCHEMA = sql.Identifier("footballfan")


def table(name: str) -> sql.Composed:
    return sql.SQL("{}.{}").format(APP_SCHEMA, sql.Identifier(name))
