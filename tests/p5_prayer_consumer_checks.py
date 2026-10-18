"""P5 consumers for the existing isolated P13 prayer native source chain.

The owner calls these hooks around its actual donations/prayers. This module
adds original public-panel/Player/CopyStart requests and no SQL or reward source.
"""
import hashlib
import math


def _call(test, name, fields, login, reply):
    test._p5_prayer_stamp = getattr(test, "_p5_prayer_stamp", 2000000) + 1
    return test.call(name, fields, login, test._p5_prayer_stamp, reply)


def _panel(test, login):
    return _call(test, "CSFindPlayerInfo", {"playerId": login.playerId, "serverId": 1}, login, "SCFindPlayerInfo").formationInfo


def before_prayer(test, login, index=1):
    """Capture the existing level/formation after finite setup and before prayer."""
    if not hasattr(test, "_p5_prayer_bases"):
        test._p5_prayer_bases = {}
        test._p5_prayer_frozen = {}
        test.p5_prayer_checks = {"panels": [], "frozenInputs": [], "inactive": []}
    if login.playerId not in test._p5_prayer_bases:
        panel = _panel(test, login)
        test.assertEqual(len(panel.formationItemInfoList), 1)
        test._p5_prayer_bases[login.playerId] = panel


def _verify_current(test, login, prop, phase):
    base = test._p5_prayer_bases[login.playerId]
    panel = _panel(test, login)
    test.assertEqual(len(panel.formationItemInfoList), len(base.formationItemInfoList))
    original = base.formationItemInfoList[0].warInfo.baseProp
    current = panel.formationItemInfoList[0].warInfo.baseProp
    for field in original.DESCRIPTOR.fields:
        expected = getattr(original, field.name)
        if field.name in ("att", "hp", "pdef", "mdef"):
            expected *= 1 + getattr(prop, "per" + field.name) * .01
        test.assertTrue(math.isclose(getattr(current, field.name), expected, rel_tol=1e-12, abs_tol=1e-9), (phase, field.name, expected, getattr(current, field.name)))
    player = _call(test, "CSPlayer", {}, login, "SCPlayer").player
    test.assertEqual(player.fightPower, panel.fightPower)
    test.assertGreaterEqual(panel.fightPower, base.fightPower)
    test.p5_prayer_checks["panels"].append({"player": login.playerId, "phase": phase, "basePower": base.fightPower, "power": panel.fightPower, "percent": {f.name: v for f, v in prop.ListFields()}})
    return panel, player


def _verify_frozen(test, login, phase):
    first = test._p5_prayer_frozen.get(login.playerId)
    if first is None:
        return
    again = _call(test, "CSCopyFightStart", {"chapterId": "1", "missionId": "1", "taskId": "1"}, login, "SCCopyFightStart")
    test.assertEqual(again.SerializeToString(deterministic=True), first.SerializeToString(deterministic=True), "old original battle input changed after " + phase)


def after_info(test, login, index, info):
    """Check current percent replacement and capture the first real CopyStart."""
    test.assertIn(login.playerId, test._p5_prayer_bases)
    panel, player = _verify_current(test, login, info.addProp, "pray" + str(index))
    if login.playerId not in test._p5_prayer_frozen:
        before = player.stamina
        _call(test, "CSCopyEnter", {"chapterId": "1", "missionId": "1"}, login, "SCCopyEnter")
        entered = _call(test, "CSPlayer", {}, login, "SCPlayer").player
        test.assertEqual(before - entered.stamina, 10, "original chapter1 entry cost changed")
        battle = _call(test, "CSCopyFightStart", {"chapterId": "1", "missionId": "1", "taskId": "1"}, login, "SCCopyFightStart")
        owned = [actor for actor in battle.battlePlayer.formation if actor.guid == entered.heroWarId]
        test.assertEqual(len(owned), 1)
        test.assertEqual(owned[0].prop, panel.formationItemInfoList[0].warInfo.baseProp)
        test._p5_prayer_frozen[login.playerId] = battle
        wire = battle.SerializeToString(deterministic=True)
        test.p5_prayer_checks["frozenInputs"].append({"player": login.playerId, "fightId": battle.fightId, "originalStartSHA256": hashlib.sha256(wire).hexdigest(), "staminaCost": 10, "winnerClaimed": False})
    _verify_frozen(test, login, "pray" + str(index))


def after_cold(test, login, info):
    _verify_current(test, login, info.addProp, "cold")
    _verify_frozen(test, login, "cold")


def after_leave(test, login, response):
    _verify_current(test, login, response.unionPrayProp, "leave")
    test.assertFalse(response.unionPrayProp.ListFields())
    _verify_frozen(test, login, "leave")
    test.p5_prayer_checks["inactive"].append({"player": login.playerId, "phase": "actual-guild-leave"})


def after_expiry(test, login, info):
    _verify_current(test, login, info.addProp, "expiry")
    test.assertFalse(info.addProp.ListFields())
    _verify_frozen(test, login, "expiry")
    test.p5_prayer_checks["inactive"].append({"player": login.playerId, "phase": "actual-24h-expiry"})
