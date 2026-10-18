"""Opt-in original Awake HTTP/InnoDB true battle, 100 score and P5 growth acceptance.

Historical equipped normal001003 is an explicit isolated prerequisite, preserving
new-account wallets/items and all progress. No battle outcome or receipt import.
Original typed16 native proof is required; role awakening remains a separate full-scope task.
"""
import concurrent.futures
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_p7_maze_trader_mysql import P7MazeTraderMySQLTests as Transport
from test_p7_rush_party_mysql import historical_roster, fixture_sql, ROSTER_SHA
from server.codec import message
ROOT=Path(__file__).resolve().parents[1]

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

@unittest.skipUnless(os.environ.get('HYJS_P7_AWAKE_PLAYABLE_MYSQL_TEST')=='1','Root coordinated managed SQL window')
class P7AwakePlayableMySQLTests(unittest.TestCase):
    send=Transport.send
    login=Transport.login
    stamped=Transport.stamped
    call=Transport.call
    profile=Transport.profile
    start=Transport.start
    stop=Transport.stop
    setUp=Transport.setUp
    sql=Transport.sql

    def test_original_real_battle_hundred_p5_growth_cold_and_next_run(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source=Path(os.environ['HYJS_P7_AWAKE_SOURCE']).resolve()
        self.binary=Path(os.environ['HYJS_P7_AWAKE_BINARY']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        self.assertEqual(digest(self.binary),os.environ['HYJS_P7_AWAKE_BINARY_SHA256'])
        output=Path(os.environ['HYJS_P7_AWAKE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT/'analysis/p7'))
        self.assertFalse(output.exists(),'Keep each acceptance immutable')
        local=time.gmtime(time.time()+8*3600)
        self.assertIn(local.tm_wday,(4,5,6),'Original chapter5 opens Friday/Saturday/Sunday; no calendar rewrite')
        self.assertGreater(86400-local.tm_hour*3600-local.tm_min*60-local.tm_sec,300,'Two-run same-day daily-reward assertion needs five minutes before UTC+8 midnight')
        evidence=output.with_name(output.stem+'-wire');self.assertFalse(evidence.exists());evidence.mkdir(parents=True)
        for name,fields in [('CSAwakeCopySettle',set()),('CSAwakeCopyOpenBox',{'boxGuid'}),('CSEnterAwakeCopy',{'copyId','items'})]:
            self.assertEqual(set(message(name).DESCRIPTOR.fields_by_name),fields)
        calculator=Path(os.environ['HYJS_P7_AWAKE_SERVICE_TEST']).resolve()
        self.assertEqual(digest(calculator),os.environ['HYJS_P7_AWAKE_SERVICE_TEST_SHA256'])
        proof=os.environ['HYJS_P7_AWAKE_PROOF_SHA256']
        self.assertEqual(len(proof),64);self.assertTrue(all(c in '0123456789abcdef'for c in proof))
        roster=historical_roster()
        report={'status':'PENDING','binarySHA256':digest(self.binary),'source':str(self.source),'serviceTestSHA256':digest(calculator),'scopeProof':proof,
                'finiteHistoricalEquippedPrerequisiteSHA256':ROSTER_SHA,'ordinaryInventoryOrWalletImported':False,
                'battleResultOrReceiptSeeded':False,'formalAccount4Modified':False,'phoneUI':False,
                'fullChain':False,'fullAwakeContent':False,'chainScope':'local501 two true100 runs and original P5 growth','battle16NativeProof':True,'p5MaterialConsumption':False,'roleAwakeStage1':False}
        began=time.monotonic();self.managed_pool=ManagedMySQL()
        with tempfile.TemporaryDirectory(prefix='hyjs-p7-awake-') as temporary:
            self.native_log=Path(temporary)/'native.log'
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    allocation=self.managed_pool.allocate('p7_awake_http')
                    self.database,self.test_user=allocation.database,allocation.user
                    self.test_password=allocation.password
                    self.env={**os.environ,**allocation.env(),'GOMEMLIMIT':'768MiB','GOMAXPROCS':'2'}
                    self.start()
                    tables=('profiles','p7_awake_runs','p7_awake_explorations','p7_awake_fights','p6_battle_records','p5_mutation_receipts','p1_event_facts','p7_awake_states','p7_awake_policies','p7_awake_stage_rules')
                    engines=self.sql('SELECT CONCAT(TABLE_NAME,"|",ENGINE) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('+','.join('"'+x+'"'for x in tables)+');').splitlines()
                    self.assertEqual({x.split('|')[0]for x in engines},set(tables));self.assertTrue(all(x.endswith('|InnoDB')for x in engines))
                    self.assertEqual(self.sql('SELECT enabled FROM p7_awake_policies WHERE id=2;').strip(),'0')
                    source_hex=self.sql('SELECT HEX(source) FROM p7_awake_policies WHERE id=2;').strip()
                    self.assertTrue(bytes.fromhex(source_hex).decode().startswith('local-awake-forest-explore-v1:'))
                    battle_hex=self.sql('SELECT HEX(source) FROM p7_awake_policies WHERE id=3 AND enabled=0;').strip()
                    self.assertTrue(bytes.fromhex(battle_hex).decode().startswith('local-awake16-forest-v1:'))
                    identity='awake-explore-'+secrets.token_hex(6);owner=self.login(identity);stamp=380000
                    def http(name,fields,expected):
                        nonlocal stamp
                        stamp+=1
                        response=self.call(name,fields,owner,stamp,expected)
                        (evidence/f'{stamp}-{expected}.pb').write_bytes(response.SerializeToString(deterministic=True))
                        return response
                    def item_count(profile,item):
                        return sum(x['num']for x in(profile[1].get('items')or[])if x['id']==item)
                    http('CSPlayerCreate',{'playerName':'原觉醒探索','heroId':'hero_001'},'SCPlayerCreate')
                    fresh=self.profile(owner);self.sql(fixture_sql(owner.playerId,roster));prepared=self.profile(owner)
                    expected=copy.deepcopy(fresh);expected[0].update(roster['playerPatch']);expected[1].update(roster['gamePatch'])
                    self.assertEqual(prepared,expected)
                    http('CSFormationOn',{'id':roster['pet']['id'],'guid':roster['pet']['guid'],'formatPos':1},'SCFormationOn')
                    baseline=self.profile(owner)
                    self.assertEqual(baseline[1].get('items'),fresh[1].get('items'))
                    catalogue=http('CSAwakeCopyData',{},'SCAwakeCopyData')
                    chapter=next(x for x in catalogue.copys if x.chapterId=='5')
                    self.assertEqual(catalogue.lastNum,2)
                    self.assertEqual(chapter.isOpen,1,'This original weekday batch requires Friday/Saturday/Sunday; do not rewrite calendar')
                    http('CSEnterAwakeCopy',{'copyId':'501'},'ErrorMessage')
                    # Both exact source rows open in one isolated transaction. Production is never targeted.
                    activation=('START TRANSACTION; SELECT id FROM p7_awake_policies WHERE id=2 FOR UPDATE; '
                                'SELECT id FROM p7_awake_policies WHERE id=3 FOR UPDATE; '
                                'SELECT copy_id FROM p7_awake_stage_rules WHERE copy_id="501" FOR UPDATE; '
                                f'SET @ready=(SELECT COUNT(*)=1 FROM p7_awake_policies WHERE id=2 AND enabled=0 AND min_level=70 AND source=CONVERT(0x{source_hex} USING utf8mb4)) AND '
                                f'(SELECT COUNT(*)=1 FROM p7_awake_stage_rules WHERE copy_id="501" AND chapter_id="5" AND enabled=0 AND content_restored=1 AND source=CONVERT(0x{source_hex} USING utf8mb4)) AND '
                                f'(SELECT COUNT(*)=1 FROM p7_awake_policies WHERE id=3 AND enabled=0 AND min_level=70 AND source=CONVERT(0x{battle_hex} USING utf8mb4)); '
                                'UPDATE p7_awake_policies SET enabled=1 WHERE id=2 AND @ready; '
                                'UPDATE p7_awake_stage_rules SET enabled=1 WHERE copy_id="501" AND @ready; '
                                'UPDATE p7_awake_policies SET enabled=1 WHERE id=3 AND @ready; SELECT @ready; COMMIT;')
                    self.assertEqual(self.sql(activation).strip().splitlines()[-1],'1')
                    report['actualBattles']=[]
                    last_end=None;last_accepted=None;first_receipt=None
                    for run_index in (1,2):
                        enter=http('CSEnterAwakeCopy',{'copyId':'501'},'SCEnterAwakeCopy')
                        self.assertEqual((len(enter.boxes),len(enter.monsters),enter.refreshData.score),(4,1,0))
                        self.assertIn('/',enter.refreshData.hpPercent)
                        self.assertEqual(http('CSEnterAwakeCopy',{'copyId':'501'},'SCEnterAwakeCopy'),enter)
                        if run_index==2:
                            self.assertEqual(http('CSAwakeCopyFightEnd',last_end,'SCAwakeCopyFightEnd'),last_accepted)
                            self.assertEqual(self.call('CSAwakeCopySettle',{},owner,first_settle_stamp,'SCAwakeCopySettle'),first_receipt)
                        for index,box in enumerate(enter.boxes):
                            with concurrent.futures.ThreadPoolExecutor(max_workers=4)as pool:
                                replies=list(pool.map(lambda _:self.call('CSAwakeCopyOpenBox',{'boxGuid':box.guid},owner,stamp+1,'SCAwakeCopyOpenBox'),range(4)))
                            self.assertEqual(sum(bool(x.rewards)for x in replies),1)
                            self.assertTrue(all(x.refreshData.score==(index+1)*5 for x in replies))
                        before_end=self.profile(owner)
                        start=http('CSAwakeCopyFightBegin',{'monGuid':enter.monsters[0].guid},'SCAwakeCopyFightBegin')
                        self.assertEqual([x.id for x in start.battlePlayer.formation],['hero_001','hero_003'])
                        start_file=evidence/f'{run_index:03}.SCAwakeCopyFightBegin.pb';start_file.write_bytes(start.SerializeToString(deterministic=True))
                        terminal_file=evidence/f'{run_index:03}.computed.json'
                        env={**self.env,'HYJS_P7_AWAKE_HTTP_START':str(start_file),'HYJS_P7_AWAKE_HTTP_START_SHA256':digest(start_file),'HYJS_P7_AWAKE_HTTP_OUTPUT':str(terminal_file)}
                        calculation=subprocess.run([str(calculator),'-test.run=^TestP7AwakeExportOriginalHTTPStart$','-test.v'],cwd=ROOT/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=120)
                        self.assertEqual(calculation.returncode,0,(calculation.stdout+calculation.stderr)[-3000:])
                        self.assertIn('--- PASS: TestP7AwakeExportOriginalHTTPStart ',calculation.stdout)
                        terminal=json.loads(terminal_file.read_bytes());self.assertFalse(terminal['outcome']['authoritative'])
                        actual=int(terminal['outcome']['winner']==0)
                        end={'fightId':start.fightId,'isWinClient':actual,'optRecord':terminal['originalOperates']}
                        self.assertEqual(actual,1,'Preserve a true defeat; never replace the winner')
                        frozen=self.sql(f'SELECT CONCAT(scope_proof,"|",status) FROM p7_awake_fights WHERE player_id={owner.playerId} AND id="{start.fightId}";').strip()
                        self.assertEqual(frozen,proof+'|pending')
                        if run_index==1:
                            for variant in ('false-result','HP','missing-op','duplicate-op','transform'):
                                bad=copy.deepcopy(end)
                                if variant=='false-result':bad['isWinClient']=0
                                elif variant=='HP':bad['optRecord']['attackerTotalLeftHp']+=1
                                elif variant=='missing-op':bad['optRecord']['skillOpts']=bad['optRecord']['skillOpts'][1:]
                                elif variant=='duplicate-op':bad['optRecord']['skillOpts'].append(copy.deepcopy(bad['optRecord']['skillOpts'][0]))
                                else:bad['optRecord']['attackerTransOpt']={'roundNum':1}
                                http('CSAwakeCopyFightEnd',bad,'ErrorMessage');self.assertEqual(self.profile(owner),before_end)
                            http('CSAwakeCopySettle',{},'ErrorMessage')
                            self.sql('UPDATE p7_awake_policies SET enabled=0,source="closed-after-paid" WHERE id=3;')
                            self.stop();self.start();owner=self.login(identity)
                            self.assertEqual(http('CSAwakeCopyFightBegin',{'monGuid':enter.monsters[0].guid},'SCAwakeCopyFightBegin'),start)
                        with concurrent.futures.ThreadPoolExecutor(max_workers=4)as pool:
                            ended=list(pool.map(lambda _:self.call('CSAwakeCopyFightEnd',end,owner,stamp+1,'SCAwakeCopyFightEnd'),range(4)))
                        self.assertTrue(all(x==ended[0]for x in ended));accepted=ended[0]
                        self.assertEqual((accepted.isWinServer,accepted.refreshData.score,accepted.fightMonster.hpPercent),(1,100,0))
                        self.assertEqual(self.profile(owner),before_end,'Real End retains pending materials until original Settle')
                        report['actualBattles'].append({'run':run_index,'fightId':start.fightId,'startSHA256':digest(start_file),'calculationSHA256':digest(terminal_file),'actualWinner':terminal['outcome']['winner'],'actualFinalRound':terminal['outcome']['finalRound']})
                        stamp+=2;settle_stamp=stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=4)as pool:
                            receipts=list(pool.map(lambda _:self.call('CSAwakeCopySettle',{},owner,settle_stamp,'SCAwakeCopySettle'),range(4)))
                        self.assertTrue(all(x==receipts[0]for x in receipts));receipt=receipts[0]
                        self.assertEqual(receipt.score,100)
                        self.assertEqual(len(receipt.ScoreRewards),2 if run_index==1 else 0)
                        paid=self.profile(owner)
                        self.assertEqual(item_count(paid,'item_320')-item_count(baseline,'item_320'),19*run_index)
                        if run_index==1:
                            self.assertIn(paid[0]['coin'],(5100,5200))
                            material=next(x for x in receipt.totalRewards.wars if x.id=='hero_exp_002')
                            target=next(x for x in paid[1]['warriors']if x['id']=='hero_003')
                            growth_fields={'guid':target['guid'],'cost':[{'id':material.id,'guid':material.guid}]}
                            stamp+=1;growth_stamp=stamp
                            growth=self.call('CSWarriorStrengthen',growth_fields,owner,growth_stamp,'SCWarriorStrengthen')
                            grown=self.profile(owner)
                            self.assertEqual(grown[0]['coin'],paid[0]['coin']-5000)
                            self.assertFalse(any(x['guid']==material.guid for x in grown[1]['warriors']))
                            self.assertGreater(next(x for x in grown[1]['warriors']if x['guid']==target['guid'])['level'],target['level'])
                            first_receipt=receipt;first_settle_stamp=settle_stamp
                            self.stop();self.start();owner=self.login(identity)
                            self.assertEqual(http('CSAwakeCopyFightEnd',end,'SCAwakeCopyFightEnd'),accepted)
                            self.assertEqual(self.call('CSAwakeCopySettle',{},owner,settle_stamp,'SCAwakeCopySettle'),receipt)
                            self.assertEqual(self.call('CSWarriorStrengthen',growth_fields,owner,growth_stamp,'SCWarriorStrengthen'),growth)
                            self.assertEqual(self.profile(owner),grown)
                            self.sql(f'UPDATE p7_awake_policies SET enabled=1,source=CONVERT(0x{battle_hex} USING utf8mb4) WHERE id=3;')
                        else:
                            self.assertIn(paid[0]['coin']-before_end[0]['coin'],(100,200),'Same-day second100 must not mint another5000 score reward')
                        last_end=end;last_accepted=accepted
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p6_battle_records WHERE player_id={owner.playerId} AND source="p7-awake16" AND authoritative=1;').strip(),'2')
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p1_event_facts WHERE player_id={owner.playerId} AND kind="awake-local-daily-score";').strip(),'1')
                    self.assertEqual(http('CSAwakeCopyData',{},'SCAwakeCopyData').lastNum,0)
                    http('CSEnterAwakeCopy',{'copyId':'501'},'ErrorMessage')
                    report.update(status='PASS_ORIGINAL_HTTP_INNODB_AWAKE16_HUNDRED_P5_GROWTH_COLD_REPLAY',
                                  fullChain=True,p5MaterialConsumption=True,actualRuns=2,actualScoreEach=100,
                                  earnedWindStoneDelta=38,concurrentEndOnce=True,concurrentSettleOnce=True,
                                  dailyScoreRewardOnce=True,closedPolicyFrozenPending=True,oldReceiptsIsolateNextRun=True,
                                  coldP5MaterialConsumedOnce=True,roleAwakeStage1=False)
                except BaseException as error:
                    report.update(status='FAIL',error=self.managed_pool.redact(repr(error)));raise
                finally:
                    cleanup=[]
                    try:self.stop()
                    except BaseException as error:cleanup.append(type(error).__name__)
                    try:self.managed_pool.close()
                    except BaseException as error:cleanup.append(type(error).__name__)
                    report.update(wallSeconds=round(time.monotonic()-began,3),cleanupErrors=cleanup,
                                  remainingOwnedAllocations=len(self.managed_pool.connections))
                    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
                    if cleanup:self.fail('Owned resource cleanup requires inspection: '+','.join(cleanup))
if __name__=='__main__':
    unittest.main()
