"""Replay the original HTTP-cache-before-TCP login order against isolated InnoDB."""
import hashlib
import json
import time
import test_p10_voice_mysql as voice


class P10VoiceLoginMySQLTests(voice.P10VoiceMySQLTests):
    # Reuse the isolated server lifecycle without rerunning unrelated voice suites.
    test_1_actual_innodb_race_pcm_and_all_channel_permissions = None
    test_2_actual_http_tcp_cold_delete_blacklist_and_session = None

    def test_original_login_world_cache_before_tcp(self):
        started = time.monotonic()
        self.start()
        sender = self.login('voice-login-sender-' + self.test_user)
        recipient = self.login('voice-login-recipient-' + self.test_user)
        for account, name in [(sender, '世界录音发送端'), (recipient, '原登录接收端')]:
            self.call('CSPlayerCreate', {'playerName': name, 'heroId': 'hero_047'}, account, 1, 'SCPlayerCreate')
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",20) WHERE id=' + str(sender.playerId) + ';'
                 'UPDATE p10_social_policies SET enabled=1;'
                 'UPDATE p10_channel_policies SET world_enabled=1;'
                 'UPDATE p10_voice_policies SET enabled=1,retention_millis=60000,min_duration_millis=100,source="isolated-original-login-world-v1";')
        before = [self.profile(account) for account in (sender, recipient)]
        sender_socket = self.connect(sender)
        self.addCleanup(sender_socket.close)
        raw = voice.P10VoiceMySQLTests.tone(2100)
        uploaded = self.upload(sender, 'original-login-world', raw, kind=1, scope='world/1')
        name = uploaded['fileName']
        self.tcp_send(sender_socket, 'CSChatTcp', {'chatType': 1, 'isAudio': 2, 'downLoadUrl': name,
                      'audioTime': 2, 'fileSize': len(raw), 'content': ''}, sender)
        self.assertEqual(self.recv(sender_socket)[1].downLoadUrl, name)
        for kind in (0, 1):
            cached = self.call('CSChatCache', {'chatType': kind}, recipient, 10 + kind, 'SCChatCache')
            self.assertEqual([item.audioDownloadUrl for item in cached.chatList if item.isAudio == 2], [name])
        self.download(recipient, name, False)
        receiver_socket = self.connect(recipient)
        self.addCleanup(receiver_socket.close)
        self.assertEqual(self.download(recipient, name), raw)
        sender_socket.close()
        receiver_socket.close()
        self.stop()
        self.start()
        cached = self.call('CSChatCache', {}, recipient, 20, 'SCChatCache')
        self.assertEqual([item.audioDownloadUrl for item in cached.chatList if item.isAudio == 2], [name])
        self.download(recipient, name, False)
        cold_receiver = self.connect(recipient)
        self.addCleanup(cold_receiver.close)
        self.assertEqual(hashlib.sha256(self.download(recipient, name)).hexdigest(), uploaded['sha256'])
        self.assertEqual([self.profile(account) for account in (sender, recipient)], before)
        report = {'status': 'PASS_ORIGINAL_HTTP_CACHE_BEFORE_TCP_INNODB_COLD',
                  'elapsedSeconds': round(time.monotonic() - started, 3),
                  'metadataBeforeTCP': True, 'byteDownloadBeforeTCPDenied': True,
                  'authenticatedOriginalTCPThenExactPCM': True, 'coldOriginalOrderPassed': True,
                  'profileResourcesUnchanged': True, 'pcmSource': 'explicit synthetic fixture; native microphone recorded separately',
                  'productionModified': False, 'binarySHA256': self.fixed_manifest['nativeSHA256']}
        (voice.ROOT / 'analysis/p10/voice-login-initial-http-mysql-20261015.json').write_text(json.dumps(report, indent=2) + '\n')
