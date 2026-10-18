"""Restricted, project-owned temporary MySQL allocations.

Read the local-services README before any real use. Importing this module and
constructing a pool perform no service, file-credential, or database operation.
"""
from dataclasses import dataclass, field
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys

SERVICES = (Path.home() / '.codex/local-services')
MANAGER = SERVICES / 'manage.py'


@dataclass(frozen=True)
class ManagedConnection:
    database: str
    user: str
    password: str = field(repr=False)
    host: str = '127.0.0.1:3306'

    def env(self):
        """Return this allocation's application and old P13 fixture mapping."""
        values = {'HOST': self.host, 'USER': self.user, 'PASSWORD': self.password, 'DATABASE': self.database}
        return {prefix + key: value for prefix in ['HYJS_MYSQL_', 'HYJS_P13_MYSQL_'] for key, value in values.items()}


def _sql_tokens(text):
    """Mask strings/comments so administrative words in JSON stay ordinary data."""
    pattern = r"'(?:''|\\.|[^'\\])*'|\"(?:\"\"|\\.|[^\"\\])*\"|/\*[\s\S]*?\*/|--[^\n]*|#[^\n]*"
    return re.sub(pattern, lambda match: ' ' * len(match.group()), text).replace('`', '')


class ManagedMySQL:
    """Allocate through the single manager and never use root in test code."""
    def __init__(self, runner=None):
        self._runner = runner or subprocess.run
        self.connections = {}

    def _command(self, action, database, text=None, user=None):
        if action not in {'test-create', 'test-drop'}:
            raise ValueError('Only restricted test allocation actions are supported')
        if not re.fullmatch(r'hyjs_test_[a-z0-9_]{1,53}', database):
            raise ValueError('Temporary schema must use the hyjs_test_ project prefix')
        command = ['rtk', 'proxy', sys.executable, str(MANAGER), action, 'hyjs', database]
        if action == 'test-create':
            command += ['--collation', 'utf8mb4_bin']
            if user:
                if not any(connection is not None and connection.user == user for connection in self.connections.values()):
                    raise ValueError('A shared test user must already belong to this allocation pool')
                command += ['--user', user]
        result = self._runner(command, input=text, text=True, capture_output=True, timeout=90)
        if result.returncode:
            # SQL/manager diagnostics may contain supplied data. Do not copy
            # credential-bearing environment or raw output into test failures.
            raise RuntimeError('Restricted MySQL ' + action + ' failed for ' + database + ' (exit ' + str(result.returncode) + ')')
        return result.stdout

    def allocate(self, label):
        if not re.fullmatch(r'[a-z][a-z0-9_]{0,24}', label):
            raise ValueError('Use a short, non-secret lower-case test label')
        database = 'hyjs_test_' + label + '_' + secrets.token_hex(5)
        shared = next((connection for connection in self.connections.values() if connection is not None), None)
        self._command('test-create', database, user=shared.user if shared else None)
        # The manager has completed registration before returning. Register
        # ownership before parsing its private connection file so a parse
        # failure still releases this exact allocation.
        self.connections[database] = None
        try:
            values = {}
            path = SERVICES / 'test-connections' / (database + '.env')
            for line in path.read_text().splitlines():
                key, separator, value = line.partition('=')
                if separator and key:
                    if key in values:
                        raise ValueError('Duplicate private connection field')
                    values[key] = value
            if values.get('MYSQL_DATABASE') != database or values.get('MYSQL_HOST') != '127.0.0.1' or values.get('MYSQL_PORT') != '3306' or not re.fullmatch(r'hyjs_t_[a-z0-9_]+', values.get('MYSQL_USER', '')) or not values.get('MYSQL_PASSWORD'):
                raise ValueError('Managed connection does not match the shared project allocation')
            connection = ManagedConnection(database=database, user=values['MYSQL_USER'], password=values['MYSQL_PASSWORD'])
            if shared and (connection.user != shared.user or connection.password != shared.password):
                raise ValueError('Manager did not preserve this pool’s registered multi-schema account')
            self.connections[database] = connection
            return connection
        except Exception:
            self.release(database)
            raise

    def release(self, database):
        if database not in self.connections:
            raise ValueError('Refusing to release a schema not owned by this test pool')
        self._command('test-drop', database)
        del self.connections[database]

    def close(self):
        errors = []
        for database in list(self.connections)[::-1]:
            try:
                self.release(database)
            except Exception as error:
                errors.append(error)
        if errors:
            raise RuntimeError('Restricted test cleanup failed for ' + str(len(errors)) + ' owned allocation(s)') from errors[0]

    def sql(self, database, text):
        if database not in self.connections or self.connections[database] is None:
            raise ValueError('SQL requires a successfully allocated test schema')
        if '/*!' in text:
            raise ValueError('Executable MySQL comments are forbidden in test SQL')
        tokens = _sql_tokens(text)
        forbidden = r'\b(?:USE|GRANT|REVOKE|FLUSH|PURGE|SHUTDOWN|KILL|CALL|PREPARE|EXECUTE|DELIMITER|SQL_LOG_BIN)\b|\b(?:CREATE|ALTER|DROP)\s+(?:USER|DATABASE|SCHEMA)\b|\bSET\s+(?:@@\s*)?(?:GLOBAL|PERSIST)\b|\b(?:LOAD\s+DATA|INTO\s+(?:OUTFILE|DUMPFILE)|RESET\s+MASTER)\b'
        if re.search(forbidden, tokens, re.I):
            raise ValueError('Administrative and cross-schema SQL is forbidden in a managed test')
        for match in re.finditer(r'\b(FROM|JOIN|UPDATE|INTO|TABLE(?:\s+IF\s+(?:NOT\s+)?EXISTS)?|REFERENCES|TO)\s+([a-z][a-z0-9_]*)\s*\.', tokens, re.I):
            keyword, target = match.group(1).upper(), match.group(2)
            if target == 'information_schema' and keyword in {'FROM', 'JOIN'}:
                continue
            if target != database:
                raise ValueError('SQL cannot access another schema')
        if re.search(r'\b(?:hyjs|mysql|sys|performance_schema)\s*\.', tokens, re.I):
            raise ValueError('Development and system schemas are outside this test allocation')
        connection = self.connections[database]
        command = ['rtk', 'proxy', 'docker', 'exec', '-i', 'codex-local-mysql', 'sh', '-c', 'read -r MYSQL_PWD; export MYSQL_PWD; exec mysql --protocol=TCP -h127.0.0.1 --user="$1" --database="$2" --default-character-set=utf8mb4 --batch --raw --skip-column-names --binary-mode', 'hyjs-managed-test', connection.user, connection.database]
        # The application and fixture SQL share the same temporary account.
        # The secret travels only on stdin and never enters process arguments.
        result = self._runner(command, input=connection.password + '\n' + text, text=True, capture_output=True, timeout=90)
        if result.returncode:
            raise RuntimeError('Temporary-account SQL failed for ' + database + ' (exit ' + str(result.returncode) + ')')
        return result.stdout

    def redact(self, text):
        for connection in self.connections.values():
            if connection is not None:
                text = text.replace(connection.password, '[redacted]')
        return text


def provision_case(case, label):
    """Keep the original single-schema fixture attributes without old privileges."""
    pool = ManagedMySQL()
    case.managed_mysql = pool
    case.addClassCleanup(pool.close)
    connection = pool.allocate(label)
    case.database = connection.database
    case.test_user = connection.user
    case.test_password = connection.password
    case.env = dict(os.environ, **connection.env())
    return pool, connection


def case_sql(case, text):
    """Compatibility for old schema-local SQL; old user/DB DDL stays rejected."""
    database = case.database
    use = re.match(r'\s*USE\s+`?(hyjs_test_[a-z0-9_]+)`?\s*;\s*', text, re.I)
    if use:
        database, text = use.group(1), text[use.end():]
    return case.managed_mysql.sql(database, text)
