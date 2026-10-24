"""Scoped binary XML edit; no aapt recompilation or resource-table rewrite."""
import hashlib
import struct
from tools.client_branding.branding import chunks, strings_from_pool, make_pool, elements
OLD = 'com.hyjs.wallet.ClientUpdateActivity'
NEW = 'com.hyjs.compat.StartupActivity'
ANDROID = 'http://schemas.android.com/apk/res/android'

def require(ok, why):
    if not ok: raise ValueError(why)

def patch(raw):
    parts = chunks(raw)
    pool_at = [i for i,p in enumerate(parts) if p[0] == 1]
    maps = [i for i,p in enumerate(parts) if p[0] == 0x180]
    require(len(pool_at) == len(maps) == 1, 'Expected one pool/resource map')
    pi, mi = pool_at[0], maps[0]
    values, flags = strings_from_pool(parts[pi][2]); values = list(values)
    original_values = list(values)
    require(NEW not in values, 'Bootstrap already present')
    def string(value):
        if value not in values: values.append(value)
        return values.index(value)
    for value in [NEW, 'android.hardware.microphone', 'required', 'uses-feature']:
        string(value)
    ns = values.index(ANDROID)
    resource_map = list(struct.unpack('<'+'I'*((len(parts[mi][2])-8)//4),parts[mi][2][8:]))
    for key, ident in [('name',0x01010003),('required',0x0101028e)]:
        index = string(key); resource_map += [0] * max(0,index+1-len(resource_map))
        require(resource_map[index] in (0,ident), 'Wrong Android attribute ID')
        resource_map[index] = ident
    def tag(index): return values[struct.unpack_from('<I',parts[index][2],20)[0]]
    def attrs(content):
        start,width,count=struct.unpack_from('<3H',content,24)
        require(start == width == 20, 'Unexpected attributes')
        return [content[36+i*20:56+i*20] for i in range(count)]
    def attr_value(entry):
        return values[struct.unpack_from('<I',entry,16)[0]] if entry[15] == 3 else None
    stack=[]; old_start=None; old_end=None; filter_start=None; filter_end=None; app_start=None
    for i,(kind,header,content) in enumerate(parts):
        if kind == 0x102:
            name=tag(i)
            if name == 'application': app_start=i
            if name == 'activity' and any(values[struct.unpack_from('<I',a,4)[0]]=='name' and attr_value(a)==OLD for a in attrs(content)):
                require(old_start is None,'Duplicate original activity'); old_start=i
            if name == 'intent-filter' and stack and stack[-1] == old_start:
                require(filter_start is None,'Multiple launcher filters'); filter_start=i
            stack.append(i)
        elif kind == 0x103:
            require(bool(stack),'Unbalanced binary XML'); start=stack.pop()
            if start == old_start: old_end=i
            if start == filter_start: filter_end=i
    require(not stack and None not in (old_start,old_end,filter_start,filter_end,app_start),'Launcher tree missing')
    intent_parts=parts[filter_start:filter_end+1]
    decoded_filter=elements(intent_parts,values)
    require(decoded_filter == [('intent-filter',[]),('action',[(ANDROID,'name',3,'android.intent.action.MAIN')]),('category',[(ANDROID,'name',3,'android.intent.category.LAUNCHER')])], 'Unexpected launcher intent filter')
    require(not any(t=='uses-feature' and any(k=='name' and v=='android.hardware.microphone' for _,k,_,v in a) for t,a in elements(parts,values)), 'Microphone already declared')
    # Clone the original launcher's theme/label/orientation/config without its Unity metadata.
    new_start=bytearray(parts[old_start][2]); renamed=0
    for i,a in enumerate(attrs(new_start)):
        if values[struct.unpack_from('<I',a,4)[0]]=='name':
            pos=36+i*20; struct.pack_into('<I',new_start,pos+8,string(NEW)); struct.pack_into('<I',new_start,pos+16,string(NEW)); renamed+=1
    require(renamed==1,'Expected one activity name')
    new_activity=[bytes(new_start),*[p[2] for p in intent_parts],parts[old_end][2]]
    def start_node(name,entries):
        return struct.pack('<HHI4I6H',0x102,16,36+20*len(entries),1,0xffffffff,0xffffffff,string(name),20,20,len(entries),0,0,0)+b''.join(entries)
    def attribute(key,kind,data,raw_value=0xffffffff):
        return struct.pack('<IIIHBBI',ns,string(key),raw_value,8,0,kind,data)
    mic=string('android.hardware.microphone')
    feature=start_node('uses-feature',[attribute('name',3,mic,mic),attribute('required',18,0)])
    feature_end=struct.pack('<HHI4I',0x103,16,24,1,0xffffffff,0xffffffff,string('uses-feature'))
    rebuilt=[]
    for i,(kind,header,content) in enumerate(parts):
        if filter_start <= i <= filter_end: continue
        if i == pi: content=make_pool(values,flags)
        elif i == mi: content=struct.pack('<HHI',0x180,8,8+4*len(resource_map))+struct.pack('<'+'I'*len(resource_map),*resource_map)
        if i == app_start: rebuilt.extend([feature,feature_end])
        rebuilt.append(content)
        if i == old_end: rebuilt.extend(new_activity)
    output=struct.pack('<HHI',3,8,8+sum(map(len,rebuilt)))+b''.join(rebuilt)
    before=elements(parts,original_values); after=elements(chunks(output),values)
    # Every existing element/attribute remains exact; only its one intent-filter moves.
    expected=[]
    original_activity=before[[t for t,_ in before].index('activity')]
    # Use exact chunk position rather than assuming there are no other activities.
    cloned=elements([(0x102,16,new_activity[0])],values)[0]
    for i,(kind,header,content) in enumerate(parts):
        if filter_start <= i <= filter_end: continue
        if i == app_start: expected.append(('uses-feature',[(ANDROID,'name',3,'android.hardware.microphone'),(ANDROID,'required',18,0)]))
        if kind == 0x102: expected.extend(elements([(kind,header,content)],original_values))
        if i == old_end: expected.extend([cloned,*decoded_filter])
    require(after == expected,'Unrelated manifest semantic change')
    return output, {'originalElementsAndAttributesExact':True,'originalActivityAttributesAndUnityMetadataExact':True,
        'onlyOriginalNodeMovement':'MAIN/LAUNCHER intent-filter to new plain Activity',
        'newActivity':NEW,'destinationActivity':OLD,'microphoneRequired':False,
        'originalStringIndicesPreserved':True,'resourcesArscChanged':False,
        'beforeSHA256':hashlib.sha256(raw).hexdigest(),'afterSHA256':hashlib.sha256(output).hexdigest()}
