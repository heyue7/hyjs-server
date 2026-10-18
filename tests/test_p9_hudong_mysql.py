"""Original three-mode PetHudong binary frames using locked disposable InnoDB."""
import concurrent.futures
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

OWNED = ['hyjs-server/service/p9PetHudongService.go', 'hyjs-server/service/p9PetHudong_test.go', 'hyjs-server/service/p9PetIntimacyService.go', 'hyjs-server/constUtil/p9Hudong.go', 'hyjs-server/db/p9PetHudongRule.go', 'hyjs-server/db/p9PetHudongState.go', 'hyjs-server/db/sql/104_p9_pet_hudong_rules.sql', 'hyjs-server/db/sql/104_p9_pet_hudong_states.sql', 'hyjs-server/db/sql/104_p9_pet_hudong_rules.seed.sql', 'tests/test_p9_hudong_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P9_HUDONG_MYSQL_TEST') == '1', 'Explicit locked isolated P9 hudong')
class PetHudongMySQLTests(unittest.TestCase):
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
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        fcntl.flock(cls.lock.fileno(), fcntl.LOCK_EX)
        cls.addClassCleanup(cls.lock.close)
        cls.database = 'hyjs_test_p9_hudong_' + secrets.token_hex(6)
        cls.test_user = 'p9hudong_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p9-hudong-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-hudong')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P9 hudong schema/user/grants remain')
            report = shared.ROOT / 'analysis/p9/hudong-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P9_HUDONG_BASE', shared.ROOT / 'build/candidates/joint-auto-recruit-v54-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('Hudong baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
        cls.frozen_source=source;cls.baseline_source=str(base.relative_to(shared.ROOT))
        (Path(cls.temp.name)/'analysis').symlink_to(shared.ROOT/'analysis',target_is_directory=True)
        cls.source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=source,capture_output=True,timeout=180)
        if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p9_hudong_') or not cls.test_user.startswith('p9hudong_'):raise AssertionError('P9 hudong isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P9 hudong administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_three_types_actual_clock_window_no_fee_and_cold(self):
        began=time.monotonic();self.start();uid='original-hudong';login=self.login(uid);stamp=1800060000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def fail(name,fields):
            nonlocal stamp
            stamp+=1;before=self.profile(login);self.assertEqual(self.stamped(name,fields,login,stamp)[0],10001);self.assertEqual(self.profile(login),before)
        call('CSPlayerCreate',{'playerName':'原三类互动完整链','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=34,noviceGuideSchedule=8);self.write_profile(login,p,g)
        pet=call('CSPetFirstRecruit',{},'SCPetFirstRecruit').get.pets[0]
        call('CSFormationOn',{'formatPos':1,'id':pet.id,'guid':pet.guid},'SCFormationOn')
        pet=call('CSPetHudongShow',{'guid':pet.guid},'SCPetHudongShow').pet
        favorite=pet.eat.critTrend;self.assertIn(favorite,[0,1,2]);self.assertEqual(list(pet.eat.hdTimeList),[0,0,0])
        before=self.profile(login);stamp+=1;first_stamp=stamp;request={'guid':pet.guid,'type':favorite}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSPetHudong',request,login,first_stamp,'SCPetHudong'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));first=replies[0]
        self.assertEqual((first.pet.eat.exp,first.pet.eat.hdTimeList[favorite],first.pet.eat.critedValue),(6,1,6))
        self.assertEqual(self.profile(login)[0]['fightPower'],before[0]['fightPower'])
        fail('CSPetHudong',request);fail('CSPetHudong',{'guid':pet.guid,'type':3});fail('CSPetHudong',{'guid':'foreign-guid','type':0})
        # Both remaining original types use actual five-second server waits.
        for kind in [x for x in range(3) if x!=favorite]:
            time.sleep(5.05);current=call('CSPetHudong',{'guid':pet.guid,'type':kind},'SCPetHudong').pet;self.assertEqual(current.eat.critedValue,3)
        self.assertEqual(current.eat.exp,12);self.assertEqual(list(current.eat.hdTimeList),[1,1,1])
        before_cold=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.profile(login),before_cold)
        current=call('CSPetHudongShow',{'guid':pet.guid},'SCPetHudongShow').pet;self.assertEqual(list(current.eat.hdTimeList),[1,1,1])
        # Finite eligibility-clock shifts cover reward5/zero6..10 without adding
        # experience, currency or attempts. Every increase still uses native frames.
        for kind in range(3):
            for _ in range(4):
                self.sql(f"UPDATE p9_pet_hudong_states SET last_at={int(time.time()*1000)-6000} WHERE guid='{pet.guid}';")
                current=call('CSPetHudong',{'guid':pet.guid,'type':kind},'SCPetHudong').pet
        self.assertEqual((current.eat.exp,list(current.eat.hdTimeList)),(60,[5,5,5]))
        for kind in range(3):
            for _ in range(5):
                self.sql(f"UPDATE p9_pet_hudong_states SET last_at={int(time.time()*1000)-6000} WHERE guid='{pet.guid}';")
                current=call('CSPetHudong',{'guid':pet.guid,'type':kind},'SCPetHudong').pet;self.assertEqual(current.eat.critedValue,0)
        self.assertEqual((current.eat.exp,list(current.eat.hdTimeList)),(60,[10,10,10]))
        self.sql(f"UPDATE p9_pet_hudong_states SET last_at={int(time.time()*1000)-6000} WHERE guid='{pet.guid}';")
        fail('CSPetHudong',request)
        self.assertEqual(self.stamped('CSPetHudong',request,login,first_stamp)[0],10001)
        row=self.sql(f"SELECT earned,applied,capped,window_at,HEX(input),input_hash FROM p9_pet_hudong_states WHERE guid='{pet.guid}';").strip().split('\t')
        self.assertEqual(list(map(int,row[:3])),[60,60,0]);self.assertEqual(hashlib.sha256(bytes.fromhex(row[4])).hexdigest(),row[5])
        self.assertEqual(int(self.sql(f"SELECT SUM(amount) FROM p1_event_facts WHERE player_id={login.playerId} AND kind='pet_hudong';").strip()),30)
        self.assertEqual(self.sql(f"SELECT CONVERT(costs USING utf8mb4) FROM p9_pet_investments WHERE guid='{pet.guid}';").strip(),'{}')
        self.sql("UPDATE p9_pet_hudong_rules SET input='{}' WHERE id='hudong';")
        self.assertEqual(call('CSPetHudongShow',{'guid':pet.guid},'SCPetHudongShow').pet,current)
        self.sql(f"UPDATE p9_pet_hudong_rules SET input=X'{row[4]}' WHERE id='hudong'; UPDATE p9_pet_hudong_states SET window_at={int(row[3])-6*3600000},last_at={int(time.time()*1000)-6000} WHERE guid='{pet.guid}';")
        renewed=call('CSPetHudongShow',{'guid':pet.guid},'SCPetHudongShow').pet;self.assertEqual(list(renewed.eat.hdTimeList),[0,0,0]);self.assertEqual(renewed.eat.exp,60)
        stamp+=1;renew_stamp=stamp;renew_request={'guid':pet.guid,'type':renewed.eat.critTrend}
        renewed_call=self.call('CSPetHudong',renew_request,login,renew_stamp,'SCPetHudong');self.assertEqual(renewed_call.pet.eat.exp,66)
        # Cross the actual original626 threshold using ten more eligible windows.
        # Only window/CD eligibility is shifted; every XP point is native-earned.
        for _ in range(10):
            old_window=int(self.sql(f"SELECT window_at FROM p9_pet_hudong_states WHERE guid='{pet.guid}';").strip())
            self.sql(f"UPDATE p9_pet_hudong_states SET window_at={old_window-6*3600000},last_at={int(time.time()*1000)-6000} WHERE guid='{pet.guid}';")
            window_pet=call('CSPetHudongShow',{'guid':pet.guid},'SCPetHudongShow').pet
            for kind in range(3):
                for _ in range(5):
                    self.sql(f"UPDATE p9_pet_hudong_states SET last_at={int(time.time()*1000)-6000} WHERE guid='{pet.guid}';")
                    renewed_call=call('CSPetHudong',{'guid':pet.guid,'type':kind},'SCPetHudong')
        self.assertEqual((renewed_call.pet.eat.level,renewed_call.pet.eat.exp,len(renewed_call.pet.randPropList)),(1,40,1))
        self.assertGreater(self.profile(login)[0]['fightPower'],before[0]['fightPower'])
        free_totals=list(map(int,self.sql(f"SELECT earned,applied,capped FROM p9_pet_hudong_states WHERE guid='{pet.guid}';").strip().split('\t')))
        self.assertEqual(free_totals,[666,666,0])
        renew_stamp=stamp;renew_request={'guid':pet.guid,'type':2}
        saved=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSPetHudong',renew_request,login,renew_stamp,'SCPetHudong'),renewed_call);self.assertEqual(self.profile(login),saved)
        self.sql("UPDATE p9_pet_hudong_rules SET enabled=0 WHERE id='hudong';")
        fail('CSPetHudong',renew_request)
        self.assertEqual(self.call('CSPetHudong',renew_request,login,renew_stamp,'SCPetHudong'),renewed_call)
        final=self.profile(login);self.assertEqual((final[0]['coin'],final[0]['diamond']),(before[0]['coin'],before[0]['diamond']))
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('p9_pet_hudong_rules','p9_pet_hudong_states');").strip(),'InnoDB')
        report={'date':'2026-10-14','policy':'pet-hudong-v1','originalTypes':'bath0/touch1/play2','originalConstants':'6h update/5s cooldown/first5/maxExp3, original favorite doubles text; service probabilities/window anchors unknown','actualClock':'three first calls use actual5.05s waits and original no-cost frames','chain':'original first pet→guard→all three HUD modes→actual666 free intimacy crosses626 to layer1/40XP/one random attribute→real fightPower increase→cold same pet','concurrent8First':'one6XP/one count and exact response','all30Attempts':'first15 totals60XP, calls6..10 pertype zero/31st same-type refused','journalConservation':[666,666,0],'eventFacts':'181 real native mutations, current server timestamps; first30 verified separately','zeroCostInvestment':'new verified base retains empty ledger, no free XP refund material minted','window':'only old six-hour eligibility cursor fixture, reset attempts without resetting60XP','hotEdit':'current frozen input unchanged, next period requires valid rule','cold':'current exact receipt survives enabled/disabled, old changed projection rejects','sourceUnknown':'local three equal preferences,3/6 and anchored windows, critedValue actual applied projection; original private server semantics unknown; food multiplier remains existing','preparation':'level34/guide8 only, actual first3 CD real; remaining calls and rollover shift only isolated eligibility clocks; no resources/intimacy/counts inserted','phoneUI':False,'productionModified':False,'seconds':round(time.monotonic()-began,3),'inputHash':row[5],'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'ownedSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'cleanup':'whole-life filelock/dedicated schema-user-grants/temp source-binary cleanup'}
        (shared.ROOT/'analysis/p9/hudong-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
