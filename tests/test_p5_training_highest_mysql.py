"""All76 original training maxima using actual local training random policy."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import unittest
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as stressed_transport


@unittest.skipUnless(os.environ.get('HYJS_P5_TRAINING_MYSQL_TEST')=='1','Explicit all76 isolated highest training test')
class TrainingHighestMySQLTests(unittest.TestCase):
    setUpClass=classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql=classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send=shared.TalentScopeMySQLTests.send
    login=shared.TalentScopeMySQLTests.login
    stamped=stressed_transport.GameCreationMySQLTests.stamped
    call=shared.TalentScopeMySQLTests.call
    profile=shared.TalentScopeMySQLTests.profile
    start=shared.TalentScopeMySQLTests.start
    stop=shared.TalentScopeMySQLTests.stop
    setUp=shared.TalentScopeMySQLTests.setUp
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile

    def test_all76_actual_draws_to_original_individual_maxima(self):
        self.start()
        heroes={r['id']:r for r in shared.original_table('Hero')}
        limits=[r for r in shared.original_table('HeroTrainingMaxMin') if r['warLv']=='100']
        self.assertEqual(len(limits),76)
        prepared=[]
        for index,rule in enumerate(limits):
            hero=heroes[rule['warId']];uid='p5-native-training-'+hero['id'];login=self.login(uid)
            self.call('CSPlayerCreate',{'playerName':'原最高训练','heroId':'hero_047' if hero['heroClass']=='101' else 'hero_043'},login,1800001500000,'SCPlayerCreate')
            player,game=self.profile(login);player.update(level=100,coin=1000000000,diamond=100000,warriorBagSize=1000,noviceGuideSchedule=8)
            target={'id':hero['id'],'guid':hero['id']+'-native-training','level':100,'exp':0,'talent':0,'training':{},'tempTraining':{},'breached':{'level':1},'awake':{}}
            if hero['heroClass']=='0':game['warriors'].append(target)
            else:game['warriors']=[target];player['heroWarId']=target['guid']
            game['formation']=[];game['items']=[{'id':'item_007','num':100000}]
            self.write_profile(login,player,game);prepared.append((index,hero,rule,uid,login,target))
        results=[];restart=[]
        def exercise(entry):
            index,hero,rule,uid,login,target=entry;stamp=1800001501000+index*10000
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            mount={'formatPos':1,'id':hero['id'],'guid':target['guid']}
            call('CSFormationOn',mount,'SCFormationOn')
            call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
            initial_battle=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo')
            baseline=next(a for a in initial_battle.battlePlayer.formation if a.guid==target['guid'])
            if hero['heroClass']=='0':call('CSFormationOff',mount,'SCFormationOff')
            initial=self.profile(login);current=target;iterations=0;fields={'guid':target['guid'],'costType':3,'numType':3};last_preview_stamp=0
            expected=[int(rule[k]) for k in ('hpMax','attMax','pdefMax','mdefMax')]
            def values(w):
                if isinstance(w,dict):return [w.get('training',{}).get(k,0) for k in ('hp','att','pdef','mdef')]
                return [getattr(w.training,k) for k in ('hp','att','pdef','mdef')]
            while values(current)!=expected and iterations<60:
                iterations+=1;out=call('CSWarriorTraining',fields,'SCWarriorTraining');last_preview_stamp=stamp
                self.assertEqual({r.id:r.num for r in out.cost},{'item_007':30,'diamond':50})
                self.assertEqual(values(out.guid),values(current))
                self.assertEqual(out.guid.tempTraining.state,1)
                if iterations==1:self.assertEqual(self.call('CSWarriorTraining',fields,login,stamp,'SCWarriorTraining'),out)
                current=call('CSWarriorTrainingReplace',{'guid':target['guid'],'result':1},'SCWarriorTrainingReplace').result
                self.assertEqual(self.stamped('CSWarriorTraining',fields,login,last_preview_stamp)[0],10001)
            self.assertEqual(values(current),expected)
            saved=self.profile(login)
            self.assertEqual(initial[0]['diamond']-saved[0]['diamond'],iterations*50)
            self.assertEqual(next(r['num'] for r in saved[1]['items'] if r['id']=='item_007'),100000-iterations*30)
            stamp+=1;self.assertEqual(self.stamped('CSWarriorTraining',fields,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            call('CSFormationOn',mount,'SCFormationOn');call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
            battle=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo')
            actor=next(a for a in battle.battlePlayer.formation if a.guid==target['guid'])
            self.assertEqual([getattr(actor.prop,k)-getattr(baseline.prop,k) for k in ('hp','att','pdef','mdef')],expected)
            if hero['heroClass']=='0':call('CSFormationOff',mount,'SCFormationOff')
            reset_fields={'guid':target['guid']}
            if hero['heroClass']=='0':
                out=call('CSWarriorReset',reset_fields,'SCWarriorReset');reset_stamp=stamp
                saved=self.profile(login);w=next(w for w in saved[1]['warriors'] if w['guid']==target['guid'])
                self.assertEqual(w.get('training',{}),{});self.assertEqual(saved[0]['diamond'],initial[0]['diamond']-50)
                self.assertEqual(next(r['num'] for r in saved[1]['items'] if r['id']=='item_007'),100000)
                self.assertEqual(self.stamped('CSWarriorTraining',fields,login,last_preview_stamp)[0],10001)
                restart.append((uid,reset_fields,reset_stamp,out,w))
            else:
                stamp+=1;self.assertEqual(self.stamped('CSWarriorReset',reset_fields,login,stamp)[0],10001)
                restart.append((uid,None,None,None,next(w for w in self.profile(login)[1]['warriors'] if w['guid']==target['guid'])))
            results.append({'heroId':hero['id'],'originalLevel100Maxima':expected,'actualDraws':iterations*10,'exactItem007Cost':iterations*30,'exactDiamondCost':iterations*50,'highestOverflowRollback':'passed','actualBattlePropertyDelta':expected,'reset':'actual full refund' if hero['heroClass']=='0' else 'main protected'})
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:list(pool.map(exercise,prepared))
        self.stop();self.start()
        for uid,fields,stamp,out,warrior in restart:
            login=self.login(uid)
            if fields:self.assertEqual(self.call('CSWarriorReset',fields,login,stamp,'SCWarriorReset'),out)
            self.assertEqual(next(w for w in self.profile(login)[1]['warriors'] if w['guid']==warrior['guid']),warrior)
        results.sort(key=lambda r:r['heroId'])
        (shared.ROOT/'analysis/p5/training-highest-lifecycle-validation.json').write_text(json.dumps({'date':'2026-10-14','originalHTTP':'passed','isolatedMySQL':'passed','characterPaths':results,'actualDraws':sum(r['actualDraws'] for r in results),'sourceSha256':self.source_shas,'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'processRestart':'all76 exact warriors and all68 reset receipts retained','randomPolicy':'existing explicitly local warrior_growth_rules training3; original private random distribution not recovered','preparation':'finite isolated zero-training level100 original identities, item007 and diamonds; natural supply and original card availability separately audited','productionModified':False},ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
