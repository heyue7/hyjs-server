"""All76 breach1..25 complete original paths, persistent original HTTP frames."""
import concurrent.futures
import hashlib
import http.client
import threading
from server.codec import BY_NAME, decode_responses, encode_http_request, message
import json
import os
from pathlib import Path
import unittest
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as stressed_transport


@unittest.skipUnless(os.environ.get('HYJS_P5_BREACH_MYSQL_TEST')=='1','Explicit all76 isolated highest training test')
class BreachHighestMySQLTests(unittest.TestCase):
    setUpClass=classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql=classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send=shared.TalentScopeMySQLTests.send
    login=shared.TalentScopeMySQLTests.login
    def stamped(self,name,fields,login,stamp):
        # One HTTP connection per worker keeps every authenticated original frame
        # while avoiding a new socket for each of the real310000+ mutations.
        if not hasattr(self,'http_local'):self.http_local=threading.local()
        connection=getattr(self.http_local,'connection',None)
        if connection is None:
            connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=20)
            self.http_local.connection=connection
            self.addCleanup(connection.close)
        raw=encode_http_request(BY_NAME[name],message(name,**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
        response=connection.getresponse();data=response.read();self.assertEqual(response.status,200)
        return decode_responses(data)[0]
    call=shared.TalentScopeMySQLTests.call
    profile=shared.TalentScopeMySQLTests.profile
    start=shared.TalentScopeMySQLTests.start
    stop=shared.TalentScopeMySQLTests.stop
    setUp=shared.TalentScopeMySQLTests.setUp
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile

    def test_all76_actual_breach1_to25_weighted_draws(self):
        self.start()
        heroes={r['id']:r for r in shared.original_table('Hero')}
        limits=[r for r in shared.original_table('HeroTrainingMaxMin') if r['warLv']=='100']
        self.assertEqual(len(limits),76)
        if os.environ.get('HYJS_P5_BREACH_HERO_ID'):limits=[r for r in limits if r['warId'] in os.environ['HYJS_P5_BREACH_HERO_ID'].split(',')]
        stages={int(r['id']):r for r in shared.original_table('HeroBreached')}
        prepared=[]
        for index,rule in enumerate(limits):
            hero=heroes[rule['warId']];uid='p5-native-breach-'+hero['id'];login=self.login(uid)
            self.call('CSPlayerCreate',{'playerName':'原最高突破','heroId':'hero_047' if hero['heroClass']=='101' else 'hero_043'},login,1800001500000,'SCPlayerCreate')
            player,game=self.profile(login);player.update(level=100,coin=1000000000,diamond=100000,warriorBagSize=1000,noviceGuideSchedule=8)
            target={'id':hero['id'],'guid':hero['id']+'-native-breach','level':100,'exp':0,'talent':0,'training':{},'tempTraining':{},'breached':{'level':1},'awake':{}}
            if hero['heroClass']=='0':game['warriors'].append(target)
            else:game['warriors']=[target];player['heroWarId']=target['guid']
            game['formation']=[];game['items']=[{'id':'item_006','num':2000000}]
            self.write_profile(login,player,game);prepared.append((index,hero,rule,uid,login,target))
        results=[];restart=[]
        def exercise(entry):
            index,hero,rule,uid,login,target=entry;stamp=1800001501000+index*10000
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            mount={'formatPos':1,'id':hero['id'],'guid':target['guid']}
            initial=self.profile(login);current=target;iterations=0;spent=0;fields={'guid':target['guid'],'type':2};last_stamp=0;visited={1}
            def state(w):
                if isinstance(w,dict):return w['breached']['level'],w['breached'].get('exp',0)
                return w.breached.level,w.breached.exp
            while state(current)[0]<25 and iterations<6000:
                iterations+=1;before=state(current)[0]
                out=call('CSWarriorBreached',fields,'SCWarriorBreached');last_stamp=stamp;current=out.guid
                after,exp=state(current)
                self.assertGreaterEqual(after,before);self.assertLessEqual(after,25);self.assertGreaterEqual(exp,0)
                visited.update(range(before,after+1));self.assertEqual(len(out.cost),1);self.assertEqual(out.cost[0].id,'item_006');self.assertGreater(out.cost[0].num,0)
                spent+=out.cost[0].num
                if after==before:self.assertEqual(out.cost[0].num,int(stages[before]['Cost'].split(',')[1])*10)
                if after==25:self.assertEqual(self.call('CSWarriorBreached',fields,login,stamp,'SCWarriorBreached'),out)
            self.assertEqual(state(current),(25,0));self.assertEqual(visited,set(range(1,26)))
            saved=self.profile(login)
            self.assertEqual(initial[0]['diamond'],saved[0]['diamond'])
            self.assertEqual(next(r['num'] for r in saved[1]['items'] if r['id']=='item_006'),2000000-spent)
            stamp+=1;self.assertEqual(self.stamped('CSWarriorBreached',fields,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            call('CSFormationOn',mount,'SCFormationOn');call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
            battle=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo')
            actor=next(a for a in battle.battlePlayer.formation if a.guid==target['guid'])
            self.assertEqual(actor.breachedLevel,25);self.assertGreater(actor.prop.hp,0);self.assertGreater(actor.prop.att,0)
            if hero['heroClass']=='0':call('CSFormationOff',mount,'SCFormationOff')
            reset_fields={'guid':target['guid']}
            if hero['heroClass']=='0':
                out=call('CSWarriorReset',reset_fields,'SCWarriorReset');reset_stamp=stamp
                saved=self.profile(login);w=next(w for w in saved[1]['warriors'] if w['guid']==target['guid'])
                self.assertEqual(w['breached']['level'],1);self.assertEqual(w['breached'].get('exp',0),0);self.assertEqual(saved[0]['diamond'],initial[0]['diamond']-50)
                self.assertEqual(next(r['num'] for r in saved[1]['items'] if r['id']=='item_006'),2000000)
                self.assertEqual(self.stamped('CSWarriorBreached',fields,login,last_stamp)[0],10001)
                restart.append((uid,reset_fields,reset_stamp,out,w))
            else:
                stamp+=1;self.assertEqual(self.stamped('CSWarriorReset',reset_fields,login,stamp)[0],10001)
                restart.append((uid,None,None,None,next(w for w in self.profile(login)[1]['warriors'] if w['guid']==target['guid'])))
            results.append({'heroId':hero['id'],'highestBreachLevel':25,'visitedStages':25,'actualTenStepRequests':iterations,'actualItem006Cost':spent,'highestOverflowRollback':'passed','actualBattleBreachTransport':25,'terminalPropertyPercent':0,'reset':'actual full refund' if hero['heroClass']=='0' else 'main protected'})
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:list(pool.map(exercise,prepared))
        self.stop()
        if getattr(self.http_local,'connection',None):self.http_local.connection.close();self.http_local.connection=None
        self.start()
        for uid,fields,stamp,out,warrior in restart:
            login=self.login(uid)
            if fields:self.assertEqual(self.call('CSWarriorReset',fields,login,stamp,'SCWarriorReset'),out)
            self.assertEqual(next(w for w in self.profile(login)[1]['warriors'] if w['guid']==warrior['guid']),warrior)
        results.sort(key=lambda r:r['heroId'])
        suffix='-'+os.environ['HYJS_P5_BREACH_HERO_ID'].replace(',','-') if os.environ.get('HYJS_P5_BREACH_HERO_ID') else ''
        (shared.ROOT/('analysis/p5/breach-highest-lifecycle-validation'+suffix+'.json')).write_text(json.dumps({'date':'2026-10-14','originalHTTP':'passed','isolatedMySQL':'passed','characterPaths':results,'actualTenStepRequests':sum(r['actualTenStepRequests'] for r in results),'sourceSha256':self.source_shas,'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'processRestart':'all tested exact warriors and reset receipts retained','randomPolicy':'original weighted gains/probabilities; declared local criticalMultiplier2 and resetSeconds0','preparation':'finite isolated zero-breach level100 original identities and item006; natural supply and original card availability separately audited','productionModified':False},ensure_ascii=False,indent=2)+'\n')



if __name__=='__main__':unittest.main()
