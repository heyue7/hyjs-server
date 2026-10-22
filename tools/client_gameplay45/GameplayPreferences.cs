using System;
using Battle;
using UnityEngine;

namespace HYJS.Gameplay
{
    // This helper stores only the user's speed choice on this device. Gameplay,
    // qualification, battle outcomes and account state stay with original code.
    public static class Preferences
    {
        static string Key()
        {
            if (StaticData.playerData == null || StaticData.playerData.playerId <= 0) return null;
            return "hyjs.battle.speed.v1." + StaticData.playerData.ZoneId + "." + StaticData.playerData.playerId;
        }

        public static void RememberClick()
        {
            try
            {
                string key = Key();
                if (key == null || BattleManager.Instance == null) return;
                int speed = (int)BattleManager.Instance.curSpeedEnum;
                if (speed != 0 && speed != 1) return;
                PlayerPrefs.SetInt(key, speed);
                PlayerPrefs.Save();
            }
            catch (Exception error) { Debug.LogWarning("Battle preference unavailable: " + error.GetType().Name); }
        }

        public static void Restore()
        {
            try
            {
                string key = Key();
                if (key == null || BattleManager.Instance == null || StaticData.curBattleData == null) return;
                int speed = PlayerPrefs.GetInt(key, 0) == 1 ? 1 : 0;
                int type = (int)StaticData.curBattleData.battleType;
                // Match the original button's level/VIP gate for these modes.
                if (speed == 1 && (type == 1 || type == 6 || type == 12 || type == 18 || type == 16) &&
                    StaticData.playerData.level < Dict.Blo.DictConfigBlo.GetSpeed2LimitLevel() &&
                    StaticData.playerData.vip < Dict.Blo.DictConfigBlo.GetSpeed2LimitVip()) speed = 0;
                BattleSpeed choice = (BattleSpeed)speed;
                StaticData.pveSpeedCache = choice;
                StaticData.pvpSpeedCache = choice;
                StaticData.pluderSpeedCache = choice;
                BattleManager.Instance.SetSpeed(choice);
                // Refresh only original speed buttons; this never records a
                // programmatic reset or another account's previous cache.
                foreach (SpeedBtn button in UnityEngine.Object.FindObjectsOfType(typeof(SpeedBtn)))
                    if (button.gameObject.activeInHierarchy) button.Refresh();
            }
            catch (Exception error) { Debug.LogWarning("Battle preference unavailable: " + error.GetType().Name); }
        }

        public static void ApplySnowBattleMap(BattleData data)
        {
            if (data == null || StaticData.fieldId != "field_002") return;
            int type = (int)data.battleType;
            // The original ice bundle is absent. This snow battle bundle is
            // already shipped, and is used by the inspected compatibility path.
            if (type == 6 || type == 12) data.mapSceneName = "map_snowforest_001";
        }
    }
}
