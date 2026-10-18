package com.hyjs.wallet;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.DialogInterface;
import android.graphics.Color;
import android.text.InputType;
import android.view.View;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;
import android.widget.ScrollView;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URL;
import org.json.JSONObject;

/** Explicit registration, opened only by the original login registration button. */
public final class GameRegistration {
    private static AlertDialog current;
    private GameRegistration() { }

    public static void show(final String serverBase) {
        try {
            final Activity activity = (Activity) Class.forName("com.unity3d.player.UnityPlayer")
                    .getField("currentActivity").get(null);
            if (activity == null) return;
            activity.runOnUiThread(new Runnable() { public void run() { open(activity, serverBase); } });
        } catch (Exception unavailable) { /* No credentials or exception details are logged. */ }
    }

    private static int dp(Activity activity, int value) {
        return Math.round(value * activity.getResources().getDisplayMetrics().density);
    }

    private static EditText field(Activity activity, LinearLayout body, String hint, boolean password) {
        EditText field = new EditText(activity);
        field.setSingleLine(true);
        field.setHint(hint);
        field.setTextSize(16);
        field.setInputType(InputType.TYPE_CLASS_TEXT | (password ? InputType.TYPE_TEXT_VARIATION_PASSWORD : InputType.TYPE_TEXT_VARIATION_NORMAL));
        field.setImeOptions(EditorInfo.IME_ACTION_NEXT | EditorInfo.IME_FLAG_NO_EXTRACT_UI);
        body.addView(field, new LinearLayout.LayoutParams(-1, dp(activity, 52)));
        return field;
    }

    private static void open(final Activity activity, final String serverBase) {
        if (activity.isFinishing() || current != null && current.isShowing()) return;
        LinearLayout body = new LinearLayout(activity);
        body.setOrientation(LinearLayout.VERTICAL);
        body.setPadding(dp(activity,24), dp(activity,4), dp(activity,24), dp(activity,8));
        final EditText username = field(activity, body, "账号：3–32位字母、数字、_ 或 -", false);
        final EditText password = field(activity, body, "密码：至少6个字符，最多72字节", true);
        final EditText confirmation = field(activity, body, "确认密码", true);
        confirmation.setImeOptions(EditorInfo.IME_ACTION_DONE | EditorInfo.IME_FLAG_NO_EXTRACT_UI);
        final TextView message = new TextView(activity);
        message.setTextColor(Color.rgb(183,40,40));
        message.setTextSize(14);
        body.addView(message, new LinearLayout.LayoutParams(-1,-2));
        ScrollView scroll = new ScrollView(activity);
        scroll.addView(body);
        final AlertDialog dialog = new AlertDialog.Builder(activity).setTitle("注册账号")
                .setView(scroll).setNegativeButton("返回登录", null).setPositiveButton("注册", null).create();
        current = dialog;
        dialog.setCanceledOnTouchOutside(false);
        dialog.setOnDismissListener(new DialogInterface.OnDismissListener() { public void onDismiss(DialogInterface ignored) {
            username.setText(""); password.setText(""); confirmation.setText("");
            if (current == dialog) current = null;
        } });
        dialog.setOnShowListener(new DialogInterface.OnShowListener() { public void onShow(DialogInterface ignored) {
            dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(new View.OnClickListener() { public void onClick(View view) {
                final String name = username.getText().toString();
                final String secret = password.getText().toString();
                final String confirm = confirmation.getText().toString();
                if (!name.matches("[A-Za-z0-9_-]{3,32}")) { message.setText("账号需为3–32位字母、数字、下划线或短横线"); return; }
                try {
                    if (secret.codePointCount(0,secret.length()) < 6 || secret.getBytes("UTF-8").length > 72) {
                        message.setText("密码至少6个字符，最多72字节"); return;
                    }
                } catch (Exception invalid) { message.setText("密码格式无效"); return; }
                if (!secret.equals(confirm)) { message.setText("两次输入的密码不一样"); return; }
                message.setText("正在注册…");
                dialog.setCancelable(false);
                dialog.getButton(AlertDialog.BUTTON_POSITIVE).setEnabled(false);
                dialog.getButton(AlertDialog.BUTTON_NEGATIVE).setEnabled(false);
                username.setEnabled(false); password.setEnabled(false); confirmation.setEnabled(false);
                new Thread(new Runnable() { public void run() {
                    final Result result = register(serverBase, name, secret, confirm);
                    activity.runOnUiThread(new Runnable() { public void run() {
                        if (activity.isFinishing() || !dialog.isShowing()) return;
                        if (result.success) {
                            dialog.dismiss();
                            Toast.makeText(activity,"注册成功，请使用刚才的账号和密码登录",Toast.LENGTH_LONG).show();
                        } else {
                            dialog.setCancelable(true);
                            dialog.getButton(AlertDialog.BUTTON_POSITIVE).setEnabled(true);
                            dialog.getButton(AlertDialog.BUTTON_NEGATIVE).setEnabled(true);
                            username.setEnabled(true); password.setEnabled(true); confirmation.setEnabled(true);
                            message.setText(result.message);
                        }
                    } });
                } },"hyjs-register").start();
            } });
        } });
        dialog.show();
        dialog.getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
    }

    private static final class Result {
        final boolean success; final String message;
        Result(boolean success, String message) { this.success=success; this.message=message; }
    }

    private static Result register(String serverBase, String name, String password, String confirmation) {
        HttpURLConnection connection = null;
        try {
            URI base = new URI(serverBase);
            if (!"http".equals(base.getScheme()) && !"https".equals(base.getScheme())) throw new IllegalArgumentException();
            URL endpoint = new URI(base.getScheme(),null,base.getHost(),base.getPort(),"/game-account/register",null,null).toURL();
            connection=(HttpURLConnection)endpoint.openConnection();
            connection.setConnectTimeout(10000); connection.setReadTimeout(10000);
            connection.setRequestMethod("POST"); connection.setDoOutput(true);
            connection.setInstanceFollowRedirects(false);
            connection.setRequestProperty("Content-Type","application/json; charset=utf-8");
            byte[] payload=new JSONObject().put("username",name).put("password",password).put("confirmPassword",confirmation).toString().getBytes("UTF-8");
            connection.setFixedLengthStreamingMode(payload.length);
            try (OutputStream stream=connection.getOutputStream()) { stream.write(payload); }
            int status=connection.getResponseCode();
            InputStream incoming=status>=400 ? connection.getErrorStream() : connection.getInputStream();
            if (incoming==null) return new Result(false,"注册服务暂时不可用，请稍后重试");
            ByteArrayOutputStream bytes=new ByteArrayOutputStream();
            try (InputStream stream=incoming) {
                byte[] chunk=new byte[1024]; int count;
                while ((count=stream.read(chunk))!=-1) {
                    if (bytes.size()+count>16384) throw new IllegalArgumentException();
                    bytes.write(chunk,0,count);
                }
            }
            JSONObject reply=new JSONObject(new String(bytes.toByteArray(),"UTF-8"));
            JSONObject data=reply.optJSONObject("data");
            boolean success=status==200 && reply.optInt("code",-1)==0 && data!=null && data.optBoolean("registered",false) && name.equals(data.optString("username"));
            String error=reply.optString("msg","注册失败，请重试");
            return new Result(success,error.isEmpty()?"注册失败，请重试":error);
        } catch (Exception unavailable) { return new Result(false,"注册服务暂时不可用，请稍后重试"); }
        finally { if (connection!=null) connection.disconnect(); }
    }
}
