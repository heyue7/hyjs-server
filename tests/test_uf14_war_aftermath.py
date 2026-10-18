"""Pure synthetic codec/integrity tests; no database, device or battle acceptance."""
import base64
import copy
import json
import gzip
import unittest

from server.codec import message
from tools.native_batch.export_uf14_war_aftermath import ATTEMPT_FIELDS, BLOBS, EXPECTED_FULLHP_PROOF, checked, dump, go_json, project_current, sha, source_snapshot, unpack_input


class UF14WarAftermathTests(unittest.TestCase):
    def participant(self):
        battle = message('BattlePlayer', playerId=9007199254740993, serverId=1)
        battle.inst.anger = 0
        battle.inst.firstOrder = 0
        hero = battle.formation.add(id='hero_047', guid='original-guid', pose=3, skillOrder='047_1,047_2,047_3')
        hero.prop.hp = 100
        hero.prop.att = 17
        hero.inst.curHp = 100
        frame = message('InstUFPlayer', guid=battle.playerId, camp=0, state=0)
        frame.fightInfo.index.append(3)
        frame.fightInfo.hpPos.append(37)
        frame.fightInfo.maxHpPos.append(100)
        frame.fightInfo.anger = 24
        return dict(playerId=battle.playerId, camp=0, leftAt=0,
                    battle=battle.SerializeToString().hex(), frame=frame.SerializeToString().hex(),
                    battleHash=sha(battle.SerializeToString()), frameHash=sha(frame.SerializeToString()))

    def accepted_synthetic_body(self):
        # This synthetic bound journal only tests source integrity. It never
        # computes, creates a real accepted row, or supplies gameplay evidence.
        participant = self.participant()
        first = message('BattlePlayer'); first.ParseFromString(bytes.fromhex(participant['battle']))
        first.inst.firstOrder = 1
        second = message('BattlePlayer'); second.CopyFrom(first)
        second.playerId = 2; second.inst.firstOrder = 0
        typed = message('SCUFFightRecordReplay', fightId='synthetic-fixture', roomId='map_forest_002', randomSeed=7, maxRound=40, attackerDamageFactor=1, defenderDamageFactor=1)
        typed.playerList.add().CopyFrom(first); typed.playerList.add().CopyFrom(second)
        versions = dict(engineVersion='synthetic', clientVersion='synthetic', configurationHash='synthetic')
        frozen = dict(versions, attacker=go_json(first), defender=go_json(second), seed=7, battleType=14, maxRound=40, attackerDamageFactor=1, defenderDamageFactor=1, defenderFirst=False)
        input_raw = dump(frozen)
        outcome = dict(capabilityHash=EXPECTED_FULLHP_PROOF)
        terminal = dict(versions, outcome=outcome)
        blobs = dict(start_pb=typed.SerializeToString(), applied_properties=b'{}', before=dump([dict(player_id=first.playerId), dict(player_id=2)]), after=dump([dict(guid=first.playerId), dict(guid=2)]), terminal=dump(terminal))
        row = dict.fromkeys(ATTEMPT_FIELDS, 0)
        row.update(id='a' * 64, status='settled', player_id=first.playerId, target_id=2, fight_id=typed.fightId, p6_record_id='synthetic', scope_proof=EXPECTED_FULLHP_PROOF, input_hash=sha(input_raw), start_hash=sha(blobs['start_pb']), terminal_hash=sha(blobs['terminal']), offer_hash=sha(b'{}'), source_hash='')
        row.update({name: raw.hex() for name, raw in blobs.items()})
        canonical = {name: base64.b64encode(bytes.fromhex(row[name])).decode() if name in BLOBS else row[name] for name in ATTEMPT_FIELDS}
        row['source_hash'] = sha(dump(canonical))
        record = dict(versions, id='synthetic', source='p13-uf-player14-factor1', sourceFight=typed.fightId, status='computed', authoritative=1, serverResult=0, seed=7, input=input_raw.hex(), inputHash=sha(input_raw), operates='', replayHash=sha(bytes([0])), outcome=dump(outcome).hex(), outcomeHash=sha(dump(outcome)))
        return dict(attempt=row, p6=record, participants=[participant], offer=dict(snapshot='{}', sourceHash=sha(b'{}')))

    def test_bound_journal_accepts_exact_source_and_rejects_tamper(self):
        original = self.accepted_synthetic_body()
        checked(original)
        for branch in ('seed', 'input', 'terminal', 'journal', 'frame', 'proof'):
            with self.subTest(branch=branch):
                altered = copy.deepcopy(original)
                if branch == 'seed': altered['p6']['seed'] = 9
                elif branch == 'input': altered['p6']['input'] += '20'
                elif branch == 'terminal': altered['attempt']['terminal'] += '20'
                elif branch == 'journal': altered['attempt']['settled_at'] += 1
                elif branch == 'frame': altered['participants'][0]['frameHash'] = 'wrong'
                elif branch == 'proof': altered['p6']['outcomeHash'] = 'wrong'
                with self.assertRaises(ValueError): checked(altered)

    def test_exact_current_hp_ap_preserves_original_source(self):
        participant = self.participant()
        original = copy.deepcopy(participant)
        projected = project_current(participant, 1)
        self.assertEqual(projected.formation[0].inst.curHp, 37)
        self.assertEqual(projected.inst.anger, 24)
        self.assertEqual(projected.inst.firstOrder, 1)
        self.assertEqual(projected.formation[0].prop.hp, 100)
        self.assertEqual(projected.formation[0].prop.att, 17)
        self.assertEqual(projected.formation[0].guid, 'original-guid')
        self.assertEqual(projected.formation[0].skillOrder, '047_1,047_2,047_3')
        self.assertEqual(participant, original)
        self.assertEqual(go_json(projected)['playerId'], 9007199254740993)
        self.assertIsInstance(go_json(projected)['playerId'], int)

    def test_dead_left_unknown_max_missing_or_duplicate_pose_rejected(self):
        for branch in ('dead', 'left', 'max', 'missing', 'duplicate', 'identity', 'empty'):
            with self.subTest(branch=branch):
                participant = self.participant()
                frame = message('InstUFPlayer'); frame.ParseFromString(bytes.fromhex(participant['frame']))
                if branch == 'dead':
                    frame.state = 3; frame.fightInfo.hpPos[0] = 0
                elif branch == 'left':
                    participant['leftAt'] = 9
                elif branch == 'max':
                    frame.fightInfo.maxHpPos[0] = 101
                elif branch == 'missing':
                    del frame.fightInfo.hpPos[:]
                elif branch == 'duplicate':
                    frame.fightInfo.index.append(3)
                elif branch == 'identity':
                    frame.guid += 1
                elif branch == 'empty':
                    battle = message('BattlePlayer'); battle.ParseFromString(bytes.fromhex(participant['battle']))
                    del battle.formation[:]; participant['battle'] = battle.SerializeToString().hex()
                participant['frame'] = frame.SerializeToString().hex()
                with self.assertRaises(ValueError):
                    project_current(participant, 1)

    def test_bounded_single_complete_gzip_only(self):
        original = b'{"actual":"source"}'
        self.assertEqual(unpack_input(original), original)
        stored = gzip.compress(original)
        self.assertEqual(unpack_input(stored), original)
        for altered in (stored[:-1], stored + stored, stored + b'garbage'):
            with self.assertRaises(ValueError):
                unpack_input(altered)

    def test_sql_invalid_identity_has_no_callback(self):
        def forbidden(_):
            self.fail('Invalid identity reached SQL')
        for attempt, ids in (("x' OR 1=1", [1]), ('a' * 64, []), ('a' * 64, [1, 1]), ('a' * 64, [0])):
            with self.assertRaises(ValueError):
                source_snapshot(forbidden, attempt, ids)

    def test_atomic_snapshot_cannot_omit_requested_participant(self):
        queries = []
        def read(query):
            queries.append(query)
            return '{"participants":[{"playerId":1}]}'
        with self.assertRaises(ValueError):
            source_snapshot(read, 'a' * 64, [1, 2])
        self.assertEqual(len(queries), 1)
        self.assertTrue(queries[0].startswith('SELECT JSON_OBJECT('))
        self.assertNotIn('profiles', queries[0])
        self.assertNotIn('session_hash', queries[0])


if __name__ == '__main__':
    unittest.main()
