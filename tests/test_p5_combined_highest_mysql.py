"""Real consecutive combined maximum growth on disposable InnoDB and cold wire."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
import unittest
from google.protobuf.json_format import ParseDict
from server.codec import BY_NAME, message
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as stressed_transport


@unittest.skipUnless(os.environ.get('HYJS_P5_COMBINED_MYSQL_TEST')=='1','Explicit disposable combined actual highest validation')
class CombinedHighestMySQLTests(unittest.TestCase):
    setUpClass=classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql=classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send=shared.TalentScopeMySQLTests.send
    stamped=stressed_transport.GameCreationMySQLTests.stamped
    call=shared.TalentScopeMySQLTests.call
    profile=shared.TalentScopeMySQLTests.profile
    start=shared.TalentScopeMySQLTests.start
    stop=shared.TalentScopeMySQLTests.stop
    setUp=shared.TalentScopeMySQLTests.setUp
    sql=shared.TalentScopeMySQLTests.sql

    def test_original_mutations_continuous_highest_mysql_cold_exact_wire(self):
        output=Path(self.temp.name)/'actual-combined.json'
        env=dict(self.env,HYJS_P5_COMBINED_HIGHEST_SCOPE='hero_001',HYJS_P5_COMBINED_ACTOR_OUTPUT=str(output))
        began=time.monotonic()
        result=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP5ActualCombinedHighest$','-count=1','-timeout=30m'],cwd=self.frozen_source,env=env,text=True,capture_output=True,timeout=1860)
        self.assertEqual(result.returncode,0,result.stdout[-3000:]+result.stderr[-1000:])
        elapsed=round(time.monotonic()-began,3)
        data=json.loads(output.read_text());self.assertEqual(len(data['characterPaths']),1)
        actor=data['characterPaths'][0];expected=message('BattleHero');ParseDict(actor['battleHero'],expected)
        self.start()
        def login():
            opcode,response=self.send('CSUserLogin',{'platformUid':'combined-highest-hero_001','authCode':'isolated-test-only','clientTime':1700000000000})
            self.assertEqual(opcode,BY_NAME['SCUserLogin']);return response
        account=login();stamp=1800001800000
        self.call('CSCopyEnter',{'chapterId':'1','missionId':'1'},account,stamp,'SCCopyEnter')
        battle=self.call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},account,stamp+1,'SCCopyRunMapInfo')
        actual=next(w for w in battle.battlePlayer.formation if w.guid==expected.guid);self.assertEqual(actual,expected)
        self.stop();self.start();account=login()
        battle=self.call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},account,stamp+2,'SCCopyRunMapInfo')
        self.assertEqual(next(w for w in battle.battlePlayer.formation if w.guid==expected.guid),expected)
        (shared.ROOT/'analysis/p5/combined-highest-mysql-validation.json').write_text(json.dumps({'date':'2026-10-14','scope':'one original red001 first; complete76 combinations are separate full real matrices','continuousPaidOriginalDomainInnoDB':'passed','domainSeconds':elapsed,'actualBreachTenStepRequests':actor['actualTenStepBreachRequests'],'actualTrainingDraws':actor['actualTrainingDraws'],'actualSpecialtyUpgrades':actor['actualSpecialtyUpgrades'],'originalColdCopyActorWire':'entire BattleHero identical to actual consecutive growth and before/after process restart','sourceSha256':self.source_shas,'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'productionModified':False,'preparation':data['preparation'],'privatePolicyBoundary':data['privatePolicyBoundary']},ensure_ascii=False,indent=2)+'\n')
        (shared.ROOT/'analysis/p6/p5-combined-mysql-hero001-highest-actor-20261014.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
