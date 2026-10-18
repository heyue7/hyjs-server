from hyjs_managed_mysql import provision_case
"""Original phase7 eligibility/points from real signup→auto qualification, actual InnoDB/race/cold; no battle success."""
import datetime as dt,fcntl,hashlib,json,os,secrets,subprocess,time,unittest
from pathlib import Path
import test_p14_qualification_native_mysql as qual
from server.codec import BY_NAME
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P14_PROMOTION_MYSQL_TEST')=='1','explicit original promotion isolated integration')
class P14PromotionMySQLTests(unittest.TestCase):
 root_sql=classmethod(qual.P14QualificationNativeMySQLTests.root_sql.__func__)
 sql=qual.P14QualificationNativeMySQLTests.sql;setUp=qual.P14QualificationNativeMySQLTests.setUp;stop_all=qual.P14QualificationNativeMySQLTests.stop_all;send=qual.P14QualificationNativeMySQLTests.send;call=qual.P14QualificationNativeMySQLTests.call;login=qual.P14QualificationNativeMySQLTests.login;wait_sql=qual.P14QualificationNativeMySQLTests.wait_sql;tcp=qual.P14QualificationNativeMySQLTests.tcp;tcp_send=qual.P14QualificationNativeMySQLTests.tcp_send;start=qual.P14QualificationNativeMySQLTests.start;stop=qual.P14QualificationNativeMySQLTests.stop;profiles=qual.P14QualificationNativeMySQLTests.profiles
 @classmethod
 def setUpClass(cls):
  cls.source=Path(os.environ['HYJS_P14_PROMOTION_FIXED_SOURCE']);cls.prebuilt=Path(os.environ['HYJS_P14_PROMOTION_PREBUILT']);assert cls.source.name.startswith('hyjs-p14-season-qualification-promotion-') and cls.prebuilt.name.startswith('hyjs-p14-qualification-prebuilt-')
  cls.meta=json.loads((cls.prebuilt/'manifest.json').read_text())
  for path,sha in cls.meta['files'].items():assert hashlib.sha256((cls.source/path).read_bytes()).hexdigest()==sha,path
  cls.binary=cls.prebuilt/'hyjs-native';assert hashlib.sha256(cls.binary.read_bytes()).hexdigest()==cls.meta['nativeSHA256']
  cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a+');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(lambda:(fcntl.flock(cls.lock,fcntl.LOCK_UN),cls.lock.close()))
  pool, primary = provision_case(cls, 'p13_promotion_native')
  cls.realms = {1: primary.database}
  cls.realm_connections = {1: primary}
  cls.domain_connection = pool.allocate('p13_promotion_domain')
  cls.domain = cls.domain_connection.database
 def test_actual_phase7_original_query_qualified_ledger_concurrent_noon_and_cold(self):
  began=time.monotonic();self.shutdownTimes=[]
  env=dict(self.env,HYJS_P13_MYSQL_HOST='127.0.0.1:3306',HYJS_P13_MYSQL_USER=self.test_user,HYJS_P13_MYSQL_PASSWORD=self.test_password,HYJS_P13_MYSQL_DATABASE=self.domain)
  r=subprocess.run(['rtk','proxy',str(self.prebuilt/'service-race.test'),'-test.run=^TestP14Promotion','-test.count=1','-test.timeout=120s'],cwd=self.source/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=150)
  self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
  self.start(1);players=[self.login(1,'promotion-native-'+secrets.token_hex(4)) for _ in range(3)]
  for i,player in enumerate(players):self.call(1,'CSPlayerCreate',{'playerName':'原晋级员'+str(i),'heroId':'hero_047'},player,2,'SCPlayerCreate')
  self.sql(1,'UPDATE profiles SET player=JSON_SET(player,"$.level",70);UPDATE p10_scene_policies SET enabled=0;')
  for player in players:self.call(1,'CSPlayer',{},player,3,'SCPlayer')
  now=int(time.time()*1000);base=now-1500
  slots={1:(base,base+1000),2:(base+1000,now+8000),3:(now+8000,now+9000),4:(now+9000,now+22000),5:(now+22000,now+23000),6:(now+23000,now+24000),7:(now+24000,now+24000+3*86400000)}
  for stage in range(8,20):a=slots[7][1]+(stage-8)*1000;slots[stage]=(a,a+1000)
  values=','.join('(55,'+str(a)+','+str(stage)+','+str(b)+',"原阶段'+str(stage)+'","local actual promotion test published calendar")' for stage,(a,b) in slots.items());key=secrets.token_bytes(32)
  self.sql(1,'UPDATE p14_season_policies SET enabled=1,season_index=55,register_start='+str(base+1000)+',register_end='+str(now+8000)+';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES(2,1,"http://127.0.0.1:1",UNHEX("'+key.hex()+'"));UPDATE p14_audition_match_policies SET enabled=1;')
  for player in players[:2]:self.call(1,'CSKFEnterGame',{},player,400+player.playerId,'SCKFEnterGame')
  self.wait_sql(1,'SELECT COUNT(*) FROM p14_audition_qualifications WHERE season_index=55;','1',timeout=30)
  deadline=time.monotonic()+30
  while True:
   closed=self.call(1,'CSKFOpenFrightServerInfo',{},players[0],501,'SCKFOpenFrightServerInfo')
   if closed.typeStage==7:break
   self.assertLess(time.monotonic(),deadline);time.sleep(.3)
  self.assertEqual((closed.isEnterCurStage,closed.mateTime),(0,0));self.assertEqual(self.sql(1,'SELECT enabled FROM p14_promotion_policies;').strip(),'0');self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p14_promotion_states;').strip(),'0')
  before=self.profiles(1,players);self.sql(1,'UPDATE p14_promotion_policies SET enabled=1;')
  response=self.call(1,'CSKFOpenFrightServerInfo',{},players[0],502,'SCKFOpenFrightServerInfo');self.assertEqual(response.isEnterCurStage,1)
  qualifiedAt=int(self.sql(1,'SELECT qualified_at FROM p14_audition_qualifications WHERE season_index=55;').strip())
  eligible=max(slots[7][0],qualifiedAt);location=dt.timezone(dt.timedelta(hours=8));date=dt.datetime.fromtimestamp(eligible/1000,location);first=date.replace(hour=12,minute=0,second=0,microsecond=0)
  if int(first.timestamp()*1000)<eligible:first+=dt.timedelta(days=1)
  expected=20 if int(first.timestamp()*1000)<=response.currentTime else 0;self.assertEqual(response.mateTime,expected)
  outsider=self.call(1,'CSKFOpenFrightServerInfo',{},players[2],503,'SCKFOpenFrightServerInfo');self.assertEqual((outsider.isEnterCurStage,outsider.mateTime),(0,0))
  sock=self.tcp(1,players[0]);self.tcp_send(1,sock,'CSKFOpenFrightServerInfo',{},players[0]);op,frame=qual.cross.guild.P13MySQLTests.recv(self,sock);self.assertEqual((op,frame.isEnterCurStage,frame.mateTime),(BY_NAME['SCKFOpenFrightServerInfo'],1,expected))
  for name in ['CSKuafuMatchPlayer','CSKFRewardInfo']:
   op,_=self.send(1,name,{},players[0],504);self.assertEqual(op,10001)
  state=self.sql(1,'SELECT source_hash,qualification_hash,calendar_hash,points,score FROM p14_promotion_states;');self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p14_promotion_states;').strip(),'1')
  self.sql(1,'UPDATE p14_promotion_policies SET enabled=0;');off=self.call(1,'CSKFOpenFrightServerInfo',{},players[0],505,'SCKFOpenFrightServerInfo');self.assertEqual((off.isEnterCurStage,off.mateTime),(0,0));self.assertEqual(self.sql(1,'SELECT source_hash,qualification_hash,calendar_hash,points,score FROM p14_promotion_states;'),state)
  self.sql(1,'UPDATE p14_promotion_policies SET enabled=1;');fixed=self.ports[1];self.stop(1);self.start(1,fixed)
  restored=self.call(1,'CSKFOpenFrightServerInfo',{},players[0],506,'SCKFOpenFrightServerInfo');self.assertEqual((restored.typeStage,restored.isEnterCurStage,restored.mateTime),(7,1,expected));self.assertEqual(self.sql(1,'SELECT source_hash,qualification_hash,calendar_hash,points,score FROM p14_promotion_states;'),state)
  self.assertEqual(self.profiles(1,players),before);self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p14_audition_matches;').strip(),'0')
  self.assertEqual(self.sql(1,'SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA="'+self.database+'" AND TABLE_NAME LIKE "p14_promotion_%";').strip(),'InnoDB')
  self.stop(1)
  report={'status':'PASS_ORIGINAL_PHASE7_REAL_QUALIFICATION_MATCHPOINTS_STATUS','seconds':round(time.monotonic()-began,3),'producer':'original authenticated signup/cutoff/zero-roundauto, actual published phase7; independent gate defaultoff; onlytwo genuinequalifiers/thirdunregistered excluded','domain':'eight sameNoon requests one20 grant, no early points, 20cap/nextday/cold/closed/end/foreignsameID/missingreceipt/retaggedbalance guard; historical explicit serviceclock fixtures, no arbitrary qrows','native':'actual original HTTP/TCP phase7 isEnterCurStage1/mateTime exactfirstnoon calculation ('+str(expected)+'), complete3PlayerGame unchanged, original match/reward closed/no charge/winner/scorechange; samefrozen q/calendar/state across processrestart','rules':'original12h/20daily/cap20/initialscore1; firstgrant >=bothtruequalAt+stageStart uses named local source defaultoff, UTC+8 compatibility clock; no publicbattle/probability/private award fabricated','nineWorkerShutdownSeconds':self.shutdownTimes,'shared':'qualification-shared-v2 main/handler; no new shared hooks','nativeSHA256':self.meta['nativeSHA256'],'raceSHA256':self.meta['raceSHA256'],'productionModified':False,'phoneVerified':False,'cleanup':'whole-life lock/2isolatedschemas/user/grants removed; own temp fullwork/native/race cleaned after report'}
  (ROOT/'analysis/p14/promotion-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()
