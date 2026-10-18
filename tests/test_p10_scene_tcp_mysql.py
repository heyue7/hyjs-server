"""Fixed-source original HTTP/TCP regression for stale empty line-transfer receipts."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import shutil
import socket
import subprocess
import tempfile
import time
import types
import unittest

import test_p10_mysql as p10
from server.codec import BY_NAME

ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT / 'build/candidates/joint-voice-driver-v93-20261015'
OWNED = ['hyjs-server/service/p10SceneService.go', 'hyjs-server/service/p10SceneReceipt_test.go']


@unittest.skipUnless(os.environ.get('HYJS_P10_SCENE_TCP_MYSQL_TEST') == '1', 'explicit isolated scene receipt test')
class P10SceneTCPTests(unittest.TestCase):
    send = p10.P10MySQLTests.send
    login = p10.P10MySQLTests.login
    stamped = p10.P10MySQLTests.stamped
    root_sql = classmethod(p10.P10MySQLTests.root_sql.__func__)
    sql = p10.P10MySQLTests.sql
    call = p10.P10MySQLTests.call
    profile = p10.P10MySQLTests.profile
    connect = p10.P10MySQLTests.connect
    recv = p10.P10MySQLTests.recv
    tcp_send = p10.P10MySQLTests.tcp_send

    def run_command(self, args, cwd, env=None, expect_failure=False):
        result = subprocess.run(['rtk', 'proxy', *map(str, args)], cwd=cwd, env=env,
                                text=True, capture_output=True, timeout=240)
        output = result.stdout + result.stderr
        if env:
            output = output.replace(self.test_password, '[redacted]')
        if expect_failure:
            self.assertNotEqual(result.returncode, 0, output)
            self.assertIn('old line-two success replayed', output)
            self.assertIn('original TCP zero timestamp transfer rejected', output)
        else:
            self.assertEqual(result.returncode, 0, output[-6000:])
        return output

    def start(self):
        # Keep the native main as the actual PID, so SIGTERM verifies ctx shutdown.
        self.process = subprocess.Popen([str(self.binary), '--http-port', '0', '--tcp-port', '0',
                                         '--tables', str(self.source/'analysis/tables')],
                                        cwd=self.source/'hyjs-server', env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'native startup timeout')
            line = self.process.stdout.readline()
        match = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(match, line)
        self.service = types.SimpleNamespace(http_port=int(match[1]), tcp_port=int(match[2]))

    def stop(self):
        if self.process is not None:
            if self.process.poll() is None:
                self.process.terminate()
                try:
                    self.process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    self.process.kill()
                    self.process.wait()
                    self.fail('native shutdown timeout')
            self.assertEqual(self.process.returncode, 0, 'native main did not shut down cleanly')
            self.process.stdout.close()
            self.process = None

    def test_current_line_receipts_race_original_frames_and_cold(self):
        began = time.monotonic()
        self.process = None
        with tempfile.TemporaryDirectory(prefix='hyjs-p13-scene-receipt-') as temporary:
            self.source = Path(temporary).resolve()
            shutil.copytree(BASE/'hyjs-server', self.source/'hyjs-server')
            shutil.copytree(BASE/'analysis/tables', self.source/'analysis/tables')
            for name in OWNED:
                shutil.copyfile(ROOT/name, self.source/name)
            shared_path = Path(os.environ['HYJS_P10_SCENE_TCP_SHARED_MANIFEST'])
            shared = json.loads(shared_path.read_text())
            for entry in shared['files']:
                source = ROOT / entry['snapshot'] if 'snapshot' in entry else ROOT / shared['snapshot'] / entry['path']
                self.assertEqual(hashlib.sha256(source.read_bytes()).hexdigest(),entry['sha256'])
                target = self.source / entry['path']
                target.parent.mkdir(parents=True,exist_ok=True)
                shutil.copyfile(source,target)
            source_hashes = {str(p.relative_to(self.source)): hashlib.sha256(p.read_bytes()).hexdigest()
                             for p in self.source.rglob('*') if p.is_file()}
            server = self.source/'hyjs-server'
            old_overlay = self.source/'old-source-overlay.json'
            old_overlay.write_text(json.dumps({'Replace': {str(server/'service/p10SceneService.go'):
                                                          str(BASE/'hyjs-server/service/p10SceneService.go')}}))
            self.run_command(['go', 'test', '-overlay', old_overlay, './service', '-run', '^TestP10Scene(ReceiptCurrentLineOnly|OriginalTCPNoTimestamp)$', '-count=1'], server, expect_failure=True)
            self.run_command(['go', 'test', './...', '-run', '^TestP10Scene(ReceiptCurrentLineOnly|OriginalTCPNoTimestamp)$', '-count=1'], server)
            self.run_command(['go', 'vet', './service'], server)
            race = self.source/'service-race.test'
            self.run_command(['go', 'test', '-c', '-race', '-o', race, './service'], server)
            self.binary = self.source/'hyjs-native'
            self.run_command(['go', 'build', '-o', self.binary, '.'], server)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                type(self).database = 'hyjs_test_p10_scene_receipt_'+secrets.token_hex(5)
                domain_database = self.database
                type(self).test_user = 'p10_scene_'+secrets.token_hex(5)
                type(self).test_password = secrets.token_hex(24)
                native_database = 'hyjs_test_p10_scene_native_'+secrets.token_hex(5)
                self.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=self.test_user,
                                HYJS_MYSQL_PASSWORD=self.test_password, HYJS_MYSQL_DATABASE=native_database,
                                HYJS_P10_MYSQL_HOST='127.0.0.1:33068', HYJS_P10_MYSQL_USER=self.test_user,
                                HYJS_P10_MYSQL_PASSWORD=self.test_password, HYJS_P10_MYSQL_DATABASE=self.database)
                try:
                    self.root_sql('CREATE DATABASE `'+self.database+'`;CREATE DATABASE `'+native_database+'`;'
                                  "CREATE USER '"+self.test_user+"'@'%' IDENTIFIED BY '"+self.test_password+"';"
                                  'GRANT ALL ON `'+self.database+"`.* TO '"+self.test_user+"'@'%';"
                                  'GRANT ALL ON `'+native_database+"`.* TO '"+self.test_user+"'@'%';")
                    self.run_command([race, '-test.run=^TestP10Scene(ReceiptCurrentLineOnly|OriginalTCPNoTimestamp)$', '-test.count=1'], server/'service', self.env)
                    type(self).database = native_database
                    self.start()
                    uids = ['scene-receipt-'+secrets.token_hex(4) for _ in range(2)]
                    a, b = [self.login(uid) for uid in uids]
                    for login, name in [(a, '回执换线甲'), (b, '回执换线乙')]:
                        self.call('CSPlayerCreate', {'playerName':name, 'heroId':'hero_047'}, login, 1, 'SCPlayerCreate')
                    # Finite level/guide prerequisites only; no wallet, equipment or reward injection.
                    self.sql('UPDATE p10_scene_policies SET enabled=1;UPDATE profiles SET player=JSON_SET(player,"$.level",20,"$.noviceGuideSchedule",7);')
                    for login in [a,b]:
                        self.call('CSTransferScene', {'target':'field_001'}, login, 20, 'SCTransferScene')
                    before = [self.profile(x) for x in [a,b]]
                    sa, sb = self.connect(a), self.connect(b)
                    self.recv(sa); self.recv(sb)
                    self.call('CSTransferLine', {'line':2}, a, 4101, 'SCTransferLine')
                    self.assertEqual(self.recv(sa)[0], BY_NAME['SCPlayerDisappear'])
                    self.assertEqual(self.recv(sb)[0], BY_NAME['SCPlayerDisappear'])
                    self.call('CSTransferLine', {'line':1}, a, 4102, 'SCTransferLine')
                    self.assertEqual(self.recv(sa)[0], BY_NAME['SCPlayerAppear'])
                    self.assertEqual(self.recv(sb)[0], BY_NAME['SCPlayerAppear'])
                    for _ in range(2):
                        op, _ = self.stamped('CSTransferLine', {'line':2}, a, 4101)
                        self.assertEqual(op,10001)
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                        list(pool.map(lambda _:self.call('CSTransferLine', {'line':1}, a, 4102, 'SCTransferLine'),range(8)))
                    for sock in [sa,sb]:
                        sock.settimeout(.2)
                        try:
                            unexpected = self.recv(sock)
                        except socket.timeout:
                            pass
                        else:
                            self.fail('unexpected retry TCP frame: '+repr(unexpected))
                    # Close only after both observations; closing the first live client
                    # correctly publishes its disappearance to the other client.
                    sa.close();sb.close()
                    self.assertEqual([self.profile(x) for x in [a,b]], before)
                    self.stop(); self.start()
                    a,b = [self.login(uid) for uid in uids]
                    sa, sb = self.connect(a), self.connect(b)
                    self.recv(sa); self.recv(sb)
                    op,_ = self.stamped('CSTransferLine', {'line':2}, a,4101)
                    self.assertEqual(op,10001)
                    self.call('CSTransferLine', {'line':1}, a,4102,'SCTransferLine')
                    self.assertEqual(self.call('CSGetLineInfo', {},a,1,'SCGetLineInfo').line,1)
                    self.assertEqual([self.profile(x) for x in [a,b]],before)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_social_receipts WHERE domain="line-transfer";').strip(),'2')
                    for line,visual in [(2,'SCPlayerDisappear'),(1,'SCPlayerAppear')]:
                        self.tcp_send(sa,'CSTransferLine',{'line':line},a)
                        self.assertEqual(self.recv(sa)[0],BY_NAME[visual])
                        self.assertEqual(self.recv(sa)[0],BY_NAME['SCTransferLine'])
                        self.assertEqual(self.recv(sb)[0],BY_NAME[visual])
                        for _ in range(8):
                            self.tcp_send(sa,'CSTransferLine',{'line':line},a)
                            self.assertEqual(self.recv(sa)[0],BY_NAME['SCTransferLine'])
                        sb.settimeout(.2)
                        with self.assertRaises(socket.timeout):self.recv(sb)
                        sb.settimeout(3)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_social_receipts WHERE domain="line-transfer";').strip(),'2')
                    self.assertEqual([self.profile(x) for x in [a,b]],before)
                    sa.close();sb.close();self.stop()
                    report = {'status':'PASS_ORIGINAL_TCP_ZERO_STAMP_CONTINUOUS_LINE_CHANGE_RETRY_AND_HTTP_COLD_GUARD',
                              'seconds':round(time.monotonic()-began,3),'ownedGoSHA256':{n:source_hashes[n] for n in OWNED},
                              'nativeSHA256':hashlib.sha256(self.binary.read_bytes()).hexdigest(),
                              'sourceFiles':len(source_hashes),'originalCallbacks':'SCTransferLine empty body clears actors/cache and requests current position/chat',
                              'sharedManifestSHA256':hashlib.sha256(shared_path.read_bytes()).hexdigest(),'sharedFiles':shared['files'],
                              'scope':'original TCP header timestamp absent: true TCP 1→2→1 and eight current-line retries each remain connected; no generated receipts; old different-line receipt refused; current same-line eight retries no broadcast or mutation; two original clients represented by authenticated TCP sockets, not phone UI',
                              'limitations':'same numeric line after a different scene/line incarnation is not bound by old generic receipts; no claim full scene generation restored',
                              'productionModified':False,'phoneUI':False,'cleanup':'two temporary schemas/user/grants, native PIDs and complete temp source/binaries removed'}
                finally:
                    self.stop()
                    self.root_sql('DROP DATABASE IF EXISTS `'+domain_database+'`;'
                                  'DROP DATABASE IF EXISTS `'+native_database+'`;'
                                  "DROP USER IF EXISTS '"+self.test_user+"'@'%';")
                    remaining=self.root_sql('SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME IN ("'+domain_database+'","'+native_database+'");SELECT COUNT(*) FROM mysql.user WHERE User="'+self.test_user+'";')
                    self.assertEqual(remaining.strip().splitlines(),['0','0'])
                    fcntl.flock(lock,fcntl.LOCK_UN)
            (ROOT/'analysis/p10/scene-tcp-mysql-result.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
