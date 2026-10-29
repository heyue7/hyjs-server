"""Ephemeral loopback-only metadata proxy for the dedicated desert QA server.

Relays bytes unchanged. Logs only time, method, path without query, original
binary opcodes, HTTP status and elapsed time. Never records headers or bodies.
"""
import datetime,http.client,http.server,json,os,re,socket,threading,time,urllib.parse
from pathlib import Path

LISTEN=('127.0.0.1',18863)
UPSTREAM=('127.0.0.1',18861)
LOG=Path('/private/tmp/hyjs-desert-v4-qa/http-meta-18863.jsonl')
MAX_REQUEST=8*1024*1024
HOP={'connection','proxy-connection','keep-alive','proxy-authenticate','proxy-authorization','te','trailer','transfer-encoding','upgrade'}
lock=threading.Lock()
fd=os.open(str(LOG),os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
os.close(fd)

def record(method,path,opcode,status,duration,response_opcodes=None):
 row={'timeUtc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'method':method,'path':path,
      'requestOpcode':opcode,'status':status,'durationMs':round(duration,1)}
 if response_opcodes is not None:row['responseOpcodes']=response_opcodes
 line=(json.dumps(row,separators=(',',':'))+'\n').encode('utf-8')
 with lock:
  descriptor=os.open(str(LOG),os.O_WRONLY|os.O_APPEND)
  try:os.write(descriptor,line)
  finally:os.close(descriptor)

class Frames:
 def __init__(self):self.header=bytearray();self.skip=0;self.opcodes=[];self.failed=False
 def feed(self,part):
  i=0
  while i<len(part) and not self.failed:
   if self.skip:
    n=min(self.skip,len(part)-i);i+=n;self.skip-=n;continue
   n=min(8-len(self.header),len(part)-i);self.header.extend(part[i:i+n]);i+=n
   if len(self.header)<8:continue
   size=int.from_bytes(self.header[:4],'big');op=int.from_bytes(self.header[4:8],'big');self.header.clear()
   if size>4*1024*1024 or not 1<=op<=1000000 or len(self.opcodes)>=128:self.failed=True;continue
   self.opcodes.append(op);self.skip=size

class Server(http.server.ThreadingHTTPServer):
 daemon_threads=True
 allow_reuse_address=False

class Proxy(http.server.BaseHTTPRequestHandler):
 protocol_version='HTTP/1.1'
 def log_message(self,*args):pass
 def do_GET(self):self.forward()
 def do_POST(self):self.forward()
 def do_PUT(self):self.forward()
 def do_DELETE(self):self.forward()
 def do_PATCH(self):self.forward()
 def forward(self):
  start=time.monotonic();parsed=urllib.parse.urlsplit(self.path);route=parsed.path
  if len(route)>256 or not re.fullmatch(r'/[A-Za-z0-9_./-]*',route):route='<redacted>'
  method=self.command;opcode=None;status=0;response_codes=None;up=None;started=False
  try:
   if parsed.scheme or parsed.netloc or not self.path.startswith('/') or self.headers.get('Transfer-Encoding'):raise ValueError('unsupported request')
   raw_length=self.headers.get('Content-Length','0')
   if not raw_length.isdecimal():raise ValueError('invalid length')
   remaining=int(raw_length)
   if remaining>MAX_REQUEST:raise ValueError('oversized request')
   self.connection.settimeout(20)
   up=http.client.HTTPConnection(*UPSTREAM,timeout=25)
   up.putrequest(method,self.path,skip_host=True,skip_accept_encoding=True)
   for key,value in self.headers.items():
    if key.lower() not in HOP and key.lower() not in ('host','content-length'):
     up.putheader(key,value)
   up.putheader('Host','127.0.0.1:18861');up.putheader('Content-Length',str(remaining));up.putheader('Connection','close');up.endheaders()
   first=self.rfile.read(min(12,remaining)) if remaining else b''
   if len(first)!=min(12,remaining):raise EOFError('request truncated')
   if route=='/sao/' and len(first)>=12 and int.from_bytes(first[4:8],'big')==10000:opcode=int.from_bytes(first[8:12],'big')
   record(method,route,opcode,'pending',0)
   if first:up.send(first);remaining-=len(first)
   while remaining:
    part=self.rfile.read(min(65536,remaining))
    if not part:raise EOFError('request truncated')
    up.send(part);remaining-=len(part)
   response=up.getresponse();status=response.status
   self.send_response_only(status,response.reason)
   has_length=False
   for key,value in response.getheaders():
    lower=key.lower()
    if lower in HOP:continue
    if lower=='content-length':has_length=True
    self.send_header(key,value)
   if not has_length:self.send_header('Connection','close');self.close_connection=True
   self.end_headers();started=True
   frames=Frames() if route=='/sao/' else None
   while True:
    part=response.read(65536)
    if not part:break
    if frames is not None:frames.feed(part)
    self.wfile.write(part)
   self.wfile.flush()
   if frames is not None:response_codes=frames.opcodes
  except Exception:
   if not started:
    status=502
    try:self.send_response_only(502);self.send_header('Content-Length','0');self.send_header('Connection','close');self.end_headers()
    except Exception:pass
   self.close_connection=True
  finally:
   if up is not None:
    try:up.close()
    except Exception:pass
   record(method,route,opcode,status,(time.monotonic()-start)*1000,response_codes)

if __name__=='__main__':
 server=Server(LISTEN,Proxy)
 print(json.dumps({'listening':'127.0.0.1:18863','upstream':'127.0.0.1:18861','log':str(LOG),'metadataOnly':True}),flush=True)
 try:server.serve_forever(poll_interval=.25)
 except KeyboardInterrupt:pass
 finally:server.server_close()
