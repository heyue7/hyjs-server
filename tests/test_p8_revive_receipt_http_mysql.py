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

OWNED = ['hyjs-server/service/p8LifeService.go','hyjs-server/service/p8ReviveReceipt_test.go','tests/test_p8_revive_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_REVIVE_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class ReviveReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_revive_http_' + secrets.token_hex(6)
        cls.test_user = 'p8revhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-revive-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-revive-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_REVIVE_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p5/copy-award-receipt-manifest.json','analysis/p5/copy-award-shared-source-20261015/manifest.json']:
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
            report = shared.ROOT / 'analysis/p8/revive-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_revive_http_') or not cls.test_user.startswith('p8revhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_paid_free_revive_actual_currency_gather_and_cold(self):
        began=time.monotonic();self.start();uid='actual-original-revive-life';login=self.login(uid);stamp=1800470000000;cold_count=0
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
        def reject(name,old):
            saved=self.profile(login);op,msg=self.stamped(name,{},login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        def death():
            p,g=self.profile(login);g['field']['side'].update(curHp=0,state=5);g['field']['side'].pop('battlePosHps',None);g['field'].pop('fight',None);g['position']['current']={'x':1000,'y':0,'z':1000};self.write_profile(login,p,g)
        def move(point):
            nonlocal stamp
            stamp+=1;body=encode_http_request(BY_NAME['CSMove'],message('CSMove',pointNew=point,currentTime=stamp),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
            connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=60)
            try:
                connection.request('POST','/sao/',body=body,headers={'Content-Type':'application/octet-stream'});reply=connection.getresponse();self.assertEqual(reply.status,200);self.assertFalse(any(op==BY_NAME['ErrorMessage'] for op,_ in decode_responses(reply.read())))
            finally:connection.close()
            self.assertEqual(self.profile(login)[1]['position']['current'],point)
        call('CSPlayerCreate',{'playerName':'原复活回执','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=18,coin=0,diamond=0,warriorBagSize=1000,equipBagSize=1000)
        g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['ChapterId']=='1'];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        earned=0
        for index in [0,1]:
            award=call('CSCopyGetStarAward',{'chapterId':'1','starIndex':index},'SCCopyGetStarAward');earned+=sum(x.num for x in award.gain.simps if x.id=='diamond')
        self.assertEqual(earned,90);self.assertEqual(self.profile(login)[0]['diamond'],90)
        call('CSTransferScene',{'target':'field_001'},'SCTransferScene');call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition')
        death();paid,paid_stamp=once('CSResurrection',{},'SCResurrenttion');self.assertEqual(paid.cost[0].num,50);self.assertEqual(self.profile(login)[0]['diamond'],40);self.assertEqual(self.profile(login)[1]['field']['side']['curHp'],1)
        cold();self.assertEqual(self.call('CSResurrection',{},login,paid_stamp,'SCResurrenttion'),paid)
        death();paid_hot=reject('CSResurrection',paid_stamp);cold();paid_cold=reject('CSResurrection',paid_stamp)
        call('CSGetHp',{},'SCGetHp');stamp+=1;self.assertEqual(self.stamped('CSTransfer2Safe',{},login,stamp)[0],10001)
        free_begin=time.monotonic();time.sleep(10.05);free,free_stamp=once('CSTransfer2Safe',{},'SCTransfer2Safe');free_wait=round(time.monotonic()-free_begin,3);self.assertEqual(free.result,1);self.assertAlmostEqual(self.profile(login)[1]['field']['side']['curHp'],0.1);self.assertEqual(self.profile(login)[0]['diamond'],40)
        cold();self.assertEqual(self.call('CSTransfer2Safe',{},login,free_stamp,'SCTransfer2Safe'),free)
        call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition')
        raw=self.sql(f'SELECT CONCAT(pos_id,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_001" AND remaining>0 ORDER BY pos_id;')
        nodes=[(v.split('|')[0],json.loads(bytes.fromhex(v.split('|')[1]))) for v in raw.splitlines()];pos,cfg=next(x for x in nodes if x[1]['kind']=='mine' and x[1]['level']==1)
        move(cfg['point']);started=call('CSFieldResourceStartGather',{'posId':pos},'SCFieldResourceStartGather');self.assertEqual(started.result,1);self.assertEqual(self.profile(login)[1]['field']['side']['state'],4)
        free_gather=reject('CSTransfer2Safe',free_stamp);paid_gather=reject('CSResurrection',paid_stamp);cold();free_gather_cold=reject('CSTransfer2Safe',free_stamp)
        death();free_dead=reject('CSTransfer2Safe',free_stamp);cold();free_dead_cold=reject('CSTransfer2Safe',free_stamp)
        self.assertEqual(self.profile(login)[0]['diamond'],40)
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_REVIVE_STATE_LIFECYCLE_PHONE_PENDING','actualStarDiamond':earned,'actualPaidCostDiamond':50,'finalDiamond':40,'eightPaidAndFree':'one actualcost/revival exactcurrent originalbody','paidCurrentCold':'same','paidDeadHotCold':[paid_hot,paid_cold],'freeRealWaitSeconds':free_wait,'freeCurrentCold':'same','actualMiningStart':{'pos':pos,'resource':cfg['resourceId'],'originalState':4},'freeActualGatherHotCold':[free_gather,free_gather_cold],'paidActualGatherRejected':paid_gather,'freeDeadHotCold':[free_dead,free_dead_cold],'coldRestarts':cold_count,'qualification':'original18level/priorchapter1stars/capacity and explicitdeath survivalfixture only; no wallet/source resources/damageengine/winner/newauthority preparation; originalpaid50/real10second freewait/originalsafepoint/actual miningstart','frozenGoSHA256':self.source_shas,'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'baseline':self.baseline_source,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p8/revive-receipt-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
