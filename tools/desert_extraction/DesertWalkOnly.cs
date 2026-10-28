using UnityEngine;
using System;
using System.Reflection;

namespace HYJS.Desert53
{
    public static class DesertWalkOnly
    {
        public static bool ResumeLogin()
        {
            Assembly game = null;
            foreach (Assembly candidate in AppDomain.CurrentDomain.GetAssemblies())
                if (candidate.GetName().Name == "Assembly-CSharp") { game = candidate; break; }
            if (game == null) throw new InvalidOperationException("Original game assembly unavailable");
            Type login = game.GetType("Login.LoginManager", true);
            string scene = (string)login.GetField("willLoginScene", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static).GetValue(null);
            if (scene != "field_003") return false;
            Type page = game.GetType("UI.UIFieldRunMap", true);
            page.GetField("fieldId", BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static).SetValue(null, String.Empty);
            Type controller = game.GetType("ManagerController", true);
            object manager = controller.GetProperty("Instance", BindingFlags.Public | BindingFlags.Static).GetValue(null, null);
            controller.GetMethod("LoginToField", new Type[] { typeof(string) }).Invoke(manager, new object[] { "town_side_field_003" });
            return true;
        }
        // Only the rebuilt desert scene suppresses content. Other field scenes
        // retain the original handlers, character creation and resource queues.
        public static bool IsActive()
        {
            return GameObject.Find("map_desertfield_001") != null;
        }
    }
}
