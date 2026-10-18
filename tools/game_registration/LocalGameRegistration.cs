using System;
using System.Collections;
using System.Collections.Generic;
using System.Text;
using UnityEngine;
using LitJson;

// Uses the shipped NGUI login/registration fields and the inherited modern IME.
// Registration never substitutes an ordinary login or writes StaticData credentials.
public static class LocalGameRegistration
{
    private static RegistUI pending;

    private static UIInput Input(GameObject value)
    {
        return value == null ? null : value.GetComponent<UIInput>();
    }

    private static void Notice(string value)
    {
        BoxManager.CreatePopupTextBox(value, 0f);
    }

    public static void Open(UserInput login)
    {
        if (login == null || login.loginUI == null) return;
        RegistUI[] panels = login.loginUI.transform.root.GetComponentsInChildren<RegistUI>(true);
        RegistUI panel = null;
        foreach (RegistUI candidate in panels)
            if (candidate.loginUI == login.loginUI) { panel = candidate; break; }
        if (panel == null) { Notice("注册界面暂时无法打开，请重试"); return; }
        if (pending != null) { Notice("正在注册，请稍候"); return; }
        UIInput name = Input(panel.userNameInput);
        UIInput password = Input(panel.passWordInput);
        UIInput confirmation = Input(panel.surePassInput);
        if (name == null || password == null || confirmation == null)
        { Notice("注册输入框暂时无法使用，请重试"); return; }
        panel.gameObject.SetActive(true);
        login.gameObject.SetActive(false);
        name.characterLimit = 32;
        name.validation = UIInput.Validation.None;
        name.value = "";
        password.characterLimit = 72;
        confirmation.characterLimit = 72;
        password.value = "";
        confirmation.value = "";
        panel.InitPassType();
    }

    public static void Close(RegistUI panel)
    {
        if (panel == null) return;
        if (pending == panel) { Notice("正在注册，请稍候"); return; }
        Input(panel.passWordInput).value = "";
        Input(panel.surePassInput).value = "";
        panel.gameObject.SetActive(false);
        UserInput login = panel.userInputPanel == null ? null : panel.userInputPanel.GetComponent<UserInput>();
        if (login != null) login.Show();
    }

    public static void Submit(RegistUI panel)
    {
        if (panel == null || pending != null) return;
        string name = Input(panel.userNameInput).value;
        string password = Input(panel.passWordInput).value;
        string confirmation = Input(panel.surePassInput).value;
        if (name.Length < 3 || name.Length > 32)
        { Notice("账号需为3–32位字母、数字、下划线或短横线"); return; }
        foreach (char c in name)
            if (!((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-'))
            { Notice("账号仅支持字母、数字、下划线或短横线"); return; }
        if (password.Length < 6 || Encoding.UTF8.GetByteCount(password) > 72)
        { Notice("密码至少6个字符，最多72字节"); return; }
        if (password != confirmation) { Notice("两次输入的密码不一样"); return; }
        pending = panel;
        panel.StartCoroutine(Register(panel, name, password, confirmation));
    }

    private static IEnumerator Register(RegistUI panel, string name, string password, string confirmation)
    {
        WWW request = null;
        try
        {
            Dictionary<string, string> fields = new Dictionary<string, string>();
            fields["username"] = name;
            fields["password"] = password;
            fields["confirmPassword"] = confirmation;
            Dictionary<string, string> headers = new Dictionary<string, string>();
            headers["Content-Type"] = "application/json; charset=utf-8";
            string endpoint = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority) + "/game-account/register";
            request = new WWW(endpoint, Encoding.UTF8.GetBytes(JsonMapper.ToJson(fields)), headers);
            float deadline = Time.realtimeSinceStartup + 15f;
            while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            if (!request.isDone) { Notice("注册请求超时，请重试"); yield break; }
            bool success = false;
            string message = "注册失败，请重试";
            try
            {
                JsonData reply = JsonMapper.ToObject(request.text);
                if (((IDictionary)reply).Contains("msg")) message = (string)reply["msg"];
                success = request.error == null && (int)reply["code"] == 0 &&
                    (bool)reply["data"]["registered"] && (string)reply["data"]["username"] == name;
            }
            catch (Exception) { message = "注册服务暂时不可用，请稍后重试"; }
            if (!success)
            {
                Notice(String.IsNullOrEmpty(message) ? "注册失败，请重试" : message);
                yield break;
            }
            // Clear both password fields before returning to the original login.
            // The user explicitly logs in afterwards through the existing CSUserLogin.
            pending = null;
            Close(panel);
            UserInput login = panel.userInputPanel.GetComponent<UserInput>();
            if (login != null) Input(login.nameInput).value = name;
            Notice("注册成功，请输入密码登录");
        }
        finally
        {
            if (request != null) request.Dispose();
            if (pending == panel) pending = null;
            password = null;
            confirmation = null;
        }
    }
}
