"""Export read-only Arena2 diagnostic drafts from already captured actual rosters.

No database/device access occurs. This bridge retains exact actors, full skills,
gear-derived panels, source engine provenance and seed; Arena attacker-first is explicitly selected.
Only diagnostic type/callback/fight identity and declared Arena first-order change; no winner is supplied.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from server.codec import message

def digest(raw):return hashlib.sha256(raw).hexdigest()

def export(output, include_ultimates=False):
    output=Path(output).resolve()
    if not output.is_relative_to(ROOT/'analysis/p6') or output.exists():raise ValueError('Use a new immutable analysis/p6 directory')
    source=ROOT/'analysis/p6/actual-demo-readonly-branches-inputs-v140'
    raw=(source/'manifest.json').read_bytes()
    if digest(raw)!='b2f3dbbac3c16591309064a58b531a48e55d5f28d60839ca8e84c2d352c2b94a':raise ValueError('Actual source manifest changed')
    manifest=json.loads(raw);rows=[];prepared=[]
    selected=[(source,manifest['cases'][i],i)for i in [0,1,4,5]]
    ultimate_source=ROOT/'analysis/p6/actual-demo-ultimate-mirror-inputs-v147'
    if include_ultimates:
        ultimate_raw=(ultimate_source/'manifest.json').read_bytes()
        if digest(ultimate_raw)!='67e7353303e974251b6c46038726f99fe0d1bbc7d52e8e1080e57af8901138ce':raise ValueError('Actual complete-party mirror source changed')
        selected.extend((ultimate_source,c,i)for i,c in enumerate(json.loads(ultimate_raw)['cases']))
    # Current public Arena query freezes attacker-first. Preserve that actual
    # admitted boundary and both original directions with two independent seeds.
    for index,(case_source,case,source_index) in enumerate(selected):
        wire_raw=(case_source/case['wire']).read_bytes();frozen_raw=(case_source/case['frozenInput']).read_bytes()
        if digest(wire_raw)!=case['wireSHA256'] or digest(frozen_raw)!=case['frozenInputSHA256']:raise ValueError('Source case SHA mismatch')
        wire=message('SCFightPlayback');wire.ParseFromString(wire_raw)
        if wire.battleType!=11 or wire.HasField('optRecord') or (case_source==source and case['firstCamp']!=0):raise ValueError('Unexpected actual source mode')
        frozen=json.loads(frozen_raw)
        wire.battleType=2;wire.fightid='readonly-actual-legacy-arena-'+str(index)
        # Publish the current Arena producer's original attacker-first policy
        # explicitly. All actor/skill/panel/GUID/currentHP bytes remain unchanged.
        wire.attacker.inst.firstOrder=1;wire.defender1.inst.firstOrder=0
        frozen['defenderFirst']=False
        frozen['attacker']['inst']['firstOrder']=1;frozen['defender']['inst']['firstOrder']=0
        callback=message('SCPvpFightResult',fightId=wire.fightid,randSeed=wire.randSeed,maxRoundNum=40)
        callback.playerList.add().CopyFrom(wire.attacker);callback.playerList.add().CopyFrom(wire.defender1)
        frozen['battleType']=2
        frozen.setdefault('appliedProperties',{}).setdefault('source',[]).append('Explicit local diagnostic type2 bridge from immutable actual128b/140/147 actor source; declared current Arena attacker-first1/0, no actual Arena query, challenge, payment, rank, result or private-server claim')
        name=f'{index:03d}'
        data={name+'.pb':wire.SerializeToString(deterministic=True),name+'.SCPvpFightResult.pb':callback.SerializeToString(deterministic=True),name+'.frozen-input.json':(json.dumps(frozen,ensure_ascii=False,indent=2)+'\n').encode()}
        prepared.append(data)
        rows.append({'case':'actual-legacy-arena2-bridge/'+case['case'],'battleType':2,'fightId':wire.fightid,'seed':wire.randSeed,'firstCamp':0,'wire':name+'.pb','wireSHA256':digest(data[name+'.pb']),'typedCallback':name+'.SCPvpFightResult.pb','typedCallbackSHA256':digest(data[name+'.SCPvpFightResult.pb']),'frozenInput':name+'.frozen-input.json','frozenInputSHA256':digest(data[name+'.frozen-input.json']),'sourceCase':source_index,'sourceInputManifest':str((case_source/'manifest.json').relative_to(ROOT)),'sourceActualInputSHA256':manifest['sourceActualInputSHA256'],'nativeScope':'original-pvp-automatic-ai','status':'awaiting-original-automatic-ai'})
    output.mkdir()
    for items in prepared:
        for name,data in items.items():(output/name).write_bytes(data)
    result={'scope':'Read-only local diagnostic Arena2 typed bridge; immutable actual Demo4/3 actors, full three skills, talent, gear-derived flat panels and GUID retained. Seeds retained from actual140/147 diagnostic branches; current public Arena attacker-first1/0 is explicitly selected, including the prior mirror defender-first sources. This is not an actual query-produced/paid Arena match or a server result. No expected winner, no DB/device access, no scope authority inferred. Original typed SetData applies map_arena_001.', 'sourceManifest':str((source/'manifest.json').relative_to(ROOT)),'sourceManifestSHA256':digest(raw),'sourceActorEngineVersion':manifest['engineVersion'],'cases':rows,'nativeCompared':False,'authoritative':False,'includesCompleteOriginalUltimateMirrors':include_ultimates}
    (output/'manifest.json').write_text(json.dumps(result,indent=2)+'\n')
    return result

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--output',type=Path,required=True);parser.add_argument('--include-ultimates',action='store_true');args=parser.parse_args()
    result=export(args.output,args.include_ultimates);print(str(args.output/'manifest.json'),digest((args.output/'manifest.json').read_bytes()),'cases',len(result['cases']))
