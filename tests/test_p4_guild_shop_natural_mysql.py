"""Real published peaceful guild-war award -> donate -> original shop8 -> talent growth."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest

import test_p13_mysql as guild
import test_p13_union_fight_field_mysql as field
import test_p13_union_fight_settlement_mysql as producer
import test_p5_talent_scope_mysql as shared

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT/'analysis/p4/guild-shop-manifest.json'


@unittest.skipUnless(os.environ.get('HYJS_P4_GUILD_SHOP_MYSQL_TEST') == '1', 'Explicit real guild award-to-shop8 InnoDB flow')
class GuildShopNaturalMySQLTests(unittest.TestCase):
    send = guild.P13MySQLTests.send
    call = guild.P13MySQLTests.call
    stamped = guild.P13MySQLTests.stamped
    profile = guild.P13MySQLTests.profile
    recv = guild.P13MySQLTests.recv
    tcp_send = guild.P13MySQLTests.tcp_send
    connect = guild.P13MySQLTests.connect
    start = field.P13UnionFightFieldMySQLTests.start
    stop = producer.P13UnionFightSettlementMySQLTests.stop
    receive = field.P13UnionFightFieldMySQLTests.receive
    set_clock = field.P13UnionFightFieldMySQLTests.set_clock
    root_sql = classmethod(guild.P13MySQLTests.root_sql.__func__)
    write_profile = shared.TalentScopeMySQLTests.write_profile

    @classmethod
    def setUpClass(cls):
        # Freeze all dependencies and precompile before acquiring the SQL lifecycle lock.
        manifest = json.loads(MANIFEST.read_text())
        cls.fixed = tempfile.TemporaryDirectory(prefix='hyjs-p13-zero-settlement-shop8-')
        cls.addClassCleanup(cls.fixed.cleanup)
        cls.source = Path(cls.fixed.name).resolve()
        base = ROOT/manifest['base']
        shutil.copytree(base/'hyjs-server',cls.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'))
        (cls.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
        sources = [ROOT/p for p in manifest['dependencies']] + [MANIFEST]
        for path in sources:
            config = json.loads(path.read_text())
            entries = config.get('owned',config.get('files'))
            entries = [{'path':p,'sha256':h}for p,h in entries.items()] if isinstance(entries,dict) else entries
            for entry in entries:
                if not entry['path'].startswith('hyjs-server/'):
                    continue
                original = Path(config['snapshot'])/entry['path']
                if not original.is_absolute():original=ROOT/original
                if hashlib.sha256(original.read_bytes()).hexdigest()!=entry['sha256']:
                    raise AssertionError('fixed dependency drift '+entry['path'])
                shutil.copy2(original,cls.source/entry['path'])
        shared_handler = json.loads((ROOT/'analysis/p4/guild-shop-shared-source-20261015/manifest.json').read_text())['files'][0]
        path = ROOT/shared_handler['snapshot']
        if hashlib.sha256(path.read_bytes()).hexdigest()!=shared_handler['sha256']:
            raise AssertionError('root fixed shared handler drift')
        shutil.copy2(path,cls.source/shared_handler['path'])
        cls.runtime_sha = {e['path']:hashlib.sha256((cls.source/e['path']).read_bytes()).hexdigest()for e in manifest['owned']if e['path'].endswith('.go')}
        cls.shared_sha = shared_handler['sha256']
        # Only this discarded native main gets the existing strict own-schema calendar clock.
        main = cls.source/'hyjs-server/main.go'
        text = main.read_text().replace('"hyjs-server/config"','"hyjs-server/config"\n"hyjs-server/constUtil"\n"strings"',1)
        marker='defer cancel()\n'
        if text.count(marker)!=1:raise AssertionError('native clock insertion marker')
        text=text.replace(marker,marker+'''
        if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_") { return errors.New("guild shop native schema guard") }
        ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time {
            raw, err := os.ReadFile(os.Getenv("HYJS_P13_FIELD_NATIVE_CLOCK")); if err != nil { panic(err) }
            at, err := strconv.ParseInt(string(raw), 10, 64); if err != nil { panic(err) }
            return time.UnixMilli(at)
        })
''',1)
        main.write_text(text)
        cls.native_binary = str(cls.source/'guild-shop-native')
        for args in [['gofmt','-w',str(main)],['go','test','./service','-run','^TestP4GuildShopOriginalDualCost$','-count=1','-timeout=3m'],['go','build','-o',cls.native_binary,'.']]:
            result=subprocess.run(['rtk','proxy',*args],cwd=cls.source/'hyjs-server',capture_output=True,text=True,timeout=240)
            if result.returncode:raise AssertionError((result.stdout+result.stderr)[-6000:])
        cls.native_main_sha=hashlib.sha256(main.read_bytes()).hexdigest()
        field.SOURCE=cls.source
        producer.SOURCE=cls.source
        cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a+')
        fcntl.flock(cls.lock,fcntl.LOCK_EX)
        cls.addClassCleanup(cls.lock.close)
        def verify_cleanup():
            left=cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split()!=['0','0','0']:raise AssertionError('guild shop SQL cleanup incomplete')
            if hasattr(cls,'report'):
                cls.report['cleanupCounts']=[0,0,0]
                (ROOT/'analysis/p4/guild-shop-natural-mysql-validation.json').write_text(json.dumps(cls.report,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(verify_cleanup)
        guild.P13MySQLTests.setUpClass.__func__(cls)
        cls.binary=cls.native_binary
        cls.native_root=cls.source

    def setUp(self):
        self.logins={}
        self.uids={}
        guild.P13MySQLTests.setUp(self)

    def login(self,uid):
        result=guild.P13MySQLTests.login(self,uid)
        self.logins[result.playerId]=result
        self.uids[result.playerId]=uid
        return result

    def sql(self,text):
        # The inherited original producer needs only finite level/diamond/guild-level qualification.
        # Its unused 20m player coin fixture is zeroed; donation and talents below use actual star coin.
        text=text.replace('"$.coin",20000000','"$.coin",0')
        return guild.P13MySQLTests.sql(self,text)

    def test_original_actual_award_two_costs_shop8_talents_and_cold(self):
        began=time.monotonic()
        # Reuse the exact stable real signup/match/mine/two-round/offline P1 reward producer.
        # No race/max-matrix rerun and no new winner or badge fixture is introduced.
        producer.P13UnionFightSettlementMySQLTests.test_2_original_tcp_http_signup_mine_result_p1_offline_cold(self)
        awarded=[int(v)for v in self.sql('SELECT DISTINCT player_id FROM p1_reward_deliveries WHERE source_key LIKE "p13-war:%" ORDER BY player_id;').split()]
        actor=next(self.logins[id]for id in awarded if sum(v.get('num',0)for v in (self.profile(self.logins[id])[1].get('items')or[])if v['id']=='item_311')==1)
        p,g=self.profile(actor)
        self.assertEqual(p.get('coin',0),0)
        # Only the original historical-star eligibility is finite. Coin and stones are actually awarded.
        g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0}for r in shared.original_table('CopyMission')if r['ChapterId']=='27']
        g['copy']['passedChapterInfo']=[]
        self.write_profile(actor,p,g)
        source=self.call('CSCopyGetStarAward',{'chapterId':'27','starIndex':1},actor,8100,'SCCopyGetStarAward')
        source_coin=sum(v.num for v in source.gain.simps if v.id=='coin')
        source_stones=sum(v.num for v in source.gain.simps if v.id=='item_005')
        self.assertEqual(source_stones,200)
        self.call('CSTransferScene',{'target':'town_main'},actor,8101,'SCTransferScene')
        self.sql('UPDATE p13_guild_policies SET economy_enabled=1;')
        donated=self.call('CSUnionDonate',{'donateType':'coin'},actor,8102,'SCUnionDonate')
        self.assertEqual(donated.contributionAdd,20)
        self.assertEqual(self.profile(actor)[0].get('coin',0),source_coin-100000)
        member=self.sql(f'SELECT contribution FROM p13_guild_members WHERE player_id={actor.playerId};').strip()
        self.assertEqual(member,'20')
        self.assertEqual(self.stamped('CSShopProductList',{'shopType':8},actor,8103)[0],10001)
        month=time.strftime('%Y%m',time.localtime())
        key='p4:guild:'+month
        self.sql(f'UPDATE p4_commerce_rules SET enabled=1 WHERE id="{key}";')
        listing=self.call('CSShopProductList',{'shopType':8},actor,8104,'SCShopProductList')
        self.assertEqual(listing.contribution,20)
        self.assertEqual(len(listing.shopItemList),1)
        item=listing.shopItemList[0]
        self.assertEqual([(r.id,r.num)for r in item.exchangeList],[('item_311',1),('contribution',1)])
        self.assertEqual((item.condition.value,item.canBuy),(1,1))
        buy={'shopType':8,'shopItemId':item.id,'buyCount':1}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:
            responses=list(pool.map(lambda _:self.call('CSShopBuy',buy,actor,8105,'SCShopBuy'),range(8)))
        bought=responses[0]
        self.assertTrue(all(r==bought for r in responses))
        self.assertEqual([(r.id,r.num)for r in bought.cost],[('item_311',1),('contribution',1)])
        self.assertEqual([(r.id,r.num)for r in bought.get.simps],[('item_005',50)])
        self.assertEqual(bought.contribution,19)
        self.assertEqual(self.sql(f'SELECT contribution FROM p13_guild_members WHERE player_id={actor.playerId};').strip(),'20')
        self.assertEqual(self.stamped('CSShopBuy',buy,actor,8106)[0],10001)
        p,g=self.profile(actor)
        self.assertEqual(sum(r.get('num',0)for r in (g.get('items')or[])if r['id']=='item_311'),0)
        self.assertEqual(sum(r.get('num',0)for r in (g.get('items')or[])if r['id']=='item_005'),250)
        hero_guid=p['heroWarId']
        hero=next(w for w in g['warriors']if w['guid']==hero_guid)
        self.assertEqual(hero.get('talent',0),0)
        uses=[]
        for tier in ['1','2']:
            node=next(r for r in shared.original_table('TalentAdvenced')if r['warId']==hero['id']and r['talentLevel']==tier)
            costs=[{'id':v.split(',')[0],'num':int(v.split(',')[1])}for v in node['cost'].split(';')]
            used=self.call('CSWarriorTalent',{'guid':hero_guid,'talent':tier,'cost':costs},actor,8200+int(tier),'SCWarriorTalent')
            self.assertEqual(used.guid.talent,int(tier))
            uses.append({'tier':tier,'costs':costs})
        expected_coin=source_coin-130000
        p,g=self.profile(actor)
        self.assertEqual((p['coin'],p['contribution']),(expected_coin,19))
        self.assertEqual(sum(r.get('num',0)for r in (g.get('items')or[])if r['id']=='item_005'),20)
        uid=self.uids[actor.playerId]
        self.stop();self.start();actor=self.login(uid)
        cold=self.call('CSShopProductList',{'shopType':8},actor,8300,'SCShopProductList')
        self.assertEqual((cold.contribution,cold.shopItemList[0].dayLeftTimes,cold.shopItemList[0].canBuy),(19,0,2))
        self.assertEqual(self.call('CSShopBuy',buy,actor,8105,'SCShopBuy'),bought)
        p,g=self.profile(actor)
        self.assertEqual((p['coin'],p['contribution']),(expected_coin,19))
        self.assertEqual(next(w for w in g['warriors']if w['guid']==hero_guid)['talent'],2)
        self.assertEqual(self.sql(f'SELECT contribution FROM p13_guild_members WHERE player_id={actor.playerId};').strip(),'20')
        self.stop()
        type(self).report={'status':'PASS_REAL_P13_AWARD311_DONATION_DUAL_COST_ORIGINAL_SHOP8_TALENT_AND_COLD','seconds':round(time.monotonic()-began,3),'ownedGoSHA256':self.runtime_sha,'sharedHandlerSHA256':self.shared_sha,'nativeMainClockCopySHA256':self.native_main_sha,'producer':'analysis/p13/stable-guild-zero-fight-source-20261015/manifest.json','actualParticipationBadge':1,'actualStarCoin':source_coin,'actualDonationCoin':100000,'usableContribution':[0,20,19],'cumulativeContribution':[0,20,20],'originalOrderedCosts':[{'id':'item_311','num':1},{'id':'contribution','num':1}],'actualBoughtStones':50,'actualOriginalTalentUses':uses,'actualFinalStones':20,'actualFinalCoin':expected_coin,'eightOriginalPurchases':'one badge and one usable contribution consumed once','coldAfterUse':True,'originalUIShopType':8,'phoneUI':False,'productionModified':False,'finitePrerequisites':'Inherited stable original zero-combat producer real9characters/6guilds/signups/matching/mining/calendar/P1 offline reward. Its unused20mcoin fixture set0; only level30/diamond1000/guild2/calendar qualification remains finite. Then own historicalchapter27stars prepares eligibility only: actual original star coin405000/stone200 -> real100000donation -> balance20/cumulative20 -> actualparticipationbadge1+contribution1 -> originalshop50stones -> exact230stones/30000coin talents1/2. No coin/badge/contribution/growth resources inserted and no combatwinner fixture.'}


if __name__=='__main__':
    unittest.main()
