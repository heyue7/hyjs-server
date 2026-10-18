"""资源副本：原 APK 协议、真实 Go 进程、临时存档和 InnoDB 事务。"""
import concurrent.futures
import csv
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests
from test_go_activity import MySQLActivityTests


class GoDailyCopyTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate
    seed = GoEquipmentTests.seed
    stamped = GoFormationTests.stamped

    def sql(self, query, args=()):
        with self.service.store.connect() as conn:
            return conn.execute(query, args).fetchall()

    def profile(self, login):
        row = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
        return json.loads(row['player']), json.loads(row['game'])

    def create(self, uid, level=100):
        login = self.login(uid)
        self.assertEqual(self.send('CSPlayerCreate', {'playerName': '资源副本', 'heroId': 'hero_043'}, login)[0],
                         BY_NAME['SCPlayerCreate'])
        self.seed(login, level=level, coin=0)
        self.mutate(login, lambda p, g: p.update(stamina=200))
        return login

    def table(self, name):
        path = Path(__file__).resolve().parents[1] / 'analysis/tables' / ('Table' + name + '.tsv')
        with path.open() as f:
            rows = list(csv.reader(f, delimiter='\t'))
        return [dict(zip(rows[1], row)) for row in rows[5:] if row and row[0]]

    def info(self, login):
        op, reply = self.send('CSDailyCopyState', login=login)
        self.assertEqual(op, BY_NAME['SCDailyCopyState'], str(reply))
        return reply

    def type_info(self, login, kind):
        return next(item for item in self.info(login).items if item.type == kind)

    def open_chapter(self, login, kinds=('Coin', 'EquipRefineExp')):
        info = self.info(login)
        item = next(item for item in info.items if item.isOpen == 1 and item.type in kinds)
        return item, item.dailyCopyState[0].ChapterId

    def start_fight(self, login, chapter, stamp=100):
        op, reply = self.stamped('CSDailyCopyFightStart', {'ChapterId': chapter}, login, stamp)
        self.assertEqual(op, BY_NAME['SCDailyCopyFightStart'], str(reply))
        return reply

    def end_fields(self, start, chapter, win=True):
        return {'ChapterId': chapter, 'fightId': start.fightId, 'isWin': int(win),
                'optRecord': {'finalRoundNum': 1, 'attackerTotalLeftHp': 1 if win else 0,
                              'defenderTotalLeftHp': 0 if win else sum(a.prop.hp for a in start.monster.formation)}}

    def finish(self, login, fields):
        op, reply = self.send('CSDailyCopyFightEnd', fields, login)
        self.assertEqual(op, BY_NAME['SCDailyCopyFightEnd'], str(reply))
        return reply

    def test_original_weekday_level_gate_stamina_and_no_query_progress(self):
        login = self.create('daily-copy-gates', level=1)
        before = self.profile(login)
        info = self.info(login)
        day = datetime.now(timezone(timedelta(hours=8))).isoweekday()
        rules = {r['Type']: r for r in self.table('DailyCopy')}
        chapters = {r['ChapterId']: r for r in self.table('DailyCopyChapter')}
        self.assertEqual(len(info.items), 16)
        self.assertEqual(sum(len(i.dailyCopyState) for i in info.items), 48)
        for item in info.items:
            rule = rules[item.type]
            self.assertEqual(item.isOpen, int(str(day) in rule['OpenDay'].split(',')))
            self.assertEqual(item.lastTime, int(rule['Count']))
            for chapter in item.dailyCopyState:
                raw = chapters[chapter.ChapterId]
                self.assertEqual(chapter.isOpen, 0)
                self.assertEqual((chapter.LevelLimit, chapter.Recommend, chapter.Drop),
                                 (int(raw['LevelLimit']), int(raw['Recommend']), raw['Drop']))
        item, chapter = self.open_chapter(login)
        self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': chapter}, login, 100)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.sql('SELECT * FROM daily_copy_states WHERE player_id=?', (login.playerId,)), [])
        self.assertEqual(self.sql('SELECT * FROM daily_copy_start_receipts WHERE player_id=?', (login.playerId,)), [])
        self.mutate(login, lambda p, g: p.update(level=100, stamina=0))
        self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': chapter}, login, 100)[0], 10001)
        self.assertEqual(self.type_info(login, item.type).lastTime, 1)
        closed = next(i for i in self.info(login).items if i.isOpen == 0)
        self.mutate(login, lambda p, g: p.update(stamina=200))
        self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': closed.dailyCopyState[0].ChapterId},
                                     login, 100)[0], 10001)
        self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': '999'}, login, 100)[0], 10001)
        self.assertEqual(self.profile(login)[0]['stamina'], 200)

    def test_concurrent_start_end_restart_and_frozen_policy_with_exact_rewards(self):
        login = self.create('daily-copy-parallel')
        item, chapter = self.open_chapter(login)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            starts = list(pool.map(lambda _: self.stamped('CSDailyCopyFightStart', {'ChapterId': chapter}, login, 100),
                                   range(8)))
        self.assertTrue(all(op == BY_NAME['SCDailyCopyFightStart'] for op, _ in starts), str(starts))
        first = starts[0][1]
        self.assertTrue(all(reply == first for _, reply in starts))
        self.assertEqual((first.roomId, first.maxRoundNum, first.battlePlayer.playerId),
                         ('map_forest_002', 40, login.playerId))
        enemies = [r for r in self.table('DailyCopyEnemy') if r['ChapterId'] == chapter]
        self.assertEqual(len(first.monster.formation), len(enemies))
        for actor, raw in zip(first.monster.formation, enemies):
            self.assertEqual((actor.id, actor.pose, actor.prop.hp, actor.prop.att),
                             (raw['HeroId'], int(raw['Location']), float(raw['Hp']), float(raw['Attack'])))
        cost = int(next(r for r in self.table('DailyCopy') if r['Type'] == item.type)['Stamina'])
        self.assertEqual(self.profile(login)[0]['stamina'], 200 - cost)
        self.assertEqual(self.type_info(login, item.type).lastTime, 0)
        self.assertEqual(self.start_fight(login, chapter, 101), first)
        self.stop(); self.start(); login = self.login('daily-copy-parallel')
        self.assertEqual(self.start_fight(login, chapter, 101), first)
        self.sql("UPDATE daily_copy_policies SET enabled=0,win_stars=1 WHERE id='local_v1'")
        fields = self.end_fields(first, chapter)
        before = self.profile(login)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(lambda _: self.send('CSDailyCopyFightEnd', fields, login), range(8)))
        self.assertTrue(all(op == BY_NAME['SCDailyCopyFightEnd'] for op, _ in results), str(results))
        result = results[0][1]
        self.assertTrue(all(reply == result for _, reply in results))
        # The client receives this authoritative wallet before its animated
        # AddBalanceReward. Preserve the snapshot while applying inventory deltas.
        self.assertEqual(self.finish(login, fields), result)
        self.assertEqual(self.last_frames[1][0], BY_NAME['SCPlayer'])
        self.assertEqual(self.last_frames[1][1].player.coin, self.profile(login)[0]['coin'])
        self.assertEqual((result.isWin, result.starNum, result.lastTime), (1, 5, 0))
        key, amount = next(r for r in self.table('DailyCopyChapter') if r['ChapterId'] == chapter)['Drop'].split(',')
        self.assertEqual([(r.id, r.num) for r in result.get.simps], [(key, int(amount))])
        if key == 'coin':
            self.assertEqual(self.profile(login)[0]['coin'], before[0]['coin'] + int(amount))
        else:
            saved = next(r for r in self.profile(login)[1]['items'] if r['id'] == key)
            self.assertEqual(saved['num'], int(amount))
        self.assertEqual(self.profile(login)[0]['stamina'], 200 - cost)
        self.assertEqual(self.start_fight(login, chapter, 101), first)
        self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': chapter}, login, 102)[0], 10001)
        changed = dict(fields, isWin=0)
        self.assertEqual(self.send('CSDailyCopyFightEnd', changed, login)[0], 10001)
        self.stop(); self.start(); login = self.login('daily-copy-parallel')
        self.assertEqual(self.finish(login, fields), result)

    def test_invalid_owner_hp_round_and_reward_overflow_preserve_unsettled_fight(self):
        login = self.create('daily-copy-validation')
        _, chapter = self.open_chapter(login)
        first = self.start_fight(login, chapter)
        fields = self.end_fields(first, chapter)
        foreign = self.create('daily-copy-foreign')
        self.assertEqual(self.send('CSDailyCopyFightEnd', fields, foreign)[0], 10001)
        self.assertEqual(self.send('CSDailyCopyFightEnd', dict(fields, ChapterId='999'), login)[0], 10001)
        self.assertEqual(self.send('CSDailyCopyFightEnd', dict(fields, fightId='foreign'), login)[0], 10001)
        for key, values in [('attackerTotalLeftHp', [-1, float('nan'), float('inf'), 1e200]),
                            ('defenderTotalLeftHp', [-1, float('nan'), float('inf'), 1e200, 1]),
                            ('finalRoundNum', [0, 41])]:
            for value in values:
                bad = dict(fields, optRecord=dict(fields['optRecord'], **{key: value}))
                self.assertEqual(self.send('CSDailyCopyFightEnd', bad, login)[0], 10001)
        self.assertEqual(self.send('CSDailyCopyFightEnd', dict(fields, optRecord={}), login)[0], 10001)
        row = self.sql('SELECT reward FROM daily_copy_fights WHERE fight_id=?', (first.fightId,))[0]
        key, _ = row['reward'].split(',')
        if key == 'coin':
            self.mutate(login, lambda p, g: p.update(coin=2**63 - 1))
        else:
            self.mutate(login, lambda p, g: g.update(items=[{'id': key, 'num': 2**31 - 1}]))
        before = self.profile(login)
        self.assertEqual(self.send('CSDailyCopyFightEnd', fields, login)[0], 10001)
        self.assertEqual(self.profile(login), before)
        row = self.sql('SELECT settled,response IS NULL AS is_empty FROM daily_copy_fights WHERE fight_id=?', (first.fightId,))[0]
        self.assertEqual(int(row['settled']), 0)
        self.assertEqual(int(row['is_empty']), 1)
        self.mutate(login, lambda p, g: (p.update(coin=0), g.update(items=[])))
        self.assertEqual(self.finish(login, fields).isWin, 1)

    def test_three_attempts_share_difficulties_and_loss_grants_nothing(self):
        login = self.create('daily-copy-three-attempts')
        item, first_chapter = self.open_chapter(login, ('GreatTrea', 'PetEquip'))
        self.assertEqual(item.lastTime, 3)
        # Tuesday's GreatTrea costs 100 per attempt. Fund only this isolated
        # quota test from the original table so all weekdays cover three attempts.
        stamina_cost = int(next(row for row in self.table('DailyCopy') if row['Type'] == item.type)['Stamina'])
        stamina_total = stamina_cost * item.lastTime
        self.mutate(login, lambda p, g: p.update(stamina=stamina_total))
        for index, chapter in enumerate(item.dailyCopyState):
            start = self.start_fight(login, chapter.ChapterId, 100 + index)
            self.assertEqual(self.profile(login)[0]['stamina'], stamina_total - stamina_cost * (index+1))
            if index == 0:
                self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': item.dailyCopyState[1].ChapterId},
                                             login, 101)[0], 10001)
            fields = self.end_fields(start, chapter.ChapterId, win=False)
            before = self.profile(login)
            result = self.finish(login, fields)
            self.assertEqual((result.isWin, result.starNum, result.lastTime), (0, 0, 2-index))
            self.assertFalse(result.get.simps or result.get.wars or result.get.treas)
            self.assertEqual(self.profile(login), before)
            self.assertEqual(self.finish(login, fields), result)
        self.assertEqual(self.type_info(login, item.type).lastTime, 0)
        # Keep enough stamina for attempt four, isolating the exhausted daily quota.
        self.mutate(login, lambda p, g: p.update(stamina=stamina_cost))
        before = self.profile(login)
        self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': first_chapter}, login, 103)[0], 10001)
        self.assertEqual(self.profile(login), before)

    def test_material_instances_and_weighted_resources_are_persisted_once(self):
        login = self.create('daily-copy-materials')
        item, chapter = self.open_chapter(login, ('HeroExp', 'TreasureExp'))
        start = self.start_fight(login, chapter)
        fields = self.end_fields(start, chapter)
        result = self.finish(login, fields)
        if item.type == 'HeroExp':
            self.assertEqual([w.id for w in result.get.wars], ['hero_exp_004']*4)
            self.assertEqual(len({w.guid for w in result.get.wars}), 4)
        else:
            self.assertEqual([t.treasureId for t in result.get.treas], ['trea_010']*4)
            self.assertEqual(len({t.guid for t in result.get.treas}), 4)
        before = self.profile(login)
        self.assertEqual(self.finish(login, fields), result)
        self.assertEqual(self.profile(login), before)
        item, chapter = self.open_chapter(login, ('GreatTrea', 'PetEquip'))
        start = self.start_fight(login, chapter, 200)
        row = self.sql('SELECT reward FROM daily_copy_fights WHERE fight_id=?', (start.fightId,))[0]
        rewards = {key: int(amount) for key, amount in (part.split(',') for part in row['reward'].split(';') if part)}
        original = next(r for r in self.table('DailyCopyChapter') if r['ChapterId'] == chapter)['Drop1']
        for part in original.split('|'):
            key, options = part.split(':')
            self.assertIn(rewards.get(key, 0), [int(option.split(',')[0]) for option in options.split(';')])
        result = self.finish(login, self.end_fields(start, chapter))
        self.assertEqual({r.id: r.num for r in result.get.simps}, rewards)

    def test_pending_main_fight_blocks_new_daily_copy_without_charging(self):
        login = self.create('daily-copy-main-exclusion')
        _, chapter = self.open_chapter(login)
        op, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyEnter'], str(entry))
        op, main = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1',
                                               'taskId': entry.jobList[0].taskId}, login)
        self.assertEqual(op, BY_NAME['SCCopyFightStart'], str(main))
        before = self.profile(login)
        self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': chapter}, login, 100)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.sql('SELECT * FROM daily_copy_start_receipts WHERE player_id=?',
                                 (login.playerId,)), [])

    def test_main_entry_without_fight_can_begin_daily_then_main_waits_for_settlement(self):
        login = self.create('daily-copy-main-start-exclusion')
        _, chapter = self.open_chapter(login)
        op, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyEnter'], str(entry))
        start = self.start_fight(login, chapter)
        before = self.profile(login)
        request = {'chapterId': '1', 'missionId': '1', 'taskId': entry.jobList[0].taskId}
        self.assertEqual(self.send('CSCopyFightStart', request, login)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.finish(login, self.end_fields(start, chapter))
        op, main = self.send('CSCopyFightStart', request, login)
        self.assertEqual(op, BY_NAME['SCCopyFightStart'], str(main))
        self.assertTrue(main.fightId)

    def test_active_daily_blocks_new_main_entry_sweep_and_field_fight(self):
        login = self.create('daily-copy-all-new-fight-exclusion')
        _, chapter = self.open_chapter(login)
        op, transfer = self.send('CSTransferScene', {'target': 'field_001'}, login)
        self.assertEqual(op, BY_NAME['SCTransferScene'], str(transfer))
        self.assertEqual(transfer.result, 1)
        op, position = self.send('CSCurrentPosition', login=login)
        self.assertEqual(op, BY_NAME['SCCurrentPosition'], str(position))
        monsters = dict(self.last_frames)[BY_NAME['SCSideMonster']].monsters
        self.assertTrue(monsters)
        # 隔离角色补足已通关五星记录；扫荡仍通过原协议校验与战斗互斥入口。
        def passed(player, game):
            game['copy']['passedCopyInfo'] = [{'chapterId': '1', 'missionId': '1',
                                               'star': 5, 'challengeTimes': 0}]
        self.mutate(login, passed)
        start = self.start_fight(login, chapter)
        before = self.profile(login)
        self.assertEqual(self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.stamped('CSCopySweep', {'chapterId': '1', 'missionId': '1'}, login, 100)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.send('CSSidePveFightStart', {'monsterId': monsters[0].uuid}, login)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.type_info(login, next(r['Type'] for r in self.table('DailyCopyChapter')
                                                  if r['ChapterId'] == chapter)).lastTime, 0)
        self.finish(login, self.end_fields(start, chapter))
        op, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyEnter'], str(entry))
        op, main = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1',
                                               'taskId': entry.jobList[0].taskId}, login)
        self.assertEqual(op, BY_NAME['SCCopyFightStart'], str(main))

    def test_prior_daily_business_day_does_not_block_new_main_fight(self):
        login = self.create('daily-copy-prior-day-exclusion')
        item, chapter = self.open_chapter(login)
        start = self.start_fight(login, chapter)
        # 对真实已开始战斗仅移动业务日，模拟午夜后仍保存的未结算战斗。
        yesterday = (datetime.now(timezone(timedelta(hours=8))) - timedelta(days=1)).date().isoformat()
        self.sql('UPDATE daily_copy_fights SET day=? WHERE fight_id=?', (yesterday, start.fightId))
        self.sql('UPDATE daily_copy_states SET day=? WHERE player_id=? AND type=?',
                 (yesterday, login.playerId, item.type))
        self.assertEqual(self.type_info(login, item.type).lastTime, 1)
        op, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyEnter'], str(entry))
        op, main = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1',
                                               'taskId': entry.jobList[0].taskId}, login)
        self.assertEqual(op, BY_NAME['SCCopyFightStart'], str(main))
        self.assertTrue(main.fightId)
        result = self.finish(login, self.end_fields(start, chapter))
        self.assertEqual(result.lastTime, 0)
        self.assertEqual(self.type_info(login, item.type).lastTime, 1)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_DAILY_COPY_TEST') == '1', '需显式启用独立 MySQL 资源副本回归')
class MySQLDailyCopyTests(GoDailyCopyTests):
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate

    def test_mysql_innodb_daily_quota_and_start_receipt_keys(self):
        names = ('daily_copy_policies', 'daily_copy_states', 'daily_copy_fights', 'daily_copy_start_receipts')
        rows = self.sql("SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ('daily_copy_policies','daily_copy_states','daily_copy_fights','daily_copy_start_receipts')", (self.database,))
        self.assertEqual({r['TABLE_NAME']: r['ENGINE'] for r in rows}, {name: 'InnoDB' for name in names})
        for table, columns in [('daily_copy_states', ['player_id', 'day', 'type']),
                               ('daily_copy_start_receipts', ['player_id', 'stamp'])]:
            rows = self.sql("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION", (self.database, table))
            self.assertEqual([r['COLUMN_NAME'] for r in rows], columns)


del GoEquipmentTests
del GoFormationTests
del MySQLActivityTests
