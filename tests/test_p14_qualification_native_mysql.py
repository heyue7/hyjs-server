from hyjs_managed_mysql import provision_case
"""Two actual Go realms + original frames + real HMAC qualification outbox/offline/cold, isolated four schemas."""
import fcntl,hashlib,hmac,http.client,json,os,re,secrets,selectors,subprocess,time,unittest
from pathlib import Path
import test_p14_cross_mysql as cross
from server.codec import BY_NAME
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P14_QUALIFICATION_NATIVE_MYSQL_TEST')=='1','explicit actual qualification two-realm native')
class P14QualificationNativeMySQLTests(unittest.TestCase):
 root_sql=classmethod(cross.P14CrossMySQLTests.root_sql.__func__)
 sql=cross.P14CrossMySQLTests.sql;setUp=cross.P14CrossMySQLTests.setUp;stop_all=cross.P14CrossMySQLTests.stop_all;send=cross.P14CrossMySQLTests.send;call=cross.P14CrossMySQLTests.call;login=cross.P14CrossMySQLTests.login;wait_sql=cross.P14CrossMySQLTests.wait_sql;tcp=cross.P14CrossMySQLTests.tcp;tcp_send=cross.P14CrossMySQLTests.tcp_send
 @classmethod
 def setUpClass(cls):
  cls.source=Path(os.environ['HYJS_P14_QUALIFICATION_FIXED_SOURCE']);cls.prebuilt=Path(os.environ['HYJS_P14_QUALIFICATION_PREBUILT']);assert cls.source.name.startswith('hyjs-p14-season-qualification-') and cls.prebuilt.name.startswith('hyjs-p14-qualification-prebuilt-')
  cls.meta=json.loads((cls.prebuilt/'manifest.json').read_text())
  for path,sha in cls.meta['files'].items():assert hashlib.sha256((cls.source/path).read_bytes()).hexdigest()==sha,path
  cls.binary=cls.prebuilt/'hyjs-native';assert hashlib.sha256(cls.binary.read_bytes()).hexdigest()==cls.meta['nativeSHA256']
  cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a+');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(lambda:(fcntl.flock(cls.lock,fcntl.LOCK_UN),cls.lock.close()))
  pool, primary = provision_case(cls, 'p13_qual_native1')
  cls.realm_connections = {1: primary, 2: pool.allocate('p13_qual_native2')}
  cls.realms = {zone: connection.database for zone, connection in cls.realm_connections.items()}
  cls.domain_connections = {zone: pool.allocate('p13_qual_domain' + str(zone)) for zone in [1, 2]}
  cls.domain = {zone: connection.database for zone, connection in cls.domain_connections.items()}
 def start(self,zone,fixed=None):
  env=dict(self.env,HYJS_MYSQL_DATABASE=self.realms[zone]);hp,tp=fixed or (0,0)
  process=subprocess.Popen([str(self.binary),'--server-id',str(zone),'--http-port',str(hp),'--tcp-port',str(tp),'--tables',str(self.source/'analysis/tables')],cwd=self.source/'hyjs-server',env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True);self.processes[zone]=process
  with selectors.DefaultSelector() as ready:
   ready.register(process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90));line=process.stdout.readline()
  match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(match,line.replace(self.test_password,'[redacted]'));self.ports[zone]=(int(match[1]),int(match[2]))
 def stop(self,zone):
  process=self.processes.pop(zone,None)
  if process is None:return
  began=time.monotonic()
  if process.poll() is None:process.terminate()
  try:process.wait(timeout=10)
  except subprocess.TimeoutExpired:process.kill();process.wait();self.fail('nine-worker native shutdown timeout')
  if process.stdout:process.stdout.close()
  self.assertEqual(process.returncode,0,'native nine-worker shutdown failed');self.shutdownTimes.append(round(time.monotonic()-began,3))
 def internal(self,zone,raw,key=None,signature=None):
  if signature is None:signature=hmac.new(key,raw,hashlib.sha256).hexdigest()
  connection=http.client.HTTPConnection('127.0.0.1',self.ports[zone][0],timeout=10)
  try:
   connection.request('POST','/internal/p14/qualification',body=raw,headers={'Content-Type':'application/json','X-HYJS-P14-Signature':signature});r=connection.getresponse();return r.status,json.loads(r.read())
  finally:connection.close()
 def profiles(self,zone,players):
  return [[json.loads(bytes.fromhex(x)) for x in self.sql(zone,'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(player.playerId)+';').strip().split('|')] for player in players]
 def test_actual_domain_and_native_offline_signed_certificate_original_frames_cold_and_shutdown(self):
  began=time.monotonic();self.shutdownTimes=[]
  env=dict(self.env,HYJS_P13_MYSQL_HOST='127.0.0.1:3306',HYJS_P13_MYSQL_USER=self.test_user,HYJS_P13_MYSQL_PASSWORD=self.test_password,HYJS_P13_MYSQL_DATABASE=self.domain[1],HYJS_P14_QUALIFICATION_FOREIGN_DATABASE=self.domain[2])
  r=subprocess.run(['rtk','proxy',str(self.prebuilt/'service-race.test'),'-test.run=^TestP14QualificationActual','-test.count=1','-test.timeout=120s'],cwd=self.source/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=150)
  self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
  self.start(1);self.start(2)
  uids={zone:['qual-native-'+str(zone)+'-'+secrets.token_hex(4) for _ in range(2)] for zone in [1,2]};players={zone:[self.login(zone,uid) for uid in uids[zone]] for zone in [1,2]}
  self.assertEqual([x.playerId for x in players[1]],[x.playerId for x in players[2]])
  for zone in [1,2]:
   for i,player in enumerate(players[zone]):self.call(zone,'CSPlayerCreate',{'playerName':'资格服'+str(zone)+'员'+str(i),'heroId':'hero_047'},player,2,'SCPlayerCreate')
   self.sql(zone,'UPDATE profiles SET player=JSON_SET(player,"$.level",70);UPDATE p10_scene_policies SET enabled=0;')
   for player in players[zone]:self.call(zone,'CSPlayer',{},player,3,'SCPlayer')
  now=int(time.time()*1000);base=now-1500;key=secrets.token_bytes(32)
  slots={1:(base,base+1000),2:(base+1000,now+12000),3:(now+12000,now+13000),4:(now+13000,now+300000)}
  for stage in range(5,20):slots[stage]=(now+300000+(stage-5)*1000,now+301000+(stage-5)*1000)
  values=','.join('(44,'+str(a)+','+str(stage)+','+str(b)+',"原阶段'+str(stage)+'","local isolated actual qualification calendar")' for stage,(a,b) in slots.items())
  for zone in [1,2]:
   self.sql(zone,'UPDATE p14_season_policies SET enabled=1,season_index=44,register_start='+str(base+1000)+',register_end='+str(now+12000)+';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES('+str(3-zone)+',1,"http://127.0.0.1:'+str(self.ports[3-zone][0])+'",UNHEX("'+key.hex()+'"));UPDATE p14_audition_match_policies SET enabled=1;')
   self.assertEqual(self.sql(zone,'SELECT enabled FROM p14_qualification_federation_policies;').strip(),'0')
   for player in players[zone]:self.call(zone,'CSKFEnterGame',{},player,400+player.playerId,'SCKFEnterGame')
   self.call(zone,'CSKFEnterFrightGame',{},players[zone][0],500,'SCKFEnterFrightGame')
  before={zone:self.profiles(zone,players[zone]) for zone in [1,2]}
  for zone in [1,2]:
   self.wait_sql(zone,'SELECT COUNT(*) FROM p14_audition_qualifications WHERE season_index=44;','1',timeout=35)
   self.assertEqual(self.sql(zone,'SELECT round_count FROM p14_audition_qualifications WHERE season_index=44;').strip(),'0');self.assertEqual(self.sql(zone,'SELECT COUNT(*) FROM p14_qualification_deliveries;').strip(),'0')
  fixed=dict(self.ports);self.stop(2)
  for zone in [1,2]:self.sql(zone,'UPDATE p14_qualification_federation_policies SET enabled=1;')
  self.wait_sql(1,'SELECT COUNT(*) FROM p14_qualification_deliveries WHERE confirmed=0 AND attempt_count>0;','1',timeout=20)
  payload1=bytes.fromhex(self.sql(1,'SELECT HEX(payload) FROM p14_qualification_deliveries;').strip());self.assertEqual(self.sql(2,'SELECT COUNT(*) FROM p14_foreign_qualifications;').strip(),'0')
  self.stop(1);self.start(1,fixed[1]);self.assertEqual(bytes.fromhex(self.sql(1,'SELECT HEX(payload) FROM p14_qualification_deliveries;').strip()),payload1)
  self.start(2,fixed[2])
  for zone in [1,2]:
   self.wait_sql(zone,'SELECT COUNT(*) FROM p14_qualification_deliveries WHERE confirmed=1;','1',timeout=25);self.wait_sql(zone,'SELECT COUNT(*) FROM p14_foreign_qualifications WHERE season_index=44 AND server_id='+str(3-zone)+';','1')
  payload2=bytes.fromhex(self.sql(2,'SELECT HEX(payload) FROM p14_qualification_deliveries;').strip())
  for zone,payload in [(1,payload2),(2,payload1)]:
   status,reply=self.internal(zone,payload,key);self.assertEqual((status,reply['code'],reply['data']['confirmed']),(200,0,True))
   obj=json.loads(payload)
   invalid=[(json.dumps(dict(obj,unknownField=1),separators=(',',':')).encode(),None),(payload+b'{}',None),(b' '*(1048577),None),(payload,'not-hex'),(payload,'00'),(json.dumps(dict(obj,source='wrong-source'),separators=(',',':')).encode(),None)]
   for raw,signature in invalid:
    _,reply=self.internal(zone,raw,key,signature);self.assertEqual(reply['code'],7001,'strict handler parameter boundary')
   self.assertEqual(self.sql(zone,'SELECT COUNT(*) FROM p14_foreign_qualifications;').strip(),'1')
   result=self.call(zone,'CSKuaFuMatchResult',{},players[zone][0],601,'SCKuaFuMatchResult');self.assertEqual((result.result,result.matchResult.winTimes,result.matchResult.failTimes),(1,0,0))
   sock=self.tcp(zone,players[zone][0]);self.tcp_send(zone,sock,'CSKFPlayerInfo',{'playerId':players[3-zone][0].playerId,'serverId':3-zone,'peakMatchIndex':44},players[zone][0]);op,frame=cross.guild.P13MySQLTests.recv(self,sock);self.assertEqual((op,frame.formationInfo.name),(BY_NAME['SCKFPlayerInfo'],'资格服'+str(3-zone)+'员0'))
   for name in ['CSKuafuMatchPlayer','CSKFRewardInfo']:
    op,_=self.send(zone,name,{},players[zone][0],602);self.assertEqual(op,10001)
  sources={zone:self.sql(zone,'SELECT payload_hash,boundary_hash,qualification_hash FROM p14_foreign_qualifications;') for zone in [1,2]}
  for zone in [1,2]:self.stop(zone)
  for zone in [1,2]:self.start(zone,fixed[zone])
  for zone in [1,2]:
   self.assertEqual(self.sql(zone,'SELECT payload_hash,boundary_hash,qualification_hash FROM p14_foreign_qualifications;'),sources[zone]);self.assertEqual(self.profiles(zone,players[zone]),before[zone]);self.assertEqual(self.sql(zone,'SELECT COUNT(*) FROM p14_audition_matches;').strip(),'0')
   self.assertEqual(self.sql(zone,'SELECT COUNT(*) FROM p14_receipts WHERE operation IN ("CSKFGuess","CSKFRewardInfo");').strip(),'0')
   self.stop(zone)
  report={'status':'PASS_ACTUAL_TWO_REALMS_ORIGINAL_FRAMES_QUALIFICATION_HTTP_OUTBOX_COLD','seconds':round(time.monotonic()-began,3),'actualSource':'two original authenticated level70 fixture players per actual realm, same localIDs stay separate, real original HTTP signup/upload→published cutoff→zero-roundauto qualification; no qualification/cutoff/foreign rows injected','transport':'independent two actual Go processes/eachInnoDB, default federationoff; destination offline persistent outbox attempt; source restart frozen exactpayload; destination restart true signedHTTPimport/confirmation; exactretry one certificate, all original combined server/player metadata preserved','boundaries':'unknownFields/trailingJSON/body1MiB+1/invalidhex/wrongSource all rejected; no duplicate records; original TCP foreignprofile and HTTP zero-win/loss autoResult, match/reward requests explicitclosed','cold':'same importedpayload/boundary/qualification hashes after bothrestart; fullPlayerGame for all4 unchanged; no imaginarymatch/score/winner/reward','shutdownSeconds':self.shutdownTimes,'workers':9,'sharedManifestSHA256':hashlib.sha256((ROOT/'analysis/p14/qualification-shared-v2-source-20261015/manifest.json').read_bytes()).hexdigest(),'nativeSHA256':self.meta['nativeSHA256'],'raceSHA256':self.meta['raceSHA256'],'productionModified':False,'phoneVerified':False,'cleanup':'wholelife sharedfilelock; four isolatedschemas/testuser/grants removed; temporary sources/binaries removed after exact report'}
  (ROOT/'analysis/p14/qualification-native-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()
