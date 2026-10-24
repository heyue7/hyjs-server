"""Borrow the existing QA allocation. Never owns/stops/releases its app or database."""
import concurrent.futures, http.client, json, os, secrets, sys, time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path[:0]=[str(ROOT),str(ROOT/'tests')]
from hyjs_managed_mysql import ManagedMySQL,ManagedConnection,SERVICES
from server.codec import BY_NAME,CLASSES,encode_http_request,decode_responses
PRIVATE=ROOT/'.local-env/feedback65';OUT=Path(__file__).parent
runtime=json.loads((PRIVATE/'runtime.json').read_text());database=runtime['database']
assert database.startswith('hyjs_test_feedback65_')
values=dict(line.split('=',1) for line in (SERVICES/'test-connections'/f'{database}.env').read_text().splitlines() if '=' in line)
assert values['MYSQL_DATABASE']==database and values['MYSQL_HOST']=='127.0.0.1' and values['MYSQL_PORT']=='3306'
connection=ManagedConnection(database,values['MYSQL_USER'],values['MYSQL_PASSWORD'])
pool=ManagedMySQL();pool.connections[database]=connection

def sql(text):return pool.sql(database,text)
def call(name,fields=None,login=None,stamp=None):
 stamp=stamp or int(time.time()*1000)
 raw=encode_http_request(BY_NAME[name],CLASSES[name](**(fields or {})),player_id=login.playerId if login else 0,zone_id=1,session=bytes(login.sessionKey) if login else b'0'*16,timestamp=stamp)
 c=http.client.HTTPConnection('127.0.0.1',28979,timeout=60);begin=time.perf_counter()
 try:
  c.request('POST','/sao/',raw,{'Content-Type':'application/octet-stream'});r=c.getresponse();data=r.read();assert r.status==200
  frames=decode_responses(data);return frames[0][1],round((time.perf_counter()-begin)*1000,3)
 finally:c.close()
def expected(name,fields,login,response,stamp=None):
 out,ms=call(name,fields,login,stamp)
 if out.DESCRIPTOR.name!=response:raise AssertionError(f'{name} returned {out.DESCRIPTOR.name}: '+str(out)[:300])
 return out,ms
def register():
 username='qa65-feedback-'+secrets.token_hex(5);password=secrets.token_hex(16)
 c=http.client.HTTPConnection('127.0.0.1',28979,timeout=20)
 try:
  c.request('POST','/game-account/register',json.dumps(dict(username=username,password=password,confirmPassword=password,nickname='隔离试炼验收')),{'Content-Type':'application/json','Origin':runtime['origin']});r=c.getresponse();body=json.loads(r.read());assert r.status==200 and body['code']==0
 finally:c.close()
 login,_=expected('CSUserLogin',dict(platformUid=username,authCode=password,clientTime=int(time.time()*1000)),None,'SCUserLogin')
 assert login.playerId>0
 expected('CSPlayerCreate',dict(playerName='延迟QA'+secrets.token_hex(2),heroId='hero_047'),login,'SCPlayerCreate')
 return dict(username=username,password=password,playerId=login.playerId),login
