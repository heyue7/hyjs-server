using System;
using UnityEngine;

namespace HYJS.ClientUpdate
{
    // Version() is invoked by original login code on Unity's main thread.
    public static class Bridge
    {
        static Gate gate;
        public static string Version(GameConfig original) { return Version(); }
        public static string DisplayVersion(GameConfig original)
        {
            Version();
            using (AndroidJavaClass java = new AndroidJavaClass("com.hyjs.wallet.ClientUpdate"))
                return java.CallStatic<string>("displayVersion");
        }
        public static string Version()
        {
            if (gate == null)
            {
                GameObject go = new GameObject("HYJSClientUpdateGate");
                UnityEngine.Object.DontDestroyOnLoad(go);
                gate = go.AddComponent<Gate>();
            }
            using (AndroidJavaClass java = new AndroidJavaClass("com.hyjs.wallet.ClientUpdate"))
                return java.CallStatic<string>("version");
        }
    }
    public sealed class Gate : MonoBehaviour
    {
        float next;
        bool returned;
        bool LoginVisible()
        {
            // Inspect the shipped LoginUI component, independent of prefab hierarchy.
            foreach (LoginUI login in UnityEngine.Object.FindObjectsOfType(typeof(LoginUI)))
                if (login.gameObject.activeInHierarchy) return true;
            return false;
        }
        bool SafeForNotice()
        {
            // Keep notices pending throughout battle playback and result screens.
            if (Battle.BattleManager.Instance != null && Battle.BattleManager.Instance.battleEngine != null) return false;
            if (LoginVisible()) return true;
            // A visible original town root proves a settled navigation screen.
            // Unknown/loading/field/settlement scenes defer until a known safe screen.
            if (!(Room.RoomManager.currentRoom is Room.RoomTown)) return false;
            foreach (UI.TownPage page in UnityEngine.Object.FindObjectsOfType(typeof(UI.TownPage)))
                if (page.gameObject.activeInHierarchy) return true;
            return false;
        }
        void Update()
        {
            if (Time.realtimeSinceStartup < next) return;
            next = Time.realtimeSinceStartup + 0.5f;
            try
            {
                using (AndroidJavaClass java = new AndroidJavaClass("com.hyjs.wallet.ClientUpdate"))
                {
                    bool force = java.CallStatic<bool>("blocked");
                    if (force)
                    {
                        if (!returned)
                        {
                            using (AndroidJavaClass wallet = new AndroidJavaClass("com.hyjs.wallet.WalletActivity"))
                                wallet.CallStatic("bindSession", (long)0, "");
                            if (!LoginVisible()) ManagerController.Instance.BackToLogin();
                            returned = true;
                        }
                        // Keep forced policy at login even if another original callback navigates away.
                        if (!LoginVisible()) { ManagerController.Instance.BackToLogin(); return; }
                    }
                    else returned = false;
                    if (force || SafeForNotice())
                    {
                        string pending = java.CallStatic<string>("pending");
                        if (!String.IsNullOrEmpty(pending)) java.CallStatic("display", pending);
                    }
                }
            }
            catch (Exception error) { Debug.LogWarning("Client update gate unavailable: " + error.GetType().Name); }
        }
    }
}
