"""Actual isolated ordinary book source uses the original phone automatic AI result.
This test exports only frozen battle wire; the root agent owns device execution.
No client victory or books are inserted. Each original native response is consumed
before the next fresh fight is produced, keeping the actual source inventory.
"""
import concurrent.futures
import copy
import hashlib
import json
import os
from pathlib import Path
import time
import unittest
from google.protobuf.json_format import MessageToDict
from server.codec import message
import test_copy_full_scope_mysql as common
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_COPY_BOOK_NATIVE_TEST')=='1','Explicit original phone AI and disposable book-source chain')
class OriginalBookNativeMySQLTests(common.OriginalCopyFullMySQLTests):
    __unittest_skip__=False
    __unittest_skip_why__=""
    test_all_original_chapters_quests_rewards_books_specialty_and_cold_receipts=None

    def test_real_native_boss_books_to_zero_specialty(self):
        folder=shared.ROOT/'analysis/p6/copy-book-native-chain-20261014';folder.mkdir(parents=True,exist_ok=True)
        manifest_path=folder/'manifest.json';self.assertFalse((folder/'000.pb').exists(),'Keep prior immutable native source case; choose a fresh evidence folder before another run')
        began=time.monotonic();self.start();login=self.login('original-native-book-chain');stamp=1800002200000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'原书源真实战斗','heroId':'hero_043'},'SCPlayerCreate')
        player,game=self.profile(login)
        nodes=[row for row in shared.original_table('TalentAdvenced') if row['warId']=='hero_001' and row['talentLevel']!='0']
        material_costs={};donors=1
        for row in nodes:
            for part in row['cost'].split(';'):
                key,num=part.split(',');num=int(num)
                if key=='hero_001':donors+=num
                else:material_costs[key]=material_costs.get(key,0)+num
        target_frag=next(r for r in shared.original_table('Frag') if r['target']=='hero_002')
        supplies={k:v for k,v in material_costs.items() if k!='coin'}
        supplies.update(frag_hero_001=donors*150);supplies[target_frag['id']]=int(target_frag['num'])
        self.assertFalse(any(k in supplies for k in ('item_439','item_440')))
        player.update(level=100,coin=1000000000,diamond=1000,stamina=100000,warriorBagSize=1000,noviceGuideSchedule=8)
        game['items']=[{'id':k,'num':v} for k,v in sorted(supplies.items())]
        # Only mission41/quests49 and the first three encounters of mission42 are
        # explicit isolated prerequisites. This does not certify chapters1..7.
        game['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if int(r['MissionId'])<42]
        game['quests']=[r['questId'] for r in shared.original_table('StoryQuest') if int(r['order'])<50]
        self.write_profile(login,player,game)
        grown=[]
        for _ in range(donors):grown.append(call('CSCombine',{'targetId':'hero_001'},'SCCombine').get.wars[0])
        hero=grown[0];remaining=[w.guid for w in grown[1:]]
        player,game=self.profile(login);next(w for w in game['warriors'] if w['guid']==hero.guid)['level']=100;self.write_profile(login,player,game)
        actual_activations=[]
        for node in nodes:
            costs=[]
            for part in node['cost'].split(';'):
                key,num=part.split(',');num=int(num)
                if key=='hero_001':
                    for _ in range(num):costs.append({'id':hero.id,'guid':remaining.pop(0)})
                else:costs.append({'id':key,'num':num})
            result=call('CSWarriorTalent',{'guid':hero.guid,'talent':node['talentLevel'],'cost':costs},'SCWarriorTalent');actual_activations.append(node['talentLevel']);self.assertEqual(result.guid.guid,hero.guid)
        self.assertFalse(remaining)
        call('CSFormationOn',{'formatPos':2,'id':hero.id,'guid':hero.guid},'SCFormationOn')
        manifest={'date':'2026-10-14','cases':[],'status':'preparing','sourceSha256':self.source_shas,'productionModified':False,'preparation':'actual original fragment combines and paid highest hero001 talents; isolated level100/mission41/quest49 and first3 mission42 encounters prerequisite only; no books439/440 prepared','privatePolicyBoundary':'relative-weights-v1 SQL ordinaryDropOdds compatibility; original private server probability algorithm/SpecialOdds unknown','actualTalentNodes':actual_activations,'actualCombinedOriginalHeroes':donors,'nativeCompared':False,'authoritative':False}
        results=[]
        for index in range(30):
            player,game=self.profile(login);books={x['id']:x['num'] for x in game['items']}
            if books.get('item_439',0)>=20 and books.get('item_440',0)>=15:break
            call('CSCopyEnter',{'chapterId':'8','missionId':'42'},'SCCopyEnter')
            player,game=self.profile(login);game['run']['passed']=['165','166','167'];self.write_profile(login,player,game)
            start=call('CSCopyFightStart',{'chapterId':'8','missionId':'42','taskId':'168'},'SCCopyFightStart');self.assertEqual(len(start.battleMonsterList),1)
            wire=message('SCFightPlayback',fightid=start.fightId,randSeed=start.randSeed,battleType=1,attacker=start.battlePlayer,defender2=start.battleMonsterList[0],maxRound=start.maxRound,attackerFactor=1,defenderFactor=1)
            raw=wire.SerializeToString(deterministic=True);filename=f'{index:03d}.pb';(folder/filename).write_bytes(raw)
            case={'wire':filename,'wireSHA256':hashlib.sha256(raw).hexdigest(),'fightId':start.fightId,'seed':start.randSeed,'battleType':1,'scope':'actual-original-mission42-task168-specialty-books','nativeResult':f'{index:03d}.native.jsonl','status':'awaiting-original-automatic-ai','highestTalents':len(actual_activations),'hero001ActualActor':MessageToDict(next(x for x in start.battlePlayer.formation if x.guid==hero.guid))}
            manifest['cases'].append(case);manifest['status']='awaiting-original-automatic-ai';manifest_path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
            result_path=folder/case['nativeResult'];deadline=time.monotonic()+float(os.environ.get('HYJS_COPY_NATIVE_RESULT_TIMEOUT','900'))
            while True:
                if result_path.exists():
                    try:events=[json.loads(x) for x in result_path.read_text().splitlines() if x]
                    except (ValueError,OSError):events=[]
                    if events and events[-1].get('kind')=='result':break
                self.assertLess(time.monotonic(),deadline,'Await original root-owned phone AI result; no source victory or books invented')
                time.sleep(.2)
            inputs=[e for e in events if e.get('kind')=='input'];self.assertEqual(len(inputs),1)
            self.assertEqual((inputs[0]['fightId'],inputs[0]['seed'],inputs[0]['battleType']),(start.fightId,start.randSeed,1))
            self.assertIn('automatic',inputs[0]['scope'],'Book natural winner requires original automatic PVE AI, not empty recorded controls')
            result=events[-1];self.assertIs(type(result.get('attackerWon')),bool)
            final=next(e['state'] for e in reversed(events) if e.get('kind')=='state')
            party=message('BattlePlayer');party.CopyFrom(start.battlePlayer)
            native_actors={x['heroId']:x for x in final['attacker']['actors']};self.assertEqual(len(native_actors),len(party.formation))
            for actor in party.formation:
                hp=native_actors[actor.id]['attribute']['hp'];self.assertGreaterEqual(hp,0);self.assertLessEqual(hp,actor.prop.hp);actor.inst.curHp=hp
            operate=message('BattleOperate',finalRoundNum=final['round'],attackerTotalLeftHp=sum(x['attribute']['hp'] for x in final['attacker']['actors']),defenderTotalLeftHp=sum(x['attribute']['hp'] for x in final['defender']['actors']))
            for event in events:
                if event.get('kind')=='skill_press':operate.skillOpts.add(roundNum=event['round'],roundTime=event['time'],camp=event['data']['camp'],actorUid=event['data']['actorUid'],skillIndex=event['data']['skillIndex'])
            finish={'fightId':start.fightId,'result':int(result['attackerWon']),'battlePlayer':party,'optRecord':operate}
            before_player,before_game=self.profile(login);stamp+=1
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSCopyFightEnd',finish,login,stamp,'SCCopyFightEnd'),range(8)))
            self.assertTrue(all(r==replies[0] for r in replies));case.update(status='original-native-settled',attackerWon=result['attackerWon'],originalResultSHA256=hashlib.sha256(result_path.read_bytes()).hexdigest(),originalOperationSHA256=hashlib.sha256(operate.SerializeToString(deterministic=True)).hexdigest(),actualGain=MessageToDict(replies[0].gain))
            results.append(copy.deepcopy(case))
            if not result['attackerWon']:
                player,game=self.profile(login);self.assertEqual(game['items'],before_game['items']);self.fail('Original highest-talent actor lost source battle; no books granted')
            if index==0:
                self.stop();self.start();login=self.login('original-native-book-chain');self.assertEqual(self.call('CSCopyFightEnd',finish,login,stamp,'SCCopyFightEnd'),replies[0])
            manifest_path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
        else:self.fail('Actual original drops did not reach20/15 within30 paid source battles')
        zero=call('CSCombine',{'targetId':'hero_002'},'SCCombine').get.wars[0]
        player,game=self.profile(login);target=next(w for w in game['warriors'] if w['guid']==zero.guid);self.assertEqual(target.get('skillRefine',[]),[]);target['level']=75;self.write_profile(login,player,game)
        call('CSFormationOn',{'formatPos':3,'id':zero.id,'guid':zero.guid},'SCFormationOn')
        before=call('CSCopyRunMapInfo',{'chapterId':'8','missionId':'42'},'SCCopyRunMapInfo');before_actor=next(w for w in before.battlePlayer.formation if w.guid==zero.guid)
        fields={'roleGuid':zero.guid,'type':2};stamp+=1;paid_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSUpRoleSkillRefineLevel',fields,login,paid_stamp,'SCUpRoleSkillRefineLevel'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));self.assertEqual([(x.id,x.num) for x in replies[0].cost],[('item_439',20),('item_440',15)])
        after=call('CSCopyRunMapInfo',{'chapterId':'8','missionId':'42'},'SCCopyRunMapInfo');after_actor=next(w for w in after.battlePlayer.formation if w.guid==zero.guid);self.assertEqual(after_actor.prop.mdef-before_actor.prop.mdef,18)
        self.stop();self.start();login=self.login('original-native-book-chain');self.assertEqual(self.call('CSUpRoleSkillRefineLevel',fields,login,paid_stamp,'SCUpRoleSkillRefineLevel'),replies[0])
        manifest.update(status='complete',nativeCompared=False,originalNativeSourceWinsVerified=True,authoritative=False,naturalBooksDirectlyPrepared=False,naturalBookConsumption={'item_439':20,'item_440':15,'mdefAdd':18,'eightConcurrentAndCold':'one actual consumption/identical response'},seconds=round(time.monotonic()-began,3));manifest_path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
        (shared.ROOT/'analysis/p5/copy-book-native-chain-validation.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
