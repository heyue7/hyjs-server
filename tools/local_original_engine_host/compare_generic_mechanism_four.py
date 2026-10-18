"""Strict fixed four phone/host/Go terminal comparison; platform stack explicit."""
import base64
import hashlib
import json
from pathlib import Path
import sys
from itertools import zip_longest
from generic_mechanism_four import ROOT, SOURCE, SOURCE_SHA, sha


def differences(a, b, path="$", output=None):
    output = [] if output is None else output
    if type(a) is not type(b):
        output.append({"path":path,"expected":a,"actual":b}); return output
    if isinstance(a, dict):
        for key in sorted(set(a)|set(b)):
            if key not in a or key not in b:
                output.append({"path":path+"."+key,"missing":"expected" if key not in a else "actual"})
            else: differences(a[key],b[key],path+"."+key,output)
    elif isinstance(a,list):
        if len(a)!=len(b):output.append({"path":path,"expectedLength":len(a),"actualLength":len(b)})
        for i,(x,y) in enumerate(zip(a,b)):differences(x,y,path+"["+str(i)+"]",output)
    elif a!=b:output.append({"path":path,"expected":a,"actual":b})
    return output


def main():
    sys.path.insert(0,str(ROOT))
    from server.codec import message
    source=json.loads(SOURCE.read_bytes());assert sha(SOURCE)==SOURCE_SHA
    host_path=ROOT/'analysis/playable-plan-20261017/generic-original-host-r1/manifest.json'
    phone_path=ROOT/'analysis/p6/generic-four-phone-r1-20261017/manifest.json'
    host=json.loads(host_path.read_bytes());phone=json.loads(phone_path.read_bytes())
    assert host['caseSet']=='generic-mechanism-four' and host['status']=='EXECUTED_PENDING_STRICT_COMPARE'
    assert phone['sourceManifestSHA256']==SOURCE_SHA and len(phone['cases'])==4
    hostrows={r['wireSHA256']:r for r in host['caseSets'][0]['cases']};phonerows={r['wireSHA256']:r for r in phone['cases']}
    cases=[]
    for row in source['cases']:
        wire=SOURCE.parent/row['wire'];assert sha(wire)==row['wireSHA256']
        hr=hostrows[row['wireSHA256']];pr=phonerows[row['wireSHA256']]
        hp=(host_path.parent/'runtime'/hr['localWire']).with_suffix('.native.jsonl');pp=phone_path.parent/pr['nativeResult']
        assert sha(hp)==host['outputPins'][str(hp.relative_to(host_path.parent))] and sha(pp)==pr['nativeSHA256']
        h=[json.loads(line) for line in hp.open()];p=[json.loads(line) for line in pp.open()]
        diff=differences(h,p)
        stacks=[];payload=[]
        for d in diff:
            path=d['path'];index=int(path.split('[')[1].split(']')[0]) if path.startswith('$[') else -1
            if 0<=index<len(h) and h[index].get('kind')=='ai_notify' and (path==f'$[{index}].data.stack' or path.startswith(f'$[{index}].data.stack[')):stacks.append(d)
            else:payload.append(d)
        result=p[-1];states=[x['state'] for x in p if x.get('kind')=='state'];state=states[-1];records=[x['data'] for x in p if x.get('kind')=='original_final_operates'];assert len(records)==1 and result['kind']=='result'
        gpath=ROOT/'analysis/playable-plan-20261017/generic-mechanism-inputs-r1'/(row['family']+'.go-final.json');g=json.loads(gpath.read_bytes())
        proto=message('BattleOperate',**g['originalOperates']);raw=proto.SerializeToString(deterministic=True);original=base64.b64decode(records[0]['protobufBase64'],validate=True)
        final=[(camp,a) for camp,key in enumerate(['attacker','defender']) for a in state[key]['actors']]
        terminal=[]
        for (camp,a),b in zip(final,g['finalActors'],strict=True):
            assert (camp,a['uid'],a['heroId'],a['state'],a['attribute']['hp'],a['attribute']['maxHp'])==(b['camp'],b['uid'],b['heroId'],b['state'],b['currentHp'],b['maxHp'])
            terminal.append({'camp':camp,'uid':a['uid'],'heroId':a['heroId'],'state':a['state'],'currentHp':a['attribute']['hp'],'maxHp':a['attribute']['maxHp']})
        assert original==raw and result['attackerWon']==(g['outcome']['winner']==0) and state['round']==g['outcome']['finalRound']
        cases.append({'case':row['case'],'phoneEvents':len(p),'hostEvents':len(h),'fullSchemaStatus':'PASS' if not diff else 'FAIL','battlePayloadStatusExceptExplicitPlatformStack':'PASS' if not payload else 'FAIL','platformStackDifferences':stacks,'otherDifferences':payload,'originalFinalProtocolBytesStatus':'PASS','originalFinalProtocolSHA256':hashlib.sha256(original).hexdigest(),'zeroToleranceWinnerRoundAndAllFinalActors':'PASS','terminal':terminal,'phoneSHA256':sha(pp),'hostSHA256':sha(hp),'fixedGoFinalSHA256':sha(gpath)})
    report={'schema':'generic-four-phone-host-go-strict-v1','caseCount':4,'sourceManifestSHA256':SOURCE_SHA,'phoneManifestSHA256':sha(phone_path),'hostManifestSHA256':sha(host_path),'numericTolerance':0,'arrayOrderingChanged':False,'originalFilesRewritten':False,'cases':cases,'allBattlePayloadsAndTerminalsExact':all(not r['otherDifferences'] for r in cases),'newNativeObservedScope':'Four mechanism representatives only, not all 523 admitted branch shapes','nativeCompared':False,'authoritative':False}
    out=ROOT/'analysis/playable-plan-20261017/generic-phone-host-strict-comparison-r2.json';assert not out.exists();out.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'allBattlePayloadsAndTerminalsExact':report['allBattlePayloadsAndTerminalsExact'],'cases':[{'case':r['case'],'fullSchemaStatus':r['fullSchemaStatus'],'battlePayloadStatus':r['battlePayloadStatusExceptExplicitPlatformStack'],'platformStackDifferences':len(r['platformStackDifferences']),'otherDifferences':len(r['otherDifferences'])} for r in cases]},ensure_ascii=False))
    return 0 if report['allBattlePayloadsAndTerminalsExact'] else 1


if __name__=='__main__':raise SystemExit(main())
