"""Original concurrent character creation on a dedicated disposable InnoDB."""
import concurrent.futures
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message

import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_CREATE_MYSQL_TEST') == '1', 'Explicit isolated creation test')
class GameCreationMySQLTests(unittest.TestCase):
    setUpClass = classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql = classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send = shared.TalentScopeMySQLTests.send
    login = shared.TalentScopeMySQLTests.login
    call = shared.TalentScopeMySQLTests.call
    profile = shared.TalentScopeMySQLTests.profile
    start = shared.TalentScopeMySQLTests.start
    stop = shared.TalentScopeMySQLTests.stop
    setUp = shared.TalentScopeMySQLTests.setUp
    sql = shared.TalentScopeMySQLTests.sql

    def stamped(self, name, fields, login, stamp):
        raw = encode_http_request(BY_NAME[name], message(name, **fields), player_id=login.playerId,
                                  zone_id=1, session=bytes(login.sessionKey), timestamp=stamp)
        # Twelve simultaneous creations also exercise shared fact initialization;
        # allow bounded stressed startup latency without changing any original frame.
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=20)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200)
            return decode_responses(data)[0]
        finally:
            connection.close()

    def test_twelve_accounts_eight_same_account_protected_retry_and_restart(self):
        self.start()
        logins = [self.login('create-concurrent-' + secrets.token_hex(6)) for _ in range(12)]
        fields = {'playerName': '原并发创建', 'heroId': 'hero_043'}
        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
            replies = list(pool.map(lambda account: self.call('CSPlayerCreate', fields, account, 1800000300000, 'SCPlayerCreate'), logins))
        self.assertEqual(len(replies), 12)
        identities = set()
        for account in logins:
            player, game = self.profile(account)
            self.assertEqual(len(game['warriors']), 1)
            self.assertEqual((game['warriors'][0]['id'], game['warriors'][0]['guid']), ('hero_043', player['heroWarId']))
            self.assertEqual((player['coin'], player['diamond']), (0, 0))
            identities.add(player['heroWarId'])
        self.assertEqual(len(identities), 12)
        uid = 'create-single-' + secrets.token_hex(6)
        account = self.login(uid)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.call('CSPlayerCreate', fields, account, 1800000300100, 'SCPlayerCreate'), range(8)))
        self.assertTrue(all(reply == replies[0] for reply in replies))
        saved = self.profile(account)
        self.assertEqual(len(saved[1]['warriors']), 1)
        changed = {'playerName': '重试不同名字', 'heroId': 'hero_047'}
        self.call('CSPlayerCreate', changed, account, 1800000300101, 'SCPlayerCreate')
        self.assertEqual(self.profile(account), saved)
        self.stop()
        self.start()
        account = self.login(uid)
        self.call('CSPlayerCreate', changed, account, 1800000300102, 'SCPlayerCreate')
        self.assertEqual(self.profile(account), saved)
        (shared.ROOT / 'analysis/p5/create-concurrency-validation.json').write_text(json.dumps({
            'date': '2026-10-14', 'independentConcurrentAccounts': 12, 'sameAccountConcurrentCreate': 8,
            'originalHTTP': 'passed', 'uniqueMainIdentities': 12, 'sameAccountCreatesOneIdentity': True,
            'changedHeroAndNameRetryProtection': 'passed', 'serverRestartProtection': 'passed',
            'binarySha256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),
            'profileServiceSha256': self.source_shas['service/profileService.go'],
            'productionModified': False, 'isolatedMySQL': True}, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
