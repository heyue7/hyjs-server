"""Generic scene handshake over original HTTP/TCP and an isolated InnoDB database."""
import os
from pathlib import Path
import secrets
import subprocess
import unittest

import test_p13_mysql as guild
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

SOURCE = Path(os.environ.get('HYJS_SCENE_HANDSHAKE_SOURCE', str(field.ROOT))).resolve()


@unittest.skipUnless(os.environ.get('HYJS_SCENE_HANDSHAKE_MYSQL_TEST') == '1', 'explicit isolated scene test')
class ScenePositionHandshakeMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end = None
    test_2_native_original_frames_private_instances_relogin_end_restart = None

    @classmethod
    def setUpClass(cls):
        if SOURCE != field.ROOT and not (SOURCE.parent == field.ROOT / 'build/candidates' and (SOURCE / 'manifest.json').is_file()):
            raise AssertionError('fixed candidate source guard')
        guild.P13MySQLTests.setUpClass.__func__(cls)

    def test_original_old_town_move_cold_login_current_and_new_room_move(self):
        self.native_root = SOURCE
        result = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', self.binary, '.'], cwd=SOURCE / 'hyjs-server', capture_output=True, text=True, timeout=180)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.start()
        uid = 'scene-handshake-' + secrets.token_hex(5)
        login = self.login(uid)
        self.call('CSPlayerCreate', {'playerName': '换场原协议', 'heroId': 'hero_047'}, login, 1, 'SCPlayerCreate')
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000) WHERE id=' + str(login.playerId) + '; UPDATE p13_guild_policies SET management_enabled=1;')
        self.call('CSUnionCreate', {'name': '换场' + secrets.token_hex(3)}, login, 2, 'SCUnionCreate')
        sock = self.connect(login)
        self.call('CSTransferScene', {'target': 'town_union'}, login, 3, 'SCTransferScene')
        self.drain(sock)
        old = {'pointNew': {'x': -9.357699, 'y': 10.6129045, 'z': 37.3301735}, 'currentTime': 1700000000000}
        self.tcp_send(sock, 'CSMove', old, login)
        self.assertEqual(self.drain(sock), [])
        self.tcp_send(sock, 'CSLoginScene', {}, login)
        frame = self.receive(sock, 'SCLoginScene')
        self.assertEqual(frame.self.position.scene, 'town_union')
        self.assertAlmostEqual(frame.self.position.current.x, 22.85771, places=5)
        self.drain(sock)
        self.assertTrue(self.profile(login)[1]['position_loading'])
        self.stop()
        self.start()
        login = self.login(uid)
        sock = self.connect(login)
        self.drain(sock)
        self.assertTrue(self.profile(login)[1]['position_loading'])
        self.tcp_send(sock, 'CSMove', old, login)
        self.assertEqual(self.drain(sock), [])
        self.tcp_send(sock, 'CSCurrentPosition', {}, login)
        frame = self.receive(sock, 'SCCurrentPosition')
        self.assertAlmostEqual(frame.self.position.current.x, 22.85771, places=5)
        self.assertAlmostEqual(frame.self.position.current.y, 1.192649, places=5)
        self.assertAlmostEqual(frame.self.position.current.z, 22.85441, places=5)
        self.drain(sock)
        self.assertFalse(self.profile(login)[1].get('position_loading', False))
        self.tcp_send(sock, 'CSMove', {'pointNew': {'x': 23, 'y': 1.192649, 'z': 23}, 'currentTime': 1700000000001}, login)
        self.drain(sock)
        point = self.profile(login)[1]['position']['current']
        self.assertEqual((point['x'], point['z']), (23, 23))


if __name__ == '__main__':
    unittest.main()
