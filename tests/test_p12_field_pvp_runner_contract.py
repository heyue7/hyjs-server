"""Pure runner-boundary tests; synthetic messages are not battle/native proof."""
import copy
import hashlib
import unittest

from server.codec import BY_NAME, CLASSES
import test_p12_field_pvp_mysql as runner


class FieldPVPRunnerContractTests(unittest.TestCase):
    def fixture(self):
        response = CLASSES['SCSidePvpFightResult'](fightId='bounded-synthetic', attPlayerId=2,
                                                 defPlayerId=3, winPlayerId=2, code=1)
        replay = CLASSES['SidePvpFightData'](maxRoundCount=40)
        terminal = {'finalActors': [], 'outcome': {'attacker': {'inst': {'anger': 12}},
                                                  'defender': {'inst': {'anger': 5}}}}
        side = CLASSES['SCFieldPlayerState']()
        for camp, player_id, initial_hp, final_hp, ap in [(0, 2, 1000, 750, 12), (1, 3, 10, 0, 5)]:
            source = replay.attPlayer if camp == 0 else replay.defPlayer
            source.playerId = player_id
            source.inst.firstOrder = 1 if camp == 0 else 0
            actor = source.formation.add(id='hero_047', guid='owned-' + str(player_id), pose=3)
            actor.prop.hp = 1000
            actor.inst.curHp = initial_hp
            digest = hashlib.sha256(actor.SerializeToString(deterministic=True)).hexdigest()
            terminal['finalActors'].append({'camp': camp, 'guid': actor.guid, 'heroId': actor.id,
                                            'pose': 3, 'currentHp': final_hp, 'maxHp': 1000,
                                            'initialActorSHA256': digest})
            entry = side.player.add(playerId=player_id, curHp=final_hp / 1000,
                                    anger=ap, state=5 if final_hp == 0 else 0)
            entry.battlePosHps.add(battlePos=3, hpPercent=final_hp / 1000)
        current = CLASSES['SCPlayer'](player={'playerId': 2, 'coin': 102000})
        frames = [(BY_NAME['SCSidePvpFightResult'], response),
                  (BY_NAME['SCFieldPlayerState'], side), (BY_NAME['SCPlayer'], current)]
        return response, replay, terminal, frames

    def test_original_order_and_terminal_projection(self):
        response, replay, terminal, frames = self.fixture()
        result = runner.verify_pk_delivery(frames, response, terminal, 2)
        self.assertEqual(result['terminalActors'], 2)
        self.assertEqual(runner.verify_replay_terminal(replay, terminal), {'0': 250, '1': 10})

    def test_early_side_or_duplicate_result_cannot_pass(self):
        response, _, terminal, frames = self.fixture()
        for bad in [[frames[1], frames[0], frames[2]], [frames[0], *frames]]:
            with self.assertRaises(AssertionError):
                runner.verify_pk_delivery(bad, response, terminal, 2)

    def test_initial_ap_or_foreign_owner_cannot_pass(self):
        response, _, terminal, frames = self.fixture()
        frames[1][1].player[0].anger = 0
        with self.assertRaises(AssertionError):
            runner.verify_pk_delivery(frames, response, terminal, 2)
        response, _, terminal, frames = self.fixture()
        with self.assertRaises(AssertionError):
            runner.verify_pk_delivery(frames, response, terminal, 99)

    def test_changed_source_and_hp_cannot_pass(self):
        response, replay, terminal, frames = self.fixture()
        replay.attPlayer.formation[0].prop.hp += 1
        with self.assertRaises(AssertionError):
            runner.verify_replay_terminal(replay, terminal)
        changed = copy.deepcopy(terminal)
        changed['finalActors'][0]['currentHp'] -= 1
        with self.assertRaises(AssertionError):
            runner.verify_pk_delivery(frames, response, changed, 2)


if __name__ == '__main__':
    unittest.main()
