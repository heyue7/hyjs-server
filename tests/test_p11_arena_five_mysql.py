"""Original lower-rank PvP five loop: existing23101 only, no product code.

Root coordinates a single managed SQL window. Reuse qualified193 canonical
binary, one real FindMatch per loop, five distinct original timestamps. Strong
and weak actual sources exercise both outcomes without manufacturing winners.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import selectors
import shutil
import subprocess
import tempfile
import time
import types
import unittest
from hyjs_managed_mysql import ManagedMySQL
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P11_ARENA_FIVE_MYSQL_TEST')=='1','explicit coordinated single managed SQL window')
class P11ArenaFiveMySQLTests(unittest.TestCase):
    send=Transport.send
    login=Transport.login
    stamped=Transport.stamped
    call=Transport.call
    profile=Transport.profile
    stop=Transport.stop
    setUp=Transport.setUp
    sql=Transport.sql

    def start(self):
        self.process=subprocess.Popen([str(self.binary),'--http-port','0','--tcp-port','0','--tables',str(self.source/'analysis/tables'),'--log-file',str(self.log_path)],cwd=self.source/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector()as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ)
            self.assertTrue(ready.select(90),'native startup timeout')
            line=self.process.stdout.readline()
        ports=__import__('re').search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line)
        self.assertIsNotNone(ports,line)
        self.service=types.SimpleNamespace(http_port=int(ports[1]),tcp_port=int(ports[2]))

    def test_original_reachable_lower_rank_five_real_outcomes_short_energy_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        proof=json.loads((ROOT/'analysis/p11/arena-five-prebuilt-proof-v194.json').read_text())
        self.assertEqual(proof['status'],'PINNED_QUALIFIED193_PREBUILT_NO_NEW_GO')
        self.source=Path(proof['source']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        for rel,expected in proof['runtimeSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(),expected,rel)
        prebuilt=Path(proof['binary']).resolve()
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(),proof['binarySHA256'])
        helper=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper,'483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        report=dict(status='PENDING',sourceProofSHA256=hashlib.sha256((ROOT/'analysis/p11/arena-five-prebuilt-proof-v194.json').read_bytes()).hexdigest(),source=proof['source'],binarySHA256=proof['binarySHA256'],managedHelperSHA256=helper,formalModified=False,phoneUI=False,cases=[])
        began=time.monotonic();checkpoint=ROOT/'analysis/p11/arena-five-native-checkpoint.json'
        with tempfile.TemporaryDirectory(prefix='hyjs-arena-five-native-')as temporary:
            self.binary=Path(temporary)/'arena-five-native';shutil.copy2(prebuilt,self.binary)
            self.log_path=Path(temporary)/'native.log'
            self.managed_pool=ManagedMySQL();allocation=None
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    allocation=self.managed_pool.allocate('p11_arena_five')
                    self.database,self.test_user=allocation.database,allocation.user
                    self.test_password=allocation.password
                    self.env=dict(os.environ,**allocation.env(),GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    self.start();stamp=210000
                    def http(name,fields,actor,expected):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,actor,stamp,expected)
                    levels=[100,20,20,100,20,100]
                    identities=['original-five-'+secrets.token_hex(4)for _ in levels]
                    actors=[];initial=[]
                    for i,(identity,level)in enumerate(zip(identities,levels)):
                        actor=self.login(identity);actors.append(actor)
                        http('CSPlayerCreate',{'playerName':'原竞技五连'+str(i),'heroId':'hero_047'},actor,'SCPlayerCreate')
                        player,game=self.profile(actor);initial.append((player,game))
                        self.assertEqual((player.get('coin',0),player.get('diamond',0),player.get('energy',0)),(0,0,100))
                        self.assertEqual(next(v for v in game['warriors']if v['guid']==player['heroWarId'])['level'],1)
                        # Only finite level/prior guide completion qualify the UI.
                        # The two full loops preserve original Create Energy100.
                        self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',{level},'$.noviceGuideSchedule',13) WHERE id={actor.playerId};")
                    self.sql('UPDATE p11_arena_policies SET enabled=1;UPDATE p11_arena_fight_policies SET enabled=1;')
                    self.assertEqual(self.sql('SELECT enabled FROM p11_arena_prize_policies;').strip(),'0')
                    # Negative eligibility only: a separate actor represents a
                    # historical Energy5 balance. This reduction is explicit,
                    # not described as a naturally earned/spent source.
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.energy',5) WHERE id={actors[4].playerId};")
                    histories=[];stable_profiles={}
                    suffix=os.environ.get('HYJS_P11_ARENA_FIVE_SUFFIX')=='1'
                    tail=os.environ.get('HYJS_P11_ARENA_FIVE_TAIL_ONLY')=='1'
                    cases=[('strong',actors[0],actors[1],5),('weak',actors[2],actors[3],5),('short-energy',actors[4],actors[5],2)]
                    if suffix:
                        prefix=json.loads((ROOT/'analysis/p11/arena-five-first-round-failure-v194.json').read_text())
                        self.assertEqual(prefix['binarySHA256'],proof['binarySHA256'])
                        self.assertEqual(prefix['cases'][0]['label'],'strong')
                        self.assertEqual(prefix['cases'][0]['actualChildren'],5)
                        self.assertEqual(prefix['cases'][0]['actualEnergyCost'],10)
                        report['preservedPrefix']={'path':'analysis/p11/arena-five-first-round-failure-v194.json','sha256':hashlib.sha256((ROOT/'analysis/p11/arena-five-first-round-failure-v194.json').read_bytes()).hexdigest(),'strongFiveAlreadyPassed':prefix['cases'][0],'strongPrefixColdReplayed':False,'originalFailureRetained':True,'correction':'Only the runner expected loser BoxList incorrectly; real loser has no cards and no grant, challenger win/defaultoff has three coin0 previews. Product and binary unchanged.'}
                        cases=cases[1:]
                    if tail:
                        self.assertTrue(suffix)
                        weak=json.loads((ROOT/'analysis/p11/arena-five-second-round-failure-v194.json').read_text())
                        self.assertEqual(weak['binarySHA256'],proof['binarySHA256'])
                        self.assertEqual(weak['cases'][0]['label'],'weak')
                        self.assertEqual(weak['cases'][0]['actualChildren'],5)
                        self.assertEqual(weak['cases'][0]['actualEnergyCost'],10)
                        report['preservedWeakPrefix']={'path':'analysis/p11/arena-five-second-round-failure-v194.json','sha256':hashlib.sha256((ROOT/'analysis/p11/arena-five-second-round-failure-v194.json').read_bytes()).hexdigest(),'weakFiveAlreadyPassed':weak['cases'][0],'weakPrefixColdReplayed':False,'originalFailureRetained':True,'correction':'Runner compared list returned by profile with a tuple of identical elements; both negative rollback and cold reference now retain list type. Product/binary unchanged.'}
                        cases=cases[-1:]
                    for label,owner,target,expected in cases:
                        listing=http('CSPvpFindMatch',{},owner,'SCPvpFindMatch')
                        target_info=next(v for v in listing.rankList if v.info.playerId==target.playerId)
                        ranks=self.sql(f'SELECT player_id,position FROM p11_arena_members WHERE player_id IN ({owner.playerId},{target.playerId}) ORDER BY player_id;').strip()
                        rank_map={int(a):int(b)for a,b in(v.split('\t')for v in ranks.splitlines())}
                        self.assertGreater(target_info.pvpRank,rank_map[owner.playerId],'original UI five button is hidden for higher-ranked targets')
                        public=http('CSPlayer',{},owner,'SCPlayer').player
                        count=min(5,public.energy//2)
                        self.assertEqual(count,expected)
                        start_energy=public.energy
                        query_rows=self.sql(f'SELECT id,snapshot_hash,seed,viewer_rank,opponent_rank FROM p11_arena_matches WHERE viewer_id={owner.playerId} ORDER BY id;')
                        self.assertTrue(query_rows.strip())
                        result_ids=[];actual_winners=[]
                        for index in range(count):
                            # Original PVPSweepPage repeats23101 without a new
                            # match request. Each request retains its own receipt.
                            args={'playerId':target.playerId,'serverId':1}
                            stamp+=1;paid_stamp=stamp
                            result=self.call('CSPvpFightResult',args,owner,paid_stamp,'SCPvpFightResult')
                            self.assertIn(result.winPlayerId,[owner.playerId,target.playerId])
                            actual_winners.append(result.winPlayerId);result_ids.append(result.fightId)
                            self.assertEqual([(v.id,v.num)for v in result.result.simps],[('energy',-2)])
                            self.assertEqual([(v.id,v.num)for v in result.boxList],[('coin',0)]*3 if result.winPlayerId==owner.playerId else [])
                            self.assertEqual([v.playerId for v in result.playerList],[owner.playerId,target.playerId])
                            self.assertTrue(all(len(v.formation)==1 and v.formation[0].skillOrder=='047_1,047_2,047_3'for v in result.playerList))
                            current=http('CSPlayer',{},owner,'SCPlayer').player
                            self.assertEqual(current.energy,start_energy-2*(index+1))
                            self.assertEqual(self.sql(f'SELECT id,snapshot_hash,seed,viewer_rank,opponent_rank FROM p11_arena_matches WHERE viewer_id={owner.playerId} ORDER BY id;'),query_rows,'five loop created a hidden FindMatch or changed old freeze')
                            self.assertEqual(self.sql(f'SELECT player_id,position FROM p11_arena_members WHERE player_id IN ({owner.playerId},{target.playerId}) ORDER BY player_id;').strip(),ranks)
                            # One real same-child retry verifies the sequence's
                            # timestamp identity while avoiding duplicate awards.
                            if index==2:self.assertEqual(self.call('CSPvpFightResult',args,owner,paid_stamp,'SCPvpFightResult'),result)
                            histories.append((owner,args,paid_stamp,result))
                        if label=='strong':self.assertIn(owner.playerId,actual_winners,'true challenger-win branch not exercised')
                        if label=='weak':self.assertIn(target.playerId,actual_winners,'true challenger-defeat branch not exercised')
                        self.assertEqual(len(set(result_ids)),count)
                        self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p11_arena_fight_settlements WHERE player_id={owner.playerId};').strip(),str(count))
                        self.assertEqual(self.sql(f'SELECT COUNT(DISTINCT p6_record_id) FROM p11_arena_fight_settlements WHERE player_id={owner.playerId};').strip(),str(count))
                        player,game=self.profile(owner)
                        self.assertEqual((player.get('coin',0),player.get('diamond',0),player.get('popularity',0)),(0,0,0))
                        if label=='short-energy':
                            self.assertEqual(player['energy'],1)
                            stamp+=1;self.assertEqual(self.stamped('CSPvpFightResult',{'playerId':target.playerId,'serverId':1},owner,stamp)[0],10001)
                            self.assertEqual(self.profile(owner),[player,game])
                            self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p11_arena_fight_settlements WHERE player_id={owner.playerId};').strip(),'2')
                        stable_profiles[owner.playerId]=[player,game]
                        report['cases'].append(dict(label=label,owner=owner.playerId,target=target.playerId,currentRanks=rank_map,initialEnergy=start_energy,actualChildren=count,actualEnergyCost=count*2,finalEnergy=player['energy'],actualWinners=actual_winners,uniqueFightIDs=result_ids,querySnapshotsUnchanged=True))
                    self.sql('UPDATE p11_arena_policies SET enabled=0;UPDATE p11_arena_fight_policies SET enabled=0;')
                    self.stop();self.start()
                    reconnected={a.playerId:self.login(name)for a,name in zip(actors,identities)}
                    for owner,args,paid_stamp,result in histories:
                        current=reconnected[owner.playerId]
                        self.assertEqual(self.call('CSPvpFightResult',args,current,paid_stamp,'SCPvpFightResult'),result)
                        self.assertEqual(self.profile(current),stable_profiles[owner.playerId])
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_fight_settlements;').strip(),str(len(histories)))
                    report.update(status='PASS_ORIGINAL_REACHABLE23101_FIVE_STRONG_WEAK_SHORT_ENERGY_COLD',originalBatchToken=False,extraFindMatchBetweenChildren=False,actualTotalEnergyCost=2*len(histories),actualPaidChildren=len(histories),coldOldChildren=len(histories),combinedValidatedStrongWeakShortChildren=len(histories)+(10 if tail else 5 if suffix else 0),combinedValidatedEnergyCost=2*(len(histories)+(10 if tail else 5 if suffix else 0)),qualification='Six original Create047, finite Playerlevels100/20 and prior Guide13 only; original main1/GUID/fullthree skills preserved. Full strong/weak loops retain originalEnergy100. Separate short-energy actor finite historicalEnergy5 is negative eligibility preparation, not a natural currency/consumption source.',randomness='Current declared local match freezes one real server seed per query; each child independently computes and commits its actual result from that unchanged original input. No invented new randomness or five-win guarantee, no copied winner. Product code unchanged.')
                except BaseException as error:
                    report.update(status='FAIL',failure=self.managed_pool.redact(str(error)));raise
                finally:
                    try:self.stop()
                    finally:
                        if allocation is not None:self.managed_pool.release(allocation.database)
                        self.managed_pool.close()
                    report.update(managedAllocationsRemoved=True,seconds=round(time.monotonic()-began,3))
                    checkpoint.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        (ROOT/'analysis/p11/arena-five-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
