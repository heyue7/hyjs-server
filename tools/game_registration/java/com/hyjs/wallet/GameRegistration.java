package com.hyjs.wallet;

import android.app.Activity;
import android.app.Dialog;
import android.widget.Button;
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
    private static Dialog current;
    private GameRegistration() { }

    public static void show(final String serverBase) {
        // An explicit registration entry cancels queued automatic attempts.
        LoginCache.pause();
        try {
            final Activity activity = (Activity) Class.forName("com.unity3d.player.UnityPlayer")
                    .getField("currentActivity").get(null);
            if (activity == null) return;
            activity.runOnUiThread(new Runnable() { public void run() { open(activity, serverBase); } });
        } catch (Exception unavailable) { /* No credentials or exception details are logged. */ }
    }

    private static void open(final Activity activity, final String serverBase) {
        if (activity.isFinishing() || current != null && current.isShowing()) return;
        final LinearLayout body=LoginStyle.body(activity,"注册账号");
        final EditText username=LoginStyle.field(activity,body,"账号：3–32位字母、数字、_ 或 -",false);
        final EditText nickname=LoginStyle.field(activity,body,"昵称：1–24个字",false);
        nickname.setText("冒险者");
        final EditText password=LoginStyle.field(activity,body,"密码：至少6个字符，最多72字节",true);
        final EditText confirmation=LoginStyle.field(activity,body,"确认密码",true);
        confirmation.setImeOptions(EditorInfo.IME_ACTION_DONE|EditorInfo.IME_FLAG_NO_EXTRACT_UI);
        final TextView message=new TextView(activity);message.setTextColor(Color.rgb(183,40,40));message.setTextSize(14);
        LinearLayout.LayoutParams messageLayout=new LinearLayout.LayoutParams(-1,-2);messageLayout.topMargin=LoginStyle.dp(activity,4);body.addView(message,messageLayout);
        final Button submit=LoginStyle.button(activity,"注 册",true),back=LoginStyle.button(activity,"返回登录",false);
        LinearLayout.LayoutParams submitLayout=new LinearLayout.LayoutParams(-1,LoginStyle.dp(activity,44));submitLayout.topMargin=LoginStyle.dp(activity,8);body.addView(submit,submitLayout);
        LinearLayout.LayoutParams backLayout=new LinearLayout.LayoutParams(-1,LoginStyle.dp(activity,36));backLayout.topMargin=LoginStyle.dp(activity,8);body.addView(back,backLayout);
        final Dialog dialog=new Dialog(activity);dialog.requestWindowFeature(android.view.Window.FEATURE_NO_TITLE);
        dialog.setContentView(LoginStyle.panel(activity,body));current=dialog;
        final String[] successful=new String[]{"",""};
        dialog.setCanceledOnTouchOutside(false);
        dialog.setOnDismissListener(ignored->{
            username.setText("");nickname.setText("");password.setText("");confirmation.setText("");
            if(current==dialog)current=null;
            if(!successful[0].isEmpty())LoginCache.registered(successful[0],successful[1]);else LoginCache.reopen();
            successful[0]="";successful[1]="";
        });
        back.setOnClickListener(view->dialog.dismiss());
        submit.setOnClickListener(view->{
            if(!submit.isEnabled())return;
            // Capture fields on Android's UI thread; the network worker receives
            // immutable values and never reads EditText objects in the background.
            final String name=username.getText().toString();
            final String nick=nickname.getText().toString();
            final String secret=password.getText().toString();
            final String confirm=confirmation.getText().toString();
            if(!name.matches("[A-Za-z0-9_-]{3,32}")){message.setText("账号需为3–32位字母、数字、下划线或短横线");return;}
            if(nick.trim().isEmpty()||!nick.equals(nick.trim())||nick.codePointCount(0,nick.length())>24||nick.matches("(?s).*[\\p{Cc}\\p{Cf}].*")){message.setText("请填写1–24个字的昵称，前后不要留空格");return;}
            try{
                if(secret.codePointCount(0,secret.length())<6||secret.getBytes("UTF-8").length>72){message.setText("密码至少6个字符，最多72字节");return;}
            }catch(Exception invalid){message.setText("密码格式无效");return;}
            if(!secret.equals(confirm)){message.setText("两次输入的密码不一样");return;}
            message.setText("正在注册…");dialog.setCancelable(false);submit.setEnabled(false);back.setEnabled(false);
            username.setEnabled(false);nickname.setEnabled(false);password.setEnabled(false);confirmation.setEnabled(false);
            new Thread(()->{
                final Result result=register(serverBase,name,nick,secret,confirm);
                activity.runOnUiThread(()->{
                    if(activity.isFinishing()||!dialog.isShowing())return;
                    if(result.success){
                        successful[0]=name;successful[1]=secret;dialog.dismiss();
                        Toast.makeText(activity,"注册成功，请登录",Toast.LENGTH_LONG).show();
                    }else{
                        dialog.setCancelable(true);submit.setEnabled(true);back.setEnabled(true);
                        username.setEnabled(true);nickname.setEnabled(true);password.setEnabled(true);confirmation.setEnabled(true);message.setText(result.message);
                    }
                });
            },"hyjs-register").start();
        });
        confirmation.setOnEditorActionListener((text,action,event)->{if(action==EditorInfo.IME_ACTION_DONE){submit.performClick();return true;}return false;});
        dialog.show();LoginStyle.fit(activity,dialog);body.setFocusableInTouchMode(true);body.requestFocus();
    }

    private static final class Result {
        final boolean success; final String message;
        Result(boolean success, String message) { this.success=success; this.message=message; }
    }

    private static Result register(String serverBase, String name, String nickname, String password, String confirmation) {
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
            byte[] payload=new JSONObject().put("username",name).put("nickname",nickname).put("password",password).put("confirmPassword",confirmation).toString().getBytes("UTF-8");
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
