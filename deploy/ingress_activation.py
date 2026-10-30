"""Scoped ingress preflight and guarded replacement for an existing deployment.

The caller holds the deployment lock. Other administrators must also coordinate
their writes: POSIX rename does not provide an inode/content compare-and-swap.
"""

from dataclasses import dataclass
import hashlib
import json
import os
from pathlib import Path
import stat
import sys
import tempfile


class IngressError(RuntimeError):
    pass


@dataclass(frozen=True)
class FileState:
    data: bytes
    device: int
    inode: int
    mode: int
    uid: int
    gid: int
    mtime_ns: int
    ctime_ns: int
    xattrs: tuple


def _stat_key(value):
    return (value.st_dev, value.st_ino, value.st_mode, value.st_uid, value.st_gid,
            value.st_size, value.st_mtime_ns, value.st_ctime_ns)


def _file_state(path):
    flags = os.O_RDONLY | getattr(os, 'O_NOFOLLOW', 0) | getattr(os, 'O_CLOEXEC', 0)
    fd = os.open(path, flags)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode):
            raise IngressError('Ingress destination is not a regular file')
        chunks = []
        while True:
            chunk = os.read(fd, 65536)
            if not chunk:
                break
            chunks.append(chunk)
        if all(hasattr(os, name) for name in ('listxattr', 'getxattr', 'setxattr')):
            attrs = tuple((name, os.getxattr(fd, name)) for name in sorted(os.listxattr(fd)))
        elif sys.platform.startswith('linux'):
            raise IngressError('Cannot preserve Linux ingress extended attributes')
        else:
            # Local tests may run on a non-Linux host without Python xattr APIs.
            attrs = ()
        after = os.fstat(fd)
        current = os.stat(path, follow_symlinks=False)
        if _stat_key(before) != _stat_key(after) or _stat_key(after) != _stat_key(current):
            raise IngressError('Ingress destination changed while being inspected')
        return FileState(b''.join(chunks), after.st_dev, after.st_ino,
                         stat.S_IMODE(after.st_mode), after.st_uid, after.st_gid,
                         after.st_mtime_ns, after.st_ctime_ns, attrs)
    finally:
        os.close(fd)


def _link_state(enabled, destination):
    value = os.lstat(enabled)
    if not stat.S_ISLNK(value.st_mode):
        raise IngressError('Existing ingress enabled path must be a symlink')
    target = os.readlink(enabled)
    if enabled.resolve(strict=True) != destination.resolve(strict=True):
        raise IngressError('Existing ingress link points to a different destination')
    return _stat_key(value), target


def _require_unchanged(destination, enabled, expected_file, expected_link):
    if _link_state(enabled, destination) != expected_link or _file_state(destination) != expected_file:
        raise IngressError('Ingress destination or enabled link changed externally; refusing replacement')


def _nginx_quote(path):
    return '"' + str(path).replace('\\', '\\\\').replace('"', '\\"').replace('$', '\\$') + '"'


def preflight(candidate, expected_sha256, run):
    """Check candidate syntax separately; the live full configuration is checked later."""
    candidate = Path(candidate).read_bytes()
    if hashlib.sha256(candidate).hexdigest() != expected_sha256:
        raise IngressError('Ingress candidate differs from the verified artifact')
    with tempfile.TemporaryDirectory(prefix='hyjs-nginx-preflight-') as directory:
        isolated_candidate = Path(directory) / 'site.conf'
        isolated_candidate.write_bytes(candidate)
        isolated_candidate.chmod(0o600)
        config = Path(directory) / 'nginx.conf'
        config.write_text('error_log stderr;\npid ' + _nginx_quote(Path(directory) / 'nginx.pid') +
                          ';\nevents {}\nhttp { access_log off; include ' + _nginx_quote(isolated_candidate) + '; }\n')
        run('nginx', '-t', '-c', str(config))
    return candidate


def _write_recovery(stage, destination, enabled, original):
    recovery = Path(tempfile.mkdtemp(prefix='.ingress-recovery-', dir=stage))
    os.chmod(recovery, 0o700)
    try:
        fd = os.open(recovery / 'original.conf', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'wb') as output:
            output.write(original.data)
            output.flush()
            os.fsync(output.fileno())
        metadata = {'destination': str(destination), 'enabled': str(enabled),
                    'sha256': hashlib.sha256(original.data).hexdigest(),
                    'mode': original.mode, 'uid': original.uid, 'gid': original.gid,
                    'xattrs': {name: value.hex() for name, value in original.xattrs}}
        fd = os.open(recovery / 'metadata.json', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as output:
            json.dump(metadata, output, sort_keys=True)
            output.flush()
            os.fsync(output.fileno())
    except BaseException:
        # Keep any partial private evidence for manual inspection.
        raise IngressError('Could not record ingress recovery evidence at ' + str(recovery)) from None
    return recovery


def _discard_recovery(recovery):
    (recovery / 'original.conf').unlink()
    (recovery / 'metadata.json').unlink()
    recovery.rmdir()


def _atomic_replace(destination, enabled, data, attributes, expected_file, expected_link):
    fd, name = tempfile.mkstemp(prefix='.' + destination.name + '.hyjs-', dir=destination.parent)
    temporary = Path(name)
    try:
        current = os.fstat(fd)
        if (current.st_uid, current.st_gid) != (attributes.uid, attributes.gid):
            os.fchown(fd, attributes.uid, attributes.gid)
        os.fchmod(fd, attributes.mode)
        if hasattr(os, 'listxattr'):
            for key in os.listxattr(fd):
                if key not in dict(attributes.xattrs):
                    os.removexattr(fd, key)
        for key, value in attributes.xattrs:
            os.setxattr(fd, key, value)
        with os.fdopen(fd, 'wb') as output:
            fd = -1
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
        created = _file_state(temporary)
        if (created.data != data or (created.mode, created.uid, created.gid, created.xattrs) !=
                (attributes.mode, attributes.uid, attributes.gid, attributes.xattrs)):
            raise IngressError('Prepared ingress attributes differ from the original')
        _require_unchanged(destination, enabled, expected_file, expected_link)
        os.replace(temporary, destination)
        return created.device, created.inode
    finally:
        if fd >= 0:
            os.close(fd)
        if temporary.exists():
            temporary.unlink()


def update_existing(destination, enabled, desired, stage, run):
    """Update an existing site and retain a private recovery snapshot on failure.

    File and symlink identity, bytes and attributes are checked immediately
    before each rename. This narrows, but cannot eliminate, races with writers
    that ignore the caller's deployment lock.
    """
    destination, enabled, stage = Path(destination), Path(enabled), Path(stage)
    original = _file_state(destination)
    original_link = _link_state(enabled, destination)
    _require_unchanged(destination, enabled, original, original_link)
    if original.data == desired:
        run('nginx', '-t')
        _require_unchanged(destination, enabled, original, original_link)
        return

    recovery = _write_recovery(stage, destination, enabled, original)
    published = None
    reload_attempted = False
    try:
        identity = _atomic_replace(destination, enabled, desired, original, original, original_link)
        current = _file_state(destination)
        if ((current.device, current.inode) != identity or current.data != desired or
                (current.mode, current.uid, current.gid, current.xattrs) !=
                (original.mode, original.uid, original.gid, original.xattrs)):
            raise IngressError('Ingress changed during installation')
        published = current
        _require_unchanged(destination, enabled, published, original_link)
        run('nginx', '-t')
        _require_unchanged(destination, enabled, published, original_link)
        reload_attempted = True
        run('systemctl', 'reload', 'nginx')
        _require_unchanged(destination, enabled, published, original_link)
    except BaseException as error:
        if published is None:
            raise IngressError('Ingress update did not complete; preserve and inspect ' + str(recovery) +
                               '; no automatic recovery when installed identity is uncertain') from error
        try:
            _require_unchanged(destination, enabled, published, original_link)
            restored_id = _atomic_replace(destination, enabled, original.data, original, published, original_link)
            restored = _file_state(destination)
            if ((restored.device, restored.inode) != restored_id or restored.data != original.data or
                    (restored.mode, restored.uid, restored.gid, restored.xattrs) !=
                    (original.mode, original.uid, original.gid, original.xattrs)):
                raise IngressError('Restored ingress failed its identity or attribute check')
            _require_unchanged(destination, enabled, restored, original_link)
            run('nginx', '-t')
            _require_unchanged(destination, enabled, restored, original_link)
            if reload_attempted:
                run('systemctl', 'reload', 'nginx')
                _require_unchanged(destination, enabled, restored, original_link)
        except BaseException as restore_error:
            raise IngressError('Ingress update failed and recovery was unsafe or failed; preserve and inspect ' +
                               str(recovery) + '; recovery detail: ' + str(restore_error)) from error
        raise IngressError('Ingress update failed; original restored and checked; preserve evidence at ' +
                           str(recovery)) from error
    _discard_recovery(recovery)
