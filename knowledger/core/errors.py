from enum import StrEnum


class ErrorKind(StrEnum):
    CONFIG = "config_error"
    STORE = "store_error"
    INDEX = "index_error"
    QUERY = "query_error"


class KnowledgerError(Exception):
    def __init__(self, kind: ErrorKind, message: str, cause: Exception | None = None):
        self.kind = kind
        self.message = message
        self.cause = cause

    def __str__(self) -> str:
        if self.cause is None:
            return f"{self.kind}: {self.message}"
        return f"{self.kind}: {self.message}: {self.cause}"
