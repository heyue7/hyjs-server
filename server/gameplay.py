"""Private single-player PVE compatibility, using the client's recovered tables.

Combat simulation still runs in the original client. This module is not an
authoritative multiplayer battle engine.
"""
import csv
import json
import math
from pathlib import Path
import secrets


class Gameplay:
    def __init__(self, store):
        self.store = store
        root = Path(__file__).resolve().parent.parent / 'analysis/tables'
        self.tables = {}
        for name in ('Hero', 'CopyMission', 'CopyTask', 'CopyEnemy', 'DemoHero', 'CopyExp', 'UserExp', 'StoryQuest'):
            with (root / f'Table{name}.tsv').open() as file:
                rows = list(csv.reader(file, delimiter='\t'))
            self.tables[name] = [dict(zip(rows[1], r)) for r in rows[5:] if r and r[0]]

    @staticmethod
    def number(row, name, default=0):
        return float(row.get(name) or default)

    def actor(self, row, guid, pose, enemy=False):
        keys = {'att':'Attack', 'hp':'Hp', 'pdef':'PhyDefend', 'mdef':'MagicDefend'} if enemy else {
            'att':'attBase', 'hp':'hpBase', 'pdef':'pdefBase', 'mdef':'mdefBase'}
        prop = {k:self.number(row, v) for k,v in keys.items()}
        prop.update({k:self.number(row,k) for k in ('hit','dodge','cri','antiCri','criFactor','damageAdd','damageReduce')})
        return {'id':row['HeroId' if enemy else 'id'], 'guid':guid,
                'pose':pose, 'level':int(row.get('Level') or 1), 'prop':prop,
                'inst':{'curHp':prop['hp']}, 'bloodNumber':int(row.get('bloodNumber') or 1),
                'skillOrder':','.join(row.get(k,'') for k in ('SkillId1','SkillId2','UltSkillId')),
                'SkillCastChance':self.number(row,'SkillCastChance',100),
                'talentList':[row[f'Talent{i}'] for i in range(6) if row.get(f'Talent{i}')]}

    def player(self, row, game):
        profile = json.loads(row['player'])
        heroes = {h['id']:h for h in self.tables['Hero']}
        warriors = {h['guid']:h for h in game['warriors']}
        formation = []
        for slot in game['formation']:
            for thing in slot['things']:
                if thing['pos'] == 0:
                    warrior = warriors[thing['guid']]
                    actor = self.actor(heroes[warrior['id']], warrior['guid'], slot['battlePos'])
                    formation.append(actor)
        return {'playerId':row['id'], 'serverId':1, 'level':profile['level'],
                'nickName':profile['name'], 'formation':formation, 'inst':{'anger':0}}

    def enter(self, player_id, chapter, mission):
        # Restrict progression until rewards/quest unlocking for subsequent levels is verified.
        if (chapter,mission) != ('1','1'):
            raise ValueError('当前实机联调仅开放第 1 章第 1 关')
        level = next(x for x in self.tables['CopyMission'] if x['MissionId']==mission)
        tasks = [x for x in self.tables['CopyTask'] if x['MissionId']==mission]
        with self.store.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute('SELECT * FROM profiles WHERE id=?',(player_id,)).fetchone()
            game = json.loads(row['game'])
            run = game.get('run')
            if not run or run.get('complete') or run['mission'] != mission:
                game['run'] = {'chapter':chapter, 'mission':mission, 'passed':[], 'complete':False}
                db.execute('UPDATE profiles SET game=? WHERE id=?',(json.dumps(game),player_id))
        run = game['run']
        remaining=[t for t in tasks if t['TaskId'] not in run['passed']]
        jobs=[]
        from .codec import message
        for task in remaining:
            job={}
            for field in message('CopyJobInfo').DESCRIPTOR.fields:
                key = field.name[0].upper()+field.name[1:]
                value = task.get(key,'')
                if value:
                    job[field.name] = value if field.type==9 else (int(float(value)) if field.type==5 else float(value))
            jobs.append(job)
        return {'taskId':remaining[0]['TaskId'], 'jobList':jobs, 'boxList':[], 'sceneName':level['SceneName']}

    def begin(self, player_id, chapter, mission, task_id):
        task = next((x for x in self.tables['CopyTask'] if x['TaskId']==task_id and x['MissionId']==mission),None)
        if task is None:
            raise ValueError('Invalid battle task')
        with self.store.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute('SELECT * FROM profiles WHERE id=?',(player_id,)).fetchone()
            game = json.loads(row['game'])
            run = game.get('run')
            if not run or run['complete'] or (run['chapter'],run['mission']) != (chapter,mission):
                raise ValueError('Enter this mission before starting a fight')
            tasks=[x['TaskId'] for x in self.tables['CopyTask'] if x['MissionId']==mission]
            remaining=[x for x in tasks if x not in run['passed']]
            if not remaining or task_id!=remaining[0]:
                raise ValueError('Complete the previous encounter first')
            active=run.get('fight')
            if active and active['task']==task_id and not active.get('settled'):
                return active['start']
            groups={}
            for enemy in self.tables['CopyEnemy']:
                if enemy['TaskId']==task_id:
                    group=int(enemy['GroupIndex'])
                    groups.setdefault(group,[]).append(self.actor(enemy,'monster-'+enemy['CopyEnemyId'],int(enemy['Location']),True))
            response={'fightId':secrets.token_hex(12), 'randSeed':secrets.randbelow(2**31-1),
                      'fightScene':task['SceneName'], 'maxRound':40, 'battlePlayer':self.player(row,game),
                      'battleMonsterList':[{'monsterId':int(task_id)*100+g, 'serverId':1,
                                           'taskId':task_id,'groupIndex':g,'formation':formation,'inst':{'anger':0}}
                                          for g,formation in sorted(groups.items())]}
            if task.get('IsDemoFight') == '1' and not game['copy']['passedCopyInfo']:
                # Original first-clear helper lineup (Kirito and Asuna for task 4).
                helpers=[self.actor(h,'demo-'+h['DemoHeroId'],int(h['Weight']),True)
                         for h in self.tables['DemoHero'] if h['TaskId']==task_id]
                response['battlePlayer']['formation'].extend(helpers)
            run['fight']={'task':task_id,'start':response,'settled':False}
            db.execute('UPDATE profiles SET game=? WHERE id=?',(json.dumps(game),player_id))
        return response

    def settle(self, player_id, body):
        with self.store.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row=db.execute('SELECT * FROM profiles WHERE id=?',(player_id,)).fetchone()
            game, player=json.loads(row['game']),json.loads(row['player'])
            run=game.get('run',{})
            fight=run.get('fight',{})
            if fight.get('start',{}).get('fightId') != body.fightId:
                raise ValueError('Unknown fight for this player')
            if fight.get('settled'):
                return fight['result']
            if body.result not in (0,1) or body.battlePlayer.playerId != player_id:
                raise ValueError('Invalid fight result')
            expected={h['guid']:h for h in fight['start']['battlePlayer']['formation']}
            observed={h.guid:h for h in body.battlePlayer.formation}
            if set(expected)!=set(observed):
                raise ValueError('Battle lineup changed during combat')
            for guid,h in observed.items():
                hp=h.inst.curHp
                if h.id!=expected[guid]['id'] or not math.isfinite(hp) or not 0<=hp<=expected[guid]['prop']['hp']:
                    raise ValueError('Invalid remaining HP')
            if body.result and not any(h.inst.curHp>0 for h in observed.values()):
                raise ValueError('A winning party needs a surviving hero')
            complete=False
            if body.result:
                run['passed'].append(fight['task'])
                tasks=[x['TaskId'] for x in self.tables['CopyTask'] if x['MissionId']==run['mission']]
                complete=all(t in run['passed'] for t in tasks)
            result={'isCopyComplete':int(complete), 'star':5 if complete else 0,
                    'fightResult':body.result,'gain':{},'currentResource':[],'cost':[]}
            if complete:
                reward=next(x for x in self.tables['CopyExp'] if int(x['level'])==player['level'])
                coin=int(reward['coin'])
                player['coin']+=coin
                exp=int(reward['exp'])
                player['exp']+=exp
                levels={int(x['level']):x for x in self.tables['UserExp']}
                while player['level']+1 in levels and player['exp']>=int(levels[player['level']]['exp']):
                    player['exp']-=int(levels[player['level']]['exp'])
                    player['level']+=1
                    for resource in ('stamina','energy'):
                        player[resource]+=int(levels[player['level']][resource] or 0)
                result['gain']={'simps':[{'id':'coin','num':coin},{'id':'exp','num':exp}]}
                game['copy']['passedCopyInfo']=[{'chapterId':run['chapter'],'missionId':run['mission'],
                                                'star':5,'challengeTimes':1,'isOpen':1}]
                game['copy']['passedChapterInfo']=[{'chapterId':run['chapter'],'star':5}]
                run['complete']=True
            fight['settled']=True
            fight['result']=result
            db.execute('UPDATE profiles SET player=?,game=? WHERE id=?',
                       (json.dumps(player,ensure_ascii=False),json.dumps(game),player_id))
        return result

    def leave(self, player_id):
        with self.store.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row=db.execute('SELECT game FROM profiles WHERE id=?',(player_id,)).fetchone()
            game=json.loads(row['game'])
            game.pop('run',None)
            db.execute('UPDATE profiles SET game=? WHERE id=?',(json.dumps(game),player_id))

    def quest_reward(self, player_id, quest_id):
        if quest_id != 'quest_1':
            raise ValueError('后续任务尚未完成联调')
        with self.store.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row=db.execute('SELECT * FROM profiles WHERE id=?',(player_id,)).fetchone()
            game,player=json.loads(row['game']),json.loads(row['player'])
            if quest_id in game['quests']:
                raise ValueError('任务奖励已领取')
            if not any(x['missionId']=='1' and x['star']>0 for x in game['copy']['passedCopyInfo']):
                raise ValueError('请先完成第 1 关')
            table=next(x for x in self.tables['StoryQuest'] if x['questId']==quest_id)
            rewards=[]
            for part in table['rewards'].split(';'):
                key,num=part.split(',');num=int(num)
                rewards.append({'id':key,'num':num})
                if key in ('coin','diamond'):
                    player[key]+=num
                else:
                    item=next((x for x in game['items'] if x['id']==key),None)
                    if item:
                        item['num']+=num
                    else:
                        game['items'].append({'id':key,'num':num})
            game['quests'].append(quest_id)
            db.execute('UPDATE profiles SET player=?,game=? WHERE id=?',
                       (json.dumps(player,ensure_ascii=False),json.dumps(game),player_id))
        return {'get':{'simps':rewards}}
