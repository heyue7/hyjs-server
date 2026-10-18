from hyjs_managed_mysql import provision_case
"""Verify local public profiles on two actual authenticated original realms."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import time
import unittest

from server.codec import BY_NAME
import test_p14_prayer_typed17_source_mysql as native

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P10_PROFILE_REALM_TEST') == '1', 'explicit isolated two-realm profile test')
class P10ProfileRealmMySQLTests(native.P14PrayerTyped17SourceMySQLTests):
    __unittest_skip__ = False
    test_actual_two_realms_paid_prayer_purpose2_strong_weak_source = None

    @classmethod
    def setUpClass(cls):
        cls.source = Path(os.environ['HYJS_P10_PROFILE_REALM_SOURCE']).resolve()
        assert cls.source.name.startswith('hyjs-p10-profile-realm-')
        cls.prebuilt = cls.source / 'prebuilt'
        cls.meta = json.loads((cls.prebuilt / 'manifest.json').read_text())
        for path, digest in cls.meta['files'].items():
            assert hashlib.sha256((cls.source / path).read_bytes()).hexdigest() == digest, path
        cls.binary = cls.prebuilt / 'hyjs-native'
        assert hashlib.sha256(cls.binary.read_bytes()).hexdigest() == cls.meta['native_sha256']
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
        fcntl.flock(cls.lock, fcntl.LOCK_EX)
        cls.addClassCleanup(lambda: (fcntl.flock(cls.lock, fcntl.LOCK_UN), cls.lock.close()))
        pool, primary = provision_case(cls, 'p13_profile1')
        cls.realm_connections = {1: primary, 2: pool.allocate('p13_profile2')}
        cls.realms = {zone: connection.database for zone, connection in cls.realm_connections.items()}
        cls.env.pop('HYJS_P13_FIELD_NATIVE_CLOCK', None)

    def test_original_local_profile_two_realms_http_tcp_cold(self):
        began = time.monotonic()
        self.shutdownTimes = []
        for zone in [1, 2]:
            self.start(zone)
        uids = {z: 'profile-real-realm-' + str(z) + '-' + secrets.token_hex(4) for z in [1, 2]}
        logins = {z: self.login(z, uids[z]) for z in [1, 2]}
        self.assertEqual(logins[1].playerId, logins[2].playerId)
        for zone in [1, 2]:
            self.call(zone, 'CSPlayerCreate', {'playerName': '资料实服' + str(zone), 'heroId': 'hero_047'}, logins[zone], 2, 'SCPlayerCreate')
        before = {z: self.profiles(z, [logins[z]]) for z in [1, 2]}
        original = {}
        for zone in [1, 2]:
            login = logins[zone]
            original[zone] = self.call(zone, 'CSFindPlayerInfo', {'playerId': login.playerId, 'serverId': zone}, login, 100, 'SCFindPlayerInfo')
            self.assertEqual(original[zone].formationInfo.name, '资料实服' + str(zone))
            self.assertEqual(original[zone], self.call(zone, 'CSFindPlayerInfo', {'playerId': login.playerId, 'serverId': 0}, login, 101, 'SCFindPlayerInfo'))
            sock = self.tcp(zone, login)
            for target in [0, zone]:
                self.tcp_send(zone, sock, 'CSFindPlayerInfo', {'playerId': login.playerId, 'serverId': target}, login)
                op, frame = native.cross.guild.P13MySQLTests.recv(self, sock)
                self.assertEqual((op, frame), (BY_NAME['SCFindPlayerInfo'], original[zone]))
            sock.close()
            for bad in [-1, 3 - zone, 3]:
                self.assertEqual(self.send(zone, 'CSFindPlayerInfo', {'playerId': login.playerId, 'serverId': bad}, login, 102)[0], 10001)
                bad_socket = self.tcp(zone, login)
                self.tcp_send(zone, bad_socket, 'CSFindPlayerInfo', {'playerId': login.playerId, 'serverId': bad}, login)
                # The shared original TCP validator closes invalid requests;
                # its HTTP sibling returns an ErrorMessage frame.
                self.assertEqual(bad_socket.recv(1), b'')
                bad_socket.close()
            self.assertEqual(self.profiles(zone, [login]), before[zone])
        fixed = self.ports[2]
        self.stop(2)
        self.start(2, fixed)
        logins[2] = self.login(2, uids[2])
        self.assertEqual(self.call(2, 'CSFindPlayerInfo', {'playerId': logins[2].playerId, 'serverId': 2}, logins[2], 103, 'SCFindPlayerInfo'), original[2])
        self.assertEqual(self.profiles(2, [logins[2]]), before[2])
        for zone in [1, 2]:
            self.stop(zone)
        proof = {'status': 'PASS_ACTUAL_LOCAL_PROFILE_REALM1_REALM2_HTTP_TCP_COLD', 'seconds': round(time.monotonic() - began, 3), 'sourceProofSHA256': hashlib.sha256((self.prebuilt / 'manifest.json').read_bytes()).hexdigest(), 'sameLocalPlayerID': logins[1].playerId, 'realmNames': [original[z].formationInfo.name for z in [1, 2]], 'validTargets': {'1': [0, 1], '2': [0, 2]}, 'invalidTargets': {'1': [-1, 2, 3], '2': [-1, 1, 3]}, 'originalTCPWithoutHTTPStamp': True, 'invalidTCPClosed': True, 'fullPlayerGameUnchanged': True, 'shutdownSeconds': self.shutdownTimes, 'phone': False, 'productionModified': False}
        (ROOT / 'analysis/p10/profile-realm-native-checkpoint.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
        print('actual original two-realm profiles PASS', proof['seconds'])


if __name__ == '__main__':
    unittest.main()
