"""Finite owned QA prerequisites; no earned wins, reward receipts or production data."""
import hashlib,json,os,time
from pathlib import Path
import protocol as q

assert q.database.startswith('hyjs_test_feedback65_')
account,login=q.register()
assert account['playerId']==1, 'Fresh owned QA allocation required'
fd=os.open(q.PRIVATE/'account.json',os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'w') as f:json.dump(account,f)
raw=(q.ROOT/'analysis/p7/party-rush-isolated-roster-r2-20261016.json').read_bytes()
assert hashlib.sha256(raw).hexdigest()=='e60e9774a43efe58f67b9eb84c1a30cf40a3a30f2e5f68c87d063d3aeb29a5c9'
source=json.loads(raw)
player,game=json.loads(q.sql('SELECT player FROM profiles WHERE id=1;')),json.loads(q.sql('SELECT game FROM profiles WHERE id=1;'))
player.update(level=75,heroWarId=source['playerPatch']['heroWarId'],coin=1000000,diamond=100000)
for key in ['warriors','formation','equipment']:game[key]=source['gamePatch'][key]
# The captured pet is excluded; no pet ownership/projection or historical KV is imported.
for row in game['formation']:row['things']=[x for x in row['things'] if x['pos']!=7]
encoded=lambda v:json.dumps(v,separators=(',',':'),ensure_ascii=False).encode().hex()
q.sql('UPDATE profiles SET player=CONVERT(0x'+encoded(player)+' USING utf8mb4),game=CONVERT(0x'+encoded(game)+' USING utf8mb4) WHERE id=1;')
now=int(time.time()*1000)
q.sql(f'UPDATE admin_game_servers SET open_at={now-10*86400000} WHERE id=1;')
wire=q.CLASSES['SCRebelFightStart']()
wire.ParseFromString((q.ROOT/'analysis/p12/wanted-playable-20261016/native-inputs/000.SCRebelFightStart.pb').read_bytes())
hps={'wanted_'+a.guid.removeprefix('wanted-'):a.prop.hp for a in wire.battleMonsterList[0].formation}
assert len(hps)==4 and sum(hps.values())==405371
q.sql("INSERT INTO p12_wanted_instances (id,server_id,finder_id,finder_name,wanted_id,level,difficulty,state,created_at,expires_at,shared_at,killed_at,killer_id,current_hp,max_hp,monster_hp_json,definition_json,definition_hash) SELECT 'wanted65-phone-1',1,1,'QA65', '1',1,1,'active',"+str(now)+','+str(now+1800000)+",0,0,0,405371,405371,CONVERT(0x"+encoded(hps)+" USING utf8mb4),definition_json,definition_hash FROM p12_wanted_policies WHERE id=1;")
report={'playerId':1,'database':q.database,'finitePreparation':{'rosterSourceSHA256':hashlib.sha256(raw).hexdigest(),'level':75,'petExcluded':True,'coin':1000000,'diamond':100000,'realmOpeningDay':11,'wantedInstance':'original Wanted1 four-body finite instance; no discovery reward'},'importedWins':0,'importedRewardReceipts':0,'phoneLoggedIn':False}
(q.OUT/'phone-fixture.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report))
