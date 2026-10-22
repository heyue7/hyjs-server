package com.hyjs.wallet;

import android.app.Activity;
import android.app.Dialog;
import android.content.pm.PackageInfo;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.Process;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.Window;
import android.widget.*;
import org.json.JSONObject;
import java.io.*;
import java.net.*;
import java.lang.reflect.Field;
import java.util.UUID;
import java.util.Date;
import java.util.Locale;
import java.util.TimeZone;
import java.text.SimpleDateFormat;
import java.util.concurrent.*;

/** Policy polling owns no game/network state. Unity Gate performs all engine calls. */
public final class ClientUpdate {
    private static final String PREFS = "hyjs-client-update";
    private static final String ACCEPTED = "declaration.accepted";
    private static final String DECLARATION = "本 APK 仅供个人学习、研究及非商业交流使用。请勿用于售卖、倒卖、收费下载、付费入群、充值运营、广告变现等直接或间接盈利活动。分享或转载时请保留本声明，不得冒用他人名义或误导使用者。体验过程中可能存在功能不完善、运行异常或数据丢失等情况，请理性参与，勿将游戏内账号、道具及数据用于交易。";
    private static final String STARTUP = UUID.randomUUID().toString().replace("-", "");
    private static final Handler UI = new Handler(Looper.getMainLooper());
    private static final ExecutorService IO = Executors.newSingleThreadExecutor();
    private static Activity activity;
    private static String origin;
    private static volatile JSONObject status;
    private static volatile Object statusAuth;
    private static volatile boolean blocked;
    private static volatile boolean active;
    private static volatile boolean accepted;
    private static volatile long buildCode;
    private static volatile String versionName = "";
    private static boolean busy;
    private static Dialog dialog;
    private static Dialog declaration;
    private static String displayedJson;
    private static final Runnable POLL = new Runnable(){public void run(){check();}};
    public static String version(){if(buildCode==0 && activity!=null)readVersion(activity);return "hyjs/"+buildCode+"/"+STARTUP;}
    public static String displayVersion(){if(versionName.length()==0 && activity!=null)readVersion(activity);return versionName;}
    private static void readVersion(Activity a){
        // Package manifest is the only version source; the packaged values cannot drift.
        try {
            PackageInfo info=a.getPackageManager().getPackageInfo(a.getPackageName(),0);
            buildCode=Build.VERSION.SDK_INT>=28?info.getLongVersionCode():info.versionCode;
            versionName=info.versionName==null?"":info.versionName.trim();
        }catch(Exception ignored){buildCode=0;versionName="";}
    }
    public static synchronized void boot(Activity a){activity=a;
        // Leaving one activity replaces its windows; drop stale handles before re-showing.
        if(declaration!=null){if(declaration.isShowing())declaration.dismiss();declaration=null;}
        if(dialog!=null){if(dialog.isShowing())dialog.dismiss();dialog=null;displayedJson=null;}
        try {
            InputStream in=a.getAssets().open("local-wallet.json"); origin=new JSONObject(read(in)).getString("origin").replaceAll("/+$", "");
            if(!origin.startsWith("https://") && !origin.startsWith("http://"))origin=null;
        }catch(Exception ignored){origin=null;}
        readVersion(a);
        accepted=a.getSharedPreferences(PREFS,0).getBoolean(ACCEPTED,false);resume();}
    public static void resume(){active=true; UI.removeCallbacks(POLL); if(!accepted){declaration();return;} UI.post(POLL);}
    public static void pause(){active=false;UI.removeCallbacks(POLL);}
    public static boolean blocked(){return !accepted || blocked;}
    public static String pending(){JSONObject s=status; return s==null?"":s.toString();}
    private static String read(InputStream in)throws Exception {try{ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] b=new byte[4096];int n;while((n=in.read(b))!=-1){if(out.size()+n>131072)throw new IOException();out.write(b,0,n);}return out.toString("UTF-8");}finally{in.close();}}
    private static Object credentials()throws Exception {Field f=Class.forName("com.hyjs.wallet.WalletActivity").getDeclaredField("credentials");f.setAccessible(true);return f.get(null);}
    private static Object field(Object o,String name)throws Exception {Field f=o.getClass().getDeclaredField(name);f.setAccessible(true);return f.get(o);}
    private static JSONObject payload(Object auth)throws Exception {
        JSONObject p=new JSONObject().put("buildCode",buildCode).put("startupId",STARTUP);
        long player=((Number)field(auth,"player")).longValue();String session=(String)field(auth,"session");
        if(player>0 && session.matches("[a-fA-F0-9]{32}"))p.put("playerId",player).put("sessionHex",session);return p;
    }
    private static JSONObject request(String path,JSONObject body)throws Exception {
        HttpURLConnection c=(HttpURLConnection)new URL(origin+path).openConnection();
        try{c.setRequestMethod("POST");c.setInstanceFollowRedirects(false);c.setConnectTimeout(8000);c.setReadTimeout(12000);c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json; charset=utf-8");c.setRequestProperty("Cache-Control","no-store");byte[] bytes=body.toString().getBytes("UTF-8");c.setFixedLengthStreamingMode(bytes.length);OutputStream out=c.getOutputStream();out.write(bytes);out.close();if(c.getResponseCode()!=200)throw new IOException();JSONObject reply=new JSONObject(read(c.getInputStream()));if(reply.optInt("code",-1)!=0)throw new IOException();return reply.getJSONObject("data");}finally{c.disconnect();}
    }
    private static void check(){
        if(!active || busy || activity==null || origin==null || !accepted)return;busy=true;
        IO.execute(new Runnable(){public void run(){JSONObject result=null;Object auth=null;try{auth=credentials();result=request("/client-update/status",payload(auth));if(auth!=credentials())result=null;}catch(Exception ignored){}final JSONObject value=result;final Object identity=auth;
            UI.post(new Runnable(){public void run(){busy=false;if(value!=null){try{if(identity!=credentials()){if(active)UI.postDelayed(POLL,1000);return;}}catch(Exception ignored){return;}statusAuth=identity;status=value;blocked=value.optBoolean("blocked",false);if(blocked && dialog!=null && dialog.isShowing() && Boolean.TRUE.equals(dialog.getWindow().getDecorView().getTag()) && !value.toString().equals(displayedJson))dialog.dismiss();if(blocked && dialog!=null && dialog.isShowing() && !Boolean.TRUE.equals(dialog.getWindow().getDecorView().getTag()))dialog.dismiss();if(!blocked && dialog!=null && dialog.isShowing() && Boolean.TRUE.equals(dialog.getWindow().getDecorView().getTag()))dialog.dismiss();}if(active)UI.postDelayed(POLL,Math.max(15,Math.min(300,value==null?30:value.optInt("pollSeconds",60)))*1000L);}});
        }});
    }
    private static void ack(final JSONObject s,final String kind,final Object identity){IO.execute(new Runnable(){public void run(){try{request("/client-update/ack",payload(identity).put("batchId",s.getString("batchId")).put("kind",kind));}catch(Exception ignored){}}});}
    /** Called only from Gate.Update on the Unity thread, marshalled to Android UI. */
    public static void display(final String json){UI.post(new Runnable(){public void run(){try{
        if(!accepted)return;
        final JSONObject s=new JSONObject(json);JSONObject latest=status;if(latest==null || !latest.toString().equals(json) || !active || activity==null || activity.isFinishing())return;
        final Object displayAuth=statusAuth;final boolean force=s.optBoolean("blocked",false);final String batch=s.optString("batchId", "");
        if(dialog!=null && dialog.isShowing())return;
        if(!force && (buildCode>=s.optLong("latestBuild",0) || !s.optBoolean("noticeEnabled",false) || batch.length()==0 || activity.getSharedPreferences(PREFS,0).getBoolean("seen."+batch,false)))return;
        dialog=new Dialog(activity);dialog.requestWindowFeature(Window.FEATURE_NO_TITLE);dialog.setCancelable(!force);dialog.setCanceledOnTouchOutside(false);
        LinearLayout box=new LinearLayout(activity);box.setOrientation(LinearLayout.VERTICAL);box.setPadding(dp(28),dp(24),dp(28),dp(22));
        GradientDrawable bg=new GradientDrawable();bg.setColor(Color.rgb(245,249,250));bg.setStroke(dp(2),Color.rgb(124,174,187));bg.setCornerRadius(dp(3));box.setBackground(bg);
        TextView title=new TextView(activity);title.setText(maintenance(s,force)?(force?"正在维护":"维护预告"):(force?"需要更新":"更新提示"));title.setTextColor(Color.rgb(43,70,81));title.setTextSize(22);box.addView(title);
        ScrollView scroll=new ScrollView(activity);TextView content=new TextView(activity);content.setText(message(s,force));content.setTextColor(Color.rgb(49,68,77));content.setTextSize(17);content.setPadding(0,dp(18),0,dp(22));scroll.addView(content);box.addView(scroll,new LinearLayout.LayoutParams(-1,bodyHeight()));
        TextView channel=new TextView(activity);channel.setText("请前往QQ群下载最新版");channel.setTextColor(Color.rgb(43,70,81));channel.setTextSize(16);channel.setSingleLine(true);channel.setPadding(0,dp(10),0,dp(16));box.addView(channel,new LinearLayout.LayoutParams(-1,-2));
        Button button=new Button(activity);button.setText(force?"重新检查":"知道了");button.setTextColor(Color.WHITE);GradientDrawable face=new GradientDrawable();face.setColor(Color.rgb(40,161,185));face.setStroke(dp(1),Color.rgb(104,202,216));button.setBackground(face);box.addView(button,new LinearLayout.LayoutParams(-1,dp(48)));
        button.setOnClickListener(v->{ack(s,"acknowledged",displayAuth);if(force){button.setText("检查中…");button.setEnabled(false);UI.removeCallbacks(POLL);check();UI.postDelayed(()->{button.setText("重新检查");button.setEnabled(true);},1500);}else dialog.dismiss();});
        dialog.setContentView(box);dialog.show();dialog.getWindow().setLayout(Math.min(activity.getResources().getDisplayMetrics().widthPixels-dp(40),dp(460)),-2);dialog.getWindow().getDecorView().setTag(force);displayedJson=json;
        if(!force)activity.getSharedPreferences(PREFS,0).edit().putBoolean("seen."+batch,true).apply();ack(s,"shown",displayAuth);
    }catch(Exception ignored){}}});}
    /**
     * First-use declaration. It owns the screen before any policy check runs: polling and
     * update notices stay suspended until this device accepts, and the dialog itself cannot
     * be cancelled by the back key, outside taps or activity recreation.
     */
    private static void declaration(){
        if(activity==null || activity.isFinishing() || accepted)return;
        if(declaration!=null && declaration.isShowing())return;
        // A declaration raised while an update dialog is visible sits above it.
        if(dialog!=null && dialog.isShowing()){dialog.dismiss();dialog=null;displayedJson=null;}
        final Dialog d=new Dialog(activity);declaration=d;
        d.requestWindowFeature(Window.FEATURE_NO_TITLE);d.setCancelable(false);d.setCanceledOnTouchOutside(false);
        d.setOnKeyListener((ignored,key,event)->key==KeyEvent.KEYCODE_BACK);
        LinearLayout box=new LinearLayout(activity);box.setOrientation(LinearLayout.VERTICAL);box.setPadding(dp(28),dp(24),dp(28),dp(22));
        GradientDrawable bg=new GradientDrawable();bg.setColor(Color.rgb(245,249,250));bg.setStroke(dp(2),Color.rgb(124,174,187));bg.setCornerRadius(dp(3));box.setBackground(bg);
        TextView title=new TextView(activity);title.setText("使用声明");title.setTextColor(Color.rgb(43,70,81));title.setTextSize(22);box.addView(title);
        ScrollView scroll=new ScrollView(activity);TextView content=new TextView(activity);content.setText(DECLARATION);content.setTextColor(Color.rgb(49,68,77));content.setTextSize(17);content.setLineSpacing(dp(3),1f);content.setPadding(0,dp(18),0,dp(18));scroll.addView(content);
        LinearLayout.LayoutParams body=new LinearLayout.LayoutParams(-1,bodyHeight());body.bottomMargin=dp(16);box.addView(scroll,body);
        LinearLayout actions=new LinearLayout(activity);actions.setOrientation(LinearLayout.HORIZONTAL);
        Button quit=face("退出",Color.rgb(196,72,72),Color.rgb(214,110,110));
        Button understand=face("了解",Color.rgb(46,126,207),Color.rgb(118,179,235));
        quit.setMinHeight(dp(48));understand.setMinHeight(dp(48));
        LinearLayout.LayoutParams left=new LinearLayout.LayoutParams(0,-2,1f);left.rightMargin=dp(10);
        actions.addView(quit,left);actions.addView(understand,new LinearLayout.LayoutParams(0,-2,1f));box.addView(actions);
        // Declining leaves no local record, so the next launch asks again.
        quit.setOnClickListener(v->{v.setEnabled(false);activity.finishAndRemoveTask();Process.killProcess(Process.myPid());});
        // Accepting is device-scoped and survives relaunches, version upgrades and account changes.
        understand.setOnClickListener(v->{if(activity.getSharedPreferences(PREFS,0).edit().putBoolean(ACCEPTED,true).commit()){accepted=true;d.dismiss();if(declaration==d)declaration=null;resume();}});
        d.setContentView(box);d.show();d.getWindow().setLayout(Math.min(activity.getResources().getDisplayMetrics().widthPixels-dp(40),dp(460)),-2);
    }
    private static Button face(String text,int color,int stroke){
        Button button=new Button(activity);button.setText(text);button.setTextSize(18);button.setTextColor(Color.WHITE);
        GradientDrawable drawable=new GradientDrawable();drawable.setColor(color);drawable.setStroke(dp(1),stroke);button.setBackground(drawable);return button;
    }
    /** Keeps the scrollable body short enough for landscape and narrow screens to show both buttons. */
    private static int bodyHeight(){
        int pixels=activity.getResources().getDisplayMetrics().heightPixels;
        return Math.max(dp(88),Math.min(dp(180),pixels-dp(300)));
    }
    private static boolean maintenance(JSONObject s,boolean force){String reason=s.optString("reason","").toLowerCase(Locale.US);return s.optBoolean("maintenance",false) || reason.contains("maintenance") || reason.contains("维护") || (!force && "maintenance".equals(s.optString("mode")) && s.optLong("maintenanceStartAt",0)>System.currentTimeMillis());}
    private static String time(long milliseconds){SimpleDateFormat format=new SimpleDateFormat("yyyy-MM-dd HH:mm",Locale.CHINA);format.setTimeZone(TimeZone.getTimeZone("Asia/Shanghai"));return format.format(new Date(milliseconds))+"（北京时间）";}
    private static String message(JSONObject s,boolean force){
        StringBuilder text=new StringBuilder();
        String title=s.optString("title","").trim(),body=s.optString("content","").trim(),version=s.optString("versionName","").trim();
        if(title.length()>0)text.append(title).append("\n\n");
        if(version.length()>0)text.append("最新版本：").append(version).append("\n");
        text.append("当前版本：").append(versionName.length()>0?versionName:"未知").append("\n\n");
        if(maintenance(s,force)){
            text.append(force?"服务器正在维护，暂时无法进入游戏。":"维护开始后将暂时无法进入游戏。").append("\n");
            long start=s.optLong("maintenanceStartAt",0),end=s.optLong("maintenanceEndAt",0);
            if(start>0)text.append("维护开始：").append(time(start)).append("\n");
            text.append(end>0?"预计结束："+time(end):"维护结束时间待定").append("\n实际开放以重新检查结果为准。\n\n");
        }else if(force)text.append("当前版本暂时无法进入游戏，更新后可重新登录。\n\n");
        else if("next_login".equals(s.optString("mode")))text.append("本次在线可继续游玩，下次登录前需要更新。\n\n");
        else text.append("发现新版本，可稍后更新。\n\n");
        if(body.length()>0)text.append(body).append("\n\n");
        return text.toString().trim();
    }
    private static int dp(int n){return Math.round(n*activity.getResources().getDisplayMetrics().density);}
}
