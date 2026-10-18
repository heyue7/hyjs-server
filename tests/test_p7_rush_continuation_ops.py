"""Offline operational-plan guard checks; no database or runtime endpoint."""
import copy
import importlib.util
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('p7_ops_test', ROOT / 'tools/p7_rush_continuation_ops.py')
ops = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ops)


class P7RushContinuationOpsTests(unittest.TestCase):
    def setUp(self):
        rules = {}
        for key, (file, source, version) in ops.CONSTS.items():
            rules[key] = {'sourceHash': ops.sha(ops.named_const(ROOT, file, source).encode())}
            if version:
                rules[key]['version'] = ops.named_const(ROOT, file, version)
        self.selected = {'rules': rules, 'stages': ops.expected_stages(ROOT, rules)}
        row = lambda rule, **extra: dict(id=1, enabled=0, sourceHash=rules[rule]['sourceHash'], **extra)
        self.state = {
            'profiles': [dict(id=i, playerHash='a'*64, gameHash='b'*64) for i in [1, 4]],
            'p7_rush_policies': [row('rush', eliteBuyCost=50, trialResetCost=50)],
            'p7_rush_scoped_policies': [row('scoped')],
            'p7_rush_manual_policies': [row('manual'), dict(id=2, enabled=0, sourceHash=rules['eliteManual']['sourceHash'])],
            'p7_section_reward_policies': [row('section', version='local-first-section-v1', publishedAt=0, startAt=0, endAt=0, sectionFrom=1, sectionTo=3, threeReward='item_001,1', sixReward='item_001,2', nineReward='item_001,3')],
            'p7_rush_addition_policies': [row('addition', version=rules['addition']['version'], publishedAt=0, startAt=0, endAt=0, threeOption='peratt,0.03', sixOption='perhp,0.06', nineOption='perpdef,0.09')],
            'p7_rush_continuation_policies': [row('continuation', version=rules['continuation']['version'], firstFloor=4, lastFloor=6)],
            'p7_rush_stage_rules': [dict(value, enabled=0) for value in self.selected['stages'].values()],
            'p6_combat_rules': [dict(id=1, authoritativeEnabled=0, nativeCapabilityHash='', clientHash='c'*64, maxRounds=40, timeoutMillis=10000, sourceHash='d'*64)],
        }

    def test_one_transaction_and_ready_guard_cover_every_update(self):
        query = ops.guarded_activation_query(self.state, self.selected)
        self.assertEqual(query.count('START TRANSACTION'), 1)
        self.assertEqual(query.count('COMMIT'), 1)
        self.assertEqual(query.count('SET @ready=COALESCE'), 1)
        mutations = [line for line in query.splitlines() if line.startswith('UPDATE ')]
        self.assertEqual(len(mutations), 7)
        self.assertTrue(all('AND @ready=1' in line for line in mutations))
        manual = next(line for line in mutations if 'p7_rush_manual_policies' in line)
        self.assertIn('WHERE id=1 AND @ready=1', manual)
        self.assertNotIn('UPDATE p6_', query)
        self.assertFalse(any('offers' in line or 'profiles' in line for line in mutations))
        self.assertEqual(len(re.findall(r'FOR UPDATE', query)), len(ops.SPECS))
        self.assertIn('AND @published>=(SELECT published_at', query)

    def test_missing_or_wrong_source_range_npc_and_profile_refuse_before_sql(self):
        variants = []
        for mutate in [
            lambda x: x['p7_rush_continuation_policies'].clear(),
            lambda x: x['p7_rush_addition_policies'][0].update(sourceHash='0'*64),
            lambda x: x['p7_rush_continuation_policies'][0].update(lastFloor=7),
            lambda x: x['p7_rush_stage_rules'][0].update(enemyHash='0'*64),
            lambda x: x['profiles'][0].update(playerHash=''),
            lambda x: x['p6_combat_rules'][0].update(authoritativeEnabled=1),
            lambda x: x['p7_rush_manual_policies'][1].update(enabled=1),
        ]:
            changed = copy.deepcopy(self.state)
            mutate(changed)
            variants.append(changed)
        for changed in variants:
            with self.assertRaises(RuntimeError):
                ops.guarded_activation_query(changed, self.selected)

    def test_original_enabled_elite_is_preserved_when_shared_gates_already_on(self):
        self.state['p7_rush_policies'][0]['enabled'] = 1
        self.state['p7_rush_scoped_policies'][0]['enabled'] = 1
        self.state['p7_rush_manual_policies'][1]['enabled'] = 1
        self.assertEqual(ops.blockers(self.state, self.selected), [])
        restore = ops.restore_values(self.state, self.selected)
        self.assertEqual([r['id'] for r in restore['p7_rush_manual_policies']], [1])
        self.assertEqual(len(restore['p7_rush_stage_rules']), 18)
        self.assertNotIn('profiles', restore)
        self.assertNotIn('p6_combat_rules', restore)

    def test_extra_enabled_trial_and_duplicate_period_window_never_expand_silently(self):
        unknown = dict(self.state['p7_rush_stage_rules'][0], id='foreign-floor', enabled=1)
        self.state['p7_rush_stage_rules'].append(unknown)
        self.assertTrue(ops.blockers(self.state, self.selected))
        self.state['p7_rush_stage_rules'].pop()
        for table in ['p7_rush_policies', 'p7_rush_scoped_policies', 'p7_rush_manual_policies', 'p7_section_reward_policies', 'p7_rush_addition_policies', 'p7_rush_continuation_policies']:
            self.state[table][0]['enabled'] = 1
        for stage in self.state['p7_rush_stage_rules']:
            stage['enabled'] = 1
        for table in ['p7_section_reward_policies', 'p7_rush_addition_policies']:
            self.state[table][0].update(publishedAt=100, startAt=100, endAt=100+7776000000)
        self.assertTrue(ops.already_active(self.state, self.selected))


if __name__ == '__main__':
    unittest.main()
