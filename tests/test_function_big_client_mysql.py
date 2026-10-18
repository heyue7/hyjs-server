"""Prebuilt original full-resource notification and once-only P1 reward contract.

Normal Create and a finite type1 HTTP signal; no full flag, balance or inventory
injection. This cannot attest the original Android resource/full-client state.
Root owns execution, managed MySQL allocation and the prebuilt binary window.
"""
import concurrent.futures
import fcntl
import hashlib
import http.client as http_client
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from server.codec import CLASSES, BY_NAME, parse_http
import test_continued_gameplay_mysql as wire
import test_local_wallet_mysql as app_transport

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = [('CSBigClientReward', {'type': 1}), ('CSQueryUserReward', {}),
               ('CSAddRewardsToPlayer', {'rewardIds': [123]})]
    for name, fields in vectors:
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 912001)
        packet = parse_http(raw)
        if packet.opcode != BY_NAME[name] or packet.decode() != CLASSES[name](**fields):
            raise AssertionError('Original request differs: ' + name)
    if BY_NAME['CSBigClientReward'] != 20213 or BY_NAME['SCBigClientReward'] != 20214:
        raise AssertionError('Original resource notification opcode differs')
    if CLASSES['SCBigClientReward'].DESCRIPTOR.fields:
        raise AssertionError('Original resource acknowledgement must have no reward fields')
    for name, fields in {'SCQueryUserReward': {'rewardItemInfoList'},
                         'RewardItemInfo': {'rewardId', 'rewardList'},
                         'SCAddRewardsToPlayer': {'result'}, 'ResultObject': {'simps'},
                         'BaseResource': {'id', 'num'}}.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original reward consumer differs: ' + name)
    return dict(originalRequestVectors=len(vectors), notificationType=1, originalDiamond=300,
                finitePrerequisite='normal Create plus finite original HTTP notification; no resource attestation',
                clientCallbackOnlyContinuesLogin=True, phoneFullClientVerified=False,
                directBalanceOrInventoryFixture=False, serverFullFlagFixture=False,
                notificationAddsBalance=False, rewardCenterClaimAddsBalance=True)


def source_contract(source):
    game = (source / 'hyjs-server/service/gameService.go').read_text()
    if 's.bigClientReward(ctx, row.Id)' not in game:
        raise AssertionError('BigClient reward dispatch missing')
    handler = (source / 'hyjs-server/handler/gameHandler.go').read_text()
    if 'case *pb.CSBigClientReward:' not in handler or 'constUtil.BigClientRewardType' not in handler:
        raise AssertionError('Original type guard missing')
    service = (source / 'hyjs-server/service/bigClientRewardService.go').read_text()
    for token in ['updateProfileWithTx', 's.EnqueueRewardNotice', 'BigClientRewardSource',
                  'game.FullClientVerified = true', '&pb.SCBigClientReward{}']:
        if token not in service:
            raise AssertionError('BigClient reward transaction missing: ' + token)
    constants = (source / 'hyjs-server/constUtil/bigClientReward.go').read_text()
    if 'original-big-client-reward-v1' not in constants:
        raise AssertionError('Original once-only source identity differs')
    if 's.InitializeP1(ctx)' not in (source / 'hyjs-server/main.go').read_text():
        raise AssertionError('P1 normal initialization missing')
    lines = (source / 'analysis/tables/TableGameConfigure.tsv').read_text().splitlines()
    rows = [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]
    if next(r['NameValue'] for r in rows if r['StaticName'] == 'BigClientReward') != 'diamond,300':
        raise AssertionError('Original full-resource reward differs')
    return protocol_preflight()


def preflight():
    source = Path(os.environ.get('HYJS_BIG_CLIENT_SOURCE', ROOT / 'build/candidates/function-player-rename-r1-20261017')).resolve()
    binary = Path(os.environ.get('HYJS_BIG_CLIENT_BINARY', ROOT / 'build/hyjs-server-function-player-rename-r1-20261017-pending')).resolve()
    validation = Path(os.environ.get('HYJS_BIG_CLIENT_VALIDATION', ROOT / 'analysis/p3/function-player-rename-r1-incremental-20261017.json')).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a direct fixed candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_bytes())['files']:
        path = (source / entry['path']).resolve()
        if not path.is_relative_to(source) or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Fixed candidate pin differs: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Exact completed qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use the exact prebuilt qualified app')
    for rel in ['server/codec.py', 'server/protocol/schema.json', 'analysis/tables/TableGameConfigure.tsv']:
        if sha(source / rel) != sha(ROOT / rel):
            raise AssertionError('Original protocol/table differs: ' + rel)
    return source, binary, sha(manifest), source_contract(source)


@unittest.skipUnless(os.environ.get('HYJS_BIG_CLIENT_MYSQL_TEST') == '1', 'Root prebuilt managed window')
class FunctionBigClientMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    profile, stop, sql = transport.profile, transport.stop, transport.sql

    def start(self):
        app_transport.SOURCE_ROOT = self.source
        app_transport.LocalWalletMySQLTests.start(self, wallet=False)

    def stamped(self, name, fields, owner, stamp):
        raw = wire.encode_original_http(name, fields, owner.playerId, bytes(owner.sessionKey), stamp)
        connection = http_client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=40)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            frames = wire.decode_responses(response.read())
            self.assertEqual(len(frames), 1, 'One original frame; no additional balance projection')
            return frames[0]
        finally:
            connection.close()

    def raw_profile(self, owner):
        return self.sql(f'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id={owner.playerId};').strip()

    def deliveries(self, owner):
        return self.sql('SELECT id,rewards,created_at,expires_at,claimed_at,HEX(result) '
                        'FROM p1_reward_deliveries '
                        f"WHERE player_id={owner.playerId} AND source_key='original-big-client-reward-v1';").strip()

    def assert_once(self, owner):
        for table in ['p1_reward_deliveries', 'p1_mail_messages']:
            count = self.sql(f'SELECT COUNT(*) FROM {table} '
                             f"WHERE player_id={owner.playerId} AND source_key='original-big-client-reward-v1';").strip()
            self.assertEqual(count, '1', table)

    def test_original_notification_reward_once_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_BIG_CLIENT_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/function-big-client-reward-r1-20261017'))
        self.assertFalse(output.exists(), 'Keep previous attempt evidence')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__), binarySHA256=sha(binary),
                      sourceManifestSHA256=manifest_sha, contract=contract, phoneUI=False,
                      originalFullResourceStateProven=False, productionModified=False,
                      externalPayment=False, phase='allocation')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-big-client-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('big_client')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-bigclient-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '奖励保护', 'heroId': 'hero_043'}, protected, 912000, 'SCPlayerCreate')
                    protected_before = self.raw_profile(protected)
                    uid = 'demo-bigclient-' + suffix
                    owner = self.login(uid)
                    self.assertGreater(owner.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '完整资源奖励', 'heroId': 'hero_047'}, owner, 912001, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    before = self.raw_profile(owner)
                    self.assertEqual(initial[0].get('diamond', 0), 0)
                    self.assertFalse(initial[1].get('full_client_verified', False))
                    report['phase'] = 'original-type-validation'
                    for fields in [{}, {'type': -1}, {'type': 0}, {'type': 2}]:
                        self.call('CSBigClientReward', fields, owner, 912010, 'ErrorMessage')
                    self.assertEqual(self.raw_profile(owner), before)
                    self.assertEqual(self.deliveries(owner), '')
                    report['phase'] = 'original-type1-notification-no-balance-delta'
                    ack = self.call('CSBigClientReward', {'type': 1}, owner, 912100, 'SCBigClientReward')
                    self.assertEqual(ack.SerializeToString(), b'')
                    notified = self.profile(owner)
                    self.assertEqual(notified[0], initial[0])
                    self.assertEqual(notified[1], dict(initial[1], full_client_verified=True))
                    self.assert_once(owner)
                    listing = self.call('CSQueryUserReward', {}, owner, 912101, 'SCQueryUserReward')
                    rewards = list(listing.rewardItemInfoList)
                    self.assertEqual(len(rewards), 1)
                    reward = rewards[0]
                    self.assertEqual([(v.id, v.num) for v in reward.rewardList], [('diamond', 300)])
                    self.assertEqual(int(self.deliveries(owner).split('\t')[0]), reward.rewardId)
                    # The original empty notification callback has no local gain.
                    # Only this actual P1 claim returns the 300-diamond increment.
                    report['phase'] = 'original-p1-claim300'
                    claim_fields = {'rewardIds': [reward.rewardId]}
                    claim = self.call('CSAddRewardsToPlayer', claim_fields, owner, 912102, 'SCAddRewardsToPlayer')
                    self.assertEqual([(v.id, v.num) for v in claim.result.simps], [('diamond', 300)])
                    self.assertEqual(claim.result, CLASSES['ResultObject'](simps=[{'id': 'diamond', 'num': 300}]))
                    claimed = self.profile(owner)
                    self.assertEqual(claimed[0], dict(initial[0], diamond=300))
                    self.assertEqual(claimed[1], notified[1])
                    receipt = self.deliveries(owner)
                    cols = receipt.split('\t')
                    self.assertEqual(cols[1], 'diamond,300')
                    self.assertGreater(int(cols[4]), 0)
                    self.assertEqual(bytes.fromhex(cols[5]), claim.result.SerializeToString())
                    final_bytes = self.raw_profile(owner)
                    report['phase'] = 'eight-notifications-after-claim-no-revival'
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        results = list(workers.map(lambda i: self.call('CSBigClientReward', {'type': 1}, owner, 912200+i, 'SCBigClientReward'), range(8)))
                    self.assertTrue(all(result == ack for result in results))
                    self.assert_once(owner)
                    self.assertEqual(self.deliveries(owner), receipt)
                    self.assertEqual(self.raw_profile(owner), final_bytes)
                    self.assertEqual(len(self.call('CSQueryUserReward', {}, owner, 912210, 'SCQueryUserReward').rewardItemInfoList), 0)
                    spoof = types.SimpleNamespace(playerId=protected.playerId, sessionKey=owner.sessionKey)
                    self.call('CSBigClientReward', {'type': 1}, spoof, 912211, 'ErrorMessage')
                    self.call('CSAddRewardsToPlayer', claim_fields, protected, 912212, 'ErrorMessage')
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    self.assertEqual(self.deliveries(protected), '')
                    report['phase'] = 'cold-original-claim-retry-empty-and-notification-once'
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.raw_profile(owner), final_bytes)
                    retry = self.call('CSAddRewardsToPlayer', claim_fields, owner, 912102, 'SCAddRewardsToPlayer')
                    self.assertEqual(retry.result, CLASSES['ResultObject']())
                    self.assertEqual(self.call('CSBigClientReward', {'type': 1}, owner, 912100, 'SCBigClientReward'), ack)
                    self.assertEqual(len(self.call('CSQueryUserReward', {}, owner, 912300, 'SCQueryUserReward').rewardItemInfoList), 0)
                    self.assert_once(owner)
                    self.assertEqual(self.deliveries(owner), receipt)
                    self.assertEqual(self.raw_profile(owner), final_bytes)
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    self.assertEqual(self.deliveries(protected), '')
                    engine = self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME IN ('profiles','p1_reward_deliveries','p1_mail_messages');").strip()
                    self.assertEqual(engine, 'InnoDB')
                    report.update(status='PASS', fullChain=True, scope='finite original HTTP notification to P1 once-only claim and cold replay',
                                  original20213to20214=True, originalP1ClaimDiamond=300, invalidTypesRejected=4,
                                  notificationNoBalanceDelta=True, eightConcurrentNotificationsOnce=True,
                                  originalClaimIncrementOnly=True, coldClaimRetryEmpty=True,
                                  coldStoredResultExact=True, coldNotificationDoesNotRevive=True,
                                  noFullFlagInjection=True, noBalanceOrInventoryInjection=True, protected1Exact=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    try:
                        self.stop()
                    except BaseException as error:
                        report.update(status='FAIL', fullChain=False, cleanupFailure='owned-app-stop', cleanupError=str(error))
                        raise
                    finally:
                        try:
                            self.pool.close()
                        except BaseException as error:
                            report.update(status='FAIL', fullChain=False, cleanupFailure='managed-allocation-release', cleanupError=str(error))
                            raise
                        finally:
                            report['seconds'] = round(time.monotonic() - began, 3)
                            report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                            if self.pool.connections:
                                report.update(status='FAIL', fullChain=False, cleanupFailure=True)
                            output.parent.mkdir(parents=True, exist_ok=True)
                            output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()


