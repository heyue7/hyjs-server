"""Patch one original CIL threshold byte in the exact official79 managed DLL."""
import argparse,hashlib,json,sys,zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT/'tools'))
from awake_diff_rows.build import methods
from local_asset_ready.remove_gate import exact_body
BASE_SHA='60075046b9dfde0a00083b07a3cca878d071b604fcb31bad63d8c21be821eff8'
MANAGED_SHA='2dfd8ca0869c24dc55e90df179ec3c545f4f91b60c44974f03460a8280d9e725'
TARGET='UI.UIInstPlayerData::GetAvailableHerosForFormation'
IL_SHA='c8808d70d4c54fd141656f433f3fe3be185587859af96cb638c2b9a86d522697'
ENTRY='assets/bin/Data/Managed/Assembly-CSharp.dll'
sha=lambda b:hashlib.sha256(b).hexdigest()
def digest(p):
 with Path(p).open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def build(base,out):
 assert digest(base)==BASE_SHA and not out.exists();out.mkdir(parents=True)
 with zipfile.ZipFile(base)as apk:before=apk.read(ENTRY);dex=apk.read('classes2.dex')
 assert sha(before)==MANAGED_SHA
 pe=dnfile.dnPE(data=before);all_methods=methods(pe);name,rid,m=next(x for x in all_methods if x[0]==TARGET)
 body=read_method_body_from_bytes(pe.get_data(m.Rva));il=pe.get_data(m.Rva+body.header_size,body.code_size);assert sha(il)==IL_SHA
 instructions=body.instructions;constants=[i for k,i in enumerate(instructions)if i.opcode.name=='ldc.i4.s' and i.operand==12 and k>0 and instructions[k-1].opcode.name=='ldfld' and instructions[k+1].opcode.name=='bgt'];assert len(constants)==1
 inst=constants[0];offset=pe.get_offset_from_rva(m.Rva)+inst.offset+1;assert before[offset]==12 and before[offset-1]==0x1f
 after=bytearray(before);after[offset]=11;after=bytes(after);assert [i for i,(a,b)in enumerate(zip(before,after))if a!=b]==[offset]
 patched=dnfile.dnPE(data=after);new=methods(patched);changed=[];engine=0;callers=[];needle=bytes([0x6f])+(0x06000000|rid).to_bytes(4,'little')
 for (n,r,x),(nn,rr,y)in zip(all_methods,new):
  assert (n,r,x.Rva,x.Signature.value,x.struct.Flags,x.struct.ImplFlags)==(nn,rr,y.Rva,y.Signature.value,y.struct.Flags,y.struct.ImplFlags)
  if not x.Rva:continue
  a,b=exact_body(pe,x),exact_body(patched,y)
  if needle in a:callers.append(n)
  if a!=b:changed.append(n)
  else:engine+=n.startswith('Battle.Engine')
 assert changed==[TARGET] and engine==547 and len(all_methods)==len(new)
 file=out/'Assembly-CSharp.dll';file.write_bytes(after)
 sources=[Path(__file__).resolve(),Path(__file__).with_name('CandidateProbe.cs'),ROOT/'hyjs-server/constUtil/p5FormationEligibility.go',ROOT/'hyjs-server/service/p5FormationEligibilityService.go',ROOT/'tools/awake_diff_rows/build.py',ROOT/'tools/local_asset_ready/remove_gate.py']
 proof={'schema':'hyjs-feedback88-green-blue-formation-only-v1','baseAPKSHA256':BASE_SHA,'inputManagedSHA256':sha(before),'inputDexSHA256':sha(dex),'outputManagedSHA256':sha(after),'managedOnly':True,'inheritedJavaByteExact':True,'inheritedJavaPublicProtectedABIExact':True,'allowedChangedMethods':[TARGET],'payloadFiles':{ENTRY:str(file.relative_to(ROOT))},'replacementEntries':{ENTRY:sha(after)},'sourcePins':{str(p.relative_to(ROOT)):digest(p)for p in sources},'changedByteOffset':offset,'beforeQualityThreshold':12,'afterQualityThreshold':11,'original547EngineMethodsChanged':False,'originalEngineMethodsUnchanged':engine,'otherRawMethodBodiesExact':len(all_methods)-1,'callers':callers,'deviceInstalled':False}
 (out/'manifest.json').write_text(json.dumps(proof,indent=2));print(json.dumps({k:proof[k]for k in ['outputManagedSHA256','changedByteOffset','callers','originalEngineMethodsUnchanged']}))
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args();build(a.base_apk.resolve(),a.output_directory.resolve())
