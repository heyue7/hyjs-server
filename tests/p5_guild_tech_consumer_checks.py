"""P5 tech consumers attached to the owner's one actual guild funding/upgrade chain."""
import hashlib
import math


def _call(test, name, fields, login, reply):
    test._p5_tech_stamp = getattr(test, "_p5_tech_stamp", 3000000) + 1
    return test.call(name, fields, login, test._p5_tech_stamp, reply)


def _panel(test, login):
    return _call(test, "CSFindPlayerInfo", {"playerId": login.playerId, "serverId": 1}, login, "SCFindPlayerInfo").formationInfo


def before_tech(test, a, b, c, other=None):
    test._p5_tech_bases = {}
    test._p5_tech_frozen = {}
    test.p5_tech_checks = {"panels": [], "frozenInputs": [], "inactive": []}
    for login in [a, b, c] + ([other] if other is not None else []):
        panel = _panel(test, login)
        test.assertEqual(len(panel.formationItemInfoList), 1)
        test._p5_tech_bases[login.playerId] = panel


def _verify_current(test, login, level, phase):
    base = test._p5_tech_bases[login.playerId]
    panel = _panel(test, login)
    test.assertEqual(len(panel.formationItemInfoList), 1)
    original = base.formationItemInfoList[0].warInfo.baseProp
    current = panel.formationItemInfoList[0].warInfo.baseProp
    # This native publication upgrades original peratt only. The P5 domain
    # test covers all eight actual types, including the four constant units.
    for field in original.DESCRIPTOR.fields:
        expected = getattr(original, field.name)
        if field.name == "att":
            expected *= 1 + level * .01
        test.assertTrue(math.isclose(getattr(current, field.name), expected, rel_tol=1e-12, abs_tol=1e-9), (phase, field.name, expected, getattr(current, field.name)))
    player = _call(test, "CSPlayer", {}, login, "SCPlayer").player
    test.assertEqual(player.fightPower, panel.fightPower)
    test.p5_tech_checks["panels"].append({"player": login.playerId, "phase": phase, "basePower": base.fightPower, "power": panel.fightPower, "actualPerattPercentPoints": level})
    return panel, player


def _frozen(test, login, phase):
    first = test._p5_tech_frozen.get(login.playerId)
    if first is None:
        return
    again = _call(test, "CSCopyFightStart", {"chapterId": "1", "missionId": "1", "taskId": "1"}, login, "SCCopyFightStart")
    test.assertEqual(again.SerializeToString(deterministic=True), first.SerializeToString(deterministic=True), "original frozen input changed after " + phase)


def after_level(test, a, b, level, info):
    for login in [a, b]:
        panel, player = _verify_current(test, login, level, "techLevel" + str(level))
        if login.playerId not in test._p5_tech_frozen:
            before = player.stamina
            _call(test, "CSCopyEnter", {"chapterId": "1", "missionId": "1"}, login, "SCCopyEnter")
            entered = _call(test, "CSPlayer", {}, login, "SCPlayer").player
            test.assertEqual(before - entered.stamina, 10)
            battle = _call(test, "CSCopyFightStart", {"chapterId": "1", "missionId": "1", "taskId": "1"}, login, "SCCopyFightStart")
            actors = [v for v in battle.battlePlayer.formation if v.guid == entered.heroWarId]
            test.assertEqual(len(actors), 1)
            test.assertEqual(actors[0].prop, panel.formationItemInfoList[0].warInfo.baseProp)
            test._p5_tech_frozen[login.playerId] = battle
            test.p5_tech_checks["frozenInputs"].append({"player": login.playerId, "fightId": battle.fightId, "originalStartSHA256": hashlib.sha256(battle.SerializeToString(deterministic=True)).hexdigest(), "staminaCost": 10, "winnerClaimed": False})
        _frozen(test, login, "techLevel" + str(level))


def after_cold(test, a, b, info):
    for login in [a, b]:
        _verify_current(test, login, 2, "cold-and-closed-future-policy")
        _frozen(test, login, "cold")


def _inactive(test, login, phase):
    # Read persisted Player before any current query can repair stale raw FP.
    raw = test.profile(login)[0]
    expected = test._p5_tech_bases[login.playerId].fightPower
    test.assertEqual(raw["fightPower"], expected, (phase, "raw Player cache", raw["fightPower"], expected))
    _verify_current(test, login, 0, phase)
    _frozen(test, login, phase)
    test.p5_tech_checks["inactive"].append({"player": login.playerId, "phase": phase, "rawPowerBeforeQuery": raw["fightPower"]})


def after_kick(test, b):
    _inactive(test, b, "actual-remote-kick-no-prayer-history")


def after_leave(test, a):
    _inactive(test, a, "actual-chair-transfer-and-leave")


def after_dissolve(test, c, other):
    _inactive(test, c, "actual-dissolve-chairman")
    _inactive(test, other, "actual-dissolve-offline-member")
