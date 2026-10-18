"""Finite original stars -> paid350 -> original20 bags -> real town TCP throw/claim."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
import unittest

import test_p10_mysql as original
import test_p10_scene_tcp_mysql as native
import test_p5_talent_scope_mysql as shared
from server.codec import BY_NAME

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT/'analysis/p4/fireworks-supply-manifest.json'


@unittest.skipUnless(os.environ.get('HYJS_P4_FIREWORKS_MYSQL_TEST') == '1', 'Explicit original coin-to-fireworks fresh InnoDB flow')
class FireworksSupplyMySQLTests(unittest.TestCase):
    send = original.P10MySQLTests.send
    login = original.P10MySQLTests.login
    stamped = original.P10MySQLTests.stamped
    call = original.P10MySQLTests.call
    connect = original.P10MySQLTests.connect
    tcp_send = original.P10MySQLTests.tcp_send
    recv = original.P10MySQLTests.recv
    profile = shared.TalentScopeMySQLTests.profile
    sql = shared.TalentScopeMySQLTests.sql
    write_profile = shared.TalentScopeMySQLTests.write_profile
    start = native.P10SceneTCPTests.start
    stop = native.P10SceneTCPTests.stop
    setUp = original.P10MySQLTests.setUp

    @classmethod
    def root_sql(cls, text):
        if not cls.database.startswith('hyjs_test_firework_') or not cls.test_user.startswith('firework_'):
            raise AssertionError('Dedicated fireworks schema guard')
        result = subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'], input=text, text=True, capture_output=True, timeout=30)
        if result.returncode:
            raise AssertionError(result.stderr.replace(cls.test_password, '[redacted]'))
        return result.stdout

    def test_real_source_purchase_original_fireworks_throw_claim_and_cold(self):
        began = time.monotonic()
        manifest = json.loads(MANIFEST.read_text())
        base = ROOT/manifest['base']
        self.process = None
        report = {}
        sockets = []
        with tempfile.TemporaryDirectory(prefix='hyjs-fireworks-source-') as td:
            self.source = Path(td).resolve()
            shutil.copytree(base/'hyjs-server', self.source/'hyjs-server', ignore=shutil.ignore_patterns('logs'))
            (self.source/'analysis').symlink_to(base/'analysis', target_is_directory=True)
            for entry in manifest['owned']:
                if not entry['path'].startswith('hyjs-server/'):
                    continue
                source = ROOT/manifest['snapshot']/entry['path']
                self.assertEqual(hashlib.sha256(source.read_bytes()).hexdigest(), entry['sha256'])
                shutil.copy2(source, self.source/entry['path'])
            dependency = json.loads((ROOT/manifest['dependency']).read_text())
            dep = next(e for e in dependency['files'] if e['path'] == 'hyjs-server/service/p10SceneService.go')
            src = ROOT/dependency['snapshot']/dep['path']
            self.assertEqual(hashlib.sha256(src.read_bytes()).hexdigest(), dep['sha256'])
            shutil.copy2(src, self.source/dep['path'])
            self.binary = self.source/'fireworks-native'
            for args in [['go','test','./service','-run','^TestP4LuckySupplyPublication$','-count=1','-timeout=3m'], ['go','build','-o',str(self.binary),'.']]:
                result = subprocess.run(['rtk','proxy',*args], cwd=self.source/'hyjs-server', capture_output=True, text=True, timeout=220)
                self.assertEqual(result.returncode, 0, (result.stdout+result.stderr)[-5000:])
            runtime_sha = {e['path']:hashlib.sha256((self.source/e['path']).read_bytes()).hexdigest() for e in manifest['owned'] if e['path'].endswith('.go')}
            type(self).database = 'hyjs_test_firework_'+secrets.token_hex(6)
            type(self).test_user = 'firework_'+secrets.token_hex(5)
            type(self).test_password = secrets.token_hex(24)
            self.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_DATABASE=self.database, HYJS_MYSQL_USER=self.test_user, HYJS_MYSQL_PASSWORD=self.test_password)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';")
                    self.start()
                    users = ['real-fireworks-source-'+secrets.token_hex(5), 'real-bag-recipient-'+secrets.token_hex(5)]
                    owner, recipient = [self.login(uid) for uid in users]
                    stamp = 1801500000000
                    def http(name, fields, expected, actor=owner):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, actor, stamp, expected)
                    for actor, name in [(owner, '金币烟花来源'), (recipient, '真实福袋领取')]:
                        http('CSPlayerCreate', {'playerName':name,'heroId':'hero_043'}, 'SCPlayerCreate', actor)
                        p,g = self.profile(actor)
                        self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0))
                        p.update(level=70,noviceGuideSchedule=8,warriorBagSize=1000,equipBagSize=1000)
                        if actor == owner:
                            g['copy']['passedCopyInfo'] = [{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['ChapterId'] in ['26','27']]
                            g['copy']['passedChapterInfo'] = []
                        self.write_profile(actor,p,g)
                    coin_source = 0
                    star_gains = []
                    for chapter in ['26','27']:
                        for index in range(3):
                            gain = http('CSCopyGetStarAward',{'chapterId':chapter,'starIndex':index},'SCCopyGetStarAward')
                            coin_source += sum(v.num for v in gain.gain.simps if v.id=='coin')
                            star_gains.append({'chapter':chapter,'index':index,'simps':[{'id':v.id,'num':v.num}for v in gain.gain.simps]})
                    self.assertEqual(self.profile(owner)[0]['coin'],coin_source)
                    month = time.strftime('%Y%m',time.localtime())
                    child = 'limitExchange_p4_fireworks_'+month
                    directory = 'limitExchange_p4_resources_'+month
                    # Read actual published aggregate identity, whose prefix is defined by the original existing service.
                    activities = http('CSActivityList',{},'SCActivityList')
                    directory = next(a.activityId for a in activities.activityList if a.activityId.startswith('limitExchange_p4_resources_'))
                    query = {'activityId':directory}
                    before = http('CSActivityShopInfo',query,'SCActivityShopInfo')
                    self.assertFalse(any(any(r.id=='item_350' for r in v.getResource) for v in before.shopItemList))
                    self.assertEqual(self.sql(f'SELECT enabled FROM p4_commerce_rules WHERE id="{child}";').strip(),'0')
                    self.sql('UPDATE p10_social_policies SET enabled=1;UPDATE p10_scene_policies SET enabled=1;UPDATE p10_channel_policies SET world_enabled=1;UPDATE p10_lucky_policies SET fireworks_enabled=1,drop_enabled=1;')
                    self.sql(f'UPDATE p4_commerce_rules SET enabled=1 WHERE id="{child}";')
                    opened = http('CSActivityShopInfo',query,'SCActivityShopInfo')
                    self.assertEqual(list(opened.shopItemList[:-1]),list(before.shopItemList))
                    item = opened.shopItemList[-1]
                    self.assertEqual((item.name,item.getResource[0].id,item.costResource[0].num),('七彩烟花','item_350',2000000))
                    buy = {'activityId':directory,'index':item.index,'num':1,'costResources':[{'id':v.id,'num':v.num}for v in item.costResource],'getResources':[{'id':v.id,'num':v.num}for v in item.getResource]}
                    bought = http('CSBuyActivityShop',buy,'SCGetActivityShopReward')
                    buy_stamp = stamp
                    self.assertEqual(self.call('CSBuyActivityShop',buy,owner,buy_stamp,'SCGetActivityShopReward'),bought)
                    self.assertEqual(self.stamped('CSBuyActivityShop',buy,owner,stamp+1)[0],10001)
                    self.assertEqual(self.profile(owner)[0]['coin'],coin_source-2000000)
                    def tcp(sock, actor, name, fields, expected):
                        sock.settimeout(.025)
                        try:
                            while True:self.recv(sock)
                        except socket.timeout:pass
                        finally:sock.settimeout(8)
                        self.tcp_send(sock,name,fields,actor)
                        for _ in range(30):
                            op, reply = self.recv(sock)
                            if op==BY_NAME[expected]:return reply
                            self.assertNotEqual(op,10001,str(reply))
                        self.fail('Expected original TCP response '+expected)
                    def connect_town(actor):
                        sock = self.connect(actor)
                        sockets.append(sock)
                        tcp(sock,actor,'CSLoginScene',{},'SCLoginScene')
                        tcp(sock,actor,'CSCurrentPosition',{},'SCCurrentPosition')
                        return sock
                    sa = connect_town(owner)
                    sb = connect_town(recipient)
                    converted = http('CSItemUse',{'costItem':{'id':'item_350','num':1}},'SCItemUse')
                    use_stamp = stamp
                    self.assertEqual([(v.id,v.num)for v in converted.get.simps],[('item_349',20)])
                    point = self.profile(owner)[1].get('position',{}).get('current',{'x':0,'y':0,'z':0})
                    thrown = tcp(sa,owner,'CSThrowLuckyBags',{'num':20,'posArray':[point]*20},'SCThrowLuckyBags')
                    self.assertEqual([(v.id,v.num)for v in thrown.costResource],[('item_349',20)])
                    pending = http('CSCurrentLuckyBagsInfo',{},'SCCurrentLuckyBagsInfo',recipient)
                    self.assertEqual(len(pending.luckyBags),20)
                    reward_hash = self.sql('SELECT reward_hash FROM p10_lucky_policies WHERE id=1;').strip()
                    reward_rows = {r['result'] for r in shared.original_table('ItemUse') if r['itemId']=='item_349'}
                    frozen_rewards = self.sql('SELECT reward FROM p10_lucky_objects ORDER BY guid;').strip().splitlines()
                    self.assertEqual(len(frozen_rewards),20)
                    self.assertTrue(all(v in reward_rows for v in frozen_rewards))
                    first = pending.luckyBags[0].guid
                    claimed = tcp(sb,recipient,'CSSnatchOneLuckyBag',{'guid':first},'SCSnatchOneLuckyBag')
                    # Cold recovery preserves already-paid objects and lets the actual recipient collect the rest.
                    expected_wallet = [(self.profile(actor)[0].get('coin',0),self.profile(actor)[0].get('diamond',0))for actor in [owner,recipient]]
                    for sock in sockets:sock.close()
                    sockets.clear()
                    self.stop();self.start()
                    owner,recipient = [self.login(uid)for uid in users]
                    self.assertEqual([(self.profile(a)[0].get('coin',0),self.profile(a)[0].get('diamond',0))for a in [owner,recipient]],expected_wallet)
                    sa = connect_town(owner);sb = connect_town(recipient)
                    self.assertEqual(self.call('CSBuyActivityShop',buy,owner,buy_stamp,'SCGetActivityShopReward'),bought)
                    self.assertEqual(self.call('CSItemUse',{'costItem':{'id':'item_350','num':1}},owner,use_stamp,'SCItemUse'),converted)
                    pending_cold = http('CSCurrentLuckyBagsInfo',{},'SCCurrentLuckyBagsInfo',recipient)
                    self.assertEqual(len(pending_cold.luckyBags),19)
                    self.assertEqual(tcp(sb,recipient,'CSSnatchOneLuckyBag',{'guid':first},'SCSnatchOneLuckyBag'),claimed)
                    claims = [claimed]
                    for bag in pending_cold.luckyBags:
                        claims.append(tcp(sb,recipient,'CSSnatchOneLuckyBag',{'guid':bag.guid},'SCSnatchOneLuckyBag'))
                    self.assertEqual(len(http('CSCurrentLuckyBagsInfo',{},'SCCurrentLuckyBagsInfo',recipient).luckyBags),0)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_lucky_objects WHERE claimed_at>0;').strip(),'20')
                    self.assertEqual(self.sql('SELECT reward_hash FROM p10_lucky_policies WHERE id=1;').strip(),reward_hash)
                    owner_p,owner_g = self.profile(owner)
                    self.assertEqual(owner_p['coin'],coin_source-2000000)
                    self.assertEqual(sum(v.get('num',0)for v in owner_g.get('items',[])if v['id'] in ['item_349','item_350']),0)
                    report={'status':'PASS_REAL_STAR_COIN_PURCHASE_350_ORIGINAL20BAGS_TOWN_TCP_THROW20_CLAIM20_AND_COLD','seconds':round(time.monotonic()-began,3),'ownedGoSHA256':runtime_sha,'starSource':star_gains,'sourceCoin':coin_source,'sourceCost':2000000,'ownerFinalCoin':owner_p['coin'],'originalRewardPoolHashUnchanged':reward_hash,'actualFrozenRandomRewards':frozen_rewards,'originalTCPHeaderBytes':28,'actualClaims':20,'coldRestarts':1,'newTailIndex':item.index,'existingProductsUnchanged':True,'finitePrerequisites':'Only two own characters70/guide8/capacity and historical26/27stars. No350/349/coin/diamond/reward/GUID/battlewinner inserted. All policies enabled only dedicatedschema. This is local source-chain protocol evidence, not natural historicalchapter victories or phone.','guaranteed079':False,'productionModified':False,'phoneUI':False}
                finally:
                    for sock in sockets:sock.close()
                    self.stop()
                    self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';")
                    left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';")
                    self.assertEqual(left.split(),['0','0','0'])
                    report['cleanupCounts']=[0,0,0]
        if report.get('status'):
            (ROOT/'analysis/p4/fireworks-supply-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
