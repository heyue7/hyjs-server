"""Transparent root-owned isolated native P11 HTTP capture; sessions are never saved."""
import argparse,hashlib,http.client,json,re,sys
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from pathlib import Path
from google.protobuf.json_format import MessageToDict
ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT))
from server.codec import parse_http,decode_responses,OPCODES

def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--upstream',type=int,required=True)
 parser.add_argument('--output',type=Path,required=True)
 args=parser.parse_args()
 if not 1024<args.upstream<65536:parser.error('Explicit loopback QA port required')
 output=args.output.resolve()
 if not output.is_relative_to(ROOT/'analysis/p11'):parser.error('Owned QA evidence directory required')
 output.mkdir(parents=True,exist_ok=True)
 class Proxy(BaseHTTPRequestHandler):
  def log_message(self,*args):pass
  def do_POST(self):
   if self.path!='/sao/':self.send_error(404);return
   length=int(self.headers.get('Content-Length','0'))
   if not 0<length<=8388608:self.send_error(413);return
   raw=self.rfile.read(length)
   try: request=parse_http(raw)
   except ValueError:self.send_error(400);return
   name=OPCODES.get(request.opcode,'unknown')
   target=http.client.HTTPConnection('127.0.0.1',args.upstream,timeout=40)
   try:
    target.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
    response=target.getresponse();data=response.read();status=response.status
   except Exception:
    self.send_error(502);print(json.dumps({'request':name,'upstreamUnavailable':True}),flush=True);return
   finally:target.close()
   if name in ('CSGrab','CSGetGrabData','CSGetTimeRewardInfo','CSLimitPvpFindMatch','CSLimitPvpFightResult','CSLimitPvpRank','CSLimitPvpRewardInfo','CSGetLimitPvpRewardInfo','CSItemUse'):
    # Persist only decoded game messages. The frame includes session credentials and is discarded.
    body=request.decode();identity=str(getattr(body,'fightId','start'))
    if not re.fullmatch('[A-Za-z0-9_-]{1,128}',identity):identity='start'
    prefix=output/(identity+'-'+name+'-'+str(request.timestamp))
    payload=body.SerializeToString(deterministic=True)
    prefix.with_suffix('.pb').write_bytes(payload)
    frames=decode_responses(data) if status==200 else []
    assets=[]
    # Save decoded battle messages so later native comparison uses exact
    # protobuf bytes. The authenticated request/response frames stay in memory.
    for code,msg in frames:
     response_name=OPCODES.get(code,str(code))
     if response_name not in ('SCGrab','SCGetGrabData','SCGetTimeRewardInfo','SCLimitPvpFindMatch','SCLimitPvpFightResult','SCLimitPvpRank','SCLimitPvpRewardInfo','SCGetLimitPvpRewardInfo','SCItemUse','SCPlayer','ErrorMessage'):continue
     response_payload=msg.SerializeToString(deterministic=True)
     asset=prefix.with_name(prefix.name+'-'+response_name+'.pb')
     asset.write_bytes(response_payload)
     assets.append({'path':asset.name,'sha256':hashlib.sha256(response_payload).hexdigest()})
    audit={'request':name,'requestPlayerID':request.player_id,'timestamp':request.timestamp,'payloadSHA256':hashlib.sha256(payload).hexdigest(),'body':MessageToDict(body),'responseStatus':status,'responses':[{'name':OPCODES.get(code,str(code)),'body':MessageToDict(msg)} for code,msg in frames],'exactResponseAssets':assets}
    prefix.with_suffix('.json').write_text(json.dumps(audit,ensure_ascii=False,indent=2)+'\n')
   print(json.dumps({'request':name,'status':status,'bytes':len(data)}),flush=True)
   self.send_response(status);self.send_header('Content-Type','application/octet-stream');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
 with ThreadingHTTPServer(('127.0.0.1',0),Proxy) as server:
  print(json.dumps({'ready':True,'listenPort':server.server_port,'upstream':args.upstream,'sessionsCaptured':False}),flush=True)
  try:server.serve_forever(poll_interval=.5)
  except KeyboardInterrupt:pass

if __name__=='__main__':main()
