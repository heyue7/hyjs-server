"""Strict private deployment settings and safe, explicit template rendering."""
from pathlib import Path
import re
from urllib.parse import urlsplit

KEYS = (
    'HYJS_DEPLOY_SSH', 'HYJS_DEPLOY_ROOT', 'HYJS_DEPLOY_UPLOADER',
    'HYJS_PUBLIC_HOST', 'HYJS_HTTP_PORT', 'HYJS_ADMIN_PORT', 'HYJS_TCP_PORT',
    'HYJS_UPSTREAM_PORT', 'HYJS_API_ORIGIN', 'HYJS_ADMIN_ORIGIN',
)
RUNTIME_KEYS = tuple(key for key in KEYS if not key.startswith('HYJS_DEPLOY_'))

def load(path):
    values = {}
    for line in Path(path).read_text().splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        key, separator, value = line.partition('=')
        if not separator or key not in KEYS or key in values or not value or any(c.isspace() for c in value):
            raise ValueError('Invalid or duplicate deployment setting: ' + key)
        values[key] = value
    missing = set(KEYS) - values.keys()
    if missing:
        raise ValueError('Missing deployment keys: ' + ', '.join(sorted(missing)))
    if not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_-]*@[A-Za-z0-9.-]+', values['HYJS_DEPLOY_SSH']):
        raise ValueError('Invalid HYJS_DEPLOY_SSH')
    if not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_-]*', values['HYJS_DEPLOY_UPLOADER']):
        raise ValueError('Invalid HYJS_DEPLOY_UPLOADER')
    root = values['HYJS_DEPLOY_ROOT']
    if not re.fullmatch(r'/[A-Za-z0-9_./-]+', root) or '..' in root.split('/') or root in ('/', '/data', '/home', '/tmp', '/var', '/opt') or root.endswith('/'):
        raise ValueError('HYJS_DEPLOY_ROOT must be a dedicated absolute project directory')
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]*', values['HYJS_PUBLIC_HOST']):
        raise ValueError('Invalid HYJS_PUBLIC_HOST')
    ports = []
    for key in ('HYJS_HTTP_PORT', 'HYJS_ADMIN_PORT', 'HYJS_TCP_PORT', 'HYJS_UPSTREAM_PORT'):
        if not values[key].isdigit() or not 1024 <= int(values[key]) <= 65535:
            raise ValueError('Invalid port: ' + key)
        ports.append(int(values[key]))
    if len(set(ports)) != len(ports):
        raise ValueError('Deployment ports must be distinct')
    for origin_key, port_key in (('HYJS_API_ORIGIN', 'HYJS_HTTP_PORT'), ('HYJS_ADMIN_ORIGIN', 'HYJS_ADMIN_PORT')):
        parsed = urlsplit(values[origin_key])
        if parsed.scheme not in ('http', 'https') or parsed.hostname != values['HYJS_PUBLIC_HOST'] or parsed.port != int(values[port_key]) or parsed.path or parsed.query or parsed.fragment or parsed.username or parsed.password:
            raise ValueError('Origin must match public host and port: ' + origin_key)
    return values

def render(source, values):
    def replace(match):
        return values[match.group(1)]
    return re.sub(r'\$\{(HYJS_[A-Z_]+)\}', replace, source)
