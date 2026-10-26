"""Generate (never execute) an audited CAS patch for reviewed growth activities.

1. --export-sql > read.sql; root executes the SELECTs using the project account.
2. Save the resulting tab-separated table/JSON lines as the private snapshot.
3. --snapshot snapshot.tsv --sql patch.sql --manifest patch.json.

Apply only under the game's maintenance window after the normal encrypted backup.
The final temporary-table CHECK rolls the transaction back on any stale row when
the mysql client stops on errors. No reward delivery, purchase or draw receipt is
deleted or rewritten. CAS updates protect a concurrent state change.
"""
import argparse
import base64
import copy
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TABLES = {
    'admin_activity_templates': ('id', 'mode', 'default_days', 'form_json', 'enabled', 'archived'),
    'admin_activity_occurrences': ('id', 'template_id', 'mode', 'timing', 'start_day', 'duration_days', 'form_json', 'enabled', 'archived', 'consumer_key'),
    'admin_character_activity_instances': ('player_id', 'occurrence_id', 'mode', 'start_at', 'end_at', 'form_json', 'consumer_key'),
    'p4_commerce_rules': ('id', 'kind', 'source', 'enabled'),
    'p1_period_instances': ('id', 'source', 'enabled', 'goal', 'rewards', 'milestones', 'frozen'),
    'p1_period_progress': ('instance_id', 'player_id', 'snapshot'),
    'gameplay_pools': ('id', 'source'),
    'gameplay_pool_rewards': ('pool_id', 'id', 'resource_id', 'amount', 'weight', 'stock', 'guaranteed'),
    'turntable_rules': ('pool_id', 'prize_pity_rules'),
    'turntable_personals': ('pool_id', 'player_id', 'used_stock'),
    'turntable_globals': ('pool_id', 'used_stock'),
    'turntable_states': ('pool_id', 'player_id', 'prize_pity_state'),
    'daily_copy_states': ('player_id', 'day', 'type', 'used', 'limit'),
}
JSON_COLUMNS = {'form_json', 'milestones', 'frozen', 'snapshot', 'prize_pity_rules', 'used_stock', 'prize_pity_state'}
KEYS = {
    'admin_activity_templates': ('id',), 'admin_activity_occurrences': ('id',),
    'admin_character_activity_instances': ('player_id', 'occurrence_id'),
    'p4_commerce_rules': ('id',), 'p1_period_instances': ('id',),
    'p1_period_progress': ('instance_id', 'player_id'), 'gameplay_pools': ('id',),
    'gameplay_pool_rewards': ('pool_id', 'id'), 'turntable_rules': ('pool_id',),
    'turntable_personals': ('pool_id', 'player_id'), 'turntable_globals': ('pool_id',),
    'turntable_states': ('pool_id', 'player_id'),
    'daily_copy_states': ('player_id', 'day', 'type'),
}
SPECIAL = {'item_067', 'item_119', 'item_339', 'item_9760', 'item_9761'}
ORANGE = {'equip_025', 'equip_026', 'equip_027', 'equip_028'}


def compact(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'))


def decoded(raw):
    if raw is None or raw == '':
        return None
    if isinstance(raw, (dict, list)):
        return copy.deepcopy(raw)
    return json.loads(raw)


def wheel(form):
    """Retain prize IDs and the existing original pity mechanism."""
    f = copy.deepcopy(form)
    prizes = f['prizes']
    first = sum(p['resourceId'] == 'item_340' for p in prizes) == 1
    old_hard = max((r['hardPity'] for r in f['options']['prizePityRules']), default=0)
    stage = 1 if first else 2 if old_hard <= 1000 else 3
    for p in prizes:
        i = p['id']
        if i == 1:
            p.update(resourceId='MineItem_079', amount=4, stock=4, weight=1)
        elif i in (2, 3, 4):
            rid, n, weight = {2: ('item_005', 20, 2500), 3: ('item_006', 30, 2500), 4: ('item_012', 50, 3000)}[i]
            p.update(resourceId=rid, amount=n, stock=0, weight=weight)
        elif i == 8:
            p.update(amount=50, weight=2800 if first else 2000)
        elif first and i == 5:
            p.update(amount=20, weight=1800)
        elif first and i == 6:
            p['weight'] = 200
    f['options']['prizePityRules'] = [r for r in f['options']['prizePityRules'] if r['prizeId'] not in (2, 3, 4)]
    for r in f['options']['prizePityRules']:
        if r['prizeId'] == 1:
            hard = (100, 150, 200)[stage - 1]
            r.update(hardPity=hard, softStart=hard * 9 // 10, basePPM=1, softMaxPPM=20000)
    assert len(prizes) == 9 and len({p['id'] for p in prizes}) == 9
    assert sum(p['resourceId'] == 'MineItem_079' for p in prizes) == 1
    return f


def adjust_form(form, daily=False):
    f = copy.deepcopy(form)
    if f.get('mode') == 'turntable':
        return wheel(f)
    if f.get('mode') == 'login-total':
        orange = any(r['resourceId'] in ORANGE for m in f.get('milestones', []) for r in m['rewards'])
        if orange:
            f['milestones'] = [m for m in f['milestones'] if m['goal'] == 1]
    if f.get('mode') in ('recharge-total', 'diamond-spent'):
        for m in f.get('milestones', []):
            rewards = []
            for r in m['rewards']:
                rid = r['resourceId']
                if rid.startswith('item_') and rid[5:].isdigit() and 9770 <= int(rid[5:]) <= 9787:
                    continue
                if rid == 'item_119' and r['amount'] == 2:
                    rewards.extend({'resourceId': f'frag_equip_{n:03d}', 'amount': 10} for n in range(37, 41))
                    continue
                if rid.startswith('frag_'):
                    r['amount'] = min(r['amount'], 10)
                rewards.append(r)
            m['rewards'] = rewards or [{'resourceId': 'coin', 'amount': 20000}]
    if daily and f.get('mode') in ('arena-wins', 'wanted-attempts', 'copy-win'):
        last = f['milestones'][-1]
        last['rewards'] = [r for r in last['rewards'] if r['resourceId'] != 'item_374'] + [{'resourceId': 'item_374', 'amount': 200}]
    return f


def resources(spec):
    return {k: int(n) for k, n in (entry.split(',') for entry in spec.split(';') if entry)}


def adjust_spec(spec):
    out = []
    amounts = resources(spec)
    if amounts.get('item_119') == 2:
        del amounts['item_119']
        for n in range(37, 41):
            amounts[f'frag_equip_{n:03d}'] = 10
    for rid, n in amounts.items():
        if rid.startswith('item_') and rid[5:].isdigit() and 9770 <= int(rid[5:]) <= 9787:
            continue
        out.append(f'{rid},{min(n, 10) if rid.startswith("frag_") else n}')
    return ';'.join(out)


def byte_json(raw):
    """Go's []byte JSON fields are base64 inside a frozen struct."""
    if not raw:
        return [], False
    if isinstance(raw, list):
        return copy.deepcopy(raw), False
    try:
        return json.loads(raw), False
    except json.JSONDecodeError:
        return json.loads(base64.b64decode(raw)), True


def adjust_period(raw, desired_tiers=None):
    cfg = decoded(raw)
    if not cfg:
        return raw
    tiers, encoded = byte_json(cfg.get('milestones', ''))
    has_tiers = bool(tiers)
    if not tiers and cfg.get('rewards'):
        tiers = [dict(goal=cfg.get('goal', 1), rewards=cfg['rewards'])]
    if desired_tiers is not None:
        tiers = copy.deepcopy(desired_tiers)
    if not tiers:
        return raw
    orange = any(set(resources(t['rewards'])) & ORANGE for t in tiers)
    if orange:
        tiers = [t for t in tiers if t['goal'] == 1]
    else:
        for t in tiers:
            t['rewards'] = adjust_spec(t['rewards']) or 'coin,20000'
    if any(not t['rewards'] for t in tiers):
        # Zero-material paid thresholds can remain as nonempty rare-only tiers;
        # a fully empty threshold is removed while remaining order is stable.
        tiers = [t for t in tiers if t['rewards']]
    if tiers:
        cfg['goal'], cfg['rewards'] = tiers[0]['goal'], tiers[0]['rewards']
        if has_tiers:
            cfg['milestones'] = base64.b64encode(compact(tiers).encode()).decode() if encoded else tiers
    return compact(cfg)


def export_sql():
    now = '(UNIX_TIMESTAMP(NOW(3))*1000+(SELECT offset_days*86400000 FROM server_game_clocks WHERE server_id=1))'
    pool_filter = "pool_id IN (SELECT id FROM gameplay_pools WHERE source LIKE '%成长投放V12%')"
    filters = {
        'admin_activity_templates': "CONVERT(form_json USING utf8mb4) LIKE '%成长投放V12%' AND archived=0",
        'admin_activity_occurrences': "CONVERT(form_json USING utf8mb4) LIKE '%成长投放V12%' AND archived=0",
        'admin_character_activity_instances': "CONVERT(form_json USING utf8mb4) LIKE '%成长投放V12%'",
        'p4_commerce_rules': "source LIKE '%成长投放V12%'",
        'p1_period_instances': "source LIKE '%成长投放V12%'",
        'p1_period_progress': "instance_id IN (SELECT id FROM p1_period_instances WHERE source LIKE '%成长投放V12%')",
        'gameplay_pools': "source LIKE '%成长投放V12%'",
        'gameplay_pool_rewards': pool_filter, 'turntable_rules': pool_filter,
        'turntable_personals': pool_filter, 'turntable_globals': pool_filter, 'turntable_states': pool_filter,
        'daily_copy_states': "day=DATE_FORMAT(FROM_UNIXTIME(("+now+"+28800000)/1000),'%Y-%m-%d') AND type IN ('Coin','Advance','TreasureRefineExp','PetFlower','PetBreak','PetEquip','Theatric')",
    }
    for table, columns in TABLES.items():
        args = []
        for col in columns:
            textual = col in JSON_COLUMNS or col in ('pool_id', 'instance_id', 'consumer_key') or (col == 'id' and table != 'gameplay_pool_rewards')
            expr = f'CONVERT(`{col}` USING utf8mb4)' if textual else f'`{col}`'
            args.extend([repr(col), expr])
        print(f"SELECT '{table}',JSON_OBJECT({','.join(args)}) FROM `{table}` WHERE {filters[table]} LIMIT 20000;")


def make_plan(tables):
    changes = []
    disabled_occ = set()
    disabled_consumers = set()
    pool_forms = {}
    period_forms = {}

    def change(table, row, edits, why):
        edits = {k: v for k, v in edits.items() if row.get(k) != v}
        if edits:
            changes.append(dict(table=table, key={k: row[k] for k in KEYS[table]}, before={k: row.get(k) for k in edits}, after=edits, reason=why))

    def disabled_login(f):
        return f.get('mode') == 'login-total' and any(r['resourceId'] in ('item_065', 'item_9760', 'item_9761') for m in f.get('milestones', []) for r in m['rewards'])

    for table in ('admin_activity_templates', 'admin_activity_occurrences'):
        for row in tables[table]:
            f = decoded(row['form_json'])
            if f.get('mode') == 'limited-shop' or disabled_login(f):
                change(table, row, {'enabled': 0}, '撤掉集中礼包及后续个人登录赠礼；保留历史')
                if table == 'admin_activity_occurrences':
                    disabled_occ.add(row['id'])
                    disabled_consumers.add(row['consumer_key'])
            else:
                new = adjust_form(f, row.get('duration_days', row.get('default_days')) == 1)
                if new != f:
                    change(table, row, {'form_json': compact(new)}, '统一七彩奖位、首日橙装或小批次活动奖励')
                if f.get('mode') == 'turntable' and row.get('consumer_key') and not str(row['consumer_key']).startswith('character_plan_'):
                    pool_forms[row['consumer_key']] = new
                if row.get('consumer_key'):
                    period_forms[row['consumer_key']] = new
    for row in tables['admin_character_activity_instances']:
        f = decoded(row['form_json'])
        if row['occurrence_id'] in disabled_occ:
            disabled_consumers.add(row['consumer_key'])
            continue
        new = adjust_form(f, row.get('end_at', 0) - row.get('start_at', 0) == 86400000)
        if new != f:
            change('admin_character_activity_instances', row, {'form_json': compact(new)}, '同步已建立个人目录，领奖及抽奖回执保留')
        if f.get('mode') == 'turntable':
            pool_forms[row['consumer_key']] = new
        period_forms[row['consumer_key']] = new
    for row in tables['p4_commerce_rules']:
        if row['kind'] == 'sale':
            change('p4_commerce_rules', row, {'enabled': 0}, '撤掉限时特惠，改活动副本稳定获取')
    for row in tables['p1_period_instances']:
        edits = {}
        if row['id'] in disabled_consumers:
            edits['enabled'] = 0
        else:
            has_tiers = bool(row['milestones'])
            tiers = decoded(row['milestones']) if has_tiers else [dict(goal=row['goal'], rewards=row['rewards'])]
            orange = any(set(resources(t['rewards'])) & ORANGE for t in tiers)
            desired = period_forms.get(row['id'])
            changed = [dict(goal=m['goal'], rewards=';'.join(r['resourceId']+','+str(r['amount']) for r in m['rewards'])) for m in desired['milestones']] if desired and desired.get('milestones') else ([t for t in tiers if t['goal'] == 1] if orange else [dict(t, rewards=adjust_spec(t['rewards']) or 'coin,20000') for t in tiers])
            changed = [t for t in changed if t['rewards']]
            if changed != tiers:
                if has_tiers:
                    edits['milestones'] = compact(changed)
                if changed:
                    edits['goal'], edits['rewards'] = changed[0]['goal'], changed[0]['rewards']
                if not changed:
                    edits['enabled'] = 0
            if row['frozen']:
                v = adjust_period(row['frozen'], changed)
                if decoded(v) != decoded(row['frozen']):
                    edits['frozen'] = v
        change('p1_period_instances', row, edits, '停止后续橙装/集中养成包，保留已领资产')
    for row in tables['p1_period_progress']:
        if row['instance_id'] not in disabled_consumers and row['snapshot']:
            desired = period_forms.get(row['instance_id'])
            desired_tiers = [dict(goal=m['goal'], rewards=';'.join(r['resourceId']+','+str(r['amount']) for r in m['rewards'])) for m in desired['milestones']] if desired and desired.get('milestones') else None
            new = adjust_period(row['snapshot'], desired_tiers)
            if decoded(new) != decoded(row['snapshot']):
                change('p1_period_progress', row, {'snapshot': new}, '同步本期可领取规则，不动累计值及领奖流水')
    for row in tables['gameplay_pool_rewards']:
        f = pool_forms.get(row['pool_id'])
        if f:
            if not isinstance(row['id'], int) or isinstance(row['id'], bool):
                raise ValueError('Wheel prize id must retain its integer SQL type; re-export the snapshot')
            p = next((p for p in f['prizes'] if p['id'] == row['id']), None)
            if p is None:
                raise ValueError('Wheel prize absent from reviewed form: ' + row['pool_id'] + '/' + str(row['id']))
            change('gameplay_pool_rewards', row, {k: p[v] for k, v in [('resource_id', 'resourceId'), ('amount', 'amount'), ('stock', 'stock'), ('weight', 'weight')]}, '七彩石仅占一位，4颗×4次；其余普通养成资源')
    for row in tables['turntable_rules']:
        f = pool_forms.get(row['pool_id'])
        if f:
            v = compact(f['options']['prizePityRules'])
            if decoded(v) != decoded(row['prize_pity_rules']):
                change('turntable_rules', row, {'prize_pity_rules': v}, '按原递增假随机兑现单个七彩奖位的4次库存')
    for table in ('turntable_personals', 'turntable_globals'):
        for row in tables[table]:
            if row['pool_id'] not in pool_forms or not row['used_stock']:
                continue
            old = decoded(row['used_stock'])
            new = dict(old)
            new['1'] = min(4, sum(int(old.get(str(i), 0)) for i in range(1, 5)))
            for i in (2, 3, 4):
                new[str(i)] = 0
            if old != new:
                change(table, row, {'used_stock': compact(new)}, '合并原四个七彩奖位的已中奖次数；普通奖位不限制库存')
    for row in tables['turntable_states']:
        f = pool_forms.get(row['pool_id'])
        if not f or not row['prize_pity_state']:
            continue
        old = decoded(row['prize_pity_state'])
        new = copy.deepcopy(old)
        hard = next(p['hardPity'] for p in f['options']['prizePityRules'] if p['prizeId'] == 1)
        since = max((int(new.get(str(i), {}).get('since', 0)) for i in range(1, 5)), default=0)
        if '1' in new:
            new['1']['since'] = min(hard - 1, since)
        for i in ('2', '3', '4'):
            if i in new:
                new[i].update(since=0, paused=True)
        if old != new:
            change('turntable_states', row, {'prize_pity_state': compact(new)}, '保留已累积保底，合并七彩奖位并保证新阈值合法')
    for row in tables['daily_copy_states']:
        change('daily_copy_states', row, {'limit': max(2, row['used'])}, '本日活动副本改为每日两次，已完成次数保留且不回收既得奖励')
    return changes


def sql_value(value):
    if value is None:
        return 'NULL'
    if isinstance(value, (bool, int)):
        return str(int(value))
    return "X'" + str(value).encode().hex() + "'"


def render(changes):
    lines = ['-- Reviewed data-only patch; use mysql without --force.', 'CREATE TEMPORARY TABLE growth75_activity_assert (ok INT NOT NULL CHECK(ok=1));', 'START TRANSACTION;', 'SET @growth75_changed=0;']
    for m in changes:
        where = ' AND '.join(f'`{k}` <=> {sql_value(v)}' for k, v in {**m['key'], **m['before']}.items())
        sets = ','.join(f'`{k}`={sql_value(v)}' for k, v in m['after'].items())
        lines.append(f"UPDATE `{m['table']}` SET {sets} WHERE {where};")
        lines.append('SET @growth75_row=ROW_COUNT(); SET @growth75_changed=@growth75_changed+@growth75_row;')
        detail = compact(m)
        target = m['table'] + '/' + '/'.join(str(v) for v in m['key'].values())
        lines.append('INSERT INTO admin_audits(time,actor,action,target,detail) SELECT UNIX_TIMESTAMP(NOW(3))*1000,\'authorized-ops\',\'growth75.activities\',' + sql_value(target[:191]) + ',' + sql_value(detail) + ' WHERE @growth75_row=1;')
    lines.append(f'INSERT INTO growth75_activity_assert(ok) VALUES(@growth75_changed={len(changes)});')
    lines.extend(['COMMIT;', f'SELECT @growth75_changed,{len(changes)};'])
    return '\n'.join(lines) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--export-sql', action='store_true')
    parser.add_argument('--snapshot', type=Path)
    parser.add_argument('--sql', type=Path)
    parser.add_argument('--manifest', type=Path)
    args = parser.parse_args()
    if args.export_sql:
        export_sql()
        return
    if not all((args.snapshot, args.sql, args.manifest)):
        parser.error('--snapshot, --sql and --manifest are required')
    tables = {k: [] for k in TABLES}
    for line in args.snapshot.read_text().splitlines():
        if not line:
            continue
        table, raw = line.split('\t', 1)
        if table not in tables:
            raise ValueError('Unexpected table: ' + table)
        row = json.loads(raw)
        if any(k not in row for k in TABLES[table]):
            raise ValueError('Incomplete row: ' + table)
        tables[table].append(row)
    if not tables['admin_activity_occurrences']:
        raise ValueError('No reviewed activity occurrences in snapshot')
    changes = make_plan(tables)
    args.sql.parent.mkdir(parents=True, exist_ok=True)
    args.manifest.parent.mkdir(parents=True, exist_ok=True)
    args.sql.write_text(render(changes))
    args.manifest.write_text(json.dumps(dict(schema='growth75-activity-cas-v1', sourceSHA256=hashlib.sha256(args.snapshot.read_bytes()).hexdigest(), changes=changes), ensure_ascii=False, indent=2) + '\n')
    print(compact({'changes': len(changes), 'sql': str(args.sql), 'manifest': str(args.manifest), 'executed': False}))


if __name__ == '__main__':
    main()
