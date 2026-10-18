"""Opt-in original Awake HTTP/InnoDB exploration acceptance using a pinned app.

Historical equipped normal001003 is an explicit isolated prerequisite, preserving
new-account wallets/items and all progress. No battle outcome or receipt import.
Battle16 and complete P5 consumption remain separately pending native evidence.
"""
import concurrent.futures
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
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

@unittest.skipUnless(os.environ.get('HYJS_P7_AWAKE_MYSQL_TEST')=='1','Root coordinated managed SQL window')
class P7AwakeExplorationMySQLTests(unittest.TestCase):
    send=Transport.send
    login=Transport.login
    stamped=Transport.stamped
    call=Transport.call
    profile=Transport.profile
    start=Transport.start
    stop=Transport.stop
    setUp=Transport.setUp
    sql=Transport.sql

    def test_original_entry_boxes_partial_exit_cold_concurrent_once(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source=Path(os.environ['HYJS_P7_AWAKE_SOURCE']).resolve()
        self.binary=Path(os.environ['HYJS_P7_AWAKE_BINARY']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        self.assertEqual(digest(self.binary),os.environ['HYJS_P7_AWAKE_BINARY_SHA256'])
        output=Path(os.environ['HYJS_P7_AWAKE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT/'analysis/p7'))
        self.assertFalse(output.exists(),'Keep each acceptance immutable')
        evidence=output.with_name(output.stem+'-wire');self.assertFalse(evidence.exists());evidence.mkdir(parents=True)
        for name,fields in [('CSAwakeCopySettle',set()),('CSAwakeCopyOpenBox',{'boxGuid'}),('CSEnterAwakeCopy',{'copyId','items'})]:
            self.assertEqual(set(message(name).DESCRIPTOR.fields_by_name),fields)
        roster=historical_roster()
        report={'status':'PENDING','binarySHA256':digest(self.binary),'source':str(self.source),
                'finiteHistoricalEquippedPrerequisiteSHA256':ROSTER_SHA,'ordinaryInventoryOrWalletImported':False,
                'battleResultOrReceiptSeeded':False,'formalAccount4Modified':False,'phoneUI':False,
                'fullChain':False,'battle16NativeProof':False,'p5MaterialConsumption':False}
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
                    tables=('profiles','p7_awake_runs','p7_awake_explorations','p7_awake_states','p7_awake_policies','p7_awake_stage_rules')
                    engines=self.sql('SELECT CONCAT(TABLE_NAME,"|",ENGINE) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('+','.join('"'+x+'"'for x in tables)+');').splitlines()
                    self.assertEqual({x.split('|')[0]for x in engines},set(tables));self.assertTrue(all(x.endswith('|InnoDB')for x in engines))
                    self.assertEqual(self.sql('SELECT enabled FROM p7_awake_policies WHERE id=2;').strip(),'0')
                    source_hex=self.sql('SELECT HEX(source) FROM p7_awake_policies WHERE id=2;').strip()
                    self.assertTrue(bytes.fromhex(source_hex).decode().startswith('local-awake-forest-explore-v1:'))
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
                                'SELECT copy_id FROM p7_awake_stage_rules WHERE copy_id="501" FOR UPDATE; '
                                f'SET @ready=(SELECT COUNT(*)=1 FROM p7_awake_policies WHERE id=2 AND enabled=0 AND min_level=70 AND source=CONVERT(0x{source_hex} USING utf8mb4)) AND '
                                f'(SELECT COUNT(*)=1 FROM p7_awake_stage_rules WHERE copy_id="501" AND chapter_id="5" AND enabled=0 AND content_restored=1 AND source=CONVERT(0x{source_hex} USING utf8mb4)); '
                                'UPDATE p7_awake_policies SET enabled=1 WHERE id=2 AND @ready; '
                                'UPDATE p7_awake_stage_rules SET enabled=1 WHERE copy_id="501" AND @ready; SELECT @ready; COMMIT;')
                    self.assertEqual(self.sql(activation).strip().splitlines()[-1],'1')
                    enter=http('CSEnterAwakeCopy',{'copyId':'501'},'SCEnterAwakeCopy')
                    self.assertEqual((len(enter.boxes),len(enter.monsters),enter.refreshData.score),(4,1,0))
                    self.assertEqual(http('CSEnterAwakeCopy',{'copyId':'501'},'SCEnterAwakeCopy'),enter)
                    self.assertEqual(http('CSAwakeCopyData',{},'SCAwakeCopyData').lastNum,1)
                    http('CSAwakeCopyOpenBox',{'boxGuid':'foreign-node'},'ErrorMessage')
                    http('CSAwakeCopyFightBegin',{'monGuid':enter.monsters[0].guid},'ErrorMessage')
                    http('CSAwakeCopyFightEnd',{'fightId':'fake','isWinClient':1},'ErrorMessage')
                    for index,box in enumerate(enter.boxes):
                        with concurrent.futures.ThreadPoolExecutor(max_workers=4)as pool:
                            replies=list(pool.map(lambda _:self.call('CSAwakeCopyOpenBox',{'boxGuid':box.guid},owner,stamp+1,'SCAwakeCopyOpenBox'),range(4)))
                        self.assertEqual(sum(bool(x.rewards)for x in replies),1)
                        self.assertTrue(all(x.refreshData.score==(index+1)*5 for x in replies))
                    self.assertEqual(self.profile(owner),baseline,'Open box should retain pending materials until Settle')
                    self.sql('UPDATE p7_awake_policies SET enabled=0,source="changed-after-paid" WHERE id=2; UPDATE p7_awake_stage_rules SET enabled=0,content="changed-after-paid" WHERE copy_id="501";')
                    self.stop();self.start();owner=self.login(identity)
                    resumed=http('CSEnterAwakeCopy',{'copyId':'501'},'SCEnterAwakeCopy')
                    self.assertEqual(resumed.refreshData.score,20);self.assertTrue(all(x.isOpen == 1 for x in resumed.boxes))
                    with concurrent.futures.ThreadPoolExecutor(max_workers=4)as pool:
                        replies=list(pool.map(lambda _:self.call('CSAwakeCopySettle',{},owner,stamp+1,'SCAwakeCopySettle'),range(4)))
                    self.assertTrue(all(x==replies[0]for x in replies));settled=replies[0]
                    self.assertEqual((settled.score,settled.coinRewards,settled.exp,len(settled.ScoreRewards)),(20,0,0,0))
                    final=self.profile(owner)
                    self.assertEqual(item_count(final,'item_320')-item_count(baseline,'item_320'),19)
                    for key in ('coin','diamond','point','energy','stamina'):
                        self.assertEqual(final[0].get(key,0),baseline[0].get(key,0))
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p7_awake_explorations WHERE player_id={owner.playerId} AND status="settled";').strip(),'1')
                    self.stop();self.start();owner=self.login(identity)
                    self.assertEqual(http('CSAwakeCopySettle',{},'SCAwakeCopySettle'),settled)
                    self.assertEqual(self.profile(owner),final)
                    http('CSEnterAwakeCopy',{'copyId':'501'},'ErrorMessage')
                    self.assertEqual(self.sql(f'SELECT used FROM p7_awake_states WHERE player_id={owner.playerId};').strip(),'1')
                    report.update(status='PASS_ORIGINAL_HTTP_INNODB_PARTIAL_EXPLORATION',actualBoxes=4,actualScore=20,
                                  earnedWindStoneDelta=19,concurrentBoxOnce=True,concurrentSettleOnce=True,
                                  closedPolicyFrozen=True,coldResumeAndReceipt=True,battleStillRejected=True)
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
