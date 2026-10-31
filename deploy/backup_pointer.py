"""Persist the pointer to the last authenticated deployment backup."""

import json
import os
from pathlib import Path
import tempfile


def write_backup_pointer(path, record):
    """Atomically replace a private pointer after syncing its complete contents."""
    path = Path(path)
    content = json.dumps(record, indent=2)
    directory_fd = os.open(path.parent, os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0))
    try:
        fd, temporary = tempfile.mkstemp(prefix='.' + path.name + '.', suffix='.tmp', dir=path.parent)
        try:
            os.fchmod(fd, 0o600)
            with os.fdopen(fd, 'w', encoding='utf-8') as output:
                fd = None
                output.write(content)
                output.flush()
                os.fsync(output.fileno())
            os.replace(temporary, path)
            # A failure here occurs after the atomic switch; propagate it so
            # callers do not mistake an unconfirmed durable update for success.
            os.fsync(directory_fd)
        finally:
            if fd is not None:
                os.close(fd)
            try:
                os.unlink(temporary)
            except FileNotFoundError:
                pass
    finally:
        os.close(directory_fd)
