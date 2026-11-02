using System;
using System.Globalization;
using NetWork.Packets;
using ProtoBuf.Message;
using UnityEngine;

namespace HYJS.LuxuryDragon
{
    public sealed class Runtime : MonoBehaviour
    {
        static Runtime host;
        static bool wanted, inFlight;
        static float until, next, requestUntil;
        static long generation;

        public static void Schedule()
        {
            try
            {
                if (host == null)
                {
                    GameObject owner = new GameObject("HYJSLuxuryDragon");
                    DontDestroyOnLoad(owner);
                    host = owner.AddComponent<Runtime>();
                }
                wanted = true;
                until = Time.realtimeSinceStartup + 20f;
            }
            catch (Exception) { wanted = false; }
        }

        void Update() { Tick(); }

        public static void Tick()
        {
            if (!wanted) return;
            float now = Time.realtimeSinceStartup;
            if (now >= until) { wanted = false; inFlight = false; generation++; return; }
            if (inFlight && now >= requestUntil) { inFlight = false; generation++; }
            if (inFlight || now < next) return;
            UI.PageManager pages = UI.PageManager.Instance;
            if (pages == null || !(pages.currentPage is UI.UICopy)) return;
            if (StaticData.playerData == null || StaticData.playerData.playerId <= 0 ||
                StaticData.sessionKey == null || StaticData.sessionKey.Length != 16)
            { wanted = false; return; }
            long player = StaticData.playerData.playerId;
            byte[] session = (byte[])StaticData.sessionKey.Clone();
            string origin = StaticData.httpServer ?? String.Empty;
            long request = ++generation;
            inFlight = true;
            requestUntil = now + 5f;
            try
            {
                bool sent = NetAPI.SendHttp(57107, new CSLuxuryEquipmentInfo(), delegate(Packet packet, bool success)
                { Received(request, player, session, origin, packet, success); });
                if (!sent && generation == request) { inFlight = false; next = now + 0.25f; }
            }
            catch (Exception) { if (generation == request) { inFlight = false; next = now + 0.25f; } }
        }

        static bool SameSession(long player, byte[] session, string origin)
        {
            if (StaticData.playerData == null || StaticData.playerData.playerId != player ||
                (StaticData.httpServer ?? String.Empty) != origin || StaticData.sessionKey == null ||
                StaticData.sessionKey.Length != session.Length) return false;
            for (int i = 0; i < session.Length; i++) if (StaticData.sessionKey[i] != session[i]) return false;
            return true;
        }

        static void Received(long request, long player, byte[] session, string origin, Packet packet, bool success)
        {
            if (!wanted || !inFlight || request != generation) return;
            inFlight = false;
            if (!SameSession(player, session, origin)) { wanted = false; generation++; return; }
            SCLuxuryEquipmentInfo info = success && packet != null ? packet.kBody as SCLuxuryEquipmentInfo : null;
            if (info == null) { next = Time.realtimeSinceStartup + 0.25f; return; }
            UI.PageManager pages = UI.PageManager.Instance;
            if (pages == null || !(pages.currentPage is UI.UICopy)) { next = Time.realtimeSinceStartup + 0.25f; return; }
            wanted = false;
            if (info.id != 2 || info.rewardState == 2 || (info.rewardState == 0 && info.endTime <= info.currentTime))
            { StaticData.isOpeningLuxuryEquipActivity = false; return; }
            if (info.rewardState != 0 && info.rewardState != 1) return;
            StaticData.isOpeningLuxuryEquipActivity = true;
            string key = "HYJS.LuxuryDragon." + origin + "." + player.ToString(CultureInfo.InvariantCulture);
            string window = info.endTime.ToString(CultureInfo.InvariantCulture);
            if (PlayerPrefs.GetString(key, String.Empty) == window) return;
            try
            {
                string options = String.Format(CultureInfo.InvariantCulture, "id={0}&endTime={1}&rewardState={2}", info.id, info.endTime, info.rewardState);
                pages.OpenPage("LuxuryPage", options);
                PlayerPrefs.SetString(key, window);
                PlayerPrefs.Save();
            }
            catch (Exception) { /* A later successful action can retry. */ }
        }
    }
}
