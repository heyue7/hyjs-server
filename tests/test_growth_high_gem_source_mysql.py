"""Prebuilt real gold -> original grade10 choice -> mounted/recycled exact gem.

Only Player85 is a finite qualification. Gold, diamonds, the original box and
equipment all come from real business transactions. Craft level stays natural.
No compiler, formal database, phone, original battle result or new product rule.
"""
import concurrent.futures
import fcntl
import http.client as http_client
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
from server.codec import BY_NAME, CLASSES, decode_responses, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]
PREFIX = 'limitExchange_p4_gem_'
GEM = 'MineItem_028'


def preflight():
    vectors = {
        'CSGetPaymentInfo': {'channelId':'AND0003','shopId':'common'},
        'CSGenerateOrder': {'paymentId':'djsy.wuyou.andp2000','money':2000.0,'moneyType':'RMB','channelId':'AND0003','shopId':'common'},
        'CSShopBuy': {'shopType':2,'shopItemId':'p3vip7_growth_local','buyCount':1},
        'CSShopProductList': {'shopType':2},
        'CSItemUse': {'costItem':{'id':'item_345','num':1},'getItemIndex':1},
        'CSGetOpenServerReward': {'itemId':20},
        'CSActivityShopInfo': {'activityId':'actual-directory'},
        'CSBuyActivityShop': {'activityId':'actual-directory','index':1,'num':1,'costResources':[{'id':'coin','num':10000000}],'getResources':[{'id':'item_345','num':1}]},
        'CSFormationOn': {'id':'equip_005','guid':'actual-guid','formatPos':1},
        'CSFormationOff': {'id':'equip_005','guid':'actual-guid','formatPos':1},
        'CSGemMount': {'guid':'actual-guid','gemId':GEM,'pos':0},
        'CSGemUnmount': {'guid':'actual-guid','pos':0},
        'CSEquipDecompose': {'guid':['actual-guid']},
        'CSCopyEnter': {'chapterId':'1','missionId':'1'},
        'CSCopyRunMapInfo': {'chapterId':'1','missionId':'1'}, 'CSCopyLeave': {},
    }
    for name, fields in vectors.items():
        if parse_http(wire.encode_original_http(name,fields,2,b'0'*16,100)).decode() != CLASSES[name](**fields):
            raise AssertionError('Original frame mismatch: '+name)
    for name, fields in {'SCGemMount':{'equip','cost'},'SCGemUnmount':{'equip','res'},
        'SCEquipDecompose':{'get','cost'},'SCCopyRunMapInfo':{'battlePlayer'},
        'SCItemUse':{'get','cost'},'SCGetActivityShopReward':{'get','costResources'},
        'SCShopProductList':{'shopType','shopItemList'},
        'ShopItem':{'id','item','price','canBuy','dayLeftTimes','dayTotalTimes'}}.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response mismatch: '+name)
    source = Path(os.environ['HYJS_HIGH_GEM_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_HIGH_GEM_BINARY']).resolve()
    record = Path(os.environ['HYJS_HIGH_GEM_VALIDATION']).resolve()
    if source.parent != ROOT/'build/candidates':
        raise AssertionError('Use the root frozen direct candidate')
    manifest = source/'manifest.json'
    pinned = set()
    for row in json.loads(manifest.read_bytes())['files']:
        path = (source/row['path']).resolve()
        if not path.is_relative_to(source) or path in pinned or sha(path) != row['sha256']:
            raise AssertionError('Candidate pin differs: '+row['path'])
        pinned.add(path)
    if set((source/'hyjs-server').rglob('*.go'))-pinned:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(record.read_bytes())
    if proof.get('qualified') is not True or (ROOT/proof['candidate']).resolve() != source:
        raise AssertionError('Completed same-source qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Exact qualified binary required; never compile here')
    for name in ['server/codec.py','server/protocol/schema.json', *['analysis/tables/Table'+n+'.tsv'
            for n in ['MineSkill','ItemUse','Equip','EquipDecompose','ActivityNewOpen7Day','Payment','GameConfigure']]]:
        if sha(source/name) != sha(ROOT/name):
            raise AssertionError('Original contract/table differs: '+name)
    gem = next(r for r in original_table('MineSkill') if r['MineItemID']==GEM)
    box = [r for r in original_table('ItemUse') if r['itemId']=='item_345']
    if len(box)!=6 or not any(r['category']=='5' and r['odds']=='1' and r['result']==GEM+',1' for r in box):
        raise AssertionError('Original sixth-colour-choice box changed')
    if (gem['Level'],gem['Color'],gem['Value']) != ('10','1','att,4000'):
        raise AssertionError('Original tier10 ATT gem changed')
    gear = next(r for r in original_table('Equip') if r['id']=='equip_005')
    if gear['hole1'] != '1': raise AssertionError('Original gear hole mismatch')
    decompose = next(r['retRes'] for r in original_table('EquipDecompose') if r['quality']==gear['quality'])
    return source,binary,dict(requests=len(vectors),gem=GEM,gemGrade=10,property={'att':4000},
        sourcePrice=10000000,sourceMinLevel=85,originalRecycle=decompose,
        onlyFiniteQualification='Player85; original hero remains natural, crafting never injected',
        runnerDependencies={p:sha(ROOT/p) for p in ['tests/test_growth_recruit_mysql.py','tests/hyjs_managed_mysql.py','tests/test_local_wallet_mysql.py','tests/test_continued_gameplay_mysql.py']})


@unittest.skipUnless(os.environ.get('HYJS_HIGH_GEM_MYSQL_TEST')=='1','Root prebuilt managed window only')
class GrowthHighGemSourceMySQLTests(unittest.TestCase):
    send,login,call = transport.send,transport.login,transport.call
    stamped,profile,start,stop,sql = transport.stamped,transport.profile,transport.start,transport.stop,transport.sql
    json_request,wallet_body,wallet = transport.json_request,transport.wallet_body,transport.wallet
    tools,change,activity,directory = transport.tools,transport.change,transport.activity,transport.directory

    def test_actual_grade10_source_mount_protect_recycle_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source,binary,contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_HIGH_GEM_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT/'analysis/growth-first-20261016/resources'))
        self.assertFalse(output.exists(),'Keep prior attempts')
        report = dict(status='PENDING',fullChain=False,binarySHA256=sha(binary),
            sourceManifestSHA256=sha(self.source/'manifest.json'),runnerSHA256=sha(__file__),contract=contract,
            phoneUI=False,actualBattleStarted=False,highest13NaturalSourceClaimed=False,
            formalDBConnections=0,formalDBWrites=0,noMaterialCurrencyGUIDInsertion=True)
        began=time.monotonic()
        self.pool,self.process = ManagedMySQL(),None
        with tempfile.TemporaryDirectory(prefix='hyjs-high-gem-') as temporary:
            self.app_temp=types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
                try:
                    allocation=self.pool.allocate('high_gem')
                    self.database=allocation.database
                    self.env={k:v for k,v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai',GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database,'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix=secrets.token_hex(5)
                    protected=self.login('demo-highgem-protected-'+suffix)
                    self.assertEqual(protected.playerId,1)
                    self.call('CSPlayerCreate',{'playerName':'宝石保护','heroId':'hero_001'},protected,1,'SCPlayerCreate')
                    protected_before=self.profile(protected)
                    uid='demo-highgem-'+suffix
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
                    def reject(name,fields,saved=None):
                        before=self.profile(owner)
                        if saved is None: http(name,fields,'ErrorMessage')
                        else: self.call(name,fields,owner,saved,'ErrorMessage')
                        self.assertEqual(self.profile(owner),before)
                    def count(key):
                        g=self.profile(owner)[1]
                        return sum(x['num'] for slot in ['items','gems'] for x in (g.get(slot) or []) if x['id']==key)
                    http('CSPlayerCreate',{'playerName':'高阶真实宝石','heroId':'hero_001'},'SCPlayerCreate')
                    self.wallet('add',owner,requestKey=str(uuid.uuid4()),amountCents=200000)
                    products=http('CSGetPaymentInfo',{'channelId':'AND0003','shopId':'common'},'SCGetPaymentInfo')
                    product=next(p for p in products.paymentInfoList if p.paymentId=='djsy.wuyou.andp2000')
                    order,_=once('CSGenerateOrder',{'paymentId':product.paymentId,'money':product.money,
                        'moneyType':product.moneyType,'channelId':'AND0003','shopId':product.shopId},'SCGenerateOrder')
                    self.assertEqual(order.transactionData.payStatus,'PAY_STATUS_SUCCESS')
                    self.assertEqual((self.profile(owner)[0]['vip'],self.profile(owner)[0]['diamond']),(7,23000))
                    self.assertEqual(self.sql("SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='p3vip7_growth_local';").strip(),'7\t800\titem_041,1\t0')
                    self.pool.sql(self.database,"UPDATE p3_vip_shop_products SET enabled=1 WHERE id='p3vip7_growth_local' AND min_v_ip=7 AND cost=800 AND reward='item_041,1' AND enabled=0;")
                    vip_purchases=[]
                    report['vipDailyClaimProgress']=vip_purchases
                    for day in range(10):
                        if day: self.assertEqual(self.change(owner,'nextDay')['offsetDays'],day)
                        listing=http('CSShopProductList',{'shopType':2},'SCShopProductList')
                        q=next(q for q in listing.shopItemList if q.id=='p3vip7_growth_local')
                        self.assertEqual((q.canBuy,q.dayLeftTimes,q.dayTotalTimes),(1,1,1))
                        self.assertEqual([(r.id,r.num) for r in q.price],[('diamond',800)])
                        self.assertEqual([(r.id,r.num) for r in q.item],[('item_041',1)])
                        before_diamond=self.profile(owner)[0]['diamond']
                        request={'shopType':2,'shopItemId':'p3vip7_growth_local','buyCount':1}
                        attempt=dict(day=day,costDiamond=800,attempts=[])
                        vip_purchases.append(attempt)
                        if day==0:
                            # One stressed daily claim is enough to verify
                            # concurrency. Later days exercise the complete
                            # source calendar through ordinary single clicks.
                            stamp+=1
                            saved=stamp
                            raw=wire.encode_original_http('CSShopBuy',request,owner.playerId,bytes(owner.sessionKey),saved)
                            def attempt_buy(index):
                                started=time.monotonic()
                                c=http_client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=60)
                                detail={'index':index,'connectionTimeoutSeconds':60}
                                try:
                                    c.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
                                    response=c.getresponse()
                                    payload=response.read()
                                    self.assertEqual(response.status,200)
                                    code,body=decode_responses(payload)[0]
                                    detail['opcode']=code
                                    if code==BY_NAME['ErrorMessage']: detail['businessCode']=body.code
                                    return code,body
                                except BaseException as error:
                                    detail.update(errorType=type(error).__name__,error=str(error))
                                    raise
                                finally:
                                    c.close()
                                    detail['seconds']=round(time.monotonic()-started,3)
                                    attempt['attempts'].append(detail)
                            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                                rows=list(workers.map(attempt_buy,range(8)))
                            self.assertEqual(sum(code==BY_NAME['SCShopBuy'] for code,_ in rows),1)
                            errors=[body for code,body in rows if code==BY_NAME['ErrorMessage']]
                            self.assertEqual(len(errors),7)
                            self.assertTrue(all(e.code=='90003' for e in errors))
                            attempt.update(success=1,alreadyClaimed=7)
                        else:
                            started=time.monotonic()
                            http('CSShopBuy',request,'SCShopBuy')
                            attempt.update(success=1,alreadyClaimed=0)
                            attempt['attempts'].append(dict(index=0,seconds=round(time.monotonic()-started,3)))
                        self.assertEqual(self.profile(owner)[0]['diamond'],before_diamond-800)
                        self.assertEqual(count('item_041'),1)
                        bought=http('CSShopProductList',{'shopType':2},'SCShopProductList')
                        q=next(q for q in bought.shopItemList if q.id=='p3vip7_growth_local')
                        self.assertEqual((q.canBuy,q.dayLeftTimes,q.dayTotalTimes),(0,0,1))
                        attempt['costAndInventoryVerified']=True
                        opened,_=once('CSItemUse',{'costItem':{'id':'item_041','num':1}},'SCItemUse')
                        self.assertEqual(sum(x.num for x in opened.get.simps if x.id=='coin'),1000000)
                    self.assertEqual((self.profile(owner)[0]['coin'],self.profile(owner)[0]['diamond']),(10000000,15000))
                    self.change(owner,'reset')
                    before=self.profile(owner)
                    self.pool.sql(self.database,f"UPDATE profiles SET player=JSON_SET(player,'$.level',85) WHERE id={owner.playerId};")
                    self.assertEqual(self.profile(owner),[dict(before[0],level=85),before[1]])
                    self.tools(owner,'activities/apply',activityId='OpenHappy',action='open',requestId=str(uuid.uuid4()))
                    once('CSGetOpenServerReward',{'itemId':20},'SCGetOpenServerReward')
                    gear_open={'costItem':{'id':'item_055','num':1},'getItemIndex':0}
                    opened,gear_stamp=once('CSItemUse',gear_open,'SCItemUse')
                    gear=next(e for e in opened.get.equips if e.equipId=='equip_005')
                    self.assertEqual(len({e.guid for e in opened.get.equips}),4)
                    self.activity(owner,'open')
                    key,_,_,bindings=self.directory(owner)
                    binding=next(b for b in bindings if b['childId'].startswith(PREFIX) and b['productId']=='item_345')
                    listing=http('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo')
                    q=next(q for q in listing.shopItemList if q.index==binding['index'])
                    self.assertEqual([(r.id,r.num) for r in q.costResource],[('coin',10000000)])
                    self.assertEqual([(r.id,r.num) for r in q.getResource],[('item_345',1)])
                    buy={'activityId':key,'index':q.index,'num':1,
                         'costResources':[{'id':r.id,'num':r.num} for r in q.costResource],
                         'getResources':[{'id':r.id,'num':r.num} for r in q.getResource]}
                    once('CSBuyActivityShop',buy,'SCGetActivityShopReward')
                    self.assertEqual((self.profile(owner)[0]['coin'],count('item_345')),(0,1))
                    reject('CSBuyActivityShop',buy)
                    box={'costItem':{'id':'item_345','num':1},'getItemIndex':1}
                    opened,box_stamp=once('CSItemUse',box,'SCItemUse')
                    self.assertEqual([(r.id,r.num) for r in opened.get.simps],[(GEM,1)])
                    self.assertEqual((count(GEM),count('item_345')),(1,0))
                    craft_before=self.sql(f'SELECT level,exp FROM gem_craft_states WHERE player_id={owner.playerId};').strip()
                    self.assertIn(craft_before,('','1\t0'))
                    http('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
                    def panel(): return http('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer
                    native_base=panel()
                    wear={'id':gear.equipId,'guid':gear.guid,'formatPos':1}
                    http('CSFormationOn',wear,'SCFormationOn')
                    before_panel=panel()
                    mount={'guid':gear.guid,'gemId':GEM,'pos':0}
                    _,mount_stamp=once('CSGemMount',mount,'SCGemMount')
                    after=panel()
                    self.assertEqual(after.formation[0].prop.att-before_panel.formation[0].prop.att,4000)
                    self.assertEqual(count(GEM),0)
                    recycle={'guid':[gear.guid]}
                    reject('CSEquipDecompose',recycle)
                    once('CSGemUnmount',{'guid':gear.guid,'pos':0},'SCGemUnmount')
                    self.assertEqual(panel(),before_panel)
                    self.assertEqual(count(GEM),1)
                    once('CSGemMount',mount,'SCGemMount')
                    http('CSFormationOff',wear,'SCFormationOff')
                    self.assertEqual(panel(),native_base)
                    recycled,recycle_stamp=once('CSEquipDecompose',recycle,'SCEquipDecompose')
                    expected_recycle={k:int(n) for k,n in (p.split(',') for p in contract['originalRecycle'].split(';'))}
                    expected_recycle[GEM]=1
                    self.assertEqual({r.id:r.num for r in recycled.get.simps},expected_recycle)
                    self.assertEqual(count(GEM),1)
                    final_player=self.profile(owner)[0]
                    self.assertEqual((final_player['coin'],final_player['diamond'],final_player['point']),
                                     (expected_recycle['coin'],14600,expected_recycle['point']))
                    self.assertFalse(any(e['guid']==gear.guid for e in self.profile(owner)[1]['equipment']))
                    reject('CSGemMount',mount,mount_stamp)
                    reject('CSItemUse',gear_open,gear_stamp)
                    # The gem-only source receipt remains idempotent and cannot
                    # credit the returned gem a second time after recycling.
                    saved=self.profile(owner)
                    self.call('CSItemUse',box,owner,box_stamp,'SCItemUse')
                    self.assertEqual(self.profile(owner),saved)
                    http('CSCopyLeave',{},'SCCopyLeave')
                    saved=self.profile(owner)
                    self.stop();self.start();owner=self.login(uid)
                    self.assertEqual(self.profile(owner),saved)
                    self.call('CSEquipDecompose',recycle,owner,recycle_stamp,'SCEquipDecompose')
                    self.assertEqual(self.profile(owner),saved)
                    self.assertEqual(self.profile(protected),protected_before)
                    self.assertEqual(self.sql(f'SELECT level,exp FROM gem_craft_states WHERE player_id={owner.playerId};').strip(),'1\t0')
                    report.update(status='PASS',fullChain=True,paidVIP7Boxes=10,actualGoldSpent=10000000,
                        vipDailyClaimConcurrency=vip_purchases,
                        originalBox='item_345',actualGem=GEM,actualEquipment=gear.equipId,actualEquipmentGUID=gear.guid,
                        actualATTDelta=4000,protected1Exact=True,craftingStayedNaturalLevel1=True,
                        wornRecycleRejected=True,recycleReturnedSameGemOnce=True,oldEquipmentReceiptRejected=True,
                        sameStampEightOnce=True,coldExact=True)
                except BaseException as error:
                    report.update(status='FAIL',failureType=type(error).__name__,failure=str(error))
                    # Preserve bounded server evidence before the owned temp
                    # directory disappears. Never persist sessions/passwords.
                    log=Path(self.app_temp.name)/'app.log'
                    if log.exists():
                        tail=self.pool.redact(log.read_text(errors='replace')[-24000:])
                        for account in [locals().get('owner'),locals().get('protected')]:
                            if account is not None:
                                key=bytes(account.sessionKey)
                                tail=tail.replace(key.hex(),'[session-redacted]')
                                tail=tail.replace(str(key),'[session-redacted]')
                        log_output=output.with_suffix('.app.log')
                        log_output.parent.mkdir(parents=True,exist_ok=True)
                        log_output.write_text(tail)
                        report.update(serverLogTail=str(log_output.relative_to(ROOT)),serverLogTailSHA256=sha(log_output))
                    raise
                finally:
                    self.stop();self.pool.close()
                    report.update(seconds=round(time.monotonic()-began,3),ownedManagedAllocationsRemaining=len(self.pool.connections))
                    output.parent.mkdir(parents=True,exist_ok=True)
                    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__': unittest.main()
