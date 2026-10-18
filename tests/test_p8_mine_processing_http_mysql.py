"""Original worldseed currentnode/fullmain receipt originalHTTP lifecycle."""
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

OWNED = ['tests/test_p8_mine_processing_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_MINE_PROCESSING_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class MineProcessingHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p8_mineprocess_http_' + secrets.token_hex(6)
        cls.test_user = 'p8minehttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p8-mineprocess-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-mineprocess-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_MINE_PROCESSING_HTTP_BASE', shared.ROOT / 'build/candidates/joint-preview-caches-v78-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/award-collection-manifest.json','analysis/p5/refund-material-manifest.json','analysis/p5/world-seed-receipt-manifest.json','analysis/p8/gem-receipt-manifest.json','analysis/p5/empty-gem-sale-manifest.json']:
            dependency=json.loads((shared.ROOT/manifest).read_text());dependency_source=shared.ROOT/dependency['snapshot']
            for entry in dependency['owned']:
                rel=entry['path']
                if rel.startswith('hyjs-server/'):
                    source_file=dependency_source/rel
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
            report = shared.ROOT / 'analysis/p8/mine-processing-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_mineprocess_http_') or not cls.test_user.startswith('p8minehttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_lowmap_mine_refine_make_combine_transform_and_cold(self):
        began=time.monotonic();self.start();stamp=1800220000000;results=[]
        recipes=shared.original_table('MineSkill')
        for scenario in ['three_same_color_upgrade','two_different_color_conversion']:
            uid='actual-mine-process-'+scenario;login=self.login(uid);gathers=0;refines=0;fatigue_start=200;cold_count=0;gather_rows=[];refine_rows=[];initial_wait=None
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            def once(name,fields,response):
                nonlocal stamp
                stamp+=1;saved_stamp=stamp
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call(name,fields,login,saved_stamp,response),range(8)))
                self.assertTrue(all(x==replies[0] for x in replies));return replies[0],saved_stamp
            def cold():
                nonlocal login,cold_count
                saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
            def move(point):
                nonlocal stamp
                stamp+=1;body=encode_http_request(BY_NAME['CSMove'],message('CSMove',pointNew=point,currentTime=stamp),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
                connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=60)
                try:
                    connection.request('POST','/sao/',body=body,headers={'Content-Type':'application/octet-stream'});reply=connection.getresponse();self.assertEqual(reply.status,200);self.assertFalse(any(op==BY_NAME['ErrorMessage'] for op,_ in decode_responses(reply.read())))
                finally:connection.close()
                self.assertEqual(self.profile(login)[1]['position']['current'],point)
            call('CSPlayerCreate',{'playerName':'原矿石自然加工','heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,noviceGuideSchedule=8,equipBagSize=1000,warriorBagSize=1000)
            g['items']=[{'id':'item_119','num':1}]
            g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in shared.original_table('CopyMission')];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
            source_coin=0
            for box in range(3):
                award=call('CSCopyGetStarAward',{'chapterId':'27','starIndex':box},'SCCopyGetStarAward');source_coin+=next(x.num for x in award.gain.simps if x.id=='coin')
            gear=call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},'SCItemUse').get.equips[0];call('CSFormationOn',{'formatPos':1,'id':gear.equipId,'guid':gear.guid},'SCFormationOn')
            call('CSTransferScene',{'target':'field_001'},'SCTransferScene');call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition')
            self.assertEqual(self.profile(login)[1]['field']['side']['tiredness'],fatigue_start)
            final_refine=None;goal=None
            while goal is None:
                p,g=self.profile(login);counts={x['id']:x['num'] for x in (g.get('gems') or [])}
                if scenario=='three_same_color_upgrade':
                    colors=[c for c in [1,2,3,5] if counts.get('MineItem_%03d'%(12+c),0)>=6]
                    if counts.get('MineItem_081',0)>=30 and counts.get('MineItem_084',0)>=30 and colors:goal=[colors[0]]
                else:
                    colors=[c for c in range(1,7) if counts.get('MineItem_%03d'%(12+c),0)>=2]
                    if counts.get('MineItem_081',0)>=20 and counts.get('MineItem_084',0)>=20 and len(colors)>=2:
                        pair=colors[:2];target=next(c for c in [1,2,3,5] if c not in pair);goal=pair+[target]
                if goal is not None:break
                raw_count=counts.get('MineItem_007',0)
                if raw_count>=5:
                    fields={'mineId':'MineItem_007','num':raw_count//5*5}
                    if refines==0: refined,refine_stamp=once('CSMineRefine',fields,'SCMineRefine')
                    else: refined=call('CSMineRefine',fields,'SCMineRefine');refine_stamp=stamp
                    refines+=1;final_refine=(fields,refine_stamp,refined)
                    refine_rows.append({'num':fields['num'],'gain':[{'id':v.id,'num':v.num} for v in refined.res.simps],'level':refined.level,'exp':refined.exp});continue
                self.assertLess(gathers,100,'Original200fatigue exhausted; do not resetquota/prepareore/color/RNG')
                raw=self.sql(f'SELECT CONCAT(pos_id,"|",remaining,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_001" ORDER BY pos_id;')
                nodes=[(v.split('|')[0],int(v.split('|')[1]),json.loads(bytes.fromhex(v.split('|')[2]))) for v in raw.splitlines()]
                selected=next((n for n in nodes if n[1] and n[2]['kind']=='mine' and n[2]['level']==1),None)
                if selected is None:
                    self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND scene="field_001" AND remaining=0;');call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition');continue
                pos,remaining,cfg=selected;move(cfg['point']);started=call('CSFieldResourceStartGather',{'posId':pos},'SCFieldResourceStartGather');self.assertEqual(started.result,1)
                if gathers==0:
                    saved=self.profile(login);stamp+=1;self.assertEqual(self.stamped('CSFieldGetResource',{},login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
                    cold();call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition');begin=time.monotonic();time.sleep(cfg['gatherMs']/1000+.1);initial_wait=round(time.monotonic()-begin,3)
                else:self.sql(f'UPDATE p8_gather_attempts SET ready_at=1 WHERE player_id={login.playerId} AND active=1;')
                if gathers==0:claimed,_=once('CSFieldGetResource',{},'SCFieldGetResource')
                else:claimed=call('CSFieldGetResource',{},'SCFieldGetResource')
                self.assertEqual(claimed.code,1);gathers+=1
                if gathers%20==0:print({'scenario':scenario,'gathers':gathers,'fatigueRemaining':200-2*gathers,'refines':refines},flush=True)
                gather_rows.append({'resource':cfg['resourceId'],'gain':[{'id':v.id,'num':v.num} for v in claimed.res.simps]});self.assertEqual(self.profile(login)[1]['field']['side']['tiredness'],200-2*gathers)
                if remaining==1:self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND scene="field_001" AND pos_id="{pos}";')
            simple_craft=[]
            def make(color,num):
                key=next(x['MineItemID'] for x in recipes if x['Type']=='magicGem' and x['Level']=='1' and int(x['Color'])==color)
                fields={'gemId':key,'num':num};award,old=once('CSGemMake',fields,'SCGemMake');simple_craft.append((fields,old,award));return key
            if scenario=='three_same_color_upgrade':
                key=make(goal[0],3);target=next(x['MineItemID'] for x in recipes if x['Type']=='magicGem' and x['Level']=='2' and int(x['Color'])==goal[0]);out,_=once('CSGemCombineByThree',{'gemId':target},'SCGemCombineByThree');self.assertEqual([(x.id,x.num) for x in out.cost],[(key,3)])
            else:
                first=make(goal[0],1);second=make(goal[1],1);target=next(x['MineItemID'] for x in recipes if x['Type']=='magicGem' and x['Level']=='1' and int(x['Color'])==goal[2]);out,_=once('CSGemCombineByTwo',{'gemId':[first,second,target]},'SCGemCombineByTwo');self.assertEqual([(x.id,x.num) for x in out.cost],[(first,1),(second,1)])
            self.assertEqual([(x.id,x.num) for x in out.res.simps],[(target,1)])
            color=int(next(x['Color'] for x in recipes if x['MineItemID']==target));holes=[1,5,3,2];pos=holes.index(color)
            call('CSCopyEnter',{'chapterId':'27','missionId':'161'},'SCCopyEnter');before_battle=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop
            before_p,before_g=self.profile(login);before_strength=self.sql('SELECT level FROM gem_craft_states WHERE player_id='+str(login.playerId)+';').strip();mount_fields={'guid':gear.guid,'gemId':target,'pos':pos};mounted,mount_stamp=once('CSGemMount',mount_fields,'SCGemMount');self.assertEqual(mounted.equip.gems[pos],target)
            # Actual battle transport is queried without sending a winner/end.
            battle=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo')
            self.assertNotEqual(battle.battlePlayer.formation[0].prop,before_battle)
            cold();self.assertEqual(self.call('CSGemMount',mount_fields,login,mount_stamp,'SCGemMount'),mounted)
            for fields,old,award in simple_craft[-1:]:
                saved=self.profile(login);self.assertEqual(self.call('CSGemMake',fields,login,old,'SCGemMake'),award);self.assertEqual(self.profile(login),saved)
            fields,old,award=final_refine;saved=self.profile(login);self.assertEqual(self.call('CSMineRefine',fields,login,old,'SCMineRefine'),award);self.assertEqual(self.profile(login),saved)
            unmounted,_=once('CSGemUnmount',{'guid':gear.guid,'pos':pos},'SCGemUnmount');self.assertEqual([(x.id,x.num) for x in unmounted.res.simps],[(target,1)])
            restored=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop;self.assertEqual(restored,before_battle)
            saved=self.profile(login);self.assertEqual(self.stamped('CSGemMount',mount_fields,login,mount_stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            p,g=self.profile(login);self.assertEqual(p['coin'],source_coin-(30000 if scenario=='three_same_color_upgrade' else 20000));self.assertEqual(g['field']['side']['tiredness'],200-2*gathers)
            results.append({'scenario':scenario,'gathers':gathers,'fatigueSpent':gathers*2,'remainingFatigue':200-gathers*2,'sourceCoin':source_coin,'paidCraftCoin':source_coin-p['coin'],'actualColors':goal,'actualOutput':target,'naturalCraftLevel':int(before_strength),'refines':refines,'firstRealWait':initial_wait,'coldRestarts':cold_count,'actualGatherRows':gather_rows,'actualRefineRows':refine_rows,'battleTransportQueried':True})
            call('CSCopyLeave',{},'SCCopyLeave')
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_LOW_MAP_MINE_PROCESSING_PHONE_PENDING','results':results,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'finitePrerequisites':'level100/guide8/priororiginalstars/held119gearbox;naturalgemcraft startslevel1/noore/magic/crystal/powder/gem/sourcecoin/RNGinsertions;coins0diamond0;sourcefatigueactual200notreset','clock':'firstreal3.2s/restartmidgather;lateronlydedicatedattemptready/node60sec refresh clocks accelerated,physicalfatigue/quota/reward/state notprepared','unknownRule':'originalsixcolorcrystalprivateyieldunknown,currentnamedGemRule1uniformperbatch retained; sevenrainbow6combine source/highmap/longlevel separate','productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p8/mine-processing-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
