"""Separate original HTTP request identity time from the adjustable game calendar.
Only replace an existing call token; no metadata, signature or battle code changes.
"""
import argparse,hashlib,json,zipfile,sys
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
def sha(b):return hashlib.sha256(b).hexdigest()
def main():
 a=argparse.ArgumentParser();a.add_argument('--base-apk',type=Path,required=True);a.add_argument('--output-directory',type=Path,required=True);x=a.parse_args();out=x.output_directory.resolve();assert out.is_relative_to(ROOT/'analysis') and not out.exists();out.mkdir(parents=True)
 entry='assets/bin/Data/Managed/Assembly-CSharp.dll'
 with zipfile.ZipFile(x.base_apk) as z: original=z.read(entry)
 pe=dnfile.dnPE(data=original);types={str(t.TypeNamespace)+'.'+str(t.TypeName):t for t in pe.net.mdtables.TypeDef.rows}
 def method(owner,name):return next(m for m in types[owner].MethodList if str(m.row.Name)==name)
 current=method('.ServerTimerTool','CurrentUTCJavaTime');wall=method('.ServerTimerTool','ClientUtcJavaTime');web=method('NetWork.Packets.Factory.PacketFactory','WebPB2Bytes')
 old=0x06000000+current.row_index;new=0x06000000+wall.row_index
 assert current.row.Signature.value==wall.row.Signature.value
 body=read_method_body_from_bytes(pe.get_data(web.row.Rva,1048576));calls=[i for i in body.instructions if i.opcode.name=='call' and getattr(i.operand,'value',None)==old];assert len(calls)==1
 offset=pe.get_offset_from_rva(web.row.Rva)+calls[0].offset+1
 assert original[offset:offset+4]==old.to_bytes(4,'little')
 changed=bytearray(original);changed[offset:offset+4]=new.to_bytes(4,'little');changed=bytes(changed)
 assert len(changed)==len(original) and [i for i,(a,b)in enumerate(zip(original,changed)) if a!=b]==[offset]
 after=dnfile.dnPE(data=changed);unchanged=engine=0
 for (name,rid,row),(other,oid,modified) in zip(methods(pe),methods(after)):
  assert (name,rid)==(other,oid) and row.Signature.value==modified.Signature.value and row.struct.Flags==modified.struct.Flags
  if not row.Rva:continue
  if rid!=web.row_index:
   assert exact_body(pe,row)==exact_body(after,modified),name
   unchanged+=1;engine+=name.startswith('Battle.Engine')
 assert engine==547
 target=out/'Assembly-CSharp.dll';target.write_bytes(changed)
 proof=dict(schema='hyjs-request-wall-clock75-v1',baseAPKSHA256=sha(x.base_apk.read_bytes()),payloadFiles={entry:str(target.relative_to(ROOT))},replacementEntries={entry:sha(changed)},originalEngineMethodsExact=engine,unchangedInheritedRawMethodBodies=unchanged,originalMetadataABIAndTokensPreserved=True,mainAssemblyUnchanged=False,originalDexUnchanged=True,scope=dict(method='NetWork.Packets.Factory.PacketFactory::WebPB2Bytes',token=hex(0x06000000+web.row_index),oldCall=hex(old),newCall=hex(new),byteOffset=offset,changedBytes=1,requestSignatureUsesSameWireTimestamp=True),inputManagedSHA256=sha(original),sourcePins={str(Path(__file__).relative_to(ROOT)):sha(Path(__file__).read_bytes())},deviceTested=False)
 (out/'manifest.json').write_text(json.dumps(proof,indent=2));print(json.dumps(proof['scope']))
if __name__=='__main__':main()
