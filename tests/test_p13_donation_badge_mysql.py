"""Paid donation -> original P1 claim -> badge donation -> box -> coin use.

One existing qualified binary and one managed temporary database are used. The
account's level/guide and prior original chapter stars are explicit finite
qualifications. Wallets, badges, reward IDs, equipment GUIDs and donation scores
are never inserted: original star chests produce the initial spendable funds and
equipment. No natural historical victories or phone UI proof is claimed.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import unittest

from server.codec import BY_NAME, CLASSES, parse_http
from hyjs_managed_mysql import ManagedMySQL
import test_continued_gameplay_mysql as transport


ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_P13_DONATION_BADGE_SOURCE_ROOT',
                                 ROOT / 'build/candidates/playable-continuation-20261016')).resolve()
VALIDATION = Path(os.environ.get('HYJS_P13_DONATION_BADGE_VALIDATION',
                                ROOT / 'analysis/p3/playable-continuation-incremental-20261016.json')).resolve()
REPORT = Path(os.environ.get('HYJS_P13_DONATION_BADGE_REPORT',
                            ROOT / 'analysis/p13/donation-badge-protocol-20261016.json')).resolve()


def original_rows(name):
    lines = (SOURCE_ROOT / 'analysis/tables' / ('Table' + name + '.tsv')).read_text().splitlines()
    return [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]


def protocol_preflight():
    checked = transport.protocol_preflight()
    vectors = [
        ('CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 0}),
        ('CSCopyGetStarAward', {'chapterId': '26', 'starIndex': 1}),
        ('CSCopyGetStarAward', {'chapterId': '27', 'starIndex': 2}),
        ('CSQueryUserReward', {}), ('CSAddRewardsToPlayer', {'rewardIds': [1, 2]}),
        ('CSEquipStrengthen', {'guid': '0' * 32, 'times': 1,
                               'cost': [{'id': 'coin', 'num': 8}]}),
    ]
    for name, fields in vectors:
        raw = transport.encode_original_http(name, fields, 1, b'0' * 16, 1700000000000)
        decoded = parse_http(raw)
        if decoded.opcode != BY_NAME[name] or decoded.decode() != CLASSES[name](**fields):
            raise AssertionError('Donation source original request roundtrip failed: ' + name)
    for name, fields in {
        'SCQueryUserReward': ['rewardItemInfoList'], 'SCAddRewardsToPlayer': ['result'],
        'SCCopyGetStarAward': ['gain'], 'SCEquipStrengthen': [],
    }.items():
        if name not in BY_NAME or any(field not in CLASSES[name].DESCRIPTOR.fields_by_name for field in fields):
            raise AssertionError('Donation source original response changed: ' + name)
    checked['donationBadgeRequestVectorsConstructed'] = len(vectors)
    checked['badgeItemPresent'] = any(row['itemId'] == 'item_089' for row in original_rows('Item'))
    if not checked['badgeItemPresent']:
        raise AssertionError('Original donation badge item missing')
    return checked


def binary_preflight():
    if SOURCE_ROOT.parent != (ROOT / 'build/candidates').resolve() or not SOURCE_ROOT.is_dir():
        raise AssertionError('Donation badge validation requires a direct frozen candidate')
    binary = Path(os.environ.get('HYJS_P13_DONATION_BADGE_BINARY', '')).expanduser()
    if not binary.is_file():
        raise AssertionError('Set HYJS_P13_DONATION_BADGE_BINARY to the existing qualified executable')
    proof = json.loads(VALIDATION.read_text())
    if not proof.get('qualified') or (ROOT / proof['candidate']).resolve() != SOURCE_ROOT:
        raise AssertionError('Donation badge source does not match its qualification')
    # Reuse the earlier complete manifest/hash/Go-set guard without inheriting its test suite.
    transport.SOURCE_ROOT = SOURCE_ROOT
    transport.SOURCE = SOURCE_ROOT / 'manifest.json'
    transport.VALIDATION = VALIDATION
    transport.REPORT = REPORT
    previous = os.environ.get('HYJS_CONTINUED_GAMEPLAY_BINARY')
    os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = str(binary.resolve())
    try:
        return transport.binary_source_preflight()
    finally:
        if previous is None:
            os.environ.pop('HYJS_CONTINUED_GAMEPLAY_BINARY', None)
        else:
            os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = previous


@unittest.skipUnless(os.environ.get('HYJS_P13_DONATION_BADGE_MYSQL_TEST') == '1',
                     'Explicitly enable the isolated original donation badge chain')
class DonationBadgeMySQLTests(unittest.TestCase):
    # Composition selects transport only, leaving all old test/allocator methods unused.
    send = transport.ContinuedGameplayMySQLTests.send
    login = transport.ContinuedGameplayMySQLTests.login
    stamped = transport.ContinuedGameplayMySQLTests.stamped
    call = transport.ContinuedGameplayMySQLTests.call
    profile = transport.ContinuedGameplayMySQLTests.profile
    write_profile = transport.ContinuedGameplayMySQLTests.write_profile
    sql = transport.ContinuedGameplayMySQLTests.sql
    start = transport.ContinuedGameplayMySQLTests.start
    stop = transport.ContinuedGameplayMySQLTests.stop

    def test_original_paid_donation_p1_badge_box_coin_use_and_cold(self):
        preflight = protocol_preflight()
        self.binary, binary_sha, source_sha = binary_preflight()
        if REPORT.exists():
            self.fail('Preserve the existing donation badge report; select a new report path for a retry')
        self.process = None
        self.pool = ManagedMySQL()
        self.app_temp = None
        began = time.monotonic()
        result = {'status': 'RUNNING', 'binarySHA256': binary_sha, 'sourceManifestSHA256': source_sha,
                  'sourceRoot': str(SOURCE_ROOT), 'preflight': preflight, 'phoneUI': False,
                  'productionModified': False, 'runnerSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  'finitePrerequisites': 'normal created account; level30/guide8 and prior original '
                  'chapter1/26/27 mission stars5 only; initial coin/diamond remain0; no wallet, '
                  'badge, score, rewardId, item, equipment GUID, natural damage or winner preparation'}
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY: no database, app or temporary directory started')
        try:
            connection = self.pool.allocate('p13_donation_badge')
            self.database = connection.database
            self.env = {key: value for key, value in os.environ.items()
                        if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
            self.env.update(connection.env())
            self.env['TZ'] = 'Asia/Shanghai'
            self.app_temp = tempfile.TemporaryDirectory(prefix='hyjs-p13-donation-badge-')
            self.start()
            uid = 'p13-donation-badge-' + secrets.token_hex(6)
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '徽章真链', 'heroId': 'hero_047'},
                      login, 1, 'SCPlayerCreate')
            player, game = self.profile(login)
            self.assertEqual((player.get('coin', 0), player.get('diamond', 0)), (0, 0))
            self.assertFalse(any(item.get('id') == 'item_089' and item.get('num', 0)
                                 for item in (game.get('items') or [])))
            player.update(level=30, noviceGuideSchedule=8)
            selected = {'1', '26', '27'}
            game['copy']['passedCopyInfo'] = [
                {'chapterId': row['ChapterId'], 'missionId': row['MissionId'], 'star': 5,
                 'challengeTimes': 0} for row in original_rows('CopyMission') if row['ChapterId'] in selected]
            game['copy']['passedChapterInfo'] = []
            self.write_profile(login, player, game)
            stamp = 100
            source_coin = source_diamond = 0
            star_rows = []
            gear = None
            for chapter, index in [('1', 0), ('27', 0), ('27', 1), ('27', 2), ('26', 0), ('26', 1)]:
                stamp += 1
                award = self.call('CSCopyGetStarAward', {'chapterId': chapter, 'starIndex': index},
                                  login, stamp, 'SCCopyGetStarAward')
                coin = sum(item.num for item in award.gain.simps if item.id == 'coin')
                diamond = sum(item.num for item in award.gain.simps if item.id == 'diamond')
                source_coin += coin
                source_diamond += diamond
                star_rows.append({'chapter': chapter, 'index': index, 'actualCoin': coin,
                                  'actualDiamond': diamond})
                if chapter == '1':
                    self.assertEqual(len(award.gain.equips), 1)
                    gear = award.gain.equips[0]
            self.assertEqual(source_coin, 1845000)
            self.assertEqual(source_diamond, 2000)
            self.assertEqual(self.profile(login)[0]['coin'], source_coin)
            result['actualOriginalStarAwards'] = star_rows
            self.sql('UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1 WHERE id=1; '
                     'UPDATE p13_guild_donate_box_policies SET enabled=1,published_at='
                     'CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS UNSIGNED)-1000 WHERE id=1; '
                     'UPDATE p13_guild_donation_badge_policies SET enabled=1,published_at='
                     'CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS UNSIGNED)-1000 WHERE id=1;')
            self.call('CSUnionCreate', {'name': '徽章真会'}, login, 120, 'SCUnionCreate')
            self.assertEqual(self.profile(login)[0]['diamond'], source_diamond - 300)
            donate = {'donateType': 'coin'}
            with concurrent.futures.ThreadPoolExecutor(max_workers=6) as workers:
                replies = list(workers.map(lambda _: self.call('CSUnionDonate', donate, login, 200,
                                                              'SCUnionDonate'), range(6)))
            self.assertTrue(all(reply == replies[0] for reply in replies))
            for index in range(1, 15):
                self.call('CSUnionDonate', donate, login, 200 + index, 'SCUnionDonate')
            self.assertEqual(self.stamped('CSUnionDonate', donate, login, 215)[0], 10001)
            player, game = self.profile(login)
            self.assertEqual(player['coin'], source_coin - 1500000)
            self.assertEqual(sum(item.get('num', 0) for item in (game.get('items') or [])
                                 if item.get('id') == 'item_089'), 0)
            rows = self.sql('SELECT delivery_id FROM p13_guild_donation_badge_sources WHERE player_id='
                            + str(login.playerId) + ' ORDER BY ordinal;').strip().splitlines()
            reward_ids = [int(value) for value in rows]
            self.assertEqual(len(reward_ids), 15)
            listed = self.call('CSQueryUserReward', {}, login, 300, 'SCQueryUserReward')
            self.assertTrue(set(reward_ids).issubset({item.rewardId for item in listed.rewardItemInfoList}))
            # Stop new supply with unclaimed earned mail, then recover with the same binary.
            self.sql('UPDATE p13_guild_donation_badge_policies SET enabled=0 WHERE id=1;')
            self.stop()
            self.start()
            login = self.login(uid)
            self.assertEqual(self.call('CSUnionDonate', donate, login, 200, 'SCUnionDonate'), replies[0])
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_guild_donation_badge_sources;').strip(), '15')
            claim_fields = {'rewardIds': reward_ids}
            with concurrent.futures.ThreadPoolExecutor(max_workers=6) as workers:
                claims = list(workers.map(lambda _: self.call('CSAddRewardsToPlayer', claim_fields,
                                                             login, 301, 'SCAddRewardsToPlayer'), range(6)))
            granted = sum(item.num for reply in claims for item in reply.result.simps if item.id == 'item_089')
            self.assertEqual(granted, 30)
            self.assertEqual(sum(item.get('num', 0) for item in (self.profile(login)[1].get('items') or [])
                                 if item.get('id') == 'item_089'), 30)
            for index in range(30):
                self.call('CSUnionDonate', {'donateType': 'badge'}, login, 400 + index, 'SCUnionDonate')
            store = self.call('CSUnionStore', {}, login, 500, 'SCUnionStore')
            self.assertEqual(store.currentScore, 255)
            self.assertEqual([box.score for box in store.boxes], [250, 400, 500, 600])
            self.assertEqual([box.state for box in store.boxes], [1, 0, 0, 0])
            box_fields = {'boxId': store.boxes[0].boxId}
            box = self.call('CSUnionOpenDonateBox', box_fields, login, 501, 'SCUnionOpenDonateBox')
            self.assertEqual([(item.id, item.num) for item in box.get.simps], [('coin', 1000)])
            self.assertEqual(self.profile(login)[0]['coin'], source_coin - 1500000 + 1000)
            # Original star chest gear is an actual GUID; strengthen consumes real wallet coin.
            quality = next(row['quality'] for row in original_rows('Equip') if row['id'] == gear.equipId)
            cost = int(next(row['coin'] for row in original_rows('EquipExp')
                            if row['quality'] == quality and row['level'] == '1'))
            self.call('CSEquipStrengthen', {'guid': gear.guid, 'times': 1,
                                          'cost': [{'id': 'coin', 'num': cost}]},
                      login, 502, 'SCEquipStrengthen')
            wallet = source_coin - 1500000 + 1000 - cost
            self.assertEqual(self.profile(login)[0]['coin'], wallet)
            self.stop()
            self.start()
            login = self.login(uid)
            self.assertEqual(self.call('CSUnionDonate', donate, login, 200, 'SCUnionDonate'), replies[0])
            self.assertEqual(self.call('CSUnionOpenDonateBox', box_fields, login, 501,
                                       'SCUnionOpenDonateBox'), box)
            empty = self.call('CSAddRewardsToPlayer', claim_fields, login, 301, 'SCAddRewardsToPlayer')
            self.assertEqual(len(empty.result.simps), 0)
            final_player, final_game = self.profile(login)
            self.assertEqual(final_player['coin'], wallet)
            self.assertEqual(final_player['diamond'], source_diamond - 300)
            self.assertEqual(sum(item.get('num', 0) for item in (final_game.get('items') or [])
                                 if item.get('id') == 'item_089'), 0)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_guild_donation_badge_sources;').strip(), '15')
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_guild_donate_box_claims;').strip(), '1')
            result.update(status='PASS_ORIGINAL_HTTP_INNODB_PAID_BADGE_P1_DONATE_BOX_AND_COIN_USE_PHONE_PENDING',
                          actualCoinDonations=15, actualDonationCoinCost=1500000, actualMailBadgeClaim=30,
                          actualBadgeDonations=30, actualDonationScore=255, actualBoxCoin=1000,
                          actualEquipmentCoinUse=cost, finalCoin=wallet, finalDiamond=source_diamond - 300,
                          coldRestarts=2, oldCoinStampDoesNotRepublish=True,
                          oldP1ClaimEmptyDelta=True, oldBoxStampDoesNotRegrant=True)
        except Exception as error:
            result['status'] = 'FAIL_PARTIAL_PREFIX_NOT_FULL_ACCEPTANCE'
            result['failure'] = self.pool.redact(str(error))[:2000]
            raise
        finally:
            try:
                self.stop()
            except Exception:
                result['status'] = 'FAIL_OWNED_APP_CLEANUP'
                raise
            finally:
                try:
                    self.pool.close()
                    result['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    result['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                except Exception:
                    result['status'] = 'FAIL_OWNED_CLEANUP'
                    raise
                finally:
                    try:
                        if self.app_temp is not None:
                            self.app_temp.cleanup()
                            result['ownedLogDirectoryRemoved'] = True
                    finally:
                        fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                        lock.close()
                        result['seconds'] = round(time.monotonic() - began, 3)
                        REPORT.parent.mkdir(parents=True, exist_ok=True)
                        with REPORT.open('x') as output:
                            output.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
