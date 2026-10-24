# Bounded original547 per-round SetAuto anchor for the two actual feedback66 Ends.
import argparse,hashlib,json,os,subprocess,sys,time,base64,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from server.codec import message
from replay import BASE,BASE_DELIVERY_SHA,TABLE_PROOF
from run import STABLE_HOST_ENV
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()

def main():
 ap=argparse.ArgumentParser();ap.add_argument('--output-directory',type=Path,required=True);ap.add_argument('--baseline-directory',type=Path,default=BASE);a=ap.parse_args()
 out=a.output_directory.resolve();assert out.is_relative_to(ROOT) and not out.exists();out.mkdir(parents=True)
 baseline_directory=a.baseline_directory.resolve();runtime=baseline_directory/'runtime';assert sha(baseline_directory/'delivery.json')==BASE_DELIVERY_SHA
 baseline=json.loads((baseline_directory/'manifest.json').read_text())
 for p in runtime.glob('*.dll'):assert sha(p)==baseline['runtimeInputPins'][p.name],p.name
 table_file=TABLE_PROOF if TABLE_PROOF.exists() else baseline_directory.parents[2]/TABLE_PROOF.relative_to(ROOT)
 tableproof=json.loads(table_file.read_text())['originalRawTablePins'];tables={}
 for f in (runtime/'Assets').rglob('*.txt'):
  source=ROOT/'analysis/tables'/f.name.removesuffix('.txt').lower()
  if str(source.relative_to(ROOT)) not in tableproof:source=ROOT/'analysis/tables'/f.name.replace('.txt','.tsv')
  assert sha(f)==tableproof[str(source.relative_to(ROOT))]['sha256'];tables[str(f.relative_to(runtime))]=sha(f)
 assert len(tables)==162
 source=ROOT/'analysis/p6/OriginalHeadlessPlaybackCaller.cs';caller=source.read_text()
 marker='public static class OriginalHeadlessPlaybackCaller\n{';assert marker in caller
 method=r'''
    private static Dictionary<int,double> RoundAutoControls66;
    public static bool RunRushRoundLiveAuto66(SCFightPlayback identity, SCRushFightStart start, Dictionary<int,double> controls, Action<string,List<FExport>> exports, Action<EngineData> snapshots, Action<string,EngineData,object> events)
    {
        if(RoundAutoControls66!=null || controls==null || controls.Count==0)throw new InvalidOperationException("exclusive bounded round controls required");
        RoundAutoControls66=controls;
        try { return RunRushManualAutoStart(identity,start,0,exports,snapshots,events); }
        finally { RoundAutoControls66=null; }
    }
'''
 caller=caller.replace(marker,marker+method)
 begin=caller.index('    private static bool RunWithManualAutoDelay(');body=caller[begin:]
 body=body.replace('engine.NextRound();','if(RoundAutoControls66!=null)engine.SetAuto((CampType)0,false);\n                    engine.NextRound();')
 old='if (!manualAutoStarted && capture.Data.roundNum == 1 && capture.Data.roundTime >= manualAutoDelay)'
 new=r'''if (RoundAutoControls66!=null && capture.Data.curRoundCamp==(CampType)0 && !engine.IsAuto((CampType)0))
                {
                    double delay;
                    if(!RoundAutoControls66.TryGetValue(capture.Data.roundNum,out delay))throw new InvalidOperationException("missing round control");
                    if(capture.Data.roundTime>=delay) {
                        if(eventLog!=null)eventLog("round_live_auto_control",capture.Data,new Dictionary<string,object>{{"round",capture.Data.roundNum},{"requested",delay},{"actual",capture.Data.roundTime}});
                        engine.SetAuto((CampType)0,true);
                    }
                }
                if (RoundAutoControls66==null && !manualAutoStarted && capture.Data.roundNum == 1 && capture.Data.roundTime >= manualAutoDelay)'''
 assert old in body;body=body.replace(old,new);caller=caller[:begin]+body
 generated=out/'OriginalRoundLiveAutoCaller66.cs';generated.write_text(caller)
 tools=ROOT/'.tools/native-batch';dotnet=tools/'dotnet/dotnet';csc=next((tools/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
 sources=[generated]+[ROOT/'tools/native_batch'/n for n in ['NativeBatchRunner.cs','LocalPVEUICompatibility.cs','LocalActivityInformation.cs','LocalExpeditionPresentation.cs']]
 def run(label,args,seconds=60,env=None):
  started=time.monotonic();r=subprocess.run(['rtk','proxy',*map(str,args)],capture_output=True,text=True,timeout=seconds,env=env);(out/(label+'.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,(label,r.stdout+r.stderr);return time.monotonic()-started
 with tempfile.TemporaryDirectory(prefix='round66-refs-') as tmp:
  refs=Path(tmp)
  with zipfile.ZipFile(ROOT/'build/hyjs-feedback65-formal-r4.apk')as apk:
   for name in ['mscorlib.dll','System.dll','System.Core.dll']:(refs/name).write_bytes(apk.read('assets/bin/Data/Managed/'+name))
  run('compile-caller',[dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(out/'RoundLiveAutoCaller66.dll'),*['/r:'+str(p)for p in runtime.glob('*.dll')if p.name not in ['LocalOriginalCaller.dll','LocalOriginalEngineHost.dll']],*['/r:'+str(p)for p in refs.glob('*.dll')],*sources])
 refnet=next((tools/'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'));host=Path(__file__).with_name('RoundLiveAuto66Host.cs')
 run('compile-host',[dotnet,csc,'/nologo','/target:exe','/nostdlib+','/langversion:latest','/deterministic+','/out:'+str(out/'RoundLiveAuto66Host.dll'),*['/r:'+str(p)for p in refnet.glob('*.dll')],host])
 (out/'RoundLiveAuto66Host.runtimeconfig.json').write_text(json.dumps({'runtimeOptions':{'tfm':'net8.0','framework':{'name':'Microsoft.NETCore.App','version':'8.0.0'},'configProperties':{'System.Runtime.TieredCompilation':False}}}))
 fixturepath=ROOT/'hyjs-server/service/testdata/p7_round_live_auto_phone66.json';fixture=json.loads(fixturepath.read_text());start=message('SCRushFightStart');start.ParseFromString(bytes.fromhex(fixture['startHex']))
 startpath=out/'actual.SCRushFightStart.pb';startpath.write_bytes(start.SerializeToString())
 wire=message('SCFightPlayback',fightid=start.fightId,randSeed=start.randSeed,battleType=9,maxRound=start.maxRound,attackerFactor=1,defenderFactor=1);wire.attacker.CopyFrom(start.battlePlayer);wire.defender2.CopyFrom(start.battleMonsterList[0]);wirepath=out/'actual.SCFightPlayback.pb';wirepath.write_bytes(wire.SerializeToString())
 results=[]
 for i,entry in enumerate(fixture['ends']):
  end=message('CSRushFightEnd');end.ParseFromString(bytes.fromhex(entry['endHex']));controls=[]
  for op in end.optRecord.skillOpts:
   if op.camp==0 and (not controls or controls[-1]['roundNum']!=op.roundNum):controls.append({'roundNum':op.roundNum,'roundTime':op.roundTime})
  controlpath=out/f'{i:03d}.controls.json';controlpath.write_text(json.dumps(controls));output=out/f'{i:03d}.native.jsonl'
  elapsed=run(f'execute-{i:03d}',[dotnet,out/'RoundLiveAuto66Host.dll',runtime,out/'RoundLiveAutoCaller66.dll',wirepath,startpath,controlpath,output],env={**os.environ,**STABLE_HOST_ENV})
  rows=[json.loads(line)for line in output.read_text().splitlines()];final=next(x['data']for x in rows if x['kind']=='original_final_operates')
  actual=message('BattleOperate');actual.ParseFromString(base64.b64decode(final['protobufBase64']));assert actual==end.optRecord,('strict original PB mismatch',i)
  result=next(x for x in rows if x['kind']=='result');assert result['attackerWon']==(end.result==1)
  results.append({'index':i,'endSHA256':entry['sha256'],'controls':controls,'seconds':elapsed,'strictFullPB':True,'result':result,'outputSHA256':sha(output),'outputBytes':output.stat().st_size})
 size=sum(p.stat().st_size for p in out.rglob('*')if p.is_file());assert size<=50*1024*1024
 report={'schema':'feedback66-round-live-auto-original547-anchor-v1','nativePhoneSource':True,'executionOrigin':'local-coreclr-original547-not-phone-replay','originalDLLPins':baseline['runtimeInputPins'],'original162TablePins':tables,'originalCallerSourceSHA256':sha(source),'outerCallerSHA256':sha(generated),'newHostSourceSHA256':sha(host),'fixtureSHA256':sha(fixturepath),'original547Unchanged':True,'cases':results,'bytes':size,'publicFrozenModelChanged':False,'authoritative':False}
 (out/'manifest.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'cases':results,'bytes':size}))
if __name__=='__main__':main()
