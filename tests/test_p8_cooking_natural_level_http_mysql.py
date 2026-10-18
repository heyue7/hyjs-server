"""Fresh original cooking rank through daily bounded real fishing/crafting/use."""
import concurrent.futures
import http.client
from server.codec import BY_NAME, message, encode_http_request, decode_responses
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time
import unittest

import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as transport

OWNED = ['tests/test_p8_cooking_natural_level_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_COOKING_NATURAL_LEVEL_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class CookingNaturalLevelHTTPMySQLTests(unittest.TestCase):
    send = shared.TalentScopeMySQLTests.send
    login = shared.TalentScopeMySQLTests.login
    stamped = transport.GameCreationMySQLTests.stamped
    call = shared.TalentScopeMySQLTests.call
    profile = shared.TalentScopeMySQLTests.profile
    start = shared.TalentScopeMySQLTests.start
    stop = shared.TalentScopeMySQLTests.stop
    setUp = shared.TalentScopeMySQLTests.setUp
    sql = shared.TalentScopeMySQLTests.sql
    write_profile = shared.TalentScopeMySQLTests.write_profile

    @classmethod
    def setUpClass(cls):
        # The lock covers schema creation, every frame, restart and final cleanup.
        cls.database = 'hyjs_test_p8_cooknatess_http_' + secrets.token_hex(6)
        cls.test_user = 'p8minehttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p8-cooknatess-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-cooknatess-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_COOKING_NATURAL_LEVEL_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p5/copy-award-receipt-manifest.json','analysis/p5/copy-award-shared-source-20261015/manifest.json','analysis/p8/revive-receipt-manifest.json','analysis/p5/treasure-award-receipt-manifest.json','analysis/p5/battle-award-projection-manifest.json','analysis/p5/battle-award-shared-source-20261015/manifest.json']:
            dependency=json.loads((shared.ROOT/manifest).read_text());dependency_source=shared.ROOT/dependency.get('snapshot','.')
            for entry in dependency.get('owned',dependency.get('files',[])):
                rel=entry['path']
                if rel.startswith('hyjs-server/'):
                    source_file=shared.ROOT/entry['snapshot'] if 'snapshot' in entry else dependency_source/rel
                    if hashlib.sha256(source_file.read_bytes()).hexdigest()!=entry['sha256']:raise AssertionError('Dependency SHA changed')
                    target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(source_file,target)
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
        cls.frozen_source=source;cls.baseline_source=str(base.relative_to(shared.ROOT))
        (Path(cls.temp.name)/'analysis').symlink_to(shared.ROOT/'analysis',target_is_directory=True)
        cls.source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=source,capture_output=True,timeout=180)
        if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])

        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        fcntl.flock(cls.lock.fileno(), fcntl.LOCK_EX)
        cls.addClassCleanup(cls.lock.close)
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P5 fourcrystals schema/user/grants remain')
            report = shared.ROOT / 'analysis/p8/cooking-natural-level-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_cooknatess_http_') or not cls.test_user.startswith('p8minehttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_fresh_cooking_rank_natural_daily_fishing_and_use(self):
        began=time.monotonic();self.start();uid='actual-cooking-natural-rank';login=self.login(uid);stamp=1800580000000;cold_count=0;gathers=0;crafted=0;gather_rows=[];craft_rows=[];last=None
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(name,fields,response):
            nonlocal stamp
            stamp+=1;old=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call(name,fields,login,old,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],old
        def cold():
            nonlocal login,cold_count
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
        def move(point):
            nonlocal stamp
            stamp+=1;body=encode_http_request(BY_NAME['CSMove'],message('CSMove',pointNew=point,currentTime=stamp),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
            conn=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=60)
            try:
                conn.request('POST','/sao/',body=body,headers={'Content-Type':'application/octet-stream'});reply=conn.getresponse();self.assertEqual(reply.status,200);self.assertFalse(any(op==BY_NAME['ErrorMessage'] for op,_ in decode_responses(reply.read())))
            finally:conn.close()
            self.assertEqual(self.profile(login)[1]['position']['current'],point)
        call('CSPlayerCreate',{'playerName':'原自然料理升级','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,warriorBagSize=1000,equipBagSize=1000,noviceGuideSchedule=8)
        g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['ChapterId']=='27'];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        earned=0
        for box in [0,1]:
            source=call('CSCopyGetStarAward',{'chapterId':'27','starIndex':box},'SCCopyGetStarAward');earned+=sum(x.num for x in source.gain.simps if x.id=='coin')
        self.assertEqual(earned,665000);self.assertEqual(call('CSCookingLevel',{},'SCCookingLevel').level,1)
        call('CSTransferScene',{'target':'field_001'},'SCTransferScene');call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition');p,g=self.profile(login);self.assertEqual(g['field']['side']['tiredness'],200)
        fatigue=0;first_wait=None
        while crafted<24:
            p,g=self.profile(login);counts={x['id']:x['num'] for x in (g.get('cooking') or [])}
            available=next(((raw,dish,n) for raw,dish,n in [('FishItem_007','FishItem_026',counts.get('FishItem_007',0)//5),('FishItem_013+FishItem_019','FishItem_028',min(counts.get('FishItem_013',0),counts.get('FishItem_019',0))//5)] if n>0),None)
            if available:
                raw,dish,n=available;n=min(n,24-crafted);fields={'fish':{'id':dish,'num':n}};out,old=once('CSFishCooking',fields,'SCFishCooking') if crafted==0 else (call('CSFishCooking',fields,'SCFishCooking'),stamp)
                crafted+=n;last=(fields,old,out);craft_rows.append({'dish':dish,'raw':raw,'count':n,'actualCoinCost':(5000 if dish=='FishItem_026' else 15000)*n,'actualXP':25*n,'returnedLevel':out.level,'returnedExp':out.exp});continue
            self.assertLess(fatigue,200,'Originalactual200fatigue exhausted: do not resetquota/preparefish/XP/sourcecoin')
            nodes_raw=self.sql(f'SELECT CONCAT(pos_id,"|",remaining,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_001" ORDER BY pos_id;')
            nodes=[(v.split('|')[0],int(v.split('|')[1]),json.loads(bytes.fromhex(v.split('|')[2]))) for v in nodes_raw.splitlines()]
            selected=next((n for n in nodes if n[1] and n[2]['kind']=='fish' and n[2]['level']==1),None)
            if selected is None:
                self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND scene="field_001" AND remaining=0;');call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition');continue
            pos,remaining,cfg=selected;move(cfg['point']);started=call('CSFieldResourceStartGather',{'posId':pos},'SCFieldResourceStartGather');self.assertEqual(started.result,1)
            if gathers==0:
                saved=self.profile(login);stamp+=1;self.assertEqual(self.stamped('CSFieldGetResource',{},login,stamp)[0],10001);self.assertEqual(self.profile(login),saved);cold();call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition');wait_begin=time.monotonic();time.sleep(cfg['gatherMs']/1000+.1);first_wait=round(time.monotonic()-wait_begin,3)
            else:self.sql(f'UPDATE p8_gather_attempts SET ready_at=1 WHERE player_id={login.playerId} AND active=1;')
            if gathers==0:claimed,_=once('CSFieldGetResource',{},'SCFieldGetResource')
            else:claimed=call('CSFieldGetResource',{},'SCFieldGetResource')
            self.assertEqual(claimed.code,1);gathers+=1;fatigue+=cfg['fatigue'];self.assertEqual(self.profile(login)[1]['field']['side']['tiredness'],200-fatigue);gather_rows.append({'resource':cfg['resourceId'],'actualFatigue':cfg['fatigue'],'actualReward':[{'id':x.id,'num':x.num} for x in claimed.res.simps]})
            if remaining==1:self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND scene="field_001" AND pos_id="{pos}";')
            if gathers%20==0:print({'gathers':gathers,'actualFatigue':fatigue,'naturalDishCount':crafted},flush=True)
        state=call('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((state.level,state.exp),(2,0));self.assertEqual(sum(x['actualXP'] for x in craft_rows),600);actual_coin=sum(x['actualCoinCost'] for x in craft_rows);self.assertEqual(self.profile(login)[0]['coin'],earned-actual_coin)
        cold();state=call('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((state.level,state.exp),(2,0));fields,old,out=last;saved=self.profile(login);self.assertEqual(self.call('CSFishCooking',fields,login,old,'SCFishCooking'),out);self.assertEqual(self.profile(login),saved)
        p,g=self.profile(login);dish='FishItem_026';self.assertGreater(next(x['num'] for x in g['cooking'] if x['id']==dish),0);old_num=next(x['num'] for x in g['cooking'] if x['id']==dish);g['field']['side'].update(curHp=0.4,state=0);g['field']['side'].pop('battlePosHps',None);g['field'].pop('fight',None);g['position']['current']={'x':1000,'y':0,'z':1000};self.write_profile(login,p,g)
        food={'costItem':{'id':dish,'num':1}};used,food_stamp=once('CSHpRecover',food,'SCHpRecover');self.assertAlmostEqual(self.profile(login)[1]['field']['side']['curHp'],0.45);self.assertEqual(next(x['num'] for x in self.profile(login)[1]['cooking'] if x['id']==dish),old_num-1);cold();saved=self.profile(login);self.assertEqual(self.call('CSHpRecover',food,login,food_stamp,'SCHpRecover'),used);self.assertEqual(self.profile(login),saved)
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_NATURAL_COOKING_ONE_TO_TWO_SOURCE_USE_PHONE_PENDING','sourceCoin':earned,'actualCookingCoin':actual_coin,'finalCoin':earned-actual_coin,'actualNaturalXP':600,'naturalCookingLevel':2,'naturalCookingExp':0,'actualDishes':24,'actualDishUsed':dish,'finiteDamagedHpBeforeUse':0.4,'actualHpAfterUse':0.45,'gathers':gathers,'actualFatigue':fatigue,'remainingFatigue':200-fatigue,'fatigueQuotaNeverReset':True,'firstRealWait':first_wait,'coldRestarts':cold_count,'actualGatherRows':gather_rows,'actualCookingRows':craft_rows,'qualification':'only originallevel100/guide8/priorchapter27stars/bagcapacity andaftercraftexplicitHP0.4/outsidepoint survivalfixture; sourcecoin/fish/dish/craftXP/rank neverprepared; freshcooking1→actual600xp→2 throughoriginalAPI. Original200dailyfatigue retained, reach2stop. Firstrealwait andonlylaterdedicatedgatherready/node60sec clocks; no originalengine hurt/winner/calendar/highrankclaim','frozenGoSHA256':self.source_shas,'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'baseline':self.baseline_source,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p8/cooking-natural-level-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
