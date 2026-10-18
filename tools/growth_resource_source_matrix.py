"""Read original tables and emit the bounded G1/G6 source/consumer matrix.

No application, database, compiler, phone, or inventory operation. Table
references are evidence candidates; they are never counted as live producers.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def rows(name):
    lines = (ROOT / 'analysis/tables' / ('Table' + name + '.tsv')).read_text().splitlines()
    return [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]


def materials(raw):
    return {key: int(amount) for key, amount in (entry.split(',') for entry in raw.split(';') if entry)}


def audit():
    fish, mine = rows('FishSkill'), rows('MineSkill')
    fields = {row['FieldId']: int(row['levelLimit']) for row in rows('Field') if row['levelLimit']}
    levels = [fields['field_%03d' % n] for n in range(1, 7)]
    raw = []
    for index in range(6):
        raw.append({'id': 'MineItem_%03d' % (7 + index), 'amount': 5,
                    'coin': (index + 1) * 50000, 'minCharacterLevel': levels[index],
                    'dailyPortions': 20, 'consumer': 'CSMineRefine -> original random gem/crystal/powder; actual craft XP'})
    for index in range(6):
        anchor = next(row for row in fish if row['FishItemID'] == 'FishItem_%03d' % (26 + index * 4))
        for group in range(3):
            raw.append({'id': 'FishItem_%03d' % (7 + index + 6 * group), 'amount': 5,
                        'coin': materials(anchor['Material'])['coin'] * 10,
                        'minCharacterLevel': levels[index], 'dailyPortions': 20,
                        'consumer': 'CSFishCooking -> CSFishUsing or original field recovery use'})
    raw.append({'id': 'FishItem_025', 'amount': 1, 'coin': 10000000,
                'minCharacterLevel': levels[5], 'dailyPortions': 1,
                'consumer': 'FishItem_050 original recipe; still requires book065 + actual cooking6'})
    raw_ids = {row['id'] for row in raw}
    assert raw_ids == {row['FishItemID'] for row in fish if row['Type'] == 'fish'} | {
        row['MineItemID'] for row in mine if row['Type'] == 'stone'}
    books = {row['result']: row['itemId'] for row in rows('ItemUse') if row['category'] == '15'}
    recipes = []
    for row in fish:
        if row['Type'] != 'dish':
            continue
        required = materials(row['Material'])
        recipes.append({'id': row['FishItemID'], 'recipe': required,
                        'cookingLevel': int(row['LevelLimit']) if row['LevelLimit'] else None, 'requiredBook': books.get(row['SkillNeeded']),
                        'craftable': bool(required), 'rawSourcesCovered': bool(required) and set(required) <= raw_ids | {'coin'},
                        'effect': {'kind': row['ValueType'], 'originalValue': row['Value']},
                        'sourceLimit': 'gift-only FishItem_051 has no original producer; remains missing' if not required else None})
    refined = set()
    for row in mine:
        if row['Type'] == 'stone':
            for column in ('ExtractMagicStone', 'ExtractPowder', 'ExtractGem'):
                refined.update(re.findall(r'MineItem_\d+', row[column]))
    # Ordinary coloured raw gems are drawn uniformly by the existing declared
    # refinery rule; the same six original gem rows are not inserted directly.
    refined.update(row['MineItemID'] for row in mine if row['Type'] == 'gem' and row['Color'] in set('123456'))
    gems = [row for row in mine if row['Type'] == 'magicGem']
    gem_map = {(row['Color'], int(row['Level'])): row['MineItemID'] for row in gems}
    gem_ids = {row['MineItemID'] for row in gems}
    gem_recipes = []
    for row in gems:
        required = materials(row['Material'])
        if not required:
            step, quantity = (3, 6) if row['Color'] == '7' else (1, 3)
            required = {gem_map[(row['Color'], int(row['Level']) - step)]: quantity}
        assert set(required) <= refined | gem_ids | {'coin'}
        gem_recipes.append({'id': row['MineItemID'], 'colour': int(row['Color']), 'level': int(row['Level']),
                            'craftLevel': int(row['LevelLimit']), 'requiredBook': books.get(row['SkillNeeded']),
                            'request': 'CSGemMake' if row['Material'] else ('CSGemCombineBySix' if row['Color'] == '7' else 'CSGemCombineByThree'),
                            'materials': required, 'originalProperties': row['Value'],
                            'source': 'new six ore channels -> real random refine; lower gem actual craft/combine as needed',
                            'mount': 'CSGemMount / colour-matched actual equipment hole; rainbow7 accepted by original consumer'})
    item_names = {row['itemId']: row['name'] for row in rows('Item')}
    equipment_ids = set()
    for row in rows('EquipStar'):
        equipment_ids.update(materials(row['cost']))
    equipment_ids.update('item_%03d' % n for n in range(8, 12))
    equipment = [{'id': key, 'name': item_names.get(key, key),
                  'consumer': 'original equipment refine' if key in {'item_%03d' % n for n in range(8, 12)} else 'original equipment star1..5',
                  'source': 'coin: actual original Copy/star/wheel; item008: original shop slot10 diamond50/day20; item009/010/011: existing wheel ordinary pool original draw costs and genuine random awards'
                            if key == 'coin' or key in {'item_%03d' % n for n in range(8, 12)} else
                            'original empty-slot Grab + actual 1/2/3h rewards; no manufactured opponent win',
                  'limit': 'source must be independently operated; references alone do not prove natural income'}
                 for key in sorted(equipment_ids)]
    evidence = ['analysis/p8/six-map-cooking-development-20261014.json',
                'analysis/p8/cooking-natural-level-manifest.json', 'analysis/p8/ordinary-gem-thirteen-manifest.json',
                'analysis/p8/gem-thirteen-manifest.json', 'analysis/p5/gem-supply-manifest.json',
                'analysis/p5/star-grab-natural-source-audit.json', 'analysis/p8/vip-book-source-http-mysql-validation.json']
    return {'status': 'STATIC_ORIGINAL_ID_CLOSURE_NOT_NEW_BUSINESS_OR_PHONE_ACCEPTANCE',
            'baseline': 'wallet-tools-r4 app407cb04 / APKf500', 'policy': 'growth-solo-materials-v1 default-off source',
            'private193': 'existing frozen directory stays exact; enabling source requires explicit refresh to adopt all25; daily193 quota retained on date rewind',
            'monthContinuity': 'missing global month inherits latest earlier same-policy global enabled and published frozen products; existing or latest disabled month stays disabled; personal clones excluded',
            'counts': {'rawProducts': len(raw), 'dishes': len(recipes), 'craftableDishes': sum(row['craftable'] for row in recipes),
                       'gemRecipes': len(gem_recipes), 'ordinaryHighestLevel': 13},
            'rawProducts': raw, 'dishes': recipes, 'gems': gem_recipes, 'equipmentMaterials': equipment,
            'characterMaterials': [
                {'id': 'item_005', 'use': 'original talent activation', 'source': 'shop slot13: diamond20, one/day20; actual original chapter star rewards; member/activity qualify independently'},
                {'id': 'item_006', 'use': 'original character breach', 'source': 'shop slot8: diamond20, one/day20; paid earned diamond only; original breach level/XP remain'},
                {'id': 'item_007', 'use': 'original training', 'source': 'shop slot9: diamond30, five/day20; original paid/member/daily actual qualification remains'}],
            'experienceUnits': [{'id': r['id'], 'originalEatExp': int(r['expBase']),
                                 'source': '006 actual mystic product100 diamonds; other IDs require actual recruit/drop/existing reward path, not a new raw25 grant',
                                 'consumer': 'actual CSWarriorStrengthen owned material GUID + coin; class99 units are not playable heroes'}
                                for r in rows('Hero') if r['id'].startswith('hero_exp_')],
            'skillBooks': [
                {'book': 'item_063', 'skill': 'LifeSkill_085', 'box': 'item_042', 'source': 'existing default-off VIP8 parcel,900 diamonds,actual paid VIP prerequisite'},
                {'book': 'item_064', 'skill': 'LifeSkill_086', 'box': 'item_044', 'source': 'existing default-off VIP10 parcel,1100 diamonds,actual paid VIP prerequisite'},
                {'book': 'item_065', 'skill': 'SkillFishItem_051', 'box': 'item_043', 'source': 'existing default-off VIP9 parcel,1000 diamonds,actual paid VIP prerequisite'}],
            'experience': {'character': 'actual original Copy victory/sweep XP; fixed grant not tested here',
                           'partner': 'original free/paid recruits include experience units; existing mystic hero_exp_006 actual diamond commodity; strengthen consumes actual units+coin',
                           'cooking': '25 original recipes actual MakingExp; original level1->6 thresholds unchanged',
                           'gemCraft': 'actual refinery/manufacturing XP; original level1->10 thresholds unchanged'},
            'remaining': ['FishItem_051 gift has no recovered producer',
                          'books063/064/065 require existing default-off VIP8/10/9 paid parcels; no fake skill or VIP',
                          'ore refine is genuinely random; finite batch does not guarantee a selected coloured recipe',
                          'high-grade13 original coin/day/month source takes real repeated purchases; prior finite27boxes proof is not a natural source proof',
                          'legacy high-tier10 ingredient source retains its own frozen65/75/85 gates and limits; new named25 source does not rewrite it',
                          'new25 source and private-chain tests still need one qualified Go/MySQL/phone batch',
                          'chapter1 star gear001 has no gem hole; this new batch cannot claim natural downstream mount from it'],
            'reusedEvidence': [{'path': p, 'sha256': hashlib.sha256((ROOT / p).read_bytes()).hexdigest(),
                                'scope': 'historical finite prerequisites apply; not a new run'} for p in evidence],
            'tablePins': {name: hashlib.sha256((ROOT / 'analysis/tables' / ('Table' + name + '.tsv')).read_bytes()).hexdigest()
                          for name in ['Field', 'FishSkill', 'MineSkill', 'ItemUse', 'Item', 'Hero', 'EquipStar', 'GrabTimeReward', 'CookingMakeLevelup', 'GemMakeLevelup']}}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists() or not args.output.resolve().is_relative_to(ROOT / 'analysis/growth-first-20261016/resources'):
        raise SystemExit('Use a fresh owned resource analysis record')
    data = audit()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(data['counts'], indent=2))
