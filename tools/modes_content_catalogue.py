"""Inspect recovered single-player content without playing battles or using DB.

The manifest separates original catalogues, usable configuration references and
missing server-side rules. A matching icon never substitutes for a boss roster.
"""
import argparse
import collections
import csv
import hashlib
import json
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def rows(name):
    with (ROOT / "analysis/tables" / f"Table{name}.tsv").open() as stream:
        values = list(csv.reader(stream, delimiter="\t"))
    return [dict(zip(values[1], value)) for value in values[5:] if value and value[0]]


def sha(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def audit(apk):
    tables = {name: rows(name) for name in (
        "CopyChapter", "CopyMission", "CopyTask", "CopyEnemy", "StoryQuest",
        "Hero", "Skill", "RushEliteBoss", "ExpeditionMission", "ExpeditionLevel",
        "ExpeditionStory", "GameConfigure", "Text")}
    names = {row["id"]: row["text"] for row in tables["Text"]}
    heroes = {row["id"] for row in tables["Hero"]}
    skills = {row["SkillId"] for row in tables["Skill"]}
    chapters = {row["ChapterId"]: row for row in tables["CopyChapter"]}
    missions = {row["MissionId"]: row for row in tables["CopyMission"]}
    quests = {row["questId"]: row for row in tables["StoryQuest"]}
    tasks = collections.defaultdict(list)
    enemies = collections.defaultdict(list)
    failures = []
    scene_references = []
    for task in tables["CopyTask"]:
        tasks[task["MissionId"]].append(task)
    for enemy in tables["CopyEnemy"]:
        enemies[enemy["TaskId"]].append(enemy)
    with zipfile.ZipFile(apk) as archive:
        scene_files = {name.rsplit("/", 1)[-1] for name in archive.namelist()
                       if name.startswith("assets/android/map/")}

    def check(condition, description):
        if not condition:
            failures.append(description)

    copy = []
    for mission in tables["CopyMission"]:
        chapter, mission_id = mission["ChapterId"], mission["MissionId"]
        check(chapter in chapters, f"mission {mission_id}: missing chapter {chapter}")
        check(not mission["OpenId"] or mission["OpenId"] in quests,
              f"mission {mission_id}: missing opening quest {mission['OpenId']}")
        check(bool(tasks[mission_id]), f"mission {mission_id}: no task")
        if mission["SceneName"] not in scene_files:
            scene_references.append({"kind": "mission", "id": mission_id, "scene": mission["SceneName"]})
        entries = []
        for task in tasks[mission_id]:
            task_id = task["TaskId"]
            if task["SceneName"] not in scene_files:
                scene_references.append({"kind": "task", "id": task_id, "scene": task["SceneName"]})
            check(bool(enemies[task_id]), f"task {task_id}: no enemy")
            wave_ids = sorted({int(enemy["GroupIndex"]) for enemy in enemies[task_id]})
            for enemy in enemies[task_id]:
                check(enemy["HeroId"] in heroes, f"enemy {enemy['CopyEnemyId']}: missing actor")
                for key in ("SkillId1", "SkillId2", "UltSkillId"):
                    check(not enemy[key] or enemy[key] in skills,
                          f"enemy {enemy['CopyEnemyId']}: missing skill {enemy[key]}")
                check(int(enemy["Hp"]) > 0 and int(enemy["Level"]) > 0,
                      f"enemy {enemy['CopyEnemyId']}: invalid HP/level")
            entries.append({"taskId": task_id, "scene": task["SceneName"],
                            "waveIds": wave_ids, "enemyCount": len(enemies[task_id]),
                            "enemyIds": [enemy["CopyEnemyId"] for enemy in enemies[task_id]],
                            "actors": sorted({enemy["HeroId"] for enemy in enemies[task_id]})})
        copy.append({"chapterId": chapter, "missionId": mission_id,
                     "name": names.get(mission["MissionName"], mission["MissionName"]),
                     "scene": mission["SceneName"], "openingQuest": mission["OpenId"],
                     "powerCost": int(mission["PowerCost"]), "tasks": entries})

    elite = []
    for row in tables["RushEliteBoss"]:
        elite.append({"id": row["EliteTaskId"], "name": names.get(row["EliteBossName"]),
                      "description": names.get(row["EliteBossDesc"]), "icon": row["IconName"],
                      "firstReward": row["FirstReward"], "reward": row["Reward"],
                      "conditionTaskId": row["ConditionTaskId"],
                      "missing": ["original enemy formation/stats/skills", "battle scene"]})
    expedition = []
    level = {row["level"]: row for row in tables["ExpeditionLevel"]}
    for row in tables["ExpeditionMission"]:
        grade = row["level"]
        hours = int(row["time"])
        check(grade in level, f"expedition {row['missionId']}: unknown grade")
        check(hours in (4, 8, 12), f"expedition {row['missionId']}: unsupported duration")
        check(row["missionType"] in ("guard", "hunt", "collect", "kill", "war", "search"),
              f"expedition {row['missionId']}: unknown type")
        expedition.append({"id": row["missionId"], "name": row["name"], "grade": grade,
                           "type": row["missionType"], "hours": hours,
                           "experience": hours * int(level[grade]["expPerHour"]),
                           "maxHeroes": int(level[grade]["maxHeroNum"]),
                           "regularConsumerImplemented": grade != "SS",
                           "missing": ["SS publication trigger/probability"] if grade == "SS" else []})
    grades = collections.Counter(row["grade"] for row in expedition)
    for grade in ("D", "C", "B", "A", "S", "SS"):
        check(grades[grade] == 18, f"expedition {grade}: expected18 got{grades[grade]}")
    story = []
    for row in tables["ExpeditionStory"]:
        value = int(row["storyId"].rsplit("_", 1)[-1])
        group = ("ordinary" if value <= 73 else "lucky" if value <= 92 else
                 "special-random-reward" if value <= 97 else "risk" if value <= 103 else
                 "friend-help" if value == 104 else "failed" if value == 105 else "rare-item")
        story.append({"id": row["storyId"], "group": group,
                      "rewardId": row["rewardId"], "description": row["storyDesc"],
                      "consumerImplemented": group not in ("special-random-reward", "rare-item")})
    return {"kind": "static-content-index; no battle/runtime/DB verification",
            "sources": {str((ROOT / "analysis/tables" / f"Table{name}.tsv").relative_to(ROOT)):
                        sha(ROOT / "analysis/tables" / f"Table{name}.tsv") for name in tables},
            "apk": str(apk), "apkSHA256": sha(apk), "summary": {
                "copyChapters": len(chapters), "copyMissions": len(copy),
                "copyReachableTasks": sum(len(row["tasks"]) for row in copy),
                "copyReachableWaves": sum(len(task["waveIds"]) for row in copy for task in row["tasks"]),
                "copyReachableEnemies": sum(task["enemyCount"] for row in copy for task in row["tasks"]),
                "storyQuests": len(quests), "eliteBosses": len(elite),
                "expeditionMissions": len(expedition), "expeditionGradeCounts": dict(grades),
                "expeditionStories": len(story),
                "storyGroups": dict(collections.Counter(row["group"] for row in story)),
                "missingNamedSceneBundleReferences": len(scene_references),
                "missingNamedSceneBundles": sorted({row["scene"] for row in scene_references}),
                "validationFailures": len(failures)},
            "copy": copy, "elite": elite, "expedition": expedition,
            "expeditionStories": story,
            "trial": {"implementedLocalFloors": [1, 2, 3, 4, 5, 6],
                      "starOptionsPerFloor": [1, 2, 3],
                      "missingOriginal": ["total floor count", "floor enemy formations",
                                          "per-floor difficulty and winning criteria"],
                      "knownCriteria": {"1": "round limit", "2": "remaining HP threshold",
                                        "3": "casualty limit"},
                      "criteriaEvidence": "analysis/p7/rush.il.txt UI.RushPage.ParseWinCondition",
                      "criteriaConsumer": "nonzero criteria gated in p6RushSeriesService.go"},
            "sceneBundleBoundary": "Exact ZIP bundle names only; no Unity load test, bundle-internal scene enumeration or hot-update inspection. Missing names require alias/resource follow-up.",
            "missingNamedSceneBundleReferences": scene_references,
            "failures": failures}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apk", type=Path, default=ROOT / "apks/original/10056262.apk")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    report = audit(args.apk.resolve())
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report["summary"], ensure_ascii=False))
    if report["failures"]:
        print(json.dumps(report["failures"], ensure_ascii=False))
        raise SystemExit(1)


if __name__ == "__main__":
    main()
