"""Original82057 real period opponents -> exact mode20 -> no-proof atomic deny.

No seed/winner/score/wallet is injected. Only finite isolated level qualification
and an explicit prepublished local operator period are prepared. SQL remains
opt-in under the root-coordinated managed window and uses a pinned prebuilt.
"""
import fcntl
import base64
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from server.codec import CLASSES, message
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport

ROOT = Path(__file__).resolve().parents[1]


def proto_values(value):
    """Keep original generated Go JSON scalar types without int64 text coercion."""
    output = {}
    for field, item in value.ListFields():
        if field.is_repeated:
            output[field.name] = [proto_values(v) if field.message_type else v for v in item]
        elif field.message_type:
            output[field.name] = proto_values(item)
        else:
            output[field.name] = base64.b64encode(item).decode() if field.type == field.TYPE_BYTES else item
    return output


@unittest.skipUnless(os.environ.get('HYJS_P11_LIMIT_FIGHT_MYSQL_TEST') == '1',
                     'explicit coordinated managed mode20 SQL window')
class P11LimitFightMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_original82057_real_period_freeze_unsupported_atomic_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P11_LIMIT_FIGHT_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        code = json.loads((ROOT/'analysis/p11/limit-fight-code-manifest.json').read_text())
        shared = json.loads((ROOT/code['sharedManifest']).read_text())
        proof = json.loads((ROOT/'analysis/p11/limit-fight-go-validation.json').read_text())
        runtime = {}
        for row in code['files']+shared['files']:
            actual = hashlib.sha256((self.source/row['path']).read_bytes()).hexdigest()
            self.assertEqual(actual, row['sha256'], row['path'])
            runtime[row['path']] = actual
        for rel, expected in proof['indirectRuntimeSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), expected)
        prebuilt = Path(os.environ['HYJS_P11_LIMIT_FIGHT_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt, self.source/'limit-native')
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        helper_sha = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper_sha, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        began = time.monotonic()
        report = dict(status='PENDING', runtimeSHA256=runtime, managedHelperSHA256=helper_sha,
                      formalModified=False, phoneUI=False, currentMode20Proof='empty/defaultdeny')
        checkpoint = ROOT/'analysis/p11/limit-fight-native-checkpoint.json'
        def save():
            checkpoint.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
        with tempfile.TemporaryDirectory(prefix='hyjs-limit20-native-') as temporary:
            self.binary = Path(temporary)/'limit-native'
            shutil.copy2(prebuilt,self.binary)
            self.managed_pool = ManagedMySQL()
            connection = None
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    connection=self.managed_pool.allocate('p11_limit_fight')
                    self.database,self.test_user=connection.database,connection.user
                    self.test_password=connection.password
                    self.env=dict(os.environ,**connection.env(),GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    self.assertEqual(self.sql('SELECT @@character_set_client;').strip(),'utf8mb4')
                    for suffix in ['policies.sql','offers.sql','matches.sql','settlements.sql','policies.seed.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/('163_p11_limit_fight_'+suffix)).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_limit_fight_policies;').strip(),'0')
                    identities=['limit20-real-'+secrets.token_hex(5) for _ in range(2)]
                    actors=[self.login(v) for v in identities]
                    stamp=120000
                    def http(name,fields,actor,expected):
                        nonlocal stamp
                        stamp+=1
                        return self.call(name,fields,actor,stamp,expected)
                    for index,actor in enumerate(actors):
                        created=http('CSPlayerCreate',{'playerName':'原限时竞技'+str(index),'heroId':'hero_047'},actor,'SCPlayerCreate')
                        player,game=self.profile(actor)
                        self.assertEqual((player.get('coin',0),player.get('diamond',0)),(0,0))
                        self.assertTrue(created.player.heroWarId)
                        player.update(level=20,noviceGuideSchedule=8)
                        # Both actual mains retain their original GUID, complete
                        # skills and normal formation. No P6 result is prepared.
                        main=next(v for v in game['warriors'] if v['guid']==created.player.heroWarId)
                        main.update(level=20,exp=0)
                        pr=json.dumps(player,ensure_ascii=False,separators=(',',':')).encode().hex()
                        gr=json.dumps(game,ensure_ascii=False,separators=(',',':')).encode().hex()
                        self.sql(f'UPDATE profiles SET player=CONVERT(0x{pr} USING utf8mb4),game=CONVERT(0x{gr} USING utf8mb4) WHERE id={actor.playerId};')
                    owner,peer=actors
                    now=int(time.time()*1000)
                    period='native-real-limit-'+secrets.token_hex(4)
                    self.sql(f"UPDATE p11_limit_policies SET enabled=1,period_id='{period}',starts_at={now-60000},ends_at={now+7200000} WHERE id=1;")
                    for actor in [peer,owner]:
                        entered=http('CSLimitPvpFindMatch',{},actor,'SCLimitPvpFindMatch')
                        self.assertEqual((entered.myrank.pvpIntegral,entered.fightTimes,entered.allTimes),(10,0,10))
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_fight_matches;').strip(),'0')
                    request={'playerId':peer.playerId,'serverId':1}
                    for invalid_stamp in [0,-1]:
                        self.assertEqual(self.stamped('CSLimitPvpFightResult',request,owner,invalid_stamp)[0],10001)
                    self.assertEqual(self.stamped('CSLimitPvpFightResult',request,owner,stamp+1)[0],10001)
                    now=int(time.time()*1000)
                    at=now+1200
                    self.sql(f"UPDATE p11_limit_fight_policies SET enabled=1,version='native-real20-v1',published_at={now},start_at={at},end_at={at+3600000} WHERE id=1;")
                    time.sleep(max(0,at/1000-time.time())+0.02)
                    listing=http('CSLimitPvpFindMatch',{},owner,'SCLimitPvpFindMatch')
                    self.assertEqual([v.info.playerId for v in listing.rankList],[peer.playerId])
                    rows=self.sql(f"SELECT id,HEX(attacker),HEX(defender),seed,source_hash,period_hash,offer_hash,created_at,expires_at FROM p11_limit_fight_matches WHERE player_id={owner.playerId};").strip().split('\t')
                    self.assertEqual(len(rows),9)
                    a=CLASSES['BattlePlayer'].FromString(bytes.fromhex(rows[1]))
                    d=CLASSES['BattlePlayer'].FromString(bytes.fromhex(rows[2]))
                    self.assertEqual((a.playerId,d.playerId,a.inst.firstOrder,d.inst.firstOrder),(owner.playerId,peer.playerId,1,0))
                    self.assertTrue(a.formation[0].guid)
                    self.assertTrue(d.formation[0].guid)
                    self.assertEqual((a.formation[0].skillOrder,d.formation[0].skillOrder),('047_1,047_2,047_3','047_1,047_2,047_3'))
                    artifact=ROOT/'analysis/p11/limit20-actual-query-source'
                    artifact.mkdir(exist_ok=True)
                    diagnostic=message('SCLimitPvpFightResult',fightId='diagnostic-only-'+rows[0],roomId='map_arena_001',randSeed=int(rows[3]),playerList=[a,d],maxRoundNum=40)
                    (artifact/'original82057-start.pb').write_bytes(diagnostic.SerializeToString())
                    metadata=dict(diagnosticOnly=True,acceptedResult=False,winnerProvided=False,source='actual current originalFindMatch from managed finite20level originalCreate047',matchID=rows[0],sourceSHA256=rows[4],periodHash=rows[5],offerHash=rows[6],createdAt=int(rows[7]),expiresAt=int(rows[8]),frozenInput=dict(attacker=proto_values(a),defender=proto_values(d),seed=int(rows[3]),maxRound=40,battleType=20,defenderFirst=False))
                    (artifact/'input-metadata.json').write_text(json.dumps(metadata,ensure_ascii=False,indent=2)+'\n')
                    before=[self.profile(v) for v in actors]
                    ledger=self.sql('SELECT period_id,player_id,score FROM p11_limit_members ORDER BY player_id;')
                    daily=self.sql('SELECT period_id,player_id,day,used,bought,login_score FROM p11_limit_daily ORDER BY player_id;')
                    records=self.sql('SELECT COUNT(*) FROM p6_battle_records;')
                    for _ in range(2):
                        stamp+=1
                        code,error=self.stamped('CSLimitPvpFightResult',request,owner,stamp)
                        self.assertEqual(code,10001)
                        self.assertIn('服务端战斗尚未支持',str(error))
                    self.assertEqual([self.profile(v) for v in actors],before)
                    self.assertEqual(self.sql('SELECT period_id,player_id,score FROM p11_limit_members ORDER BY player_id;'),ledger)
                    self.assertEqual(self.sql('SELECT period_id,player_id,day,used,bought,login_score FROM p11_limit_daily ORDER BY player_id;'),daily)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records;'),records)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_fight_settlements;').strip(),'0')
                    self.stop();self.start()
                    actors=[self.login(v) for v in identities]
                    self.assertEqual([self.profile(v) for v in actors],before)
                    self.assertEqual(self.sql(f"SELECT source_hash FROM p11_limit_fight_matches WHERE id='{rows[0]}';").strip(),rows[4])
                    self.assertEqual(self.stamped('CSLimitPvpFightResult',request,actors[0],stamp)[0],10001)
                    report.update(status='PASS_ORIGINAL82057_REAL_QUERY_FROZEN_TYPE20_UNSUPPORTED_ATOMIC_COLD',actualMembers=2,actualDailyEntryScore=10,actualFreeTimes=10,mode20=20,maxRound=40,independentProofEmpty=True,acceptedBattleRows=0,capturedBattleRowsAdded=0,energyAndScoreDelta=0,cold=1,diagnosticSource='analysis/p11/limit20-actual-query-source/input-metadata.json',finitePrerequisites='Only originalCreate047 and finite isolated player/main20, guide8. Real explicit operator period/daily10score; no wallet/rank/score/winner/GUID/resource injection.')
                except BaseException as error:
                    report.update(status='FAIL',failure=self.managed_pool.redact(str(error)))
                    raise
                finally:
                    try:
                        self.stop()
                    finally:
                        if connection is not None:
                            self.managed_pool.release(connection.database)
                        self.managed_pool.close()
                    report.update(managedAllocationsRemoved=True,seconds=round(time.monotonic()-began,3))
                    save()
        (ROOT/'analysis/p11/limit-fight-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':
    unittest.main()
