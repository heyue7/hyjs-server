"""Load the private client API origin without embedding deployment details in source."""
import os
from pathlib import Path
from urllib.parse import urlsplit

ENV_FILE = Path(__file__).with_name(".env")
ENV_KEY = "HYJS_API_ORIGIN"


def validate_origin(value):
    """Accept an HTTP(S) origin only; route suffixes are added by the injector."""
    if not isinstance(value, str) or not value or any(c.isspace() for c in value):
        raise ValueError("API origin must be a non-empty HTTP(S) origin")
    try:
        parsed = urlsplit(value)
        port = parsed.port
        valid = (parsed.scheme in {"http", "https"} and parsed.hostname
                 and parsed.username is None and parsed.password is None
                 and parsed.path in {"", "/"} and not parsed.query and not parsed.fragment
                 and (port is None or 1 <= port <= 65535))
    except ValueError as error:
        raise ValueError("Invalid API origin") from error
    if not valid:
        raise ValueError("API origin must contain only HTTP(S), host and optional port")
    return value.rstrip("/")


def load_origin(explicit=None, env_file=ENV_FILE):
    """Priority: explicit CLI argument, process environment, then private .env file."""
    value = explicit if explicit is not None else os.environ.get(ENV_KEY)
    if value is None and Path(env_file).is_file():
        for line in Path(env_file).read_text().splitlines():
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            key, separator, candidate = line.partition("=")
            if separator and key.strip() == ENV_KEY:
                value = candidate.strip()
                if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
                    value = value[1:-1]
    if value is None:
        raise ValueError("Set HYJS_API_ORIGIN in tools/client_remote/.env, the environment or --origin")
    return validate_origin(value)
