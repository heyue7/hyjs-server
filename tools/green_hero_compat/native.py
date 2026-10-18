"""Run original Engine on the six approved compatible rows; no rendering claim."""
from pathlib import Path
import hashlib,json,struct,subprocess,os,time,shutil
ROOT=Path(__file__).resolve().parents[2]
BASE=ROOT/"analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1"
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def vi(n):
    r=bytearray()
    while n>127:r.append((n&127)|128);n>>=7
    r.append(n);return bytes(r)
def integer(k,n):return vi(k<<3)+vi(n)
def blob(k,v):
    if isinstance(v,str):v=v.encode()
    return vi(k<<3|2)+vi(len(v))+v
def dbl(k,v):return vi(k<<3|1)+struct.pack("<d",v)
def main():
    import argparse
    p=argparse.ArgumentParser();p.add_argument("--assets",type=Path,required=True);p.add_argument("--output-directory",type=Path,required=True);a=p.parse_args()
    assets=a.assets.resolve();out=a.output_directory.resolve();out.mkdir(exist_ok=False)
    work=out/"runtime";work.mkdir()
    reference=json.loads((BASE/"manifest.json").read_text())
    pins={}
    for name,digest in reference["runtimeInputPins"].items():
        src=BASE/"runtime"/name
        assert sha(src)==digest
        (work/name).symlink_to(src);pins[name]=digest
    shutil.copy2(BASE/"runtime/LocalOriginalEngineHost.runtimeconfig.json",work/"LocalOriginalEngineHost.runtimeconfig.json")
    tables=work/"Assets/OutputBundle/tables";tables.mkdir(parents=True)
    for source in (BASE/"runtime/Assets/OutputBundle/tables").glob("*.txt"):
        derived=assets/(source.stem+".tsv")
        if derived.exists():shutil.copy2(derived,tables/source.name)
        else:(tables/source.name).symlink_to(source)
    rows=(assets/"TableHero.tsv").read_text().splitlines();keys=rows[1].split("\t");heroes={r[0]:dict(zip(keys,r)) for r in [l.split("\t") for l in rows[5:]] if r}
    props={1:10,2:50,3:5,4:5,9:10,10:95,11:0,12:5,13:0,14:0,15:50}
    prop=b"".join(dbl(k,v) for k,v in props.items())
    def party(camp):
        result=integer(1,camp+1)+integer(2,1)+integer(3,1)+blob(4,"compat-green")
        for index in range(6):
            ident="hero_%03d"%(56+index);row=heroes[ident]
            hero=blob(1,ident)+blob(2,"green-%s-%s"%(camp,index))+blob(4,row["SkillId1"]+",,")+blob(5,prop)+integer(6,1)+integer(7,index+1)+blob(8,dbl(1,50))+dbl(11,100)+integer(12,1)
            result+=blob(5,hero)
        return result+blob(6,dbl(1,0))
    case=work/"cases/highest";case.mkdir(parents=True)
    wire=blob(1,"green-six-compatible-original-engine")+integer(2,42)+integer(3,11)+blob(4,party(0))+blob(5,party(1))+dbl(11,1)+dbl(12,1)+integer(13,10)
    (case/"000.pb").write_bytes(wire)
    record=dict(schema="original-engine-green-six-compatibility-v1",status="STARTED",executionOrigin="local-original-Engine-with-user-approved-derived-tables",originalDLLSHA256=pins["Assembly-CSharp.dll"],runtimeDLLPins=pins,sourceSHA256=sha(Path(__file__)),assetManifestSHA256=sha(assets/"manifest.json"),wireSHA256=sha(case/"000.pb"),originalTableOverrides=["Hero","Frag"],runtimeRenderingVerified=False,phoneExecuted=False,authoritative=False)
    (out/"manifest.json").write_text(json.dumps(record,indent=2)+"\n")
    start=time.monotonic();run=subprocess.run([str(ROOT/".tools/native-batch/dotnet/dotnet"),str(work/"LocalOriginalEngineHost.dll"),str(work),str(case)],cwd=ROOT,capture_output=True,text=True,timeout=45,env={**os.environ,**reference["runtimeEnvironment"]})
    (out/"execute.log").write_text(run.stdout+run.stderr)
    record.update(seconds=time.monotonic()-start,exitCode=run.returncode,status="ORIGINAL_ENGINE_EXECUTED_GO_COMPARE_PENDING" if run.returncode==0 else "FAILED")
    if run.returncode==0:
        trace=case/"000.native.jsonl"
        shutil.copy2(trace,out/"original.native.jsonl")
        record["traceSHA256"]=sha(trace)
        shutil.copy2(case/"000.pb",out/"original-input.pb")
    (out/"manifest.json").write_text(json.dumps(record,indent=2)+"\n")
    print(json.dumps({k:v for k,v in record.items() if k!="runtimeDLLPins"}))
    assert run.returncode==0,run.stdout+run.stderr
if __name__=="__main__":main()
