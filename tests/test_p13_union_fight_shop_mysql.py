"""P13场内商店：原HTTP/TCP和真实采矿来源消费，隔离InnoDB，不连接正式数据库。"""
import concurrent.futures
import os
import subprocess
import unittest
import test_p13_union_fight_field_mysql as field


@unittest.skipUnless(os.environ.get('HYJS_P13_SHOP_MYSQL_TEST') == '1', '显式启用场内商店专属MySQL')
class P13UnionFightShopMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end = None
    test_2_native_original_frames_private_instances_relogin_end_restart = None

    def test_1_mysql_race_actual_mining_source_stock_roles_and_cold(self):
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', '-race', './service', '-run', '^TestP13UnionFightShopActual', '-count=1', '-timeout=180s'], cwd=field.SOURCE / 'hyjs-server', env=self.env, capture_output=True, text=True, timeout=210)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME LIKE 'p13_union_fight_shop%';").strip(), 'InnoDB')

    def test_2_native_original_mining_token_shop_tcp_refresh_restart_receipt(self):
        if os.environ.get('HYJS_P13_SHOP_NATIVE_TEST') != '1':
            self.skipTest('显式启用场内商店原HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)


def publish_shop_sources(t, first):
    # 只在本测试专库发布具名本地产量和报价；APK没有原spawn/产量/商店价目表。
    t.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','shop-native-left',1,'1',-4.7,-0.12,9.98,50," + str(first-60000) + ",'隔离显式矿型1/原NPC2位置/本地产量50'),('2026-10-20','shop-native-right',1,'1',80.22,-0.33,-13.36,50," + str(first-60000) + ",'隔离显式矿型1/原NPC5位置/本地产量50');")
    rows = []
    for index, price in enumerate([10,10,20,20,30]):
        role = 1 if index in [2,3] else (2 if index == 4 else 0)
        rows.append("('2026-10-20','catalog-" + str(index) + "',1,'ufitem_" + str(16+index).zfill(3) + "'," + str(price) + ",1,2," + str(role) + "," + str(index) + ",60000," + str(first-60000) + ",'隔离具名本地报价10/10/20/20/30，每60秒恢复一个，不声称原价')")
    t.sql('INSERT INTO p13_union_fight_shop_sources(day,shop_item_id,enabled,item_id,price,quantity,buy_count,buy_condition,priority,refresh_millis,published_at,source) VALUES' + ','.join(rows) + ';')


def native_shop_flow(t, uid, first, login, sock, transfer):
    camp = transfer.player.camp
    recycler, shop = ('2','3') if camp == 0 else ('5','6')
    recycle_point = {'x':-4.7,'y':-0.12,'z':9.98} if camp == 0 else {'x':80.22,'y':-0.33,'z':-13.36}
    shop_point = {'x':-16,'y':-0.12,'z':-9} if camp == 0 else {'x':87.8,'y':-0.31,'z':-2.48}
    guid = int(t.sql("SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='" + transfer.unionFightId + "' AND ABS(x-" + str(recycle_point['x']) + ")<0.001;").strip())
    t.set_clock(first+20000)
    t.tcp_send(sock,'CSMove',{'pointNew':recycle_point},login);t.receive(sock,'SCUFElementMove')
    op,_ = t.stamped('CSUFShopList',{'npcId':''},login,601);t.assertEqual(op,10001)
    t.tcp_send(sock,'CSUFPlayerCollect',{'resGuid':guid},login)
    t.assertEqual(t.receive(sock,'SCUFPlayerCollect').collectTime[0],3)
    t.set_clock(first+23000)
    mined = t.receive(sock,'SCUFPlayerCollectEndDetail')
    t.assertEqual((mined.addResourceNum,mined.player.ownRes.resourceNum),(50,50))
    deposit = t.call('CSUFPlayerUpResource',{'npcId':recycler},login,602,'SCUFPlayerUpResource')
    t.assertEqual((deposit.resourceNum,deposit.playerReward,deposit.player.ownToken),(50,150,150))
    t.set_clock(first+33000)
    t.tcp_send(sock,'CSMove',{'pointNew':shop_point},login);t.receive(sock,'SCUFElementMove')
    t.tcp_send(sock,'CSUFShopList',{'npcId':shop},login)
    catalogue = t.receive(sock,'SCUFShopList');t.assertEqual(len(catalogue.items),5)
    t.assertTrue(all(item.leftBuyCount == 2 and item.totalBuyCount == 2 and item.refreshIntervalTime == 60000 for item in catalogue.items))
    request = {'shopItemId':'catalog-0','npcId':shop}
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        replies = list(pool.map(lambda _:t.call('CSUFPlayerBuyItem',request,login,800,'SCUFPlayerBuyItemDetail'),range(8)))
    t.assertTrue(all(reply == replies[0] for reply in replies))
    first_buy = replies[0]
    t.assertEqual((first_buy.player.ownToken,len(first_buy.player.items),first_buy.player.items[0].leftUseNum),(140,1,1))
    # 原客户端购买请求走HTTP；TCP0路径也保持同一原响应和消费事务。
    t.tcp_send(sock,'CSUFPlayerBuyItem',request,login)
    t.assertEqual(t.receive(sock,'SCUFPlayerBuyItemDetail').player.ownToken,130)
    def stock():
        t.tcp_send(sock,'CSUFShopList',{'npcId':''},login)
        return t.receive(sock,'SCUFShopList')
    t.assertEqual(next(item.leftBuyCount for item in stock().items if item.shopItemId == 'catalog-0'),0)
    op,_ = t.stamped('CSUFPlayerBuyItem',request,login,801);t.assertEqual(op,10001)
    for step in range(1,4):
        t.set_clock(first+33000+step*60000)
        t.assertEqual(next(item.leftBuyCount for item in stock().items if item.shopItemId == 'catalog-0'),1)
        bought = t.call('CSUFPlayerBuyItem',request,login,801+step,'SCUFPlayerBuyItemDetail')
        t.assertEqual(bought.player.items[0].leftUseNum,step+2)
    t.set_clock(first+273000)
    op,_ = t.stamped('CSUFPlayerBuyItem',request,login,805);t.assertEqual(op,10001)
    for index in range(1,5):
        bought = t.call('CSUFPlayerBuyItem',{'shopItemId':'catalog-'+str(index),'npcId':shop},login,910+index,'SCUFPlayerBuyItemDetail')
    t.assertEqual((bought.player.ownToken,len(bought.player.items)),(20,5))
    t.assertEqual([(item.playerGuid,item.index) for item in bought.player.items],[(login.playerId,index) for index in range(5)])
    t.sql("UPDATE p13_union_fight_shop_sources SET price=999 WHERE shop_item_id='catalog-0';")
    t.stop();t.start();login=t.login(uid);sock=t.connect(login)
    t.tcp_send(sock,'CSCurrentPosition',{},login);t.receive(sock,'SCCurrentPosition')
    restored=t.receive(sock,'SCUnionFightTransfer');t.assertEqual(restored.player,bought.player)
    t.assertEqual(next(item.price for item in stock().items if item.shopItemId=='catalog-0'),10)
    t.assertEqual(t.call('CSUFPlayerBuyItem',request,login,800,'SCUFPlayerBuyItemDetail'),first_buy)
    t.tcp_send(sock,'CSUFInit',{},login);t.assertEqual(t.receive(sock,'SCUnionFightTransfer').player.ownToken,20)
    op,_=t.stamped('CSUFPlayerBuyItem',{'shopItemId':'catalog-1','npcId':shop},login,800);t.assertEqual(op,10001)
    return login,sock


if __name__ == '__main__':
    unittest.main()
