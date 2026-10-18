"""SQL-configured local pools through the original APK wire protocol."""
import concurrent.futures
import json
import unittest
from server.codec import BY_NAME
import test_go_inventory as inventory
import test_go_formation as formation

class GoRecruitTests(unittest.TestCase):
    setUp = inventory.GoInventoryTests.setUp
    tearDown = inventory.GoInventoryTests.tearDown
    start = inventory.GoInventoryTests.start
    stop = inventory.GoInventoryTests.stop
    send = inventory.GoInventoryTests.send
    login = inventory.GoInventoryTests.login
    create = inventory.GoInventoryTests.create
    profile = inventory.GoInventoryTests.profile
    mutate = inventory.GoInventoryTests.mutate
    stamped = formation.GoFormationTests.stamped
    snapshot = formation.GoFormationTests.snapshot

    def draw(self, login, kind=2, count=1, free=0, stamp=1700000000001):
        return self.stamped('CSWarriorRecruit', {'costType':kind,'numType':count,'free':free}, login, stamp)

    def supply(self, login, **kw):
        self.mutate(login, lambda p,g:p.update(**kw))

    def sql(self, statement, args=()):
        with self.service.store.connect() as c:
            c.execute(statement, args)

    def test_free_cooldowns_inventory_and_restart_receipt(self):
        login=self.create('recruit-free')
        op,info=self.send('CSWarriorRecruitGet',login=login)
        self.assertEqual(op,BY_NAME['SCWarriorRecruitGet'])
        self.assertEqual([(x.costType,x.num,x.free) for x in info.recruit],[(1,3,True),(2,9,True)])
        for kind in [1,2]:
            op,reply=self.draw(login,kind=kind,free=1)
            self.assertEqual(op,BY_NAME['SCWarriorRecruit'],str(reply))
            self.assertEqual(len(reply.get.wars),1)
            self.assertEqual(reply.get.wars[0].breached.level,1)
            self.assertFalse(reply.cost)
            self.assertEqual(self.draw(login,kind=kind,free=1)[1],reply)
            self.assertEqual(self.draw(login,kind=kind,free=1,stamp=1700000000002)[0],10001)
        self.assertEqual(self.profile(login).coin,11000)
        before=self.snapshot(login)
        self.stop();self.start();login=self.login('recruit-free')
        self.assertEqual(self.draw(login,free=1)[0],BY_NAME['SCWarriorRecruit'])
        self.assertEqual(self.snapshot(login),before)

    def test_paid_tickets_ten_guarantee_and_resources(self):
        login=self.create('recruit-paid')
        def init(p,g):
            p.update(diamond=10000)
            g['items']=[{'id':'item_016','num':10},{'id':'item_048','num':1},{'id':'item_049','num':1}]
        self.mutate(login,init)
        # Remove natural guarantee chance until the tenth draw: tiny positive guarantee weight.
        self.sql("UPDATE gameplay_pool_rewards SET weight=CASE WHEN guaranteed=1 THEN 1 ELSE 1000000000000 END WHERE pool_id='recruit_2'")
        op,one=self.draw(login)
        self.assertEqual(op,BY_NAME['SCWarriorRecruit'])
        self.assertEqual([(r.id,r.num) for r in one.cost],[('item_048',1)])
        op,ten=self.draw(login,count=2,stamp=1700000000002)
        self.assertEqual(op,BY_NAME['SCWarriorRecruit'],str(ten))
        self.assertEqual([(r.id,r.num) for r in ten.cost],[('item_049',1)])
        self.assertEqual(len({w.guid for w in ten.get.wars}),10)
        with self.service.store.connect() as c:
            guarantees={r[0] for r in c.execute("SELECT resource_id FROM gameplay_pool_rewards WHERE pool_id='recruit_2' AND guaranteed=1")}
        self.assertTrue(any(w.id in guarantees for w in ten.get.wars))
        op,diamond=self.draw(login,count=2,stamp=1700000000003)
        self.assertEqual([(r.id,r.num) for r in diamond.cost],[('diamond',2800)])
        self.assertEqual(self.profile(login).diamond,7200)
        op,normal=self.draw(login,kind=1,count=2)
        self.assertEqual([(r.id,r.num) for r in normal.cost],[('item_016',10)])
        self.assertEqual(self.profile(login).coin,220000)

    def test_sql_hot_change_and_seed_never_overwrites(self):
        login=self.create('recruit-config')
        self.sql("DELETE FROM gameplay_pool_rewards WHERE pool_id='recruit_2'")
        self.sql("INSERT INTO gameplay_pool_rewards(pool_id,id,resource_id,amount,weight,guaranteed,stock) VALUES('recruit_2',1,'hero_007',1,100,1,0)")
        op,r=self.draw(login,free=1)
        self.assertEqual(op,BY_NAME['SCWarriorRecruit']);self.assertEqual(r.get.wars[0].id,'hero_007')
        self.stop();self.start()
        with self.service.store.connect() as c:
            self.assertEqual(c.execute("SELECT count(*) FROM gameplay_pool_rewards WHERE pool_id='recruit_2'").fetchone()[0],1)
        self.sql("UPDATE gameplay_pools SET enabled=0 WHERE id='recruit_2'")
        self.assertEqual(self.draw(login,stamp=1700000000002)[0],10001)

    def test_invalid_pool_resources_bag_and_request_roll_back(self):
        login=self.create('recruit-invalid')
        before=self.snapshot(login)
        for fields in [{'costType':3,'numType':1,'free':1},{'costType':2,'numType':2,'free':1},{'costType':2,'numType':0,'free':0}]:
            self.assertEqual(self.stamped('CSWarriorRecruit',fields,login,1700000000001)[0],10001)
            self.assertEqual(self.snapshot(login),before)
        self.assertEqual(self.draw(login)[0],10001)
        self.sql("UPDATE gameplay_pool_rewards SET weight=0 WHERE pool_id='recruit_2' AND id=1")
        self.assertEqual(self.draw(login,free=1)[0],10001);self.assertEqual(self.snapshot(login),before)
        self.sql("UPDATE gameplay_pool_rewards SET weight=1 WHERE pool_id='recruit_2' AND id=1")
        self.supply(login,warriorBagSize=1)
        before=self.snapshot(login)
        self.assertEqual(self.draw(login,free=1)[0],10001);self.assertEqual(self.snapshot(login),before)

    def test_parallel_replay_and_stamp_conflicts_do_not_double_spend(self):
        login=self.create('recruit-parallel')
        self.supply(login,diamond=1000)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.draw(login),range(8)))
        self.assertTrue(all(op==BY_NAME['SCWarriorRecruit'] for op,_ in replies))
        self.assertTrue(all(reply==replies[0][1] for _,reply in replies))
        self.assertEqual(self.profile(login).diamond,700)
        before=self.snapshot(login)
        self.assertEqual(self.draw(login,free=1)[0],10001)
        self.assertEqual(self.draw(login,stamp=1700000000000)[0],10001)
        self.assertEqual(self.snapshot(login),before)

    def test_sql_price_changes_cannot_charge_more_than_client_displays(self):
        login=self.create('recruit-price-guard')
        self.supply(login,diamond=5000)
        before=self.snapshot(login)
        self.sql("UPDATE gameplay_pools SET single_cost='diamond,999' WHERE id='recruit_2'")
        self.assertEqual(self.draw(login)[0],10001)
        self.assertEqual(self.draw(login,free=1)[0],10001)
        self.assertEqual(self.snapshot(login),before)

    def test_normal_daily_quota_and_next_day_reset(self):
        login=self.create('recruit-daily')
        for i in range(3):
            if i:
                self.mutate(login,lambda p,g:g['recruit']['1'].update(next_free=0))
            self.assertEqual(self.draw(login,kind=1,free=1,stamp=1700000000001+i)[0],BY_NAME['SCWarriorRecruit'])
        self.mutate(login,lambda p,g:g['recruit']['1'].update(next_free=0))
        self.assertEqual(self.draw(login,kind=1,free=1,stamp=1700000000004)[0],10001)
        self.mutate(login,lambda p,g:g['recruit']['1'].update(day='2000-01-01'))
        self.assertEqual(self.draw(login,kind=1,free=1,stamp=1700000000004)[0],BY_NAME['SCWarriorRecruit'])

if __name__=='__main__':unittest.main()
