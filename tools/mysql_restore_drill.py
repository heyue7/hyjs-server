#!/usr/bin/env python3
"""Only owned test engines/databases: original protocol -> backup -> restore -> cold engine recovery.

Credentials stay in memory and child environments. Dumps are private and always removed.
This tool has no argument accepting an existing database, container, credentials or server.
"""
import argparse
import concurrent.futures
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import socket
import struct
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from server.codec import BY_NAME, CLASSES, decode_responses, encode_http_request


def require(condition, label):
    if not condition:
        raise RuntimeError(label)


class RestoreDrill:
    def __init__(self):
        raise RuntimeError('Legacy engine-crash drill disabled: local tests must use managed hyjs_test_ databases and cannot create or stop MySQL instances')
        self.token = secrets.token_hex(6)
        self.database = 'hyjs_test_restore_' + self.token
        self.user = 'restore_' + self.token
        self.password = secrets.token_hex(24)
        self.account_password = secrets.token_hex(24)
        self.owned = []
        self.process = None
        self.sockets = []
        self.temp = tempfile.TemporaryDirectory(prefix='hyjs-private-restore-')
        os.chmod(self.temp.name, 0o700)
        self.binary = Path(self.temp.name) / 'hyjs-drill'
        self.backup = Path(self.temp.name) / 'private-backup.sql'
        self.report = {'scope': 'isolated test engines only', 'checks': {}, 'unfinished': [
            'production restore approval and execution', 'old schema upgrade/failure rollback',
            'dispatch lifecycle and all period settlement boundaries', 'whole P1-E.1 and P15-E.4']}

    def run_command(self, args, **kwargs):
        result = subprocess.run(['rtk', 'proxy'] + args, capture_output=True, timeout=kwargs.pop('timeout', 60), **kwargs)
        if result.returncode:
            # Never include command/stdout: SQL and protocol values can include session material.
            raise RuntimeError('isolated command failed: ' + args[0])
        return result.stdout

    def guard(self, container):
        require(container in self.owned and re.fullmatch(r'hyjs-test-restore-[ab]-[0-9a-f]{12}', container), 'owned engine guard')
        label = self.run_command(['docker', 'inspect', '--format', '{{ index .Config.Labels "hyjs.restore-drill" }}', container]).decode().strip()
        require(label == self.token, 'engine ownership label guard')

    def sql(self, container, text, database=None, root=False):
        self.guard(container)
        require(database is None or database == self.database, 'test schema guard')
        args = ['docker', 'exec', '-i']
        env = dict(os.environ)
        if root:
            env.pop('MYSQL_PWD', None)
        else:
            args += ['-e', 'MYSQL_PWD']
            env['MYSQL_PWD'] = self.password
        args += [container, 'mysql', '-uroot' if root else '-u' + self.user, '--batch', '--raw', '--skip-column-names']
        if database:
            args.append(database)
        return self.run_command(args, input=text.encode(), env=env).decode()

    def engine(self, suffix):
        name = 'hyjs-test-restore-' + suffix + '-' + self.token
        # Root has no remote grant; the ephemeral application account gets only this test schema.
        self.run_command(['docker', 'run', '-d', '--pull=never', '--name', name,
                          '--label', 'hyjs.restore-drill=' + self.token,
                          '-e', 'MYSQL_ALLOW_EMPTY_PASSWORD=1', '-e', 'MYSQL_ROOT_HOST=localhost',
                          '-p', '127.0.0.1::3306', 'mysql:8.4'])
        self.owned.append(name)
        self.wait_engine(name)
        self.sql(name, 'CREATE DATABASE `' + self.database + '` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin; '
                 "CREATE USER '" + self.user + "'@'%' IDENTIFIED BY '" + self.password + "'; "
                 'GRANT ALL ON `' + self.database + "`.* TO '" + self.user + "'@'%';", root=True)
        require(self.sql(name, "SELECT COUNT(*) FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost','127.0.0.1','::1');", root=True).strip() == '0', 'root remote grant must be absent')
        self.report['checks']['no_remote_root_' + suffix] = True
        self.report['checks']['restricted_user_' + suffix] = self.restricted_user(name)
        return name

    def wait_engine(self, name):
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            self.guard(name)
            result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', name, 'mysqladmin', '--protocol=TCP', '-h127.0.0.1', '-uroot', 'ping'], capture_output=True, timeout=10)
            if result.returncode == 0:
                return
            time.sleep(1)
        raise RuntimeError('owned engine readiness timeout')

    def restricted_user(self, container):
        self.guard(container)
        result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', '-e', 'MYSQL_PWD', container,
                                 'mysql', '-u' + self.user, '--batch'], input=b'SELECT COUNT(*) FROM mysql.user;',
                                env=dict(os.environ, MYSQL_PWD=self.password), capture_output=True, timeout=20)
        require(result.returncode != 0, 'test account unexpectedly has system grants')
        return True

    def start_service(self, container):
        self.guard(container)
        port = self.run_command(['docker', 'port', container, '3306/tcp']).decode().strip()
        require(re.fullmatch(r'127\.0\.0\.1:[0-9]+', port), 'loopback-only owned port')
        env = dict(os.environ, HYJS_MYSQL_HOST=port, HYJS_MYSQL_DATABASE=self.database,
                   HYJS_MYSQL_USER=self.user, HYJS_MYSQL_PASSWORD=self.password)
        self.process = subprocess.Popen(['rtk', 'proxy', str(self.binary), '--http-port', '0', '--tcp-port', '0',
                                         '--tables', str(ROOT / 'analysis/tables')], cwd=ROOT / 'hyjs-server', env=env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            require(ready.select(90), 'isolated game readiness timeout')
            line = self.process.stdout.readline()
        found = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        require(found is not None, 'isolated game startup failed')
        self.http_port, self.tcp_port = int(found[1]), int(found[2])

    def stop_service(self):
        for sock in self.sockets:
            sock.close()
        self.sockets = []
        if self.process:
            if self.process.poll() is None:
                self.process.terminate()
                try:
                    self.process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    self.process.kill()
                    self.process.wait()
                    raise RuntimeError('isolated game context shutdown timeout')
            self.process.stdout.close()
            self.process = None

    def call(self, name, fields=None, login=None, stamp=1, expected=None):
        args = dict(player_id=login.playerId, session=bytes(login.sessionKey)) if login else {}
        raw = encode_http_request(BY_NAME[name], CLASSES[name](**(fields or {})), zone_id=1, timestamp=stamp, **args)
        connection = http.client.HTTPConnection('127.0.0.1', self.http_port, timeout=8)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            require(response.status == 200, 'original HTTP transport status')
            frames = decode_responses(response.read())
            require(frames and frames[0][0] == BY_NAME[expected or name.replace('CS', 'SC', 1)], 'original protocol response: ' + name)
            return frames
        finally:
            connection.close()

    def login(self, uid):
        return self.call('CSUserLogin', {'platformUid': uid, 'authCode': self.account_password}, expected='SCUserLogin')[0][1]

    def tcp_send(self, sock, name, fields, login):
        payload = CLASSES[name](**fields).SerializeToString()
        sock.sendall(struct.pack('>iiiiqi', 24 + len(payload), 1, BY_NAME[name], 1, login.playerId, len(payload)) + payload)

    def tcp_recv(self, sock):
        def exact(n):
            raw = b''
            while len(raw) < n:
                chunk = sock.recv(n - len(raw))
                require(chunk, 'original TCP unexpectedly closed')
                raw += chunk
            return raw
        header = exact(8)
        size, _ = struct.unpack('>ii', header)
        require(0 <= size <= 8_000_000, 'original TCP frame bound')
        return decode_responses(header + exact(size))[0]

    def connect(self, login):
        sock = socket.create_connection(('127.0.0.1', self.tcp_port), timeout=5)
        self.sockets.append(sock)
        self.tcp_send(sock, 'CSSocketLogin', {'sessionKey': bytes(login.sessionKey)}, login)
        op, response = self.tcp_recv(sock)
        require(op == BY_NAME['SCSocketLogin'] and response.result == 100, 'original TCP session login')
        return sock

    def profile(self, container, login):
        text = self.sql(container, 'SELECT player,game FROM profiles WHERE id=' + str(login.playerId) + ';', self.database)
        player, game = text.strip().split('\t')
        return json.loads(player), json.loads(game)

    def prepare(self, source):
        self.start_service(source)
        self.uids = ['restore-' + self.token + '-' + str(i) for i in range(3)]
        players = [self.login(uid) for uid in self.uids]
        a, b, c = players
        for i, login in enumerate(players):
            self.call('CSPlayerCreate', {'playerName': '恢复真实角色' + str(i), 'heroId': 'hero_047'}, login)
        ids = ','.join(str(p.playerId) for p in players)
        # Finite test-only level preparation; all resource grants and later charges use original protocols.
        self.sql(source, 'UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.stamina",200,"$.noviceGuideSchedule",7) WHERE id IN (' + ids + '); '
                 'UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1; '
                 'UPDATE p11_grab_policies SET enabled=1,time_reward_enabled=0; '
                 'UPDATE p10_scene_policies SET enabled=0; UPDATE p12_world_boss_policies SET enabled=1,level=1,position_id=1;', self.database)
        self.call('CSPlayerLogin', {}, a, stamp=2)
        letters = self.call('CSMailList', {'mailType': 'sysMail'}, a, stamp=3)[0][1].mailBoxList[0].mailList
        letter = letters[0]
        rewards = self.call('CSQueryUserReward', {}, a, stamp=4)[0][1].rewardItemInfoList
        self.call('CSAddRewardsToPlayer', {'rewardIds': [r.rewardId for r in rewards]}, a, stamp=5)
        self.call('CSDeleteMail', {'type': letter.type, 'createTime': letter.time}, a, stamp=6)
        for p in players:
            self.sql(source, 'INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES(' + str(p.playerId) + ',"restore-finite-' + self.token + '","restore finite source","isolated fixture","coin,2000000;diamond,1000",1,0,0);', self.database)
            inbox = self.call('CSQueryUserReward', {}, p, stamp=7)[0][1].rewardItemInfoList
            reward = next(r for r in inbox if r.rewardTitle == 'restore finite source')
            self.call('CSAddRewardsToPlayer', {'rewardIds': [reward.rewardId]}, p, stamp=8)
        self.sql(source, 'INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES(' + str(a.playerId) + ',"restore-pending-' + self.token + '","restore pending source","isolated fixture","coin,77",1,0,0);', self.database)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.call('CSUnionCreate', {'name': 'RestoreGuild'}, a, stamp=101)[0][1], range(6)))
        require(all(r == replies[0] for r in replies), 'original create concurrent receipt')
        self.create_reply = replies[0]
        self.guild = self.call('CSGetUnionInfo', {}, a, stamp=102)[0][1].unionInfo.unionId
        self.call('CSUnionApply', {'unionId': self.guild}, b, stamp=103)
        self.call('CSUnionAgreeApply', {'playerId': b.playerId}, a, stamp=104)
        self.donation_reply = self.call('CSUnionDonate', {'donateType': 'coin'}, a, stamp=105)[0][1]
        self.stamina_reply = self.call('CSGetUnionStamina', {}, a, stamp=106)[0][1]
        for login in [a, b]:
            self.connect(login)
            self.call('CSTransferScene', {'target': 'town_union'}, login, stamp=107)
        self.grab_reply = self.call('CSGrab', {'index': 1}, a, stamp=108)[0][1]
        self.purchase_reply = self.call('CSBuyGrab', {'count': 1}, a, stamp=109)[0][1]
        self.call('CSGrab', {'index': 7}, b, stamp=110)
        self.release_reply = self.call('CSGiveUpGrab', {'index': 7}, b, stamp=111)[0][1]
        self.boss_reply = self.call('CSWorldBossInfo', {}, a, stamp=112)[0][1]
        self.sql(source, 'UPDATE p12_world_boss_policies SET level=2,position_id=2;', self.database)
        entry = self.call('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, c, stamp=113)[0][1]
        self.copy_task = entry.jobList[0].taskId
        self.copy_start = self.call('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': entry.jobList[0].taskId}, c, stamp=114)[0][1]
        self.saved_profiles = {p.playerId: self.profile(source, p) for p in players}
        self.ids = [p.playerId for p in players]
        self.stop_service()
        self.sql(source, 'CREATE TABLE restore_drill_probes(id INT PRIMARY KEY,phase VARCHAR(32)) ENGINE=InnoDB; INSERT INTO restore_drill_probes VALUES(1,"committed");', self.database)
        self.report['checks']['original_protocol_fixture'] = True
        self.report['checks']['concurrent_create_single_receipt'] = self.sql(source, 'SELECT COUNT(*) FROM p13_guild_receipts WHERE player_id=' + str(a.playerId) + ' AND stamp=101;', self.database).strip() == '1'
        self.report['checks']['captured_unfinished_battle'] = self.sql(source, 'SELECT COUNT(*) FROM p6_battle_records WHERE player_id=' + str(c.playerId) + ' AND status="captured";', self.database).strip() == '1'
        require(all(self.report['checks'].values()), 'fixture frozen facts')

    def dump_args(self, container):
        self.guard(container)
        return ['rtk', 'proxy', 'docker', 'exec', '-i', '-e', 'MYSQL_PWD', container, 'mysqldump', '-u' + self.user,
                '--single-transaction', '--no-tablespaces', '--set-gtid-purged=OFF', '--hex-blob', '--skip-comments', '--skip-lock-tables']

    def data_snapshot(self, container):
        args = self.dump_args(container) + ['--no-create-info', '--complete-insert', '--skip-extended-insert', '--order-by-primary',
                                          '--compact', '--skip-add-locks', '--skip-disable-keys', self.database]
        result = subprocess.run(args, env=dict(os.environ, MYSQL_PWD=self.password), capture_output=True, timeout=120)
        require(result.returncode == 0, 'test data snapshot command')
        rows = self.sql(container, 'SELECT table_name FROM information_schema.tables WHERE table_schema="' + self.database + '" ORDER BY table_name;', self.database).splitlines()
        require(rows and all(re.fullmatch(r'[a-zA-Z0-9_]+', table) for table in rows), 'owned table identifier guard')
        query = ' UNION ALL '.join("SELECT '" + table + "',COUNT(*) FROM `" + table + '`' for table in rows)
        # One query reads all counts; repeated Docker connection setup must not dominate the drill.
        counts = {row.split('\t')[0]: int(row.split('\t')[1]) for row in self.sql(container, query + ';', self.database).splitlines()}
        # SHOW CREATE may render a redundant explicit CHARACTER SET after restore. Compare effective
        # schema semantics from metadata, retaining defaults, collations, indexes, constraints and next IDs.
        definitions = []
        for table, fields, order in [
            ('TABLES', 'TABLE_NAME,ENGINE,TABLE_COLLATION,CREATE_OPTIONS,AUTO_INCREMENT', 'TABLE_NAME'),
            ('COLUMNS', 'TABLE_NAME,COLUMN_NAME,ORDINAL_POSITION,COLUMN_DEFAULT,IS_NULLABLE,COLUMN_TYPE,CHARACTER_SET_NAME,COLLATION_NAME,EXTRA,COLUMN_KEY,GENERATION_EXPRESSION', 'TABLE_NAME,ORDINAL_POSITION'),
            ('STATISTICS', 'TABLE_NAME,INDEX_NAME,NON_UNIQUE,SEQ_IN_INDEX,COLUMN_NAME,COLLATION,SUB_PART,INDEX_TYPE,IS_VISIBLE,EXPRESSION', 'TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX'),
            ('TABLE_CONSTRAINTS', 'TABLE_NAME,CONSTRAINT_NAME,CONSTRAINT_TYPE,ENFORCED', 'TABLE_NAME,CONSTRAINT_NAME'),
            ('KEY_COLUMN_USAGE', 'TABLE_NAME,CONSTRAINT_NAME,COLUMN_NAME,ORDINAL_POSITION,REFERENCED_TABLE_NAME,REFERENCED_COLUMN_NAME', 'TABLE_NAME,CONSTRAINT_NAME,ORDINAL_POSITION'),
            ('REFERENTIAL_CONSTRAINTS', 'TABLE_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_NAME,UPDATE_RULE,DELETE_RULE', 'TABLE_NAME,CONSTRAINT_NAME'),
        ]:
            scope_column = 'CONSTRAINT_SCHEMA' if table == 'REFERENTIAL_CONSTRAINTS' else 'TABLE_SCHEMA'
            schema = self.sql(container, 'SELECT ' + fields + ' FROM information_schema.' + table + ' WHERE ' + scope_column + '="' + self.database + '" ORDER BY ' + order + ';', self.database)
            definitions.append([table, schema])
        checks = self.sql(container, 'SELECT t.TABLE_NAME,c.CONSTRAINT_NAME,c.CHECK_CLAUSE FROM information_schema.CHECK_CONSTRAINTS c JOIN information_schema.TABLE_CONSTRAINTS t ON t.CONSTRAINT_SCHEMA=c.CONSTRAINT_SCHEMA AND t.CONSTRAINT_NAME=c.CONSTRAINT_NAME WHERE t.TABLE_SCHEMA="' + self.database + '" ORDER BY t.TABLE_NAME,c.CONSTRAINT_NAME;', self.database)
        definitions.append(['CHECK_CONSTRAINTS', checks])
        return hashlib.sha256(result.stdout).hexdigest(), counts, hashlib.sha256(json.dumps(definitions).encode()).hexdigest()

    def backup_restore(self, source, target):
        fd = os.open(self.backup, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(fd, 'wb') as destination:
            result = subprocess.run(self.dump_args(source) + [self.database], env=dict(os.environ, MYSQL_PWD=self.password),
                                    stdout=destination, stderr=subprocess.PIPE, timeout=120)
        require(result.returncode == 0 and self.backup.stat().st_size > 0, 'owned test logical backup')
        require(self.backup.stat().st_mode & 0o777 == 0o600, 'private backup mode')
        self.report['backup_bytes'] = self.backup.stat().st_size
        self.report['backup_sha256'] = hashlib.sha256(self.backup.read_bytes()).hexdigest()
        self.report['checks']['backup_mode_0600_directory_0700'] = (Path(self.temp.name).stat().st_mode & 0o777) == 0o700
        self.guard(target)
        with self.backup.open('rb') as origin:
            result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', '-e', 'MYSQL_PWD', target, 'mysql', '-u' + self.user, self.database],
                                    env=dict(os.environ, MYSQL_PWD=self.password), stdin=origin, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, timeout=120)
        require(result.returncode == 0, 'new test engine logical restore')
        actual = self.data_snapshot(target)
        require(actual == self.snapshot, 'all restored table counts and canonical data hash')
        self.report['checks']['all_table_counts_and_data_hash_restored'] = True
        self.report['checks']['schema_indexes_and_constraints_restored'] = True
        self.report['table_counts'] = self.snapshot[1]
        self.report['canonical_data_sha256'] = self.snapshot[0]
        self.report['schema_sha256'] = self.snapshot[2]

    def cold_recover(self, target):
        self.guard(target)
        # Keep an actual uncommitted InnoDB transaction alive, then stop mysqld by container SIGKILL.
        probe = subprocess.Popen(['rtk', 'proxy', 'docker', 'exec', '-i', '-e', 'MYSQL_PWD', target, 'mysql', '-u' + self.user,
                                  '--batch', '--raw', '--skip-column-names', '--unbuffered', self.database],
                                 env=dict(os.environ, MYSQL_PWD=self.password), stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            probe.stdin.write(b'START TRANSACTION; INSERT INTO restore_drill_probes VALUES(2,"uncommitted"); SELECT "PROBE_READY"; SELECT SLEEP(120); COMMIT;\n')
            probe.stdin.flush()
            with selectors.DefaultSelector() as ready:
                ready.register(probe.stdout, selectors.EVENT_READ)
                require(ready.select(15), 'uncommitted crash probe readiness')
                require(probe.stdout.readline().strip() == b'PROBE_READY', 'actual uncommitted transaction exists')
            self.run_command(['docker', 'kill', '--signal=KILL', target])
            probe.wait(timeout=20)
            self.run_command(['docker', 'start', target])
            self.wait_engine(target)
            require(self.sql(target, 'SELECT COUNT(*) FROM restore_drill_probes;', self.database).strip() == '1', 'InnoDB uncommitted transaction rollback')
            require(self.data_snapshot(target) == self.snapshot, 'cold recovery keeps all committed facts')
            self.report['checks']['database_engine_sigkill_recovery'] = True
            self.report['checks']['uncommitted_transaction_rolled_back'] = True
        finally:
            if probe.poll() is None:
                probe.kill()
                probe.wait()
            probe.stdin.close(); probe.stdout.close(); probe.stderr.close()

    def verify_protocol(self, target):
        self.start_service(target)
        players = [self.login(uid) for uid in self.uids]
        a, b, c = players
        require([p.playerId for p in players] == self.ids, 'restored original player identity')
        require(all(self.profile(target, p) == self.saved_profiles[p.playerId] for p in players), 'restored profile assets and game states')
        for name, fields, stamp, expected, saved in [
            ('CSUnionCreate', {'name': 'RestoreGuild'}, 101, 'SCUnionCreate', self.create_reply),
            ('CSUnionDonate', {'donateType': 'coin'}, 105, 'SCUnionDonate', self.donation_reply),
            ('CSGetUnionStamina', {}, 106, 'SCGetUnionStamina', self.stamina_reply),
            ('CSGrab', {'index': 1}, 108, 'SCGrab', self.grab_reply),
            ('CSBuyGrab', {'count': 1}, 109, 'SCBuyGrab', self.purchase_reply)]:
            require(self.call(name, fields, a, stamp=stamp, expected=expected)[0][1] == saved, 'restored historical original receipt: ' + name)
        require(self.call('CSGiveUpGrab', {'index': 7}, b, stamp=111)[0][1] == self.release_reply, 'restored released slot receipt')
        require(all(self.profile(target, p) == self.saved_profiles[p.playerId] for p in players), 'receipt replay no duplicate debit/grant')
        pending = self.call('CSQueryUserReward', {}, a, stamp=201)[0][1].rewardItemInfoList
        require(len(pending) == 1 and pending[0].rewardTitle == 'restore pending source', 'claimed and pending delivery facts')
        require(not self.call('CSMailList', {'mailType': 'allMail'}, a, stamp=202)[0][1].mailBoxList, 'deleted mail tombstone stays deleted')
        info = self.call('CSGetUnionInfo', {}, a, stamp=203)[0][1]
        require(info.unionInfo.unionId == self.guild and info.coinNum == 100000 and info.myRole == 2, 'restored guild membership and treasury')
        for p in [a, b]:
            self.connect(p)
            frames = self.call('CSLoginScene', {}, p, stamp=204)
            require(frames[0][1].self.position.scene == 'town_union', 'restored original guild territory scene')
            building = next(frame for op, frame in frames if op == BY_NAME['SCUnionBuilding'])
            require((building.heroStatueOpen, building.guardTreeOpen) == (0, 0), 'restored uncreated building state')
        peers = self.call('CSMovePlayerInfo', {'playerIds': self.ids}, a, stamp=205)[0][1].playerInfos
        require({p.info.playerId for p in peers} == {a.playerId, b.playerId}, 'restored real TCP guild visibility')
        grabbed = self.call('CSGetGrabData', {}, a, stamp=206)[0][1]
        require(next(row for row in grabbed.grabData if row.position == 1).playerId == a.playerId, 'restored occupied live slot')
        require(next(row for row in grabbed.grabData if row.position == 7).playerId == 0, 'restored ended slot stays free')
        resumed = self.call('CSWorldBossInfo', {}, a, stamp=207)[0][1]
        require(resumed.worldBossShowInfo == self.boss_reply.worldBossShowInfo, 'frozen boss cycle ignores changed policy')
        # A captured battle is still unfinished: replaying start returns the original frozen seed/participants.
        resumed_start = self.call('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': self.copy_task}, c, stamp=114)[0][1]
        require(resumed_start == self.copy_start, 'restored unfinished frozen battle')
        self.report['checks']['native_http_tcp_restore_and_receipts'] = True
        self.stop_service()

    def execute(self):
        started = time.monotonic()
        self.report['mysql_image_id'] = self.run_command(['docker', 'image', 'inspect', '--format', '{{.Id}}', 'mysql:8.4']).decode().strip()
        self.run_command(['go', 'build', '-o', str(self.binary), '.'], cwd=ROOT / 'hyjs-server', timeout=180)
        self.report['binary_sha256'] = hashlib.sha256(self.binary.read_bytes()).hexdigest()
        print('Preparing owned source engine and original gameplay facts.', flush=True)
        source = self.engine('a')
        self.prepare(source)
        self.snapshot = self.data_snapshot(source)
        print('Backing up private test data and restoring into a fresh owned engine.', flush=True)
        target = self.engine('b')
        self.backup_restore(source, target)
        print('Performing actual database-engine crash recovery with an uncommitted transaction.', flush=True)
        self.cold_recover(target)
        print('Restarting the isolated game and verifying original HTTP/TCP facts and historical receipts.', flush=True)
        self.verify_protocol(target)
        require(all(self.report['checks'].values()), 'one or more restore checks failed')
        self.report['elapsed_seconds'] = round(time.monotonic() - started, 3)
        return self.report

    def cleanup(self):
        failures = []
        try:
            self.stop_service()
        except Exception:
            failures.append('isolated service cleanup')
        for container in reversed(self.owned):
            try:
                self.guard(container)
                self.sql(container, 'DROP DATABASE IF EXISTS `' + self.database + "`; DROP USER IF EXISTS '" + self.user + "'@'%';", root=True)
                require(self.sql(container, 'SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name="' + self.database + '"; SELECT COUNT(*) FROM mysql.user WHERE user="' + self.user + '";', root=True).strip() == '0\n0', 'test database/user cleanup')
            except Exception:
                failures.append('isolated database/user cleanup')
            finally:
                try:
                    # A mismatched ownership label never authorizes deleting a foreign engine.
                    self.guard(container)
                    self.run_command(['docker', 'rm', '-f', '-v', container])
                except Exception:
                    failures.append('owned engine cleanup')
        self.owned = []
        self.temp.cleanup()
        require(not failures, 'restore drill cleanup failed; inspect only owned labelled engines')
        self.report['checks']['engines_users_databases_dumps_removed'] = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-isolated', action='store_true', required=True)
    parser.add_argument('--report', type=Path, help='Non-secret verification summary only; never the dump.')
    args = parser.parse_args()
    drill = RestoreDrill()
    try:
        report = drill.execute()
    finally:
        drill.cleanup()
    safe = json.dumps(report, ensure_ascii=False, indent=2)
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(safe + '\n')
    print(json.dumps({'checks_passed':len(report['checks']), 'elapsed_seconds':report['elapsed_seconds'],
                      'tables':len(report['table_counts']), 'cleanup':report['checks']['engines_users_databases_dumps_removed'],
                      'report':str(args.report) if args.report else None}, ensure_ascii=False))


if __name__ == '__main__':
    main()
