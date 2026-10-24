# Strict native547/Go/phone-End comparison for the fixed owned QA two-case anchor.
import argparse,base64,hashlib,json,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from server.codec import message
from google.protobuf.json_format import ParseDict
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--native-directory',type=Path,required=True);ap.add_argument('--go-directory',type=Path,required=True);a=ap.parse_args()
 manifest=a.native_directory/'manifest.json';native=json.loads(manifest.read_text());fixturepath=ROOT/'hyjs-server/service/testdata/p7_round_live_auto_phone66.json';fixture=json.loads(fixturepath.read_text());assert sha(fixturepath)==native['fixtureSHA256'];results=[]
 for case in native['cases']:
  i=case['index'];np=a.native_directory/f'{i:03d}.native.jsonl';gp=a.go_directory/f'{i:03d}.go-final.json';assert sha(np)==case['outputSHA256']
  rows=[json.loads(l)for l in np.open()];g=json.loads(gp.read_text());end=message('CSRushFightEnd');end.ParseFromString(bytes.fromhex(fixture['ends'][i]['endHex']))
  nbytes=base64.b64decode(next(r['data']['protobufBase64']for r in rows if r['kind']=='original_final_operates'));record=message('BattleOperate');record.ParseFromString(nbytes)
  go=message('BattleOperate');ParseDict(g['originalOperates'],go)
  assert record==go==end.optRecord
  assert nbytes==go.SerializeToString(deterministic=True)==end.optRecord.SerializeToString(deterministic=True)
  press=[(r['round'],r['time'],r['data']['camp'],r['data']['actorUid'],r['data']['skillIndex'])for r in rows if r['kind']=='skill_press']
  gpress=[(r['round'],r['time'],r['camp'],r['actor'],int(r.get('value',0)))for r in g['outcome']['events']if r['kind']=='skill_press'];assert press==gpress
  begin=[]
  for r in rows:
   if r['kind']!='skill_begin':continue
   data=r['data'];target=data['target']['actor']if data['target']else None
   # These fixed actual actors have only one cast target per begun skill; none is dropped.
   assert [x['actor']for x in data['castTargets']]==([target]if target else [])
   begin.append((r['round'],r['time'],data['id'],target))
  gbegin=[(r['round'],r['time'],r['skill'],r.get('target'))for r in g['outcome']['events']if r['kind']=='skill_begin'];assert begin==gbegin
  final=[r['state']for r in rows if r['kind']=='state'][-1]
  actors=[(camp,x['uid'],x['heroId'],x['state'],x['attribute']['hp'],x['attribute']['maxHp'])for camp,key in enumerate(['attacker','defender'])for x in final[key]['actors']]
  gactors=[(x['camp'],x['uid'],x['heroId'],x['state'],x['currentHp'],x['maxHp'])for x in g['finalActors']];assert actors==gactors
  assert final['round']==g['outcome']['finalRound']==record.finalRoundNum
  won=next(r['attackerWon']for r in rows if r['kind']=='result');assert won==(g['outcome']['winner']==0)==(end.result==1)
  results.append({'index':i,'nativeSHA256':sha(np),'goSHA256':sha(gp),'phoneEndSHA256':fixture['ends'][i]['sha256'],'exactPressEvents':len(press),'exactBeginTargetsAndClocks':len(begin),'exactFinalActors':len(actors),'protocolBytesExact':True,'roundWinnerHPExact':True})
 report={'schema':'feedback66-round-live-auto-strict-native-go-phone-v1','cases':results,'numericTolerance':0,'arrayOrderingChanged':False,'sourceFilesRewritten':False,'fullSchemaCompared':False,'scope':'two actual typed9 records, original547 live-AI driver; excludes platform stack identity and unobserved UI control timeline','nativeManifestSHA256':sha(manifest),'newPhoneExecutionPerformed':False}
 out=a.native_directory/'go-native-strict-comparison.json';assert not out.exists();out.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report))
if __name__=='__main__':main()
