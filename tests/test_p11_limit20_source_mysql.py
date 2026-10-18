"""Real originalFindMatch40<->20 sources, diagnostic typed20 only.

Requires one coordinated managed SQL window and a fixed prebuilt. Does not send
an accepted battle, write a winner or alter actor panels/HP/skills/random seed.
"""
import fcntl
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
from test_p11_limit_fight_mysql import proto_values

ROOT=Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P11_LIMIT20_SOURCE_MYSQL_TEST')=='1',
                     'explicit root-coordinated actual source export window')
class P11Limit20ActualSourceTests(unittest.TestCase):
    send=Transport.send
    login=Transport.login
    stamped=Transport.stamped
    call=Transport.call
    profile=Transport.profile
    start=Transport.start
    stop=Transport.stop
    setUp=Transport.setUp
    sql=Transport.sql

    def test_original_findmatch40_and20_both_directions_only(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source=Path(os.environ['HYJS_P11_LIMIT20_SOURCE_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        proof=json.loads((ROOT/'analysis/p11/limit-daily-go-validation.json').read_text())
        rows={}
        for name in ['analysis/p11/limit-fight-code-manifest.json','analysis/p11/limit-fight-shared-proposal-v169/manifest.json','analysis/p11/limit-daily-code-manifest.json','analysis/p11/limit-daily-shared-proposal-v175/manifest.json']:
            for row in json.loads((ROOT/name).read_text())['files']:
                rows[row['path']]=row
        hashes={}
        for rel,row in rows.items():
            actual=hashlib.sha256((self.source/rel).read_bytes()).hexdigest()
            self.assertEqual(actual,row['sha256'],rel)
            hashes[rel]=actual
        prebuilt=Path(os.environ['HYJS_P11_LIMIT20_SOURCE_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt,self.source/'limit-daily-native')
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(),proof['binarySHA256'])
        helper=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper,'483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        began=time.monotonic()
        report=dict(status='PENDING',runtimeSHA256=hashes,managedHelperSHA256=helper,
                    diagnosticOnly=True,acceptedResult=False,winnerProvided=False,
                    formalModified=False,phoneUI=False)
        checkpoint=ROOT/'analysis/p11/limit20-strong-source-checkpoint.json'
        def save():
            checkpoint.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        with tempfile.TemporaryDirectory(prefix='hyjs-limit20-source-') as temporary:
            self.binary=Path(temporary)/'limit-daily-native'
            shutil.copy2(prebuilt,self.binary)
            self.managed_pool=ManagedMySQL()
            connection=None
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    connection=self.managed_pool.allocate('p11_limit20_source')
                    self.database,self.test_user=connection.database,connection.user
                    self.test_password=connection.password
                    self.env=dict(os.environ,**connection.env(),GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    self.assertEqual(self.sql('SELECT @@character_set_client;').strip(),'utf8mb4')
                    for domain,suffixes in [('163_p11_limit_fight_',['policies.sql','offers.sql','matches.sql','settlements.sql','policies.seed.sql']),('164_p11_limit_daily_',['policies.sql','offers.sql','challenges.sql','snapshots.sql','awards.sql','policies.seed.sql'])]:
                        for suffix in suffixes:
                            self.sql((self.source/'hyjs-server/db/sql'/(domain+suffix)).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_limit_daily_policies;').strip(),'0')
                    identities=['limit20-source-'+secrets.token_hex(5) for _ in range(2)]
                    actors=[self.login(v) for v in identities]
                    stamp=130000
                    def http(name,fields,actor,expected):
                        nonlocal stamp
                        stamp+=1
                        return self.call(name,fields,actor,stamp,expected)
                    for actor,level in zip(actors,[40,20]):
                        created=http('CSPlayerCreate',{'playerName':'原限时源'+str(level),'heroId':'hero_047'},actor,'SCPlayerCreate')
                        player,game=self.profile(actor)
                        self.assertEqual((player.get('coin',0),player.get('diamond',0)),(0,0))
                        player.update(level=level,noviceGuideSchedule=8)
                        warrior=next(v for v in game['warriors'] if v['guid']==created.player.heroWarId)
                        warrior.update(level=level,exp=0)
                        pr=json.dumps(player,ensure_ascii=False,separators=(',',':')).encode().hex()
                        gr=json.dumps(game,ensure_ascii=False,separators=(',',':')).encode().hex()
                        self.sql(f'UPDATE profiles SET player=CONVERT(0x{pr} USING utf8mb4),game=CONVERT(0x{gr} USING utf8mb4) WHERE id={actor.playerId};')
                    now=int(time.time()*1000)
                    period='native-real40-20-'+secrets.token_hex(4)
                    self.sql(f"UPDATE p11_limit_policies SET enabled=1,period_id='{period}',starts_at={now-60000},ends_at={now+7200000} WHERE id=1;")
                    for actor in actors:
                        first=http('CSLimitPvpFindMatch',{},actor,'SCLimitPvpFindMatch')
                        self.assertEqual((first.myrank.pvpIntegral,first.fightTimes,first.allTimes),(10,0,10))
                    before=[self.profile(v) for v in actors]
                    now=int(time.time()*1000)
                    at=now+1200
                    self.sql(f"UPDATE p11_limit_fight_policies SET enabled=1,version='native-40-vs20-real-source',published_at={now},start_at={at},end_at={at+3600000} WHERE id=1;")
                    time.sleep(max(0,at/1000-time.time())+0.02)
                    artifact=ROOT/'analysis/p11/limit20-strong-actual-source'
                    artifact.mkdir(exist_ok=True)
                    outputs=[]
                    for owner,peer,label in [(actors[0],actors[1],'level40-to20'),(actors[1],actors[0],'level20-to40')]:
                        listing=http('CSLimitPvpFindMatch',{},owner,'SCLimitPvpFindMatch')
                        self.assertEqual([v.info.playerId for v in listing.rankList],[peer.playerId])
                        self.assertEqual(listing.myrank.pvpIntegral,10)
                        row=self.sql(f"SELECT id,HEX(attacker),HEX(defender),seed,source_hash,period_hash,offer_hash,created_at,expires_at FROM p11_limit_fight_matches WHERE player_id={owner.playerId} AND opponent_id={peer.playerId} ORDER BY created_at DESC,id DESC LIMIT 1;").strip().split('\t')
                        self.assertEqual(len(row),9)
                        a=CLASSES['BattlePlayer'].FromString(bytes.fromhex(row[1]))
                        d=CLASSES['BattlePlayer'].FromString(bytes.fromhex(row[2]))
                        self.assertEqual((a.playerId,d.playerId,a.inst.firstOrder,d.inst.firstOrder),(owner.playerId,peer.playerId,1,0))
                        self.assertEqual(abs(a.level-d.level),20)
                        self.assertEqual((a.formation[0].skillOrder,d.formation[0].skillOrder),('047_1,047_2,047_3','047_1,047_2,047_3'))
                        self.assertTrue(a.formation[0].guid)
                        self.assertTrue(d.formation[0].guid)
                        self.assertEqual((a.formation[0].inst.curHp,d.formation[0].inst.curHp),(a.formation[0].prop.hp,d.formation[0].prop.hp))
                        pb=message('SCLimitPvpFightResult',fightId='diagnostic-only-'+row[0],roomId='map_arena_001',randSeed=int(row[3]),playerList=[a,d],maxRoundNum=40)
                        pb_path=artifact/(label+'.pb');pb_path.write_bytes(pb.SerializeToString())
                        data=dict(diagnosticOnly=True,acceptedResult=False,winnerProvided=False,source='actual current originalFindMatch from managed originalCreate047; finite40/20qualification, normal P5 projection',matchID=row[0],sourceSHA256=row[4],periodHash=row[5],offerHash=row[6],createdAt=int(row[7]),expiresAt=int(row[8]),frozenInput=dict(attacker=proto_values(a),defender=proto_values(d),seed=int(row[3]),maxRound=40,battleType=20,defenderFirst=False))
                        metadata=artifact/(label+'.json');metadata.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
                        outputs.append(dict(label=label,metadata=str(metadata.relative_to(ROOT)),metadataSHA256=hashlib.sha256(metadata.read_bytes()).hexdigest(),pb=str(pb_path.relative_to(ROOT)),pbSHA256=hashlib.sha256(pb_path.read_bytes()).hexdigest(),attackerLevel=a.level,defenderLevel=d.level,actualHp=[a.formation[0].prop.hp,d.formation[0].prop.hp],actualAtt=[a.formation[0].prop.att,d.formation[0].prop.att]))
                    self.assertEqual([self.profile(v) for v in actors],before)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records;').strip(),'0')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_fight_settlements;').strip(),'0')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_daily_challenges;').strip(),'0')
                    report.update(status='PASS_REAL_ORIGINAL_FINDMATCH40_20_BIDIRECTION_TYPED20_DIAGNOSTIC_ONLY',outputs=outputs,actualMatchCount=2,realOriginalDailyEntryPoints=10,successfulBattles=0,resourcesUnchanged=True,finitePrerequisites='Only originalCreate047 and finite isolated Player/main40vs20 plusguide8. Original ±20 gate/±50 score/complete3skills/GUIDs/P5 panels/HP/Inst preserved; no raw panel/HP/seed/winner/fight/fragment/currency/score injection.')
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
        (ROOT/'analysis/p11/limit20-strong-source-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':
    unittest.main()
