from hyjs_managed_mysql import provision_case
"""Real v81→current P14 immutable source/engine audit, original HTTP/TCP, no formal DB or winner/award."""
import fcntl,hashlib,json,os,re,secrets,selectors,subprocess,time,unittest
from pathlib import Path
import test_p14_cross_mysql as cross
from server.codec import BY_NAME
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P14_ENGINE_UPGRADE_MYSQL_TEST')=='1','explicit isolated two-engine actual upgrade')
class P14EngineUpgradeMySQLTests(unittest.TestCase):
 root_sql=classmethod(cross.P14CrossMySQLTests.root_sql.__func__)
 sql=cross.P14CrossMySQLTests.sql;setUp=cross.P14CrossMySQLTests.setUp;stop=cross.P14CrossMySQLTests.stop;stop_all=cross.P14CrossMySQLTests.stop_all;send=cross.P14CrossMySQLTests.send;call=cross.P14CrossMySQLTests.call;login=cross.P14CrossMySQLTests.login;wait_sql=cross.P14CrossMySQLTests.wait_sql;tcp=cross.P14CrossMySQLTests.tcp;tcp_send=cross.P14CrossMySQLTests.tcp_send
 @classmethod
 def setUpClass(cls):
  cls.prebuilt=Path(os.environ['HYJS_P14_ENGINE_UPGRADE_PREBUILT']);assert cls.prebuilt.name.startswith('hyjs-p14-upgrade-prebuilt-')
  cls.meta=json.loads((cls.prebuilt/'manifest.json').read_text())
  for path,sha in cls.meta['files'].items():assert hashlib.sha256((cls.prebuilt/path).read_bytes()).hexdigest()==sha,path
  for name,sha in cls.meta['binaries'].items():assert hashlib.sha256((cls.prebuilt/name).read_bytes()).hexdigest()==sha,name
  cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a+');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(lambda:(fcntl.flock(cls.lock,fcntl.LOCK_UN),cls.lock.close()))
  pool, primary = provision_case(cls, 'p13_upgrade')
  cls.realms = {1: primary.database}
  cls.realm_connections = {1: primary}
 def start(self):
  process=subprocess.Popen(['rtk','proxy',str(self.prebuilt/'current-native'),'--server-id','1','--http-port','0','--tcp-port','0','--tables',str(self.prebuilt/'current/analysis/tables')],cwd=self.prebuilt/'current/hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True);self.processes[1]=process
  with selectors.DefaultSelector() as ready:
   ready.register(process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90));line=process.stdout.readline()
  match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(match,line.replace(self.test_password,'[redacted]'));self.ports[1]=(int(match[1]),int(match[2]))
 def run_role(self,role):
  env=dict(self.env,HYJS_P14_ENGINE_UPGRADE_ROLE=role)
  result=subprocess.run(['rtk','proxy',str(self.prebuilt/(role+'-race.test')),'-test.run=^TestP14EngineUpgrade'+('LegacyProducer' if role=='legacy' else 'CurrentConsumer')+'$','-test.count=1','-test.timeout=150s'],cwd=self.prebuilt/role/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=180)
  self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(role,result.stdout.strip())
 def test_actual_engine_upgrade_old_receipts_current_signup_original_protocol_cold_and_gates(self):
  began=time.monotonic();self.run_role('legacy')
  old=self.sql(1,'SELECT id,input_hash,outcome_hash,replay_hash,status,engine_version FROM p6_battle_records ORDER BY id;')
  self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p6_battle_records WHERE status="computed";').strip(),'1');self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p6_battle_records WHERE status="captured";').strip(),'4')
  self.run_role('current');self.assertEqual(self.sql(1,'SELECT id,input_hash,outcome_hash,replay_hash,status,engine_version FROM p6_battle_records ORDER BY id;'),old)
  self.start();self.sql(1,'UPDATE p10_scene_policies SET enabled=0;UPDATE p14_season_policies SET enabled=0;')
  uid='upgrade-native-'+secrets.token_hex(4);player=self.login(1,uid);self.call(1,'CSPlayerCreate',{'playerName':'新引擎报名','heroId':'hero_047'},player,1,'SCPlayerCreate')
  self.sql(1,'UPDATE profiles SET player=JSON_SET(player,"$.level",70) WHERE id='+str(player.playerId)+';')
  now=int(time.time()*1000);base=now-1500
  slots={1:(base,base+1000),2:(base+1000,now+6000),3:(now+6000,now+7000),4:(now+7000,now+180000)}
  for stage in range(5,20):slots[stage]=(now+180000+(stage-5)*1000,now+181000+(stage-5)*1000)
  values=','.join('(88,'+str(a)+','+str(stage)+','+str(b)+',"原阶段'+str(stage)+'","local isolated published calendar")' for stage,(a,b) in slots.items())
  self.sql(1,'UPDATE p14_season_policies SET enabled=1,season_index=88,register_start='+str(base+1000)+',register_end='+str(now+6000)+';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+';')
  self.call(1,'CSPlayer',{},player,699,'SCPlayer')
  def full_profile():return [json.loads(bytes.fromhex(x)) for x in self.sql(1,'SELECT CONCAT(HEX(player),\"|\",HEX(game)) FROM profiles WHERE id='+str(player.playerId)+';').strip().split('|')]
  before=full_profile()
  self.call(1,'CSKFEnterGame',{},player,700,'SCKFEnterGame');sock=self.tcp(1,player);self.tcp_send(1,sock,'CSKFLogin',{},player);op,registered=cross.guild.P13MySQLTests.recv(self,sock);self.assertEqual((op,registered.isEnter),(BY_NAME['SCKFLogin'],1))
  self.call(1,'CSKFEnterFrightGame',{},player,701,'SCKFEnterFrightGame')
  engine=self.sql(1,'SELECT engine_version FROM p14_members WHERE season_index=88 AND server_id=1;').strip();self.assertEqual(engine,'apk-10056262-p6-v91')
  self.wait_sql(1,'SELECT COUNT(*) FROM p14_audition_qualifications WHERE season_index=88;','1',timeout=25)
  result=self.call(1,'CSKuaFuMatchResult',{},player,702,'SCKuaFuMatchResult');self.assertEqual((result.result,result.matchResult.winTimes,result.matchResult.failTimes),(1,0,0))
  for name,fields in [('CSKuafuMatchPlayer',{}),('CSKFRewardInfo',{}),('CSKFGuessList',{})]:op,_=self.send(1,name,fields,player,703);self.assertEqual(op,10001)
  snapshot=self.sql(1,'SELECT source_hash FROM p14_audition_boundaries WHERE season_index=88;')
  self.stop(1);self.start();player=self.login(1,uid);self.assertEqual(self.call(1,'CSKuaFuMatchResult',{},player,704,'SCKuaFuMatchResult'),result)
  self.assertEqual(self.sql(1,'SELECT source_hash FROM p14_audition_boundaries WHERE season_index=88;'),snapshot)
  self.assertEqual(full_profile(),before)
  self.assertEqual(self.sql(1,'SELECT id,input_hash,outcome_hash,replay_hash,status,engine_version FROM p6_battle_records ORDER BY id;'),old)
  self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p14_audition_matches WHERE season_index=88;').strip(),'0')
  report={'status':'PASS_REAL_81_TO_91_SOURCE_AND_ORIGINAL_PROTOCOL','seconds':round(time.monotonic()-began,3),'legacyEngine':'apk-10056262-p6-v81','currentEngine':engine,'legacySource':self.meta['legacyBaseline'],'currentSource':self.meta['currentBaseline'],'actualLegacy':'11 original players registered and frozen, cryptographic five pairs, one actually computed with old complete engine, four captured; no version-field mutation','upgrade':'current actual race build cold reads old computed/replay and rejects four old captured inputs; full input/outcome/replay hashes/status/version unchanged','native':'actual original HTTP signup/explicit upload and TCP registered login query, current91 snapshot, real single-member autoPass0wins0losses/cold; actual battle/guess/reward requests refuse without authority, complete new actor Player/Game unchanged','limitations':'no authoritative winner/damage/loss/reward; one-player original autoPass is not a played battle; original phone scope pending root','cleanup':'whole-lifecycle isolated MySQL filelock, schema/user/grants cleaned; two disposable source/build directories removed after evidence capture','binarySHA256':self.meta['binaries']}
  (ROOT/'analysis/p14/engine-upgrade-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()
