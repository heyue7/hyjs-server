"""Pure diagnostic same-realm KF wrapper inputs from actual purpose3 actors.

This does not claim a natural draw/Game1, create a DB record or call a device.
Every BattlePlayer is copied byte-equivalent from the recorded actual source;
only diagnostic wrapper identity and a prior real KF seed are explicit inputs.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from server.codec import message

SCOPE = 'local-kf-replay-same-realm-distinct-player-viewer-v1'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def export(source, realm, seed_from, output):
    source, seed_from, output = map(lambda p: Path(p).resolve(), (source, seed_from, output))
    if not source.is_relative_to(ROOT/'analysis/p14') or not seed_from.is_relative_to(ROOT/'analysis/p14') or not output.is_relative_to(ROOT/'analysis/p6') or output.exists():
        raise ValueError('Use actual p14 assets and a new diagnostic p6 directory')
    source_raw = source.read_bytes()
    metadata = json.loads(source_raw)
    members = sorted((row for row in metadata['members'] if row['serverId'] == realm), key=lambda row: row['playerId'])
    if len(members) != 2 or members[0]['playerId'] <= 0 or members[0]['playerId'] == members[1]['playerId']:
        raise ValueError('Two actual distinct same-realm participants are required')
    players = []
    for member in members:
        path = source.parent/member['battleFile']
        if path.resolve().parent != source.parent: raise ValueError('Actual actor source path')
        raw = path.read_bytes()
        if sha(raw) != member['battleSHA256']: raise ValueError('Actual BattlePlayer SHA changed')
        player = message('BattlePlayer'); player.ParseFromString(raw)
        if player.serverId != realm or player.playerId != member['playerId'] or not player.HasField('inst') or player.inst.firstOrder != 0:
            raise ValueError('Keep actual joint identity and original00')
        players.append(player)
    prior_raw = seed_from.read_bytes()
    prior = message('KFReplayData'); prior.ParseFromString(prior_raw)
    if prior.randSeed <= 0: raise ValueError('Prior real positive seed required; no result picking')
    identity = sha(source_raw + str(realm).encode())[:24]
    typed = message('KFReplayData', warId='diagnostic-actual-same-realm-purpose3-' + identity, randSeed=prior.randSeed, maxRoundNum=40)
    for player in players: typed.playerList.add().CopyFrom(player)
    typed_raw = typed.SerializeToString(deterministic=True)
    output.mkdir(parents=True)
    cases=[]
    for index in (0,1):
        name='%03d'%index
        wire=message('SCFightPlayback',fightid=typed.warId,randSeed=typed.randSeed,battleType=17,maxRound=40,attackerFactor=1,defenderFactor=1)
        wire.attacker.CopyFrom(players[index]);wire.defender1.CopyFrom(players[1-index])
        wire_raw=wire.SerializeToString(deterministic=True)
        (output/(name+'.pb')).write_bytes(wire_raw)
        (output/(name+'.KFReplayData.pb')).write_bytes(typed_raw)
        cases.append(dict(case='actual-purpose3/same-realm%d/source%d-viewer'%(realm,index),wire=name+'.pb',wireSHA256=sha(wire_raw),typedCallback=name+'.KFReplayData.pb',typedCallbackSHA256=sha(typed_raw),battleType=17,fightId=typed.warId,seed=typed.randSeed,nativeScope=SCOPE,viewerServerID=realm,viewerPlayerID=players[index].playerId,damageFactors=dict(attacker=1,defender=1),originalFirstOrder=[0,0],originalGlobalAttackFirst=True,sourcePlayerSHA256=[member['battleSHA256']for member in members],diagnosticOnly=True,wrapperProjectionOnly=False,status='awaiting-independent-original-same-realm-wrapper'))
    result=dict(cases=cases,status='AWAITING_INDEPENDENT_ORIGINAL_SAME_REALM_WRAPPER',sourceManifest=str(source),sourceManifestSHA256=sha(source_raw),sourceMembers=members,diagnosticParameterBoundary='Actual source four purpose3 actors had no Game1 warId/seed. New warId is explicitly diagnostic; seed copied from one previously real KF Game1, not chosen for a winner. This is not the later natural same-realm pair instance and does not claim business match creation.',seedSource=str(seed_from),seedSourceSHA256=sha(prior_raw),canonicalBusinessViewer='Actual ordered source0 joint only; source1 viewer is display diagnostic and need not invert winner',roomBoundary='Room omitted, original KF fallbackmap_forest_001',sourceActualDraw=metadata['actualDraw'],nativeCompared=False,authoritative=False)
    (output/'manifest.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    return result


if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--source',type=Path,required=True);p.add_argument('--realm',type=int,required=True)
    p.add_argument('--seed-from',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    args=p.parse_args();result=export(args.source,args.realm,args.seed_from,args.output)
    print(json.dumps(dict(cases=len(result['cases']),scope=SCOPE,deviceOperations=0,SQL=0)))
