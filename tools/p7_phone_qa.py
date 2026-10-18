"""Root-owned original P7 phone QA on one managed temporary schema.

Default invocation is read-only help. --run requires the coordinated SQL window;
no ADB/device or formal profile access is performed. Quit/EOF/SIGINT releases the
application, registered schema/account and private credential file.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import selectors
import signal
import sys
import tempfile
import time

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT));sys.path.insert(0,str(ROOT/'tests'))
from hyjs_managed_mysql import ManagedMySQL
from test_p7_scoped_rush_managed_mysql import P7ScopedRushManagedMySQLTests
from server.codec import message


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run',action='store_true',help='Root-coordinated SQL window only')
    parser.add_argument('--binary',type=Path)
    parser.add_argument('--binary-sha256')
    parser.add_argument('--source',type=Path)
    parser.add_argument('--ttl-seconds',type=int,default=1800)
    parser.add_argument('--manual-driver',action='store_true',help='Enable the separately certified Trial9 manual-to-Auto policy in this fresh QA schema')
    parser.add_argument('--elite-manual-driver',action='store_true',help='Also enable the independent Elite10 policy after the real Trial prerequisite is earned')
    parser.add_argument('--evidence-output',type=Path,help='Write credential-free final QA database state under analysis/p7')
    args=parser.parse_args()
    if not args.run:parser.print_help();return
    if not args.binary or not args.source or not args.binary_sha256:parser.error('Explicit fixed source, built binary and SHA are required')
    binary=args.binary.resolve();source=args.source.resolve()
    if source.parent.parent!=ROOT/'build/candidates' or source.name!='hyjs-server':parser.error('Fixed managed candidate source required')
    if hashlib.sha256(binary.read_bytes()).hexdigest()!=args.binary_sha256:parser.error('Built binary SHA changed')
    evidence=args.evidence_output.resolve() if args.evidence_output else None
    if evidence and not evidence.is_relative_to(ROOT/'analysis/p7'):parser.error('Use an owned analysis/p7 evidence directory')
    if evidence:evidence.mkdir(parents=True,exist_ok=True)
    if args.manual_driver:
        manual=source/'constUtil/p6RushManualSettlement.go'
        if not manual.is_file() or 'P6RushManualNativeProof' not in manual.read_text():parser.error('Certified manual Trial source required')
    if args.elite_manual_driver:
        elite=source/'constUtil/p6RushEliteManualSettlement.go'
        if not args.manual_driver or not elite.is_file():parser.error('Independent Elite source and the real Trial driver are required')
    if 'P7OriginalScopedFloorScene' not in (source/'service/p7RushService.go').read_text():parser.error('Apply scoped navigation143 proposal before QA; FloorRoom/BattleRoom are distinct')
    # Read the current central service contract before allocating an account.
    contract=(Path.home() / '.codex/local-services/README.md').read_text()
    if 'codex-local-mysql' not in contract or 'test-create' not in contract:parser.error('Managed service contract unavailable')
    pool=ManagedMySQL()
    qa=P7ScopedRushManagedMySQLTests('test_original_public_rush_real_compute_once_cold_and_atomic_rejection')
    qa.process=None
    with tempfile.TemporaryDirectory(prefix='hyjs-p7-phone-qa-') as private:
        try:
            conn=pool.allocate('p7_phone')
            qa.managed=pool;qa.connection=conn;qa.source=source;qa.binary=str(binary)
            qa.env=dict(os.environ,**conn.env(),GOMEMLIMIT='1GiB')
            qa.start()
            uid='p7-phone-qa-'+secrets.token_hex(6)
            login=qa.login(uid)
            qa.call('CSPlayerCreate',{'playerName':'隔离原试炼手机验收','heroId':'hero_047'},login,1,'SCPlayerCreate')
            # Explicit finite preparation only. Guide8 was observed on actual
            # Demo3 already in Town; no chapter victory, currency, VIP or gear is granted.
            qa.sql("UPDATE profiles SET player=JSON_SET(player,'$.level',22,'$.noviceGuideSchedule',8) WHERE id="+str(login.playerId)+';')
            qa.call('CSFormationList',{},login,2,'SCFormationList')
            typed=message('SCRushFightStart');typed.ParseFromString((ROOT/'analysis/p6/actual-local-p7-rush-inputs-v104/000.SCRushFightStart.pb').read_bytes())
            enemy=typed.battleMonsterList[0].SerializeToString(deterministic=True).hex()
            # ReadyItem taskId is opaque network data. Elite01 is the actual
            # TableRushEliteBoss row and its original condition03 is left intact.
            for floor,task in [(1,'03'),(2,'06')]:
                qa.sql("INSERT INTO p7_rush_stage_rules(id,kind,task_id,condition_task_id,floor,star,min_level,enabled,scene,max_round,win_key,win_value,enemies,first_reward,reward,recommended_power,source) VALUES('phone-qa-trial-"+str(floor)+"',1,'"+task+"','',"+str(floor)+",1,22,1,'map_maze_001',40,0,0,0x"+enemy+",'','point,10',0,'explicit prepublished local phone QA trial; private floor unknown');")
            qa.sql("UPDATE p7_rush_stage_rules SET enabled=1,scene='map_maze_001',enemies=0x"+enemy+",win_key=0,win_value=0,source=CONCAT(source,'; explicit local phone QA NPC047; original boss formation unknown') WHERE id='elite:01' AND task_id='01' AND condition_task_id='03'; UPDATE p7_rush_policies SET enabled=1; UPDATE p7_rush_scoped_policies SET enabled=1;")
            info=qa.call('CSGetRushInfo',{},login,3,'SCGetRushInfo')
            if info.curFloorSceneName!='map_portal_001':raise AssertionError('Original RoomRush navigation asset mismatch')
            elite=qa.call('CSGetRushEliteBossInfo',{},login,4,'SCGetRushEliteBossInfo')
            first=next(b for b in elite.bossInfoList if b.eliteTaskId=='01')
            if first.isOpen!=0 or elite.leftTime!=3:raise AssertionError('No fake prerequisite or VIP is allowed')
            if qa.sql('SELECT COUNT(*) FROM p7_rush_fights; SELECT COUNT(*) FROM p6_battle_records;').strip().split()!=['0','0']:raise AssertionError('QA prep created an unplayed battle')
            if args.manual_driver:
                # Enable only this new schema's explicit Trial permission. The
                # original Elite prerequisite remains intact and uncertified.
                qa.sql('UPDATE p7_rush_manual_policies SET enabled=1 WHERE id=1;')
                if args.elite_manual_driver:
                    # Keep the original locked prerequisite and free-attempt
                    # count; the physical Trial must actually earn task03.
                    qa.sql('UPDATE p7_rush_manual_policies SET enabled=1 WHERE id=2;')
            credentials=Path(private)/'phone-login.json'
            credentials.write_text(json.dumps({'platformUid':uid,'authCode':'local-test-password','playerId':login.playerId,'httpPort':qa.service.http_port,'tcpPort':qa.service.tcp_port}))
            credentials.chmod(0o600)
            print(json.dumps({'ready':True,'privateLoginFile':str(credentials),'HTTP':qa.service.http_port,'TCP':qa.service.tcp_port,'binarySHA256':args.binary_sha256,'finitePreparation':'new original047 account; explicit level22/guide8 only; no formal3/4 read/write, no currency/VIP/gear/records/victory','source':'Trial opaque03→actual accepted win produces prerequisite03→Elite original01; FloorRoom portal, BattleRoom maze; VIP0 original3 free Elite attempts','commands':'status / quit; EOF/SIGINT also releases all managed resources'}),flush=True)
            stopped=False
            def stop(signum,frame):
                nonlocal stopped
                stopped=True
            signal.signal(signal.SIGINT,stop);signal.signal(signal.SIGTERM,stop)
            deadline=time.monotonic()+args.ttl_seconds
            with selectors.DefaultSelector() as waiting:
                waiting.register(sys.stdin,selectors.EVENT_READ)
                while not stopped and time.monotonic()<deadline:
                    if qa.process.poll() is not None:raise AssertionError('QA server exited')
                    if not waiting.select(1):continue
                    command=sys.stdin.readline().strip()
                    if command in ('','quit','exit'):break
                    if command=='status':
                        rows=qa.sql('SELECT kind,status,diagnostic,COUNT(*) FROM p7_rush_fights GROUP BY kind,status,diagnostic; SELECT task_id FROM p7_rush_progress ORDER BY task_id;')
                        print(json.dumps({'actualOwnQAStatus':rows}),flush=True)
        except Exception as error:
            raise RuntimeError(pool.redact(str(error))) from None
        finally:
            if evidence and getattr(qa,'connection',None) is not None and qa.process is not None:
                try:
                    # Export only game receipts and hashes, never profile login
                    # credentials or authenticated transport frames.
                    queries={
                        'fights':"SELECT JSON_OBJECT('fightId',CAST(id AS CHAR),'kind',kind,'taskId',task_id,'status',CAST(status AS CHAR),'diagnostic',diagnostic,'replayId',replay_id,'startHash',start_hash,'ruleHash',rule_hash) FROM p7_rush_fights ORDER BY id;",
                        'progress':"SELECT JSON_OBJECT('taskId',task_id) FROM p7_rush_progress ORDER BY task_id;",
                        'p6':"SELECT JSON_OBJECT('id',id,'status',status,'authoritative',authoritative,'source',source) FROM p6_battle_records ORDER BY id;",
                    }
                    if args.manual_driver:
                        queries['manual']="SELECT JSON_OBJECT('fightId',CAST(fight_id AS CHAR),'status',status,'scopeProof',scope_proof,'seriesHash',series_hash,'startHash',start_hash,'terminalHash',terminal_hash,'derivedCalculationSourceSHA256',derived_calculation_source_sha256,'endRequestHash',end_request_hash,'acceptedAt',accepted_at) FROM p7_rush_manual_calculations ORDER BY fight_id;"
                    audit={'binarySHA256':args.binary_sha256,'manualDriver':args.manual_driver,'eliteManualDriver':args.elite_manual_driver,'credentialsSaved':False}
                    for name,query in queries.items():audit[name]=[json.loads(row) for row in qa.sql(query).splitlines() if row.strip()]
                    (evidence/'final-database-state.json').write_text(json.dumps(audit,ensure_ascii=False,indent=2)+'\n')
                except Exception as error:
                    (evidence/'export-error.json').write_text(json.dumps({'error':pool.redact(str(error))})+'\n')
            qa.stop();pool.close()
            print('P7 phone QA application/schema/account/private connection released',flush=True)

if __name__=='__main__':main()
