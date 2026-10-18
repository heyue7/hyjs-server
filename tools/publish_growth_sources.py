"""Open this month's qualified single-player supplies without changing saves.

This local operator action only changes publication flags. Existing private
activity directories remain immutable; players explicitly refresh Type23 to
include the newly published sources in a new period.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.release_go import operations
from tools.shared_mysql import sql


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def literal(source, name, seen=()):
    if name in seen or len(seen) > 8:
        raise RuntimeError('Invalid publication constant reference')
    match = re.search(r'^const\s+' + re.escape(name) + r'\s*=\s*(.+)$', source, re.M)
    if not match:
        raise RuntimeError('Missing qualified publication constant: ' + name)
    # Read only Go string literals and named string concatenations; never
    # evaluate source text or accept a computed publication contract.
    token = r'"(?:[^"\\]|\\.)*"|[A-Za-z_][A-Za-z_0-9]*'
    if not re.fullmatch(r'\s*(?:' + token + r')(?:\s*\+\s*(?:' + token + r'))*\s*', match[1]):
        raise RuntimeError('Publication constant is not a bounded string: ' + name)
    return ''.join(json.loads(part) if part.startswith('"') else literal(source, part, (*seen, name))
                   for part in re.findall(token, match[1]))


def text_sql(value):
    return 'CONVERT(0x' + value.encode().hex() + ' USING utf8mb4)'


def fingerprint():
    rows = sql('SELECT id,SHA2(player,256),SHA2(game,256) FROM profiles ORDER BY id;'
               'SELECT player_id,balance_cents,revision FROM local_wallet_balances ORDER BY player_id;'
               'SELECT player_id,`key`,kind,delta_cents,balance_after,revision,COALESCE(order_id,\'\'),at,source '
               'FROM local_wallet_entries ORDER BY player_id,`key`;')
    return hashlib.sha256(rows.encode()).hexdigest()


def publish_vip(candidate, qualified, args, vip6=False):
    constants = (candidate / ('hyjs-server/constUtil/p3VIP6Growth.go' if vip6 else 'hyjs-server/constUtil/payment.go')).read_text()
    expected = []
    for vip, price, box in (((6, 700, 40),) if vip6 else ((7, 800, 41), (8, 900, 42), (9, 1000, 43), (10, 1100, 44))):
        expected.append({'id': literal(constants, f'P3VIP{vip}GrowthProduct'),
                         'source': literal(constants, f'P3VIP{vip}GrowthSource'),
                         'vip': vip, 'cost': price, 'reward': f'item_{box:03d},1', 'enabled': 0})
    ids = ','.join(text_sql(row['id']) for row in expected)
    query = ("SELECT JSON_OBJECT('id',id,'source',source,'vip',min_v_ip,'cost',cost,"
             "'reward',reward,'enabled',enabled) FROM p3_vip_shop_products "
             f'WHERE id IN ({ids}) ORDER BY min_v_ip;')
    before = [json.loads(line) for line in sql(query).splitlines()]
    if before != expected:
        raise RuntimeError('VIP source differs, is already published, or has an operator stop; inspect history')
    acceptance_path = args.vip6_acceptance if vip6 else args.vip_acceptance
    acceptance = json.loads(acceptance_path.read_text()) if acceptance_path else {}
    if vip6 and (acceptance.get('actualParcelID') != 'item_040'
            or acceptance.get('actualPaidVIP', 0) < 6
            or any(sorted(acceptance.get(key, [])) != ['trea_011', 'trea_013'] for key in
                   ('actualTreasureIDs', 'actualRefinedTreasureIDs', 'actualResetTreasureIDs'))):
        raise RuntimeError('Require original VIP6 parcel and both actual treasure/refine/reset consumers')
    if (acceptance.get('status') != 'PASS' or acceptance.get('binarySHA256') != qualified['binarySHA256']
            or not acceptance.get('fullChain')):
        raise RuntimeError('Require the same-binary actual wallet/orders/books/skills acceptance')
    if not args.apply:
        print(json.dumps({'ready': True, 'vipGrowth': [row['id'] for row in expected]}))
        return
    if args.record.exists() or not args.record.resolve().is_relative_to(ROOT / 'analysis'):
        raise RuntimeError('Preserve earlier activation records')
    saved = fingerprint()
    joins, conditions, updates = [], [], []
    for index, row in enumerate(expected):
        alias = 'v' + str(index)
        if index:
            joins.append(f'JOIN p3_vip_shop_products {alias} ON {alias}.id={text_sql(row["id"])}')
        conditions.extend([f'{alias}.id={text_sql(row["id"])}', f'{alias}.source={text_sql(row["source"])}',
                           f'{alias}.min_v_ip={row["vip"]}', f'{alias}.cost={row["cost"]}',
                           f'{alias}.reward={text_sql(row["reward"])}', f'{alias}.enabled=0'])
        updates.append(f'{alias}.enabled=1')
    changed = sql('START TRANSACTION; UPDATE p3_vip_shop_products v0 ' + ' '.join(joins)
                  + ' SET ' + ','.join(updates) + ' WHERE ' + ' AND '.join(conditions)
                  + '; SELECT ROW_COUNT(); COMMIT;').strip()
    if changed != str(len(expected)):
        raise RuntimeError('Atomic VIP publication guard rejected changed state')
    after = [json.loads(line) for line in sql(query).splitlines()]
    if after != [{**row, 'enabled': 1} for row in expected] or fingerprint() != saved:
        raise RuntimeError('VIP publication verification failed; inspect before further actions')
    args.record.parent.mkdir(parents=True, exist_ok=True)
    args.record.write_text(json.dumps({'serverSHA256': qualified['binarySHA256'],
        'operatorToolSHA256': digest(Path(__file__)), 'acceptanceSHA256': digest(acceptance_path),
        'before': before, 'after': after, 'onlyEnabledChanged': True, 'profilesAndWalletUnchanged': True,
        'accountFingerprintSHA256': saved, 'existingDailyQuotesAndClaimsUnchanged': True,
        'realVIPAndPaymentRequired': True, 'externalCharge': False}, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'publishedVIPGrowth': True, 'record': str(args.record)}))


def publish_first_double(candidate, qualified, args):
    constants = (candidate / 'hyjs-server/constUtil/firstDouble.go').read_text()
    version = literal(constants, 'FirstDoubleVersion')
    source = literal(constants, 'FirstDoubleSource')
    acceptance = json.loads(args.double_acceptance.read_text()) if args.double_acceptance else {}
    if (acceptance.get('status') != 'PASS' or acceptance.get('binarySHA256') != qualified['binarySHA256']
            or not acceptance.get('fullChain')):
        raise RuntimeError('Require same-binary actual lifetime bonus/wallet/concurrency acceptance')
    query = ("SELECT JSON_OBJECT('version',version,'source',source,'enabled',enabled) "
             'FROM first_double_policies WHERE id=1;')
    before = json.loads(sql(query))
    if before != {'version': version, 'source': source, 'enabled': 0}:
        raise RuntimeError('First-double policy differs or is already published; preserve operator history')
    if not args.apply:
        print(json.dumps({'ready': True, 'firstDoublePolicy': version}))
        return
    if args.record.exists() or not args.record.resolve().is_relative_to(ROOT / 'analysis'):
        raise RuntimeError('Preserve earlier activation records')
    saved = fingerprint()
    changed = sql('UPDATE first_double_policies SET enabled=1 WHERE id=1 AND enabled=0 '
                  f'AND version={text_sql(version)} AND source={text_sql(source)}; SELECT ROW_COUNT();').strip()
    after = json.loads(sql(query))
    if changed != '1' or after != {**before, 'enabled': 1} or fingerprint() != saved:
        raise RuntimeError('First-double activation verification failed; inspect before further actions')
    args.record.parent.mkdir(parents=True, exist_ok=True)
    args.record.write_text(json.dumps({'serverSHA256': qualified['binarySHA256'],
        'operatorToolSHA256': digest(Path(__file__)), 'acceptanceSHA256': digest(args.double_acceptance),
        'before': before, 'after': after, 'onlyEnabledChanged': True, 'profilesAndWalletUnchanged': True,
        'accountFingerprintSHA256': saved, 'productsAndOldOrdersUnchanged': True,
        'historicalFirstPaymentNotReset': True, 'externalCharge': False}, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'publishedFirstDouble': True, 'record': str(args.record)}))


def publish_awake_growth(candidate, qualified, args):
    acceptance = json.loads(args.awake_acceptance.read_text()) if args.awake_acceptance else {}
    if (acceptance.get('status') != 'PASS' or not acceptance.get('fullChain')
            or acceptance.get('binarySHA256') != qualified['binarySHA256']
            or acceptance.get('actualAwakeStage') != 1
            or not isinstance(acceptance.get('original337Consumed'), int)
            or acceptance['original337Consumed'] < 800
            or not acceptance.get('original330Consumed')):
        raise RuntimeError('Require same-binary original337/330 source and actual awakening acceptance')
    return publish_commerce_sources(candidate, qualified, args, args.awake_acceptance, (
        ('p5AwakeMaterialSupply.go', 'P5AwakeMaterialSupplyPrefix', 'P5AwakeMaterialSupplySource'),
        ('p5AwakeElementSupply.go', 'P5AwakeElementSupplyPrefix', 'P5AwakeElementSupplySource')))


def publish_glory_growth(candidate, qualified, args):
    acceptance = json.loads(args.glory_acceptance.read_text()) if args.glory_acceptance else {}
    ids = ['equip_041', 'equip_042', 'equip_043', 'equip_044']
    if (acceptance.get('status') != 'PASS' or not acceptance.get('fullChain')
            or acceptance.get('binarySHA256') != qualified['binarySHA256']
            or any(sorted(acceptance.get(key, [])) != ids for key in
                   ('actualCombinedEquipIDs', 'actualForgedEquipIDs', 'actualRecycledEquipIDs'))):
        raise RuntimeError('Require same-binary four original fragment/combine/forge/recycle acceptance')
    return publish_commerce_sources(candidate, qualified, args, args.glory_acceptance, (
        ('p5GloryFragmentSupply.go', 'P5GloryFragmentSupplyPrefix', 'P5GloryFragmentSupplySource'),))


def publish_pet_growth(candidate, qualified, args):
    acceptance = json.loads(args.pet_acceptance.read_text()) if args.pet_acceptance else {}
    if (acceptance.get('status') != 'PASS' or not acceptance.get('fullChain')
            or acceptance.get('binarySHA256') != qualified['binarySHA256']
            or acceptance.get('actualPetID') != 'pet_002'
            or acceptance.get('actualFoodPurchased') != 36
            or acceptance.get('actualFoodConsumed') != 32
            or acceptance.get('actualIntimacyLevel') != 1
            or acceptance.get('actualIntimacyExp') != 14
            or acceptance.get('actualRandomTalentCount') != 1):
        raise RuntimeError('Require same-binary original pet/paid food/feed/talent acceptance')
    return publish_commerce_sources(candidate, qualified, args, args.pet_acceptance, (
        ('p9PetFoodSupply.go', 'P9PetFoodSupplyPrefix', 'P9PetFoodSupplySource'),))


def publish_commerce_sources(candidate, qualified, args, acceptance_path, supplies):
    """Open only exact qualified global supply flags in one guarded update."""
    month = time.strftime('%Y%m', time.gmtime(time.time()+8*3600))
    expected = {}
    for filename, prefix, source in supplies:
        constants = (candidate/'hyjs-server/constUtil'/filename).read_text()
        expected[literal(constants, prefix)+month] = literal(constants, source)
    ids = ','.join(text_sql(key) for key in expected)
    query = ("SELECT JSON_OBJECT('id',CAST(id AS CHAR CHARACTER SET utf8mb4),'source',source,"
             "'enabled',enabled,'kind',CAST(kind AS CHAR CHARACTER SET utf8mb4),'shop',shop_type,"
             "'rulesSHA256',SHA2(rules,256),'frozenSHA256',SHA2(frozen,256)) "
             f'FROM p4_commerce_rules WHERE id IN ({ids}) ORDER BY id;')
    before = [json.loads(line) for line in sql(query).splitlines()]
    if (len(before) != len(expected) or any(row['id'] not in expected or row['source'] != expected[row['id']]
            or row['enabled'] != 0 or row['kind'] != 'exchange' or row['shop'] != 0 for row in before)):
        raise RuntimeError('Growth publication source/stop state differs; preserve operator history')
    if not args.apply:
        print(json.dumps({'ready': True, 'sources': list(expected), 'privateRefreshRequired': True}))
        return
    if args.record.exists() or not args.record.resolve().is_relative_to(ROOT/'analysis'):
        raise RuntimeError('Preserve earlier activation records')
    saved = fingerprint()
    conditions, joins, updates = [], [], []
    for index, row in enumerate(before):
        alias = 'a'+str(index)
        if index:
            joins.append(f'JOIN p4_commerce_rules {alias} ON {alias}.id={text_sql(row["id"])}')
        conditions.extend([f'{alias}.id={text_sql(row["id"])}', f'{alias}.source={text_sql(row["source"])}',
                           f'{alias}.enabled=0', f'{alias}.kind={text_sql("exchange")}', f'{alias}.shop_type=0',
                           f'SHA2({alias}.rules,256)={text_sql(row["rulesSHA256"])}',
                           f'SHA2({alias}.frozen,256) <=> '+(text_sql(row['frozenSHA256']) if row['frozenSHA256'] else 'NULL')])
        updates.append(f'{alias}.enabled=1')
    changed = sql('START TRANSACTION; UPDATE p4_commerce_rules a0 '+' '.join(joins)
                  +' SET '+','.join(updates)+' WHERE '+' AND '.join(conditions)
                  +'; SELECT ROW_COUNT(); COMMIT;').strip()
    if changed != str(len(expected)):
        raise RuntimeError('Atomic growth publication guard rejected changed state')
    after = [json.loads(line) for line in sql(query).splitlines()]
    if after != [{**row, 'enabled': 1} for row in before] or fingerprint() != saved:
        raise RuntimeError('Growth publication verification failed')
    args.record.parent.mkdir(parents=True, exist_ok=True)
    args.record.write_text(json.dumps(dict(serverSHA256=qualified['binarySHA256'],
        operatorToolSHA256=digest(Path(__file__)), acceptanceSHA256=digest(acceptance_path),
        before=before, after=after, onlyEnabledChanged=True, profilesAndWalletUnchanged=True,
        accountFingerprintSHA256=saved, privateRefreshRequired=True, externalCharge=False),
        ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({'publishedGrowthSupplies': list(expected), 'record': str(args.record)}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--qualification', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--vip-growth', action='store_true', help='Publish only the four existing VIP7..10 growth parcels')
    parser.add_argument('--vip-acceptance', type=Path, help='Actual same-binary orders/books/skills acceptance')
    parser.add_argument('--vip6-growth', action='store_true', help='Publish only the original VIP6 treasure parcel')
    parser.add_argument('--vip6-acceptance', type=Path, help='Same-binary parcel and both treasure consumers acceptance')
    parser.add_argument('--first-double', action='store_true', help='Publish only the explicit lifetime first-double policy')
    parser.add_argument('--double-acceptance', type=Path, help='Actual same-binary wallet/bonus/concurrency acceptance')
    parser.add_argument('--awake-growth', action='store_true', help='Publish only the original330/337 awakening supplies')
    parser.add_argument('--awake-acceptance', type=Path, help='Same-binary original boxes and actual awakening acceptance')
    parser.add_argument('--glory-growth', action='store_true', help='Publish only the four original glory fragment supplies')
    parser.add_argument('--glory-acceptance', type=Path, help='Same-binary four original equipment lifecycles acceptance')
    parser.add_argument('--pet-growth', action='store_true', help='Publish only the first fire-pet food supply')
    parser.add_argument('--pet-acceptance', type=Path, help='Same-binary original first-pet paid food and talent acceptance')
    args = parser.parse_args()
    if sum((args.first_double, args.vip_growth, args.awake_growth, args.glory_growth, args.pet_growth, args.vip6_growth)) > 1:
        parser.error('Choose one publication domain per reviewed action')
    qualified = json.loads(args.qualification.read_text())
    if not qualified.get('qualified') or operations()['binarySha256'] != qualified['binarySHA256']:
        raise RuntimeError('The qualified candidate must already be running')
    candidate = (ROOT / qualified['candidate']).resolve()
    if candidate.parent != ROOT / 'build/candidates':
        raise RuntimeError('Use a project fixed candidate')
    manifest = json.loads((candidate / 'manifest.json').read_text())
    if not manifest.get('stable'):
        raise RuntimeError('The candidate is not stable')
    for row in manifest['files']:
        path = (candidate / row['path']).resolve()
        if not path.is_relative_to(candidate) or digest(path) != row['sha256']:
            raise RuntimeError('Fixed source changed')
    if args.first_double:
        publish_first_double(candidate, qualified, args)
        return
    if args.awake_growth:
        publish_awake_growth(candidate, qualified, args)
        return
    if args.glory_growth:
        publish_glory_growth(candidate, qualified, args)
        return
    if args.pet_growth:
        publish_pet_growth(candidate, qualified, args)
        return
    if args.vip_growth or args.vip6_growth:
        publish_vip(candidate, qualified, args, vip6=args.vip6_growth)
        return
    recruit = (candidate / 'hyjs-server/constUtil/p15RedOSSupplies.go').read_text()
    material = (candidate / 'hyjs-server/constUtil/p5GrowthResourceSupply.go').read_text()
    version = literal(recruit, 'GrowthRecruitVersion')
    source = literal(recruit, 'GrowthRecruitSource')
    red_source = literal(recruit, 'P15RedOSSupplySource')
    mat_source = literal(material, 'P5GrowthResourceSupplySource')
    month = time.strftime('%Y%m', time.gmtime(time.time() + 8 * 3600))
    red_id = literal(recruit, 'P15RedOSSupplyPrefix') + month
    mat_id = literal(material, 'P5GrowthResourceSupplyPrefix') + month
    query = ('SELECT JSON_OBJECT(\'version\',version,\'source\',source,\'enabled\',enabled,'
             "'publishedAt',published_at) FROM growth_recruit_policies WHERE id=1;"
             "SELECT JSON_OBJECT('id',CAST(id AS CHAR CHARACTER SET utf8mb4),'source',source,"
             "'kind',CAST(kind AS CHAR CHARACTER SET utf8mb4),'shop',shop_type,"
             "'enabled',enabled,'rulesSHA256',SHA2(rules,256),'frozenSHA256',SHA2(frozen,256)) "
             f'FROM p4_commerce_rules WHERE id IN ({text_sql(red_id)},{text_sql(mat_id)}) ORDER BY id;')
    rows = [json.loads(line) for line in sql(query).splitlines()]
    if len(rows) != 3 or rows[0]['version'] != version or rows[0]['source'] != source:
        raise RuntimeError('Registered publication policy differs from the qualified source')
    goods = {row['id']: row for row in rows[1:]}
    for key, expected in ((red_id, red_source), (mat_id, mat_source)):
        if key not in goods or goods[key]['source'] != expected or goods[key]['kind'] != 'exchange' or goods[key]['shop'] != 0:
            raise RuntimeError('Monthly supply source mismatch')
    if rows[0]['publishedAt'] or any(row['enabled'] for row in rows):
        raise RuntimeError('Publication already attempted; preserve operator stops and inspect the prior record')
    if not args.apply:
        print(json.dumps({'ready': True, 'month': month, 'sources': [red_id, mat_id], 'privateRefreshRequired': True}))
        return
    if args.record.exists() or not args.record.resolve().is_relative_to(ROOT / 'analysis'):
        raise RuntimeError('Preserve earlier activation records')
    before = fingerprint()
    # All publication bits change together, conditional on the exact source and
    # product bytes read above. No product, save, balance or private period changes.
    conditions = [f'r.source={text_sql(red_source)}', f'm.source={text_sql(mat_source)}',
                  f'p.source={text_sql(source)}', f'p.version={text_sql(version)}',
                  'p.published_at=0', 'p.enabled=0', 'r.enabled=0', 'm.enabled=0']
    for alias, key in (('r', red_id), ('m', mat_id)):
        conditions.append(f'SHA2({alias}.rules,256)={text_sql(goods[key]["rulesSHA256"])}')
        frozen = goods[key]['frozenSHA256']
        conditions.append(f'SHA2({alias}.frozen,256) <=> ' + (text_sql(frozen) if frozen else 'NULL'))
    changed = sql('START TRANSACTION; UPDATE growth_recruit_policies p '
                  f'JOIN p4_commerce_rules r ON r.id={text_sql(red_id)} '
                  f'JOIN p4_commerce_rules m ON m.id={text_sql(mat_id)} '
                  'SET p.enabled=1,p.published_at=CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(3))*1000 AS UNSIGNED),'
                  'r.enabled=1,m.enabled=1 WHERE p.id=1 AND ' + ' AND '.join(conditions) +
                  '; SELECT ROW_COUNT(); COMMIT;').strip()
    if changed != '3':
        raise RuntimeError('Atomic publication guard rejected changed state')
    after = fingerprint()
    if before != after:
        raise RuntimeError('Account state changed during publication; inspect before further actions')
    final = [json.loads(line) for line in sql(query).splitlines()]
    if not all(row['enabled'] for row in final) or not final[0]['publishedAt']:
        raise RuntimeError('Publication could not be verified')
    for old, new in zip(rows[1:], final[1:]):
        if {k: v for k, v in old.items() if k != 'enabled'} != {k: v for k, v in new.items() if k != 'enabled'}:
            raise RuntimeError('Frozen monthly supply changed')
    record = {'serverSHA256': qualified['binarySHA256'], 'qualificationSHA256': digest(args.qualification),
              'operatorToolSHA256': digest(Path(__file__)),
              'month': month, 'before': rows, 'after': final, 'profilesAndWalletUnchanged': True,
              'accountFingerprintSHA256': before, 'privatePeriodsUnchanged': True,
              'privateRefreshRequired': True, 'externalCharge': False}
    args.record.parent.mkdir(parents=True, exist_ok=True)
    args.record.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'published': True, 'record': str(args.record), 'privateRefreshRequired': True}))


if __name__ == '__main__':
    main()
