"""Focused original CIL/metadata evidence; never loads the target into a CLR."""
from pathlib import Path
import sys
import dnfile
sys.path.insert(0,str(Path(__file__).resolve().parents[2]))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded

def inspect(path, names):
 pe=dnfile.dnPE(str(path)); all_methods=methods(pe)
 method_names={rid:n for n,rid,m in all_methods}; fields={}; output=[]
 for t in pe.net.mdtables.TypeDef.rows:
  name=(str(t.TypeNamespace)+'.' if str(t.TypeNamespace) else '')+str(t.TypeName)
  for f in t.FieldList: fields[f.row_index]=name+'::'+str(f.row.Name)
  if name in names:
   output.append('TYPE '+name)
   for f in t.FieldList: output.append('FIELD '+str(f.row.Name)+' flags='+str(f.row.struct.Flags))
 def resolve(token):
  try:
   table=token>>24; rid=token&0xffffff
   if table==10:
    r=pe.net.mdtables.MemberRef.rows[rid-1]; return str(getattr(r.Class.row,'TypeName',''))+'::'+str(r.Name)
   if table==6:return method_names[rid]
   if table==4:return fields[rid]
   if table==0x70:return repr(str(pe.net.user_strings.get(rid)))
  except (KeyError,IndexError,AttributeError):pass
  return hex(token)
 for name,rid,m in all_methods:
  if (name in names or name.rsplit('::',1)[0] in names) and m.Rva:
   body,code=decoded(pe,m);output.append('\nMETHOD '+name+' TOKEN '+hex(0x6000000|rid))
   for index,(op,value) in enumerate(code):
    resolved=resolve(value) if isinstance(value,int) and op.startswith(('call','ldfld','stfld','ldsfld','stsfld','ldstr','newobj','ldftn')) else str(value)
    output.append(str(index)+' '+op+' '+resolved)
 return '\n'.join(output)+'\n'
if __name__=='__main__':
 source,out,*names=sys.argv[1:];Path(out).write_text(inspect(source,set(names)))
