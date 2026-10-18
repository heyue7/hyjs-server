"""R4 prebuilt: original chapter-local 6-1 -> actual 288 payment -> suit.

Only historical CopySimpleInfo is a bounded qualification. No level, wallet
balance, VIP, currency, gear GUID or win is inserted. Wallet add/order/award,
wearing, persistence and the once-only claim are real HTTP business actions.
"""
import concurrent.futures
import fcntl
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest
import uuid

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport, sha
from test_p5_talent_scope_mysql import original_table
from server.codec import BY_NAME, CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]
EQUIPS = {'equip_033','equip_034','equip_035','equip_036'}


def contracts():
    vectors = {
        'CSOpenLuxuryEquipment': {'chapter':6,'mission':1},
        'CSLuxuryEquipmentInfo': {}, 'CSLuxuryEquipmentReward': {},
        'CSGetPaymentInfo': {'channelId':'AND0003','shopId':'common'},
        'CSGenerateOrder': {'paymentId':'djsy.wuyou.andp0288','money':288.0,'moneyType':'RMB','channelId':'AND0003','shopId':'common'},
        'CSFormationOn': {'formatPos':1,'id':'equip_033','guid':'actual-award-guid'},
        'CSCopyEnter': {'chapterId':'1','missionId':'1'},
        'CSCopyRunMapInfo': {'chapterId':'1','missionId':'1'}, 'CSCopyLeave': {},
    }
    for name,fields in vectors.items():
        if parse_http(wire.encode_original_http(name,fields,2,b'0'*16,100)).decode()!=CLASSES[name](**fields):
            raise AssertionError('Original request mismatch: '+name)
    for name,fields in {
        'SCOpenLuxuryEquipment':{'openOrNot'},
        'SCLuxuryEquipmentInfo':{'id','currentTime','endTime','rewardState'},
        'SCLuxuryEquipmentReward':{'get'},'SCGenerateOrder':{'orderId','transactionData'},
        'SCFormationOn':{'formationItem','fightpower'},'SCCopyRunMapInfo':{'battlePlayer'},
    }.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response mismatch: '+name)
    config=original_table('LuxuryEquip')
    if len(config)!=1 or any(config[0][k]!=v for k,v in {'id':'2','chapter':'6','stage':'1','money':'288','limitHours':'12'}.items()):
        raise AssertionError('Original luxury price/window changed')
    if set(config[0]['prize'].split(';')) != {e+',1' for e in EQUIPS}:
        raise AssertionError('Original four suit rewards changed')
    chapter=sorted(int(r['MissionId']) for r in original_table('CopyMission') if r['ChapterId']=='6')
    if not chapter or chapter[0]!=30 or 1 in chapter:
        raise AssertionError('Original numeric level_seq mapping changed')
    return dict(requests=len(vectors),chapter=6,localSequence=1,actualMissionID='30',
        offerID=2,moneyCents=28800,hours=12,equipmentIDs=sorted(EQUIPS),
        finitePreparation='Only historical CopySimpleInfo(6,30,5); not original-phone/natural completion',
        noLevelOrCurrencyInventoryInjection=True,claimContract='1 success + 7 already-claimed errors; no response-idempotency promise')


def preflight():
    contract=contracts()
    source=Path(os.environ['HYJS_LUXURY_SEQUENCE_SOURCE']).resolve()
    binary=Path(os.environ['HYJS_LUXURY_SEQUENCE_BINARY']).resolve()
    validation=Path(os.environ['HYJS_LUXURY_SEQUENCE_VALIDATION']).resolve()
    if source.parent!=ROOT/'build/candidates': raise AssertionError('Use direct frozen root candidate')
    manifest=source/'manifest.json'
    pinned=set()
    for row in json.loads(manifest.read_bytes())['files']:
        path=(source/row['path']).resolve()
        if not path.is_relative_to(source) or path in pinned or sha(path)!=row['sha256']:
            raise AssertionError('Candidate source pin differs: '+row['path'])
        pinned.add(path)
    if set((source/'hyjs-server').rglob('*.go'))-pinned: raise AssertionError('Unpinned Go source')
    proof=json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT/proof['candidate']).resolve()!=source:
        raise AssertionError('Completed same-candidate qualification required')
    if sha(binary)!=(proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use exact qualified binary without compiling')
    for name in ['server/codec.py','server/protocol/schema.json',*['analysis/tables/Table'+n+'.tsv'
            for n in ['LuxuryEquip','CopyMission','Equip','Payment']]]:
        if sha(source/name)!=sha(ROOT/name): raise AssertionError('Original table/protocol mismatch: '+name)
    offer=(source/'hyjs-server/service/paymentOfferService.go').read_text()
    if 's.luxuryOfferMission(config["chapter"], b.GetMission())' not in offer or 'info.GetMissionId() == missionID' not in offer:
        raise AssertionError('R4 chapter-local original sequence integration required')
    contract['runnerDependencies']={p:sha(ROOT/p) for p in ['tests/test_growth_recruit_mysql.py',
        'tests/hyjs_managed_mysql.py','tests/test_local_wallet_mysql.py','tests/test_continued_gameplay_mysql.py']}
    return source,binary,contract


@unittest.skipUnless(os.environ.get('HYJS_LUXURY_SEQUENCE_MYSQL_TEST')=='1','Root prebuilt managed window only')
class LuxuryChapterSequenceMySQLTests(unittest.TestCase):
    send,login,call=transport.send,transport.login,transport.call
    stamped,profile,start,stop,sql=transport.stamped,transport.profile,transport.start,transport.stop,transport.sql
    json_request,wallet_body,wallet=transport.json_request,transport.wallet_body,transport.wallet

    def test_original_six_one_paid_suit_and_natural_level_wear_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source,binary,contract=preflight()
        self.binary=str(binary)
        output=Path(os.environ['HYJS_LUXURY_SEQUENCE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT/'analysis/growth-first-20261016'))
        self.assertFalse(output.exists(),'Preserve earlier attempts')
        report=dict(status='PENDING',fullChain=False,runnerSHA256=sha(__file__),binarySHA256=sha(binary),
            sourceManifestSHA256=sha(self.source/'manifest.json'),contract=contract,
            actualBattleStarted=False,phoneUI=False,formalDBConnections=0,formalDBWrites=0)
        began=time.monotonic()
        self.pool,self.process=ManagedMySQL(),None
        with tempfile.TemporaryDirectory(prefix='hyjs-luxury-sequence-') as temporary:
            self.app_temp=types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
                try:
                    allocation=self.pool.allocate('luxury_seq')
                    self.database=allocation.database
                    self.env={k:v for k,v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai',GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database,'UPDATE local_wallet_policies SET enabled=1 WHERE id=1;')
                    suffix=secrets.token_hex(5)
                    protected=self.login('demo-luxury-protected-'+suffix)
                    self.assertEqual(protected.playerId,1)
                    self.call('CSPlayerCreate',{'playerName':'礼包保护','heroId':'hero_001'},protected,1,'SCPlayerCreate')
                    protected_before=self.profile(protected)
                    uid='demo-luxury-seq-'+suffix
                    owner=self.login(uid)
                    stamp=int(time.time()*1000)
                    def http(name,fields,expected):
                        nonlocal stamp
                        stamp+=1
                        return self.call(name,fields,owner,stamp,expected)
                    def once(name,fields,expected):
                        nonlocal stamp
                        stamp+=1
                        saved=stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            rows=list(workers.map(lambda _:self.call(name,fields,owner,saved,expected),range(8)))
                        self.assertTrue(all(x==rows[0] for x in rows))
                        return rows[0],saved
                    def reject(name,fields):
                        before=self.profile(owner)
                        http(name,fields,'ErrorMessage')
                        self.assertEqual(self.profile(owner),before)
                    http('CSPlayerCreate',{'playerName':'原六一礼包','heroId':'hero_001'},'SCPlayerCreate')
                    initial=self.profile(owner)
                    self.assertEqual((initial[0]['level'],initial[0].get('coin',0),initial[0].get('diamond',0)),(1,0,0))
                    request={'chapter':6,'mission':1}
                    def offer_row():
                        return self.sql(f'SELECT offer_id,start_at,end_at,cents,reward,claimed FROM p3_payment_luxury_offers WHERE player_id={owner.playerId} ORDER BY offer_id;').strip()
                    def history(chapter,mission,stars):
                        raw=json.dumps([{'chapterId':chapter,'missionId':mission,'star':stars,'challengeTimes':0}],separators=(',',':')).encode().hex()
                        self.pool.sql(self.database,"UPDATE profiles SET game=JSON_SET(game,'$.copy.passedCopyInfo',CAST(CONVERT(X'"+raw+"' USING utf8mb4) AS JSON)) WHERE id="+str(owner.playerId)+';')
                    for chapter,mission,stars in [('6','1',5),('5','30',5),('6','30',0)]:
                        history(chapter,mission,stars)
                        reject('CSOpenLuxuryEquipment',request)
                        self.assertEqual(offer_row(),'')
                        self.assertEqual(http('CSLuxuryEquipmentInfo',{},'SCLuxuryEquipmentInfo').id,0)
                    history('6','30',5)
                    self.assertEqual(http('CSOpenLuxuryEquipment',request,'SCOpenLuxuryEquipment').openOrNot,1)
                    frozen=offer_row()
                    row=frozen.split('\t')
                    self.assertEqual((int(row[0]),int(row[2])-int(row[1]),int(row[3]),row[5]),(2,12*3600*1000,28800,'0'))
                    self.assertEqual(set(row[4].split(';')),{e+',1' for e in EQUIPS})
                    info=http('CSLuxuryEquipmentInfo',{},'SCLuxuryEquipmentInfo')
                    self.assertEqual((info.id,info.endTime,info.rewardState),(2,int(row[2]),0))
                    self.assertLessEqual(int(row[1]),info.currentTime)
                    self.assertLess(info.currentTime,info.endTime)
                    self.assertEqual(http('CSOpenLuxuryEquipment',request,'SCOpenLuxuryEquipment').openOrNot,1)
                    self.assertEqual(offer_row(),frozen,'Reopening must preserve original 12h window')
                    reject('CSLuxuryEquipmentReward',{})
                    self.wallet('add',owner,requestKey=str(uuid.uuid4()),amountCents=28800)
                    products=http('CSGetPaymentInfo',{'channelId':'AND0003','shopId':'common'},'SCGetPaymentInfo')
                    product=next(p for p in products.paymentInfoList if p.paymentId=='djsy.wuyou.andp0288')
                    self.assertEqual(product.money,288.0)
                    order,_=once('CSGenerateOrder',{'paymentId':product.paymentId,'money':product.money,
                        'moneyType':product.moneyType,'channelId':'AND0003','shopId':product.shopId},'SCGenerateOrder')
                    self.assertEqual(order.transactionData.payStatus,'PAY_STATUS_SUCCESS')
                    self.assertEqual(self.wallet('snapshot',owner)['balanceCents'],0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(),'1\t-28800')
                    self.assertEqual(http('CSLuxuryEquipmentInfo',{},'SCLuxuryEquipmentInfo').rewardState,1)
                    before_award=self.profile(owner)
                    self.assertFalse(any(e['equipId'] in EQUIPS for e in before_award[1].get('equipment',[])))
                    stamp+=1
                    award_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        rows=list(workers.map(lambda _:self.stamped('CSLuxuryEquipmentReward',{},owner,award_stamp),range(8)))
                    success=[body for opcode,body in rows if opcode==BY_NAME['SCLuxuryEquipmentReward']]
                    self.assertEqual(len(success),1)
                    self.assertEqual(sum(opcode==BY_NAME['ErrorMessage'] for opcode,_ in rows),7)
                    awards=list(success[0].get.equips)
                    self.assertEqual({e.equipId for e in awards},EQUIPS)
                    self.assertEqual(len({e.guid for e in awards}),4)
                    awarded=self.profile(owner)
                    self.assertEqual({e['guid'] for e in awarded[1]['equipment']},{e.guid for e in awards})
                    self.assertEqual(awarded[0],before_award[0],'Suit claim must not add money or change natural level')
                    self.assertEqual(http('CSLuxuryEquipmentInfo',{},'SCLuxuryEquipmentInfo').rewardState,2)
                    gear=next(e for e in awards if e.equipId=='equip_033')
                    http('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
                    before_panel=http('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer
                    wear={'formatPos':1,'id':gear.equipId,'guid':gear.guid}
                    worn,wear_stamp=once('CSFormationOn',wear,'SCFormationOn')
                    after_panel=http('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer
                    delta=after_panel.formation[0].prop.att-before_panel.formation[0].prop.att
                    self.assertGreater(delta,0)
                    self.assertTrue(any(t.guid==gear.guid for t in worn.formationItem.things))
                    http('CSCopyLeave',{},'SCCopyLeave')
                    saved=self.profile(owner)
                    self.assertEqual(saved[0]['level'],1,'No level16 or other extra qualification needed')
                    self.stop();self.start();owner=self.login(uid)
                    self.assertEqual(self.profile(owner),saved)
                    self.call('CSLuxuryEquipmentReward',{},owner,award_stamp,'ErrorMessage')
                    self.assertEqual(self.profile(owner),saved)
                    self.call('CSFormationOn',wear,owner,wear_stamp,'SCFormationOn')
                    self.assertEqual(self.profile(owner),saved)
                    self.assertEqual(self.profile(protected),protected_before)
                    report.update(status='PASS',fullChain=True,actualOfferID=2,actualPaidCents=28800,
                        actualEquipmentIDs=sorted(e.equipId for e in awards),actualGUIDs=sorted(e.guid for e in awards),
                        wornEquipmentID=gear.equipId,actualPanelATTDelta=delta,naturalLevel=1,
                        awardConcurrency={'success':1,'alreadyClaimed':7},coldDuplicateRejected=True,
                        unchangedFrozenOffer=True,protected1Exact=True,originalRealClockUnchanged=True)
                except BaseException as error:
                    report.update(status='FAIL',failureType=type(error).__name__,failure=str(error))
                    raise
                finally:
                    self.stop();self.pool.close()
                    report.update(seconds=round(time.monotonic()-began,3),ownedManagedAllocationsRemaining=len(self.pool.connections))
                    output.parent.mkdir(parents=True,exist_ok=True)
                    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__': unittest.main()
