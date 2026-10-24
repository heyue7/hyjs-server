"""Bounded DEX relocation, preserving all original indexes and non-clinit bytecode/EH."""
import hashlib
import struct
import zlib
from loguru import logger
logger.remove()
from androguard.core.dex import DEX
from tools.client_compat63.axml import require
OLD_SHA='cfddb4d1b6ee15bfc8d51da311bfa7e6e1408c809c4ccd8b1e47049a85b45254'
OWNER='Lcom/hyjs/wallet/LocalAssetOverlay;'

def u16(b,p): return struct.unpack_from('<H',b,p)[0]
def u32(b,p): return struct.unpack_from('<I',b,p)[0]
def leb(b,p,signed=False):
    start=p; value=shift=0
    while True:
        require(p<len(b) and p-start<5,'Invalid LEB'); c=b[p];p+=1;value|=(c&127)<<shift;shift+=7
        if not c&128: break
    if signed and c&64:value-=1<<shift
    return value,p

def enc(value):
    output=bytearray()
    while True:
        c=value&127;value>>=7;output.append(c|(128 if value else 0))
        if not value:return bytes(output)

def layout(raw):
    require(raw[:8]==b'dex\n035\0' and u32(raw,32)==len(raw) and u32(raw,36)==112,'Unexpected DEX header')
    require(u32(raw,40)==0x12345678 and u32(raw,44)==u32(raw,48)==0,'Unsupported endian/link')
    at=u32(raw,52);count=u32(raw,at); sections=[]
    for i in range(count):
        kind,zero,n,offset=struct.unpack_from('<HHII',raw,at+4+i*12)
        require(zero==0 and n>0,'Bad map item');sections.append((kind,n,offset))
    require(len({x[0] for x in sections})==len(sections) and [x[2] for x in sections]==sorted(x[2] for x in sections),'Map overlap/order')
    offsets=[]; targets=[]; class_codes=[]; codes={}; debug={}; spans=[]; item_starts={}
    fixed={0:112,1:4,2:4,3:12,4:8,5:8,6:32}
    def ev(p):
        head=raw[p];p+=1;kind=head&31;arg=head>>5
        if kind==0x1c:
            require(arg==0,'Array arg');n,p=leb(raw,p)
            for _ in range(n):p=ev(p)
        elif kind==0x1d:
            require(arg==0,'Annotation arg');p=annotation(p)
        elif kind in (0x1e,0x1f):require(arg==0 if kind==0x1e else arg<=1,'Null/bool arg')
        else:
            require(kind in (0,2,3,4,6,0x10,0x11,0x15,0x16,0x17,0x18,0x19,0x1a,0x1b),'Unknown encoded value');p+=arg+1
        return p
    def annotation(p):
        _,p=leb(raw,p);n,p=leb(raw,p)
        for _ in range(n):_,p=leb(raw,p);p=ev(p)
        return p
    for si,(kind,n,start) in enumerate(sections):
        end=sections[si+1][2] if si+1<len(sections) else len(raw);p=start;starts=[]
        for _ in range(n):
            if kind in (0x2001,0x1001,0x1002,0x1003,0x2006,0x1000):
                aligned=(p+3)&~3;require(not any(raw[p:aligned]),'Nonzero alignment');p=aligned
            pos=p;starts.append(pos)
            if kind in fixed:
                p+=fixed[kind]
                if kind==1:offsets.append(pos);targets.append((pos,0x2002))
                elif kind==3:offsets.append(pos+8);targets.append((pos+8,0x1001))
                elif kind==6:
                    offsets.extend(pos+x for x in (12,20,24,28));targets.extend((pos+x,k) for x,k in [(12,0x1001),(20,0x2006),(24,0x2000),(28,0x2005)])
            elif kind==0x2001:
                tries=u16(raw,p+6);debugoff=u32(raw,p+8);insns=u32(raw,p+12)
                offsets.append(p+8);targets.append((p+8,0x2003));begin=p+16;p=begin+insns*2;eh_start=p
                if tries:
                    if insns&1:p+=2
                    p+=tries*8;handlers,p=leb(raw,p)
                    for _ in range(handlers):
                        size,p=leb(raw,p,True)
                        for _ in range(abs(size)):_,p=leb(raw,p);_,p=leb(raw,p)
                        if size<=0:_,p=leb(raw,p)
                codes[pos]={'rawInstructions':raw[begin:begin+insns*2],'rawEH':raw[eh_start:p],'debugOff':debugoff,'end':p}
            elif kind==0x2003:
                _,p=leb(raw,p);params,p=leb(raw,p)
                for _ in range(params):_,p=leb(raw,p)
                while True:
                    op=raw[p];p+=1
                    if op==0:break
                    args={1:1,2:1,3:3,4:4,5:1,6:1,7:0,8:0,9:1}.get(op,0)
                    for _ in range(args):_,p=leb(raw,p,op==2)
                debug[pos]=raw[pos:p]
            elif kind==0x1001:p+=4+u32(raw,p)*2
            elif kind==0x2002:
                _,p=leb(raw,p);p=raw.index(0,p)+1
            elif kind==0x2004:p=annotation(p+1)
            elif kind==0x2000:
                counts=[]
                for _ in range(4):v,p=leb(raw,p);counts.append(v)
                for _ in range(counts[0]+counts[1]):_,p=leb(raw,p);_,p=leb(raw,p)
                for group in counts[2:]:
                    method=0
                    for _ in range(group):
                        diff,p=leb(raw,p);method+=diff;_,p=leb(raw,p);location=p;code,p=leb(raw,p)
                        class_codes.append((location,p,method,code))
            elif kind==0x2005:
                count,p=leb(raw,p)
                for _ in range(count):p=ev(p)
            elif kind in (0x1002,0x1003):
                count=u32(raw,p);offsets.extend(p+4+i*4 for i in range(count));targets.extend((p+4+i*4,0x1003 if kind==0x1002 else 0x2004) for i in range(count));p+=4+count*4
            elif kind==0x2006:
                offsets.append(p);targets.append((p,0x1003));a,b,c=struct.unpack_from('<3I',raw,p+4);p+=16
                for j in range(a+b+c):offsets.append(p+4);targets.append((p+4,0x1003 if j<a+b else 0x1002));p+=8
            elif kind==0x1000:p+=4+u32(raw,p)*12
            else:raise ValueError('Unsupported actual map section '+hex(kind))
            require(pos<=p<=end,'Section item outside span '+hex(kind))
        require(end-p<=3 and not any(raw[p:end]),'Unparsed section bytes '+hex(kind))
        spans.append({'kind':hex(kind),'count':n,'start':start,'parsedEnd':p,'spanEnd':end});item_starts[kind]=set(starts)
    # Validate actual absolute-offset targets by section kind, not merely file bounds.
    for pos in offsets:
        val=u32(raw,pos);require(val==0 or 0<val<len(raw),'Offset outside file')
    for location,kind in targets:
        value=u32(raw,location);require(not value or value in item_starts[kind],'Offset does not point to expected item kind '+hex(kind))
    for pos,stop,method,code in class_codes:require(code==0 or code in codes,'Method code_off not code item')
    for value in codes.values():require(not value['debugOff'] or value['debugOff'] in debug,'Debug offset not item')
    return {'sections':sections,'offsetFields':offsets,'classCodes':class_codes,'codes':codes,'debug':debug,'spans':spans,'starts':item_starts}

def methods(dex):
    return {(c.get_name(),m.get_name(),m.get_descriptor()):m for c in dex.get_classes() for m in c.get_methods()}

def extract_pins(raw):
    """Execute this concrete, branch-free patched clinit's supported Dalvik instructions."""
    d=DEX(raw);m=methods(d)[(OWNER,'<clinit>','()V')]; code=m.get_code().get_bc().get_raw(); regs=[None]*m.get_code().get_registers_size();regtypes=[None]*len(regs);pc=0;result=None
    def reference(index):return isinstance(regtypes[index],str) and regtypes[index].startswith(('L','[','uninit:'))
    def integer(index):require(regtypes[index]=='I','Expected primitive integer register')
    while pc<len(code):
        op=code[pc];b=code[pc+1]
        if op==0x12:
            value=b>>4;regs[b&15]=value-16 if value&8 else value;regtypes[b&15]='I';pc+=2
        elif op==0x13:regs[b]=struct.unpack_from('<h',code,pc+2)[0];regtypes[b]='I';pc+=4
        elif op==0x16:regs[b]=struct.unpack_from('<h',code,pc+2)[0];regtypes[b]='J-low';regtypes[b+1]='J-high';pc+=4
        elif op==0x17:regs[b]=struct.unpack_from('<i',code,pc+2)[0];regtypes[b]='J-low';regtypes[b+1]='J-high';pc+=6
        elif op==0x1a:regs[b]=d.get_strings()[u16(code,pc+2)];regtypes[b]='Ljava/lang/String;';pc+=4
        elif op==0x22:regs[b]={'class':d.get_cm_type(u16(code,pc+2))};regtypes[b]='uninit:'+regs[b]['class'];pc+=4
        elif op==0x23:
            dst,src=b&15,b>>4;integer(src);kind=d.get_cm_type(u16(code,pc+2));regs[dst]=bytearray(regs[src]) if kind=='[B' else [None]*regs[src];regtypes[dst]=kind;pc+=4
        elif op==0x26:
            require(regtypes[b]=='[B','fill-array-data must target byte[]');payload=pc+struct.unpack_from('<i',code,pc+2)[0]*2
            require(u16(code,payload)==0x300 and u16(code,payload+2)==1,'Unexpected array payload')
            size=u32(code,payload+4);regs[b][:]=code[payload+8:payload+8+size];pc+=6
        elif op==0x01:
            dst,src=b&15,b>>4
            require(not reference(src) and regtypes[src] in ('I','F'),'Primitive move of Reference/wide/undefined register forbidden')
            regs[dst]=regs[src];regtypes[dst]=regtypes[src];pc+=2
        elif op==0x07:
            dst,src=b&15,b>>4;require(reference(src),'move-object source is not Reference')
            regs[dst]=regs[src];regtypes[dst]=regtypes[src];pc+=2
        elif op==0x04:
            dst,src=b&15,b>>4;require(regtypes[src:src+2]==['J-low','J-high'],'move-wide source is not wide pair')
            regs[dst:dst+2]=regs[src:src+2];regtypes[dst:dst+2]=regtypes[src:src+2];pc+=2
        elif op==0x70:
            method=d.get_cm_method(u16(code,pc+2));packed=u16(code,pc+4);r=[packed&15,(packed>>4)&15,(packed>>8)&15,(packed>>12)&15,b&15][:b>>4]
            if method[0]=='Ljava/lang/String;' and method[1]=='<init>':
                require(method[2]==['([B Ljava/lang/String;)','V'],'Wrong ctor');require([regtypes[x] for x in r]==['uninit:Ljava/lang/String;','[B','Ljava/lang/String;'],'String ctor register types differ');regs[r[0]]['text']=bytes(regs[r[1]]).decode(regs[r[2]]);regtypes[r[0]]='Ljava/lang/String;'
            elif method[0]==OWNER[:-1]+'$Pin;' and method[1]=='<init>':
                require([regtypes[x] for x in r]==['uninit:'+OWNER[:-1]+'$Pin;','Ljava/lang/String;','Ljava/lang/String;','J-low','J-high'],'Pin ctor register types differ');regtypes[r[0]]=OWNER[:-1]+'$Pin;';value=regs[r[2]];regs[r[0]].update(path=regs[r[1]],sha256=value['text'] if isinstance(value,dict) else value,bytes=regs[r[3]])
            else:raise ValueError('Unexpected actual clinit invoke '+str(method))
            pc+=6
        elif op==0x4d:
            require(reference(b) and regtypes[code[pc+2]]=='['+OWNER[:-1]+'$Pin;','aput-object reference types differ');integer(code[pc+3]);regs[code[pc+2]][regs[code[pc+3]]]=regs[b];pc+=4
        elif op==0x69:require(regtypes[b]=='['+OWNER[:-1]+'$Pin;','sput-object Pin[] type differs');result=regs[b];pc+=4
        elif op==0x0e:break
        else:raise ValueError('Unexpected actual clinit opcode '+hex(op))
    require(result is not None and len(result)==6,'Pin array missing')
    return [{k:row[k] for k in ('path','sha256','bytes')} for row in result]

def typed_negative_tests(raw):
    """Regression cases tied to the actual generated clinit, including the ART r8 failure."""
    d=DEX(raw);m=methods(d)[(OWNER,'<clinit>','()V')];pc=0;locations=[]
    for instruction in m.get_instructions():
        if instruction.get_name()=='move-object':locations.append(m.get_code_off()+16+pc)
        pc+=instruction.get_length()
    require(len(locations)==1,'Expected one generated object move')
    location=locations[0];require(raw[location:location+2]==bytes([7,0x62]),'Generated move shape differs')
    cases=[('primitive-move-reference',1,0x62,'Primitive move of Reference'),
        ('wide-move-reference',4,0x62,'move-wide source is not wide pair'),
        ('object-move-wide',7,0x32,'move-object source is not Reference')]
    results=[]
    for name,opcode,registers,message in cases:
        invalid=bytearray(raw);invalid[location:location+2]=bytes([opcode,registers])
        invalid[12:32]=hashlib.sha1(invalid[32:]).digest();struct.pack_into('<I',invalid,8,zlib.adler32(invalid[12:])&0xffffffff)
        try:extract_pins(bytes(invalid))
        except ValueError as error:require(message in str(error),'Wrong typed negative rejection');results.append({'case':name,'rejected':True})
        else:raise ValueError('Typed negative accepted '+name)
    return results

def patch(raw,new_sha,new_size):
    before=layout(raw);d=DEX(raw);all_methods=methods(d);target=all_methods[(OWNER,'<clinit>','()V')];old_off=target.get_code_off()
    old=before['codes'][old_off];require(u16(raw,old_off+6)==0,'clinit try handlers unsupported')
    ctor=[i for i in range(len(d.get_methods())) if d.get_cm_method(i)==['Ljava/lang/String;','<init>',['([B Ljava/lang/String;)','V']]]
    require(len(ctor)==1,'Existing byte[] String ctor required')
    strings=list(d.get_strings());old_string=strings.index(OLD_SHA);utf8=strings.index('UTF-8')
    types=[d.get_cm_type(i) for i in range(u32(raw,64))]
    byte_type=types.index('[B');string_type=types.index('Ljava/lang/String;')
    ins=old['rawInstructions'];needle=struct.pack('<BBH',0x1a,2,old_string);require(ins.count(needle)==1,'One hash instruction required')
    pos=ins.index(needle)
    replacement=struct.pack('<BBh',0x13,2,64)+struct.pack('<BBH',0x23,0x22,byte_type)
    fill_pos=pos+len(replacement);replacement+=struct.pack('<BBi',0x26,2,0)
    replacement+=struct.pack('<BBH',0x22,6,string_type)+struct.pack('<BBH',0x1a,7,utf8)
    replacement+=struct.pack('<BBHH',0x70,0x30,ctor[0],0x0726)+bytes([0x07,0x62])
    body=bytearray(ins[:pos]+replacement+ins[pos+4:])
    size_needle=bytes([0x17,3])+struct.pack('<i',563672)
    require(body.count(size_needle)==1,'One manifest size instruction required')
    at=body.index(size_needle);body[at+2:at+6]=struct.pack('<i',new_size)
    body+=b'\0'*(-len(body)%4);payload_at=len(body)
    require(len(new_sha)==64 and new_sha.isascii(),'SHA shape')
    body+=struct.pack('<HHI',0x300,1,64)+new_sha.encode('ascii')
    struct.pack_into('<i',body,fill_pos+2,(payload_at-fill_pos)//2)
    header=bytearray(raw[old_off:old_off+16]);struct.pack_into('<H',header,0,8);struct.pack_into('<I',header,8,0);struct.pack_into('<I',header,12,len(body)//2)
    newcode=bytes(header)+body
    section_index=next(i for i,x in enumerate(before['sections']) if x[0]==0x2001)
    insert=before['sections'][section_index+1][2];require(insert%4==0 and len(newcode)%4==0,'Code alignment')
    delta=len(newcode);output=bytearray(raw[:insert]+newcode+raw[insert:])
    def move(off):return off+delta if off>=insert else off
    def write32(old_location,value):struct.pack_into('<I',output,move(old_location),value)
    for location in before['offsetFields']:
        value=u32(raw,location);write32(location,move(value) if value else 0)
    for start,stop,method,code in before['classCodes']:
        updated=insert if code==old_off else move(code) if code else 0
        value=enc(updated)
        # Reject unsupported second relocation before producing an artifact; never assume widths.
        require(len(value)==stop-start,'ULEB width change requires a second relocation; safely rejected')
        output[move(start):move(stop)]=value
    mo=u32(raw,52)
    for i,(kind,count,off) in enumerate(before['sections']):
        write32(mo+4+i*12+4,count+(kind==0x2001));write32(mo+4+i*12+8,move(off))
    for location in (48,52,60,68,76,84,92,100,108):
        value=u32(raw,location);struct.pack_into('<I',output,location,move(value) if value else 0)
    struct.pack_into('<I',output,32,len(output));struct.pack_into('<I',output,104,u32(raw,104)+delta)
    output[12:32]=hashlib.sha1(output[32:]).digest();struct.pack_into('<I',output,8,zlib.adler32(output[12:])&0xffffffff)
    output=bytes(output);after=layout(output);a=DEX(output);new_methods=methods(a)
    require(list(all_methods)==list(new_methods),'Method identities/order changed')
    unchanged=0
    for key,m in all_methods.items():
        other=new_methods[key];require(m.get_access_flags()==other.get_access_flags(),'Method flags changed')
        if key==(OWNER,'<clinit>','()V'):continue
        if not m.get_code_off():require(not other.get_code_off(),'Abstract changed');continue
        x=before['codes'][m.get_code_off()];y=after['codes'][other.get_code_off()]
        require(x['rawInstructions']==y['rawInstructions'] and x['rawEH']==y['rawEH'],'Non-clinit bytecode/EH changed '+str(key))
        require(before['debug'].get(x['debugOff'])==after['debug'].get(y['debugOff']),'Debug contents changed');unchanged+=1
    for kind in (1,2,3,4,5,6):
        bsec=next(x for x in before['sections'] if x[0]==kind);asec=next(x for x in after['sections'] if x[0]==kind)
        require(bsec==asec,'Index table moved')
    require(list(d.get_strings())==list(a.get_strings()),'String index/content changed')
    require([d.get_cm_type(i) for i in range(u32(raw,64))]==[a.get_cm_type(i) for i in range(u32(output,64))],'Type refs changed')
    require([d.get_cm_method(i) for i in range(len(d.get_methods()))]==[a.get_cm_method(i) for i in range(len(a.get_methods()))],'Method refs changed')
    # Every original byte is identical except explicit absolute-offset fields, clinit code_off, header checksums.
    inherited=bytearray(output[:insert]+output[insert+delta:]);allowed=set(range(8,36))|set(range(52,56))|set(range(104,108))
    for location in before['offsetFields']:allowed.update(range(location,location+4))
    for start,stop,method,code in before['classCodes']:allowed.update(range(start,stop))
    for i in range(len(before['sections'])):allowed.update(range(mo+4+i*12+4,mo+4+i*12+12))
    require(all(x==y or i in allowed for i,(x,y) in enumerate(zip(raw,inherited))),'Unapproved original byte changed')
    oldpins=extract_pins(raw);pins=extract_pins(output);expected=[dict(row) for row in oldpins];expected[5].update(sha256=new_sha,bytes=new_size)
    require(pins==expected,'Actual patched DEX Pin values changed outside allowed entry')
    return output,{'insertionOffset':insert,'singleInsertionDelta':delta,'oldClinitCodeOff':old_off,'newClinitCodeOff':insert,
        'nonClinitMethodsInstructionsEHAndDebugExact':unchanged,'allOriginalIndexesAndReferencesExact':True,
        'allClassAPIFlagsAndOtherOriginalBytesExactOutsideRelocatedOffsetFields':True,'ulebWidthsVerifiedUnchanged':True,
        'beforeSections':before['spans'],'afterSections':after['spans'],'actualPatchedDEXInitializedPins':pins,
        'oldClinitUnreferencedCodeRetained':True,'stringIdsSortedAndByteExact':True,'actualClinitRegisterTypesChecked':True,'typedMoveNegatives':typed_negative_tests(output),
        'generatedClinitMoves':[{'opcode':i.get_name(),'operands':i.get_output()} for i in new_methods[(OWNER,'<clinit>','()V')].get_instructions() if i.get_name().startswith('move')]}
