"""Managed original P11 phone QA; help is read-only, --run owns one SQL lease.

Starts a source-pinned prebuilt, prepares two explicitly finite original047
actors and real entry/empty occupancy, then leaves every challenge to the phone.
No ADB, formal profile, guessed result, manual fee or reward is used here.
Quit/EOF/TTL/SIGINT stops the owned application and releases its schema/account.
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

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT)); sys.path.insert(0, str(ROOT/'tests'))
from hyjs_managed_mysql import ManagedMySQL
from test_p11_grab_fight_mysql import P11GrabFightMySQLTests


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true')
    parser.add_argument('--mode', choices=('grab', 'limit'), default='grab')
    parser.add_argument('--source', type=Path)
    parser.add_argument('--binary', type=Path)
    parser.add_argument('--proof', type=Path)
    parser.add_argument('--evidence-output', type=Path)
    parser.add_argument('--ttl-seconds', type=int, default=1200)
    args = parser.parse_args()
    if not args.run:
        parser.print_help(); return
    if not all((args.source, args.binary, args.proof, args.evidence_output)):
        parser.error('Fixed source/binary/prebuilt-proof and owned evidence output required')
    source, binary, proof_path, evidence = [v.resolve() for v in
                                           (args.source, args.binary, args.proof, args.evidence_output)]
    if source.parent != ROOT/'build/candidates' or not proof_path.is_relative_to(ROOT/'analysis/p11') or not evidence.is_relative_to(ROOT/'analysis/p11'):
        parser.error('Use fixed owned source and P11 proof/evidence paths')
    if not 60 <= args.ttl_seconds <= 1800 or args.mode == 'limit' and args.ttl_seconds < 180:
        parser.error('TTL must be 60..1800 seconds; limit daily QA requires at least180 seconds')
    proof = json.loads(proof_path.read_text())
    if proof.get('status') != 'PASS_DOMAIN_ALLPKG_VET_BUILD' or digest(binary) != proof.get('binarySHA256'):
        parser.error('Verified fixed prebuilt required; no compile during SQL')
    field = 'mode19IndependentProof' if args.mode == 'grab' else 'mode20IndependentProof'
    if not proof.get(field):
        parser.error('Separate independently certified original mode required')
    for rel, expected in proof['runtimeSHA256'].items():
        path = (source/rel).resolve()
        if not path.is_relative_to(source) or digest(path) != expected:
            parser.error('Source differs from qualified prebuilt: '+rel)
    contract = (Path.home() / '.codex/local-services/README.md').read_text()
    if 'test-create' not in contract or 'codex-local-mysql' not in contract:
        parser.error('Managed local-service contract unavailable')
    if (evidence/'preparation.json').exists() or (evidence/'final-database-state.json').exists():
        parser.error('Keep prior QA evidence immutable; use a fresh evidence directory')
    evidence.mkdir(parents=True, exist_ok=True)
    pool = ManagedMySQL()
    qa = P11GrabFightMySQLTests('test_actual_occupied_source_costs_once_transfer_or_retention_and_cold')
    qa.process = None
    conn = None
    with tempfile.TemporaryDirectory(prefix='hyjs-p11-phone-qa-') as private:
        try:
            conn = pool.allocate('p11_phone_'+args.mode)
            qa.managed_pool = pool
            qa.database, qa.test_user, qa.test_password = conn.database, conn.user, conn.password
            qa.source, qa.binary = source, binary
            qa.env = dict(os.environ, **conn.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
            qa.start()
            identities = ['p11-'+args.mode+'-phone-'+secrets.token_hex(6) for _ in range(2)]
            actors = [qa.login(v) for v in identities]
            for actor, level in zip(actors, (40, 20)):
                made = qa.call('CSPlayerCreate', {'playerName':'隔离原竞技'+str(level), 'heroId':'hero_047'}, actor, 1, 'SCPlayerCreate')
                player, game = qa.profile(actor)
                if player.get('coin', 0) or player.get('diamond', 0):
                    raise AssertionError('New real original actor must start without grant')
                player.update(level=max(30, level), noviceGuideSchedule=8)
                main = next(h for h in game['warriors'] if h['guid'] == made.player.heroWarId)
                main.update(level=level, exp=0)
                pr = json.dumps(player, ensure_ascii=False, separators=(',', ':')).encode().hex()
                gr = json.dumps(game, ensure_ascii=False, separators=(',', ':')).encode().hex()
                qa.sql(f'UPDATE profiles SET player=CONVERT(0x{pr} USING utf8mb4),game=CONVERT(0x{gr} USING utf8mb4) WHERE id={actor.playerId};')
            phone, peer = actors
            now = int(time.time()*1000)
            at, end = now+800, now+args.ttl_seconds*1000
            if args.mode == 'grab':
                qa.sql('UPDATE p11_grab_policies SET enabled=1;')
                claimed = qa.call('CSGrab', {'index':1}, peer, 2, 'SCGrab')
                if claimed.HasField('attPlayer') or claimed.grabData.playerId != peer.playerId:
                    raise AssertionError('Peer must actually acquire an empty original slot')
                qa.sql(f"UPDATE p11_grab_fight_policies SET enabled=1,version='local-phone-original19-v1',published_at={now},starts_at={at},ends_at={end} WHERE id=1;")
                if qa.sql('SELECT COUNT(*) FROM p11_grab_fight_settlements;').strip() != '0':
                    raise AssertionError('Phone prep cannot create an unplayed fight')
            else:
                qa.sql(f"UPDATE p11_limit_policies SET enabled=1,reward_enabled=1,period_id='local-phone-original20-v1',starts_at={now},ends_at={end+86400000} WHERE id=1;")
                qa.sql(f"UPDATE p11_limit_fight_policies SET enabled=1,version='local-phone-original20-v1',published_at={now},start_at={at},end_at={end} WHERE id=1;")
                # The phone earns the only challenger eligibility. A real local
                # two-minute cutoff lets the original daily points arrive live.
                cutoff = time.gmtime(now/1000+8*3600+120)
                second = cutoff.tm_hour*3600+cutoff.tm_min*60+cutoff.tm_sec
                qa.sql(f"UPDATE p11_limit_daily_policies SET enabled=1,version='local-phone-two-minute-cutoff-v1',published_at={now},start_at={at},end_at={end},cutoff_second={second},min_challenges=1 WHERE id=1;")
                qa.call('CSLimitPvpFindMatch', {}, peer, 2, 'SCLimitPvpFindMatch')
                if qa.sql('SELECT COUNT(*) FROM p11_limit_fight_settlements;').strip() != '0':
                    raise AssertionError('Phone prep cannot create an unplayed challenge')
            credentials = Path(private)/'phone-login.json'
            credentials.write_text(json.dumps({'platformUid':identities[0], 'authCode':'local-test-password', 'playerId':phone.playerId, 'httpPort':qa.service.http_port, 'tcpPort':qa.service.tcp_port}))
            credentials.chmod(0o600)
            report = {'mode':args.mode, 'binarySHA256':digest(binary), 'sourceProofSHA256':digest(proof_path), 'finitePreparation':'Two realCreate047 only; player40/30 main40/20 guide8. Peer original entry/empty occupancy; no phone challenge, seed, winner, resource grant or completed history.', 'phonePlayer':phone.playerId, 'peerPlayer':peer.playerId, 'credentialsSaved':False, 'completed':False}
            (evidence/'preparation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
            print(json.dumps({'ready':True, 'mode':args.mode, 'privateLoginFile':str(credentials), 'HTTP':qa.service.http_port, 'TCP':qa.service.tcp_port, 'commands':'status / quit; original phone must play; map all QA ports before fresh original restart'}), flush=True)
            stopped = False
            def stop(signum, frame):
                nonlocal stopped
                stopped = True
            signal.signal(signal.SIGINT, stop); signal.signal(signal.SIGTERM, stop)
            deadline = time.monotonic()+args.ttl_seconds
            with selectors.DefaultSelector() as waiting:
                waiting.register(sys.stdin, selectors.EVENT_READ)
                while not stopped and time.monotonic() < deadline:
                    if qa.process.poll() is not None:
                        raise AssertionError('Owned QA application stopped unexpectedly')
                    if not waiting.select(1):
                        continue
                    command = sys.stdin.readline().strip()
                    if command in ('', 'quit', 'exit'):
                        break
                    if command == 'status':
                        table = 'p11_grab_fight_settlements' if args.mode == 'grab' else 'p11_limit_fight_settlements'
                        print(json.dumps({'actualPlayedReceipts':qa.sql('SELECT COUNT(*) FROM '+table+';').strip()}), flush=True)
        except Exception as error:
            raise RuntimeError(pool.redact(str(error))) from None
        finally:
            if conn is not None and qa.process is not None:
                try:
                    table = 'p11_grab_fight_settlements' if args.mode == 'grab' else 'p11_limit_fight_settlements'
                    audit = dict(mode=args.mode, binarySHA256=digest(binary), credentialsSaved=False)
                    audit['settlements'] = qa.sql('SELECT player_id,stamp,fight_id,p6_record_id,scope_proof,response_hash,terminal_hash FROM '+table+' ORDER BY player_id,stamp;')
                    audit['p6'] = qa.sql('SELECT id,source,status,authoritative,input_hash,replay_hash,outcome_hash FROM p6_battle_records ORDER BY id;')
                    (evidence/'final-database-state.json').write_text(json.dumps(audit, ensure_ascii=False, indent=2)+'\n')
                except Exception as error:
                    (evidence/'export-error.json').write_text(json.dumps({'error':pool.redact(str(error))})+'\n')
            try:
                qa.stop()
            finally:
                pool.close()
            private_file = Path(private)/'phone-login.json'
            private_file.unlink(missing_ok=True)
            print('P11 phone QA application/schema/account/private credential file released', flush=True)


if __name__ == '__main__':
    main()
