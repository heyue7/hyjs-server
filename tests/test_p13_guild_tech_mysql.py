"""Original guild technology upgrade with real collective funds and isolated InnoDB."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import unittest
import p5_guild_tech_consumer_checks as p5
import test_p13_mysql as guild
import test_p10_mysql as transport
from server.codec import BY_NAME

ROOT=Path(__file__).resolve().parents[1]
SOURCE=Path(os.environ.get("HYJS_P13_TECH_SOURCE_ROOT",str(ROOT))).resolve()
PREBUILT=SOURCE/"prebuilt"

@unittest.skipUnless(os.environ.get("HYJS_P13_TECH_MYSQL_TEST")=="1","explicit isolated guild technology opt-in")
class P13GuildTechMySQLTests(unittest.TestCase):
    root_sql=classmethod(guild.P13MySQLTests.root_sql.__func__)
    sql=guild.P13MySQLTests.sql
    setUp=guild.P13MySQLTests.setUp
    stop=guild.P13MySQLTests.stop
    login=guild.P13MySQLTests.login
    call=guild.P13MySQLTests.call
    stamped=guild.P13MySQLTests.stamped
    profile=guild.P13MySQLTests.profile
    send=guild.P13MySQLTests.send
    recv=guild.P13MySQLTests.recv
    tcp_send=guild.P13MySQLTests.tcp_send
    connect=guild.P13MySQLTests.connect
    @classmethod
    def setUpClass(cls):
        if not SOURCE.name.startswith("hyjs-p13-prayer-"):raise AssertionError("guild technology source isolation guard")
        cls.manifest=json.loads((PREBUILT/"manifest.json").read_text())
        for relative,digest in cls.manifest["files"].items():
            if hashlib.sha256((SOURCE/relative).read_bytes()).hexdigest()!=digest:raise AssertionError("guild technology fixed source drift "+relative)
        for name,key in [("hyjs-native","native_sha256"),("service-race.test","race_sha256")]:
            if hashlib.sha256((PREBUILT/name).read_bytes()).hexdigest()!=cls.manifest[key]:raise AssertionError("guild technology prebuild drift")
        guild.P13MySQLTests.setUpClass.__func__(cls)
        cls.binary=str(PREBUILT/"hyjs-native")
    def start(self):
        # The technology path uses real wall time; no altered main or test clock is built.
        import selectors,re,types
        self.process=subprocess.Popen([self.binary,"--http-port","0","--tcp-port","0","--tables",str(SOURCE/"analysis/tables"),"--log-file",str(Path(self.temp.name)/"guild-tech.log")],cwd=SOURCE/"hyjs-server",env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector()as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90),"technology native startup timeout");line=self.process.stdout.readline()
        ports=re.search(r"HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)",line);self.assertIsNotNone(ports,line.replace(self.test_password,"[redacted]"));self.service=types.SimpleNamespace(http_port=int(ports[1]),tcp_port=int(ports[2]))
    def receive(self,sock,name):
        for _ in range(60):
            op,value=self.recv(sock)
            self.assertNotEqual(op,10001,str(value))
            if op==BY_NAME[name]:return value
        self.fail("guild technology response queue exceeded")
    def test_0_canonical_schema_default_off(self):
        for name in ["policies.sql","offers.sql","upgrades.sql","policies.seed.sql"]:self.sql((SOURCE/"hyjs-server/db/sql"/("146_p13_guild_tech_"+name)).read_text())
        self.assertEqual(self.sql("SELECT CONCAT(enabled,':',directory_json,':',published_at) FROM p13_guild_tech_policies WHERE id=1;").strip(),"0:{}:0")
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_guild_tech_%';").strip(),"InnoDB")
    def test_1_prebuilt_mysql_race_technology(self):
        result=subprocess.run(["rtk","proxy",str(PREBUILT/"service-race.test"),"-test.run","^Test(P13GuildTech|P5GuildTech)","-test.v","-test.count=1","-test.timeout=240s"],cwd=SOURCE/"hyjs-server/service",env=self.env,capture_output=True,text=True,timeout=270)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,"[redacted]"));self.assertIn("PASS: TestP13GuildTechRealFunds",result.stdout);print(result.stdout.strip())
    def test_2_original_funds_upgrade_panels_cold_kick_leave_dissolve(self):
        began=time.monotonic();self.start()
        uids=["p13-tech-"+secrets.token_hex(5)for _ in range(14)];players=[self.login(uid)for uid in uids];a,b,c=players[:3]
        for index,login in enumerate(players):self.call("CSPlayerCreate",{"playerName":"科技实员"+str(index),"heroId":"hero_047"},login,1,"SCPlayerCreate")
        ids=",".join(str(x.playerId)for x in players)
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000,"$.coin",2000000) WHERE id IN ('+ids+'); UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1; UPDATE p13_guild_tech_policies SET enabled=0;')
        for login,num in [(a,50),(b,1),(c,1),(players[3],1)]:
            self.sql('UPDATE profiles SET game=JSON_SET(game,"$.items",JSON_ARRAY_APPEND(COALESCE(JSON_EXTRACT(game,"$.items"),JSON_ARRAY()),"$",JSON_OBJECT("id","item_089","num",'+str(num)+'))) WHERE id='+str(login.playerId)+';')
        self.call("CSUnionCreate",{"name":"科技原主链"},a,100,"SCUnionCreate");gid=self.call("CSGetUnionInfo",{},a,101,"SCGetUnionInfo").unionInfo.unionId
        for index,login in enumerate(players[1:]):
            self.call("CSUnionApply",{"unionId":gid},login,200+index,"SCUnionApply");self.call("CSUnionAgreeApply",{"playerId":login.playerId},a,300+index,"SCUnionAgreeApply")
        for index,login in enumerate(players):
            for n in range(15):self.call("CSUnionDonate",{"donateType":"coin"},login,1000+index*100+n,"SCUnionDonate")
        for n in range(30):self.call("CSUnionDonate",{"donateType":"badge"},a,4000+n,"SCUnionDonate")
        self.assertEqual(self.sql("SELECT CONCAT(coin,':',badge) FROM p13_guilds WHERE id='"+gid+"';").strip(),"21000000:30")
        self.call("CSUnionOpenBuilder",{"type":"HeroStatue"},a,4500,"SCUnionOpenBuilder")
        self.assertEqual(self.sql("SELECT CONCAT(coin,':',badge) FROM p13_guilds WHERE id='"+gid+"';").strip(),"11000000:10")
        self.assertEqual(self.stamped("CSUnionHeroStatueUpgrade",{"type":"peratt"},a,5000)[0],10001)
        directory=json.dumps({"initialLevel":0,"heroStatue":["peratt","cri","hit"],"guardTree":["perhp","perpdef","permdef","anticri","dodge"]},separators=(",",":"))
        source="local-prepublished-guild-tech-directory-initial0-v1; buildinggroups-and-initiallevel-local; original80rows-cost-and-prop"
        self.sql("UPDATE p13_guild_tech_policies SET enabled=1,version='local-native-directory-initial0-v1',directory_json='"+directory+"',directory_hash='"+hashlib.sha256(directory.encode()).hexdigest()+"',published_at="+str(int(time.time()*1000)-1000)+",source='"+source+"' WHERE id=1;")
        initial=self.call("CSUnionHeroStatueInfo",{},a,5001,"SCUnionHeroStatueInfo");self.assertEqual([(x.type,x.level,x.prop)for x in initial.unionPropAddAtt],[("peratt",0,"peratt,0"),("cri",0,"cri,0"),("hit",0,"hit,0")])
        self.assertEqual(self.stamped("CSUnionHeroStatueUpgrade",{"type":"peratt"},b,5000)[0],10001)
        self.assertEqual(self.call("CSGetUnionAddBaseProp",{},a,5002,"SCGetUnionAddBaseProp").unionAddBaseProp.ListFields(),[])
        p5.before_tech(self,a,b,c,players[3])
        sa,sb=self.connect(a),self.connect(b)
        self.call("CSUnionHeroStatueUpgrade",{"type":"peratt"},a,5100,"SCUnionHeroStatueUpgrade")
        self.assertEqual(self.receive(sa,"SCPlayer").player.needRefreshUnionProp,1);self.assertEqual(self.receive(sb,"SCPlayer").player.needRefreshUnionProp,1)
        one=self.call("CSUnionHeroStatueInfo",{},a,5101,"SCUnionHeroStatueInfo");self.assertEqual((one.unionPropAddAtt[0].level,one.unionPropAddAtt[0].prop,one.coinNum,one.badgeNum),(1,"peratt,1",11000000,10))
        p5.after_level(self,a,b,1,one)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:replies=list(pool.map(lambda _:self.call("CSUnionHeroStatueUpgrade",{"type":"peratt"},a,5200,"SCUnionHeroStatueUpgrade"),range(8)))
        self.assertTrue(all(x==replies[0]for x in replies));self.assertEqual(self.receive(sa,"SCPlayer").player.needRefreshUnionProp,1);self.assertEqual(self.receive(sb,"SCPlayer").player.needRefreshUnionProp,1)
        two=self.call("CSUnionHeroStatueInfo",{},a,5201,"SCUnionHeroStatueInfo");self.assertEqual((two.unionPropAddAtt[0].level,two.unionPropAddAtt[0].prop,two.coinNum,two.badgeNum),(2,"peratt,2",1000000,0))
        p5.after_level(self,a,b,2,two)
        self.tcp_send(sb,"CSGetUnionAddBaseProp",{},b);self.assertEqual(self.receive(sb,"SCGetUnionAddBaseProp").unionAddBaseProp.peratt,2)
        self.call("CSUnionDonate",{"donateType":"badge"},b,6000,"SCUnionDonate")
        stored_before=self.profile(b)[0]["fightPower"];self.assertGreater(stored_before,330)
        frozen=self.sql("SELECT CONCAT(tech_type,':',level,':',coin,':',badge,':',source_hash) FROM p13_guild_tech_upgrades WHERE guild_id='"+gid+"' ORDER BY tech_type,level;")
        self.sql("UPDATE p13_guild_tech_policies SET enabled=0,version='closed-next',directory_json='{}',directory_hash='changed';")
        self.stop();self.start();players=[self.login(uid)for uid in uids];a,b,c=players[:3]
        cold=self.call("CSUnionHeroStatueInfo",{},a,6100,"SCUnionHeroStatueInfo");self.assertEqual((cold.unionPropAddAtt[0].level,cold.unionPropAddAtt[0].prop,cold.coinNum,cold.badgeNum),(2,"peratt,2",1000000,1))
        self.assertEqual(self.sql("SELECT CONCAT(tech_type,':',level,':',coin,':',badge,':',source_hash) FROM p13_guild_tech_upgrades WHERE guild_id='"+gid+"' ORDER BY tech_type,level;"),frozen)
        p5.after_cold(self,a,b,cold)
        # These original one-badge donations persist current source-derived FP;
        # dissolution then has a real non-base cache to remove, even while offline.
        for index,login in enumerate([c,players[3]]):
            self.call("CSUnionDonate",{"donateType":"badge"},login,6150+index,"SCUnionDonate")
            self.assertGreater(self.profile(login)[0]["fightPower"],330)
        self.call("CSUnionHeroStatueUpgrade",{"type":"peratt"},a,5200,"SCUnionHeroStatueUpgrade")
        self.call("CSRemoveUnionPlayer",{"playerId":b.playerId},a,6200,"SCRemoveUnionPlayer")
        p5.after_kick(self,b)
        self.assertEqual(self.profile(b)[0]["fightPower"],330)
        self.assertEqual(self.call("CSGetUnionAddBaseProp",{},b,6201,"SCGetUnionAddBaseProp").unionAddBaseProp.ListFields(),[])
        self.call("CSTransferChairman",{"playerId":c.playerId},a,6300,"SCTransferChairman");self.call("CSQuitUnion",{},a,6301,"SCQuitUnion")
        p5.after_leave(self,a)
        self.assertEqual(self.profile(a)[0]["fightPower"],330)
        self.call("CSDissolveUnion",{},c,6400,"SCDissolveUnion")
        p5.after_dissolve(self,c,players[3])
        self.assertEqual(self.call("CSGetUnionAddBaseProp",{},players[3],6401,"SCGetUnionAddBaseProp").unionAddBaseProp.ListFields(),[])
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_guild_tech_upgrades WHERE guild_id='"+gid+"';").strip(),"2")
        self.stop()
        report={"status":"PASS_REAL_GUILD_TECH_FUNDS_UPGRADE_COLD_REVOCATION","seconds":round(time.monotonic()-began,3),"sourceProofSHA256":hashlib.sha256((PREBUILT/"manifest.json").read_bytes()).hexdigest(),"actualSource":"14 actual original members each15 original100k coin donations ->21m; original30 badge donations ->30; original builder10m20; real original tech0->1(0 cost)->2(10m10)","finitePrerequisites":"14 original created accounts explicitly level30/coin2m/diamond1000; item089 leader50/three members1; no guild funds, levels, effect, winner injected","actualUpgrades":2,"onceConcurrent":8,"closedColdFrozenRows":frozen.splitlines(),"rawMemberPowerBeforeKick":stored_before,"rawMemberPowerAfterKick":330,"P5":self.p5_tech_checks,"phone":False,"productionModified":False}
        (ROOT/"analysis/p13/tech-native-checkpoint.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n");print("original guild-tech14members/21m/30badge/realupgrade/8once/cold/kick/quit/dissolve PASS",round(time.monotonic()-began,3))

if __name__=="__main__":
    with open("/tmp/hyjs-isolated-mysql.lock","a")as mutex:
        fcntl.flock(mutex,fcntl.LOCK_EX);unittest.main()
