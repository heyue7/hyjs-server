"""Original prayer HTTP chain and actual InnoDB concurrency; owns no formal data."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time
import unittest
import p5_prayer_consumer_checks as p5
import test_p13_mysql as guild
import test_p13_union_fight_field_mysql as field

ROOT=Path(__file__).resolve().parents[1]
SOURCE=Path(os.environ.get("HYJS_P13_PRAYER_SOURCE_ROOT",str(ROOT))).resolve()
PREBUILT=SOURCE/"prebuilt"

@unittest.skipUnless(os.environ.get("HYJS_P13_PRAYER_MYSQL_TEST")=="1","explicit isolated prayer database opt-in")
class P13GuildPrayerMySQLTests(unittest.TestCase):
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
    start=field.P13UnionFightFieldMySQLTests.start
    set_clock=field.P13UnionFightFieldMySQLTests.set_clock
    @classmethod
    def setUpClass(cls):
        if not SOURCE.name.startswith("hyjs-p13-prayer-"):raise AssertionError("prayer source isolation guard")
        cls.manifest=json.loads((PREBUILT/"manifest.json").read_text())
        for relative,digest in cls.manifest["files"].items():
            if hashlib.sha256((SOURCE/relative).read_bytes()).hexdigest()!=digest:raise AssertionError("prayer fixed source drift "+relative)
        for name,key in [("hyjs-native","native_sha256"),("service-race.test","race_sha256")]:
            if hashlib.sha256((PREBUILT/name).read_bytes()).hexdigest()!=cls.manifest[key]:raise AssertionError("prayer prebuild drift")
        guild.P13MySQLTests.setUpClass.__func__(cls)
        field.SOURCE=SOURCE
        cls.native_root=SOURCE
        cls.binary=str(PREBUILT/"hyjs-native")
        cls.clock=Path(cls.temp.name)/"prayer-clock"
        cls.env["HYJS_P13_FIELD_NATIVE_CLOCK"]=str(cls.clock)
    def test_0_canonical_mysql_schema_default_off(self):
        for name in ["145_p13_guild_prayer_policies.sql","145_p13_guild_prayer_rolls.sql","145_p13_guild_prayer_policies.seed.sql"]:
            self.sql((SOURCE/"hyjs-server/db/sql"/name).read_text())
        self.assertEqual(self.sql("SELECT CONCAT(enabled,':',published_at,':',pool_json) FROM p13_guild_prayer_policies WHERE id=1;").strip(),"0:0:[]")
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME IN ('p13_guild_prayer_policies','p13_guild_prayer_rolls');").strip(),"InnoDB")

    def test_1_prebuilt_actual_mysql_race_prayer(self):
        result=subprocess.run(["rtk","proxy",str(PREBUILT/"service-race.test"),"-test.run","^TestP13GuildPrayer","-test.v","-test.count=1","-test.timeout=180s"],cwd=SOURCE/"hyjs-server/service",env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,"[redacted]"))
        self.assertIn("PASS: TestP13GuildPrayerSameRequestConcurrentSingleDebit",result.stdout)
        print(result.stdout.strip())
    def test_2_original_http_contribution_prayer_cold_expiry_and_leave(self):
        began=time.monotonic()
        at=int(time.time()*1000)-60000
        self.set_clock(at);self.start()
        uids=["p13-prayer-"+secrets.token_hex(5)for _ in range(2)]
        a,b=[self.login(uid)for uid in uids]
        for login,name in [(a,"祈祷原会长"),(b,"祈祷原成员")]:self.call("CSPlayerCreate",{"playerName":name,"heroId":"hero_047"},login,1,"SCPlayerCreate")
        ids=",".join(str(x.playerId)for x in [a,b])
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000,"$.coin",2000000) WHERE id IN ('+ids+'); UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1; UPDATE p13_guild_prayer_policies SET enabled=0;')
        self.call("CSUnionCreate",{"name":"原祈祷主链"},a,100,"SCUnionCreate")
        guildID=self.call("CSGetUnionInfo",{},a,101,"SCGetUnionInfo").unionInfo.unionId
        self.call("CSUnionApply",{"unionId":guildID},b,102,"SCUnionApply")
        self.call("CSUnionAgreeApply",{"playerId":b.playerId},a,103,"SCUnionAgreeApply")
        for n in range(15):self.call("CSUnionDonate",{"donateType":"coin"},a,200+n,"SCUnionDonate")
        self.assertEqual(self.profile(a)[0]["contribution"],300)
        # Guild create/join uses real wall time; start controlled prayer time after those actual facts.
        at=int(time.time()*1000)
        self.set_clock(at)
        p5.before_prayer(self,a,1)
        self.assertEqual(self.stamped("CSUnionPray",{},a,300)[0],10001)
        pool=json.dumps([{"type":"peratt","value":1,"weight":1},{"type":"perhp","value":2,"weight":3},{"type":"perpdef","value":3,"weight":2},{"type":"permdef","value":4,"weight":2}],separators=(",",":"))
        source="local-prepublished-original-prayer-v1; poolweights-and-percent-operator-defined; original100contribution-day3-duration24h; singleeffect-overwrite"
        self.sql("UPDATE p13_guild_prayer_policies SET enabled=1,version='local-native-original-prayer-v1',source='"+source+"',pool_json='"+pool+"',pool_hash='"+hashlib.sha256(pool.encode()).hexdigest()+"',published_at="+str(at-1000)+" WHERE id=1;")
        self.set_clock(at+1000)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8)as executor:
            responses=list(executor.map(lambda _:self.call("CSUnionPray",{},a,300,"SCUnionPray"),range(8)))
        self.assertTrue(all(response==responses[0]for response in responses))
        first=self.call("CSUnionLightChurchInfo",{},a,301,"SCUnionLightChurchInfo")
        self.assertEqual((first.prayTimes,first.contribution,first.invalidTime),(1,200,at+1000+86400000))
        p5.after_info(self,a,1,first)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_guild_prayer_rolls WHERE player_id="+str(a.playerId)+";").strip(),"1")
        self.assertEqual(self.call("CSGetUnionAddBaseProp",{},a,302,"SCGetUnionAddBaseProp").unionPrayProp,first.addProp)
        sa=self.connect(a)
        self.tcp_send(sa,"CSGetUnionAddBaseProp",{},a)
        self.assertEqual(self.recv(sa)[1].unionPrayProp,first.addProp)
        for n in [2,3]:
            self.set_clock(at+n*1000)
            self.call("CSUnionPray",{},a,310+n,"SCUnionPray")
            info=self.call("CSUnionLightChurchInfo",{},a,320+n,"SCUnionLightChurchInfo")
            self.assertEqual((info.prayTimes,info.contribution),(n,300-n*100))
            self.assertEqual(info.invalidTime,at+n*1000+86400000)
            p5.after_info(self,a,n,info)
        self.assertEqual(self.stamped("CSUnionPray",{},a,330)[0],10001)
        for n in range(5):self.call("CSUnionDonate",{"donateType":"coin"},b,400+n,"SCUnionDonate")
        p5.before_prayer(self,b,1)
        self.set_clock(at+4000)
        self.call("CSUnionPray",{},b,410,"SCUnionPray")
        second=self.call("CSUnionLightChurchInfo",{},b,411,"SCUnionLightChurchInfo")
        self.assertEqual((second.prayTimes,second.contribution),(1,0))
        p5.after_info(self,b,1,second)
        self.assertEqual(self.profile(a)[0]["coin"],500000)
        self.assertEqual(self.sql("SELECT contribution FROM p13_guild_members WHERE player_id="+str(a.playerId)+";").strip(),"300")
        frozen=self.sql("SELECT CONCAT(stamp,':',source_hash,':',HEX(effect),':',expires_at) FROM p13_guild_prayer_rolls WHERE player_id="+str(a.playerId)+" ORDER BY ordinal;")
        self.sql("UPDATE p13_guild_prayer_policies SET enabled=0,version='closed-next-version',pool_json='[]',pool_hash='changed';")
        self.call("CSUnionPray",{},a,313,"SCUnionPray")
        self.stop();self.set_clock(at+3600000);self.start();a,b=[self.login(uid)for uid in uids]
        cold=self.call("CSUnionLightChurchInfo",{},a,340,"SCUnionLightChurchInfo")
        self.assertEqual(cold.addProp,info.addProp);self.assertEqual(cold.invalidTime,info.invalidTime)
        p5.after_cold(self,a,cold)
        self.assertEqual(self.sql("SELECT CONCAT(stamp,':',source_hash,':',HEX(effect),':',expires_at) FROM p13_guild_prayer_rolls WHERE player_id="+str(a.playerId)+" ORDER BY ordinal;"),frozen)
        self.call("CSTransferChairman",{"playerId":b.playerId},a,350,"SCTransferChairman")
        self.call("CSQuitUnion",{},a,351,"SCQuitUnion")
        left=self.call("CSGetUnionAddBaseProp",{},a,352,"SCGetUnionAddBaseProp")
        self.assertEqual(left.unionPrayProp.ListFields(),[]);self.assertEqual(left.unionAddBaseProp.ListFields(),[])
        p5.after_leave(self,a,left)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_guild_prayer_rolls WHERE player_id="+str(a.playerId)+";").strip(),"3")
        self.set_clock(second.invalidTime)
        expired=self.call("CSUnionLightChurchInfo",{},b,412,"SCUnionLightChurchInfo")
        self.assertEqual(expired.invalidTime,0);self.assertEqual(expired.prayTimes,0);self.assertEqual(expired.addProp.ListFields(),[])
        p5.after_expiry(self,b,expired)
        self.stop()
        report={"status":"PASS_ORIGINAL_HTTP_TCP_P13_PRAYER_P5_PUBLIC_PANEL_COPY_START_COLD_EXPIRY_LEAVE","seconds":round(time.monotonic()-began,3),"sourceProofSHA256":hashlib.sha256((PREBUILT/"manifest.json").read_bytes()).hexdigest(),"actualSource":"15 original donations100000coin ->300 contribution ->3 actual100contribution prayers; b five actual donations->one prayer","finitePrerequisites":"Two original created accounts; explicit level30/coin2000000/diamond1000; no contribution, prayer effect, winner, rewards, or frozen battle injection","actualRolls":{"a":3,"b":1},"idempotentOriginalHTTP":8,"policyDefaultOff":True,"unknownPrivatePool":"explicit named local weighted percent pool; all immutable actual draws","frozenRows":frozen.splitlines(),"P5":self.p5_prayer_checks,"phone":False,"productionModified":False}
        (ROOT/"analysis/p13/prayer-native-checkpoint.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("original HTTP57131/57123 + TCP property read + P5 public panel/CopyStart, real15donations/3prayers/8once/cold/expiry/leave PASS",round(time.monotonic()-began,3))

if __name__=="__main__":
    with open("/tmp/hyjs-isolated-mysql.lock","a")as mutex:
        fcntl.flock(mutex,fcntl.LOCK_EX)
        unittest.main()
