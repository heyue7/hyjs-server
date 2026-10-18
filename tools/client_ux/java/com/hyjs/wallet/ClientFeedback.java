package com.hyjs.wallet;

import android.app.Activity;
import android.app.Dialog;
import android.graphics.Color;
import android.graphics.drawable.ColorDrawable;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.webkit.WebView;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.Spinner;
import android.widget.ArrayAdapter;
import android.widget.TextView;
import android.widget.Toast;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.lang.reflect.Field;
import java.net.URI;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.UUID;
import org.json.JSONArray;
import org.json.JSONObject;

/** Original customer-service entry, using the game's already-bound account session. */
public final class ClientFeedback {
    private static ClientFeedback current;
    private final Activity activity;
    private final String origin, owner;
    private final Session session;
    private Dialog dialog;
    private LinearLayout content;
    private TextView status;
    private boolean busy;
    private String createKey, createText, replyKey, replyText;

    private static final class Session {
        final long player, generation; final String token;
        Session(long player,String token,long generation){this.player=player;this.token=token;this.generation=generation;}
        boolean same(Session other){return other!=null&&player==other.player&&generation==other.generation&&token.equals(other.token);}
    }
    private static Object field(Object owner,String name) throws Exception {
        Field field=owner.getClass().getDeclaredField(name);field.setAccessible(true);return field.get(owner);
    }
    private static Session session() {
        try {
            // Reuse exactly the existing bridge's pause/switch/reconnect validation.
            Field f=Class.forName("com.hyjs.wallet.WalletActivity").getDeclaredField("credentials");f.setAccessible(true);
            Object credentials=f.get(null);
            long player=((Number)field(credentials,"player")).longValue();
            String token=(String)field(credentials,"session");
            long generation=((Number)field(credentials,"generation")).longValue();
            return player>0&&token.matches("[a-fA-F0-9]{32}")?new Session(player,token,generation):null;
        } catch(Exception absent){return null;}
    }
    private static Activity activity() throws Exception {
        return (Activity)Class.forName("com.unity3d.player.UnityPlayer").getField("currentActivity").get(null);
    }

    public static void prepareNotices() {
        // The original announcement frame stays in Unity. Only its SDK WebView
        // background becomes transparent; never touch the wallet or other pages.
        try {
            final Activity activity=activity();final Handler handler=new Handler(Looper.getMainLooper());
            handler.post(new Runnable(){int attempts;
                public void run(){
                    if(activity.isFinishing())return;
                    if(clearNotice(activity.getWindow().getDecorView())||++attempts>=40)return;
                    handler.postDelayed(this,250);
                }
            });
        } catch(Exception absent){ }
    }
    private static boolean clearNotice(View view) {
        boolean found=false;
        if(view instanceof WebView){
            WebView web=(WebView)view;String url=web.getUrl();
            if(url!=null&&url.matches("https?://[^/]+/notices(?:[?#].*)?")){
                web.setBackgroundColor(Color.TRANSPARENT);found=true;
            }
        }
        if(view instanceof ViewGroup){ViewGroup group=(ViewGroup)view;for(int i=0;i<group.getChildCount();i++)found=clearNotice(group.getChildAt(i))||found;}
        return found;
    }

    public static void show(final String origin,final String owner) {
        try {final Activity a=activity();a.runOnUiThread(new Runnable(){public void run(){
            if(current!=null&&current.dialog.isShowing())return;
            Session session=session();
            if(session==null){Toast.makeText(a,"请先登录后联系客服",Toast.LENGTH_SHORT).show();closeOwner(owner);return;}
            current=new ClientFeedback(a,origin,owner,session);current.open();
        }});}catch(Exception absent){ }
    }
    private ClientFeedback(Activity activity,String origin,String owner,Session session){this.activity=activity;this.origin=origin;this.owner=owner;this.session=session;}
    private static void closeOwner(String owner){
        try {Class.forName("com.unity3d.player.UnityPlayer").getMethod("UnitySendMessage",String.class,String.class,String.class).invoke(null,owner,"Close","");}catch(Exception ignored){ }
    }
    private int dp(int value){return Math.round(value*activity.getResources().getDisplayMetrics().density);}
    private TextView text(String value,int size){TextView label=new TextView(activity);label.setText(value);label.setTextSize(size);label.setTextColor(Color.rgb(48,65,72));label.setPadding(dp(6),dp(7),dp(6),dp(7));return label;}
    private Button button(String value,Runnable action){Button button=new Button(activity);button.setText(value);button.setOnClickListener(v->{if(!busy)action.run();});return button;}
    private EditText input(String hint,boolean multiline){EditText edit=new EditText(activity);edit.setHint(hint);edit.setTextSize(16);edit.setTextColor(Color.rgb(40,52,60));edit.setInputType(InputType.TYPE_CLASS_TEXT|(multiline?InputType.TYPE_TEXT_FLAG_MULTI_LINE:0));edit.setSingleLine(!multiline);edit.setImeOptions(android.view.inputmethod.EditorInfo.IME_FLAG_NO_EXTRACT_UI);if(multiline){edit.setMinLines(3);edit.setMaxLines(6);}content.addView(edit,new LinearLayout.LayoutParams(-1,-2));return edit;}
    private void open(){
        dialog=new Dialog(activity);dialog.setTitle("联系客服");
        LinearLayout shell=new LinearLayout(activity);shell.setOrientation(LinearLayout.VERTICAL);shell.setPadding(dp(16),dp(8),dp(16),dp(12));shell.setBackgroundColor(Color.rgb(231,235,233));
        TextView title=text("联系客服",21);title.setTextColor(Color.rgb(27,103,146));shell.addView(title);
        status=text("",14);shell.addView(status);
        ScrollView scroll=new ScrollView(activity);content=new LinearLayout(activity);content.setOrientation(LinearLayout.VERTICAL);scroll.addView(content);shell.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));
        LinearLayout actions=new LinearLayout(activity);actions.addView(button("我的反馈",()->list(1)),new LinearLayout.LayoutParams(0,-2,1));actions.addView(button("提交反馈",()->compose()),new LinearLayout.LayoutParams(0,-2,1));actions.addView(button("关闭",()->dialog.dismiss()),new LinearLayout.LayoutParams(0,-2,1));shell.addView(actions);
        dialog.setContentView(shell);dialog.setOnDismissListener(ignored->{current=null;closeOwner(owner);});dialog.show();
        dialog.getWindow().setBackgroundDrawable(new ColorDrawable(Color.TRANSPARENT));
        dialog.getWindow().setLayout(Math.min(dp(760),Math.round(activity.getResources().getDisplayMetrics().widthPixels*.86f)),Math.round(activity.getResources().getDisplayMetrics().heightPixels*.86f));
        dialog.getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        final Handler identityCheck=new Handler(Looper.getMainLooper());
        identityCheck.postDelayed(new Runnable(){public void run(){
            if(!dialog.isShowing())return;
            if(!session.same(session())){dialog.dismiss();return;}
            identityCheck.postDelayed(this,500);
        }},500);
        list(1);
    }
    private void message(String value){status.setText(value);}
    private interface Success{void receive(JSONObject data)throws Exception;}
    private void request(String route,JSONObject body,Success success){
        if(busy)return;
        if(!session.same(session())){dialog.dismiss();return;}
        busy=true;message("正在读取…");
        new Thread(()->{
            JSONObject data=null;String error=null;
            try{body.put("playerId",session.player).put("sessionHex",session.token);data=post(route,body);}catch(Exception failure){error=failure.getMessage();}
            final JSONObject result=data;final String problem=error;
            activity.runOnUiThread(()->{
                busy=false;
                if(!dialog.isShowing())return;
                if(!session.same(session())){dialog.dismiss();Toast.makeText(activity,"账号已变化，请重新打开客服",Toast.LENGTH_SHORT).show();return;}
                if(problem!=null){message(problem);return;}
                try{message("");success.receive(result);}catch(Exception invalid){message("反馈数据暂时无法显示，请重试");}
            });
        },"hyjs-feedback").start();
    }
    private JSONObject post(String route,JSONObject body)throws Exception{
        HttpURLConnection connection=null;
        try{
            URI base=new URI(origin);URL url=new URI(base.getScheme(),null,base.getHost(),base.getPort(),route,null,null).toURL();
            connection=(HttpURLConnection)url.openConnection();connection.setConnectTimeout(10000);connection.setReadTimeout(10000);connection.setInstanceFollowRedirects(false);connection.setRequestMethod("POST");connection.setDoOutput(true);connection.setRequestProperty("Content-Type","application/json; charset=utf-8");
            byte[] raw=body.toString().getBytes("UTF-8");connection.setFixedLengthStreamingMode(raw.length);try(OutputStream output=connection.getOutputStream()){output.write(raw);}
            int status=connection.getResponseCode();InputStream incoming=status>=400?connection.getErrorStream():connection.getInputStream();if(incoming==null)throw new Exception("反馈服务暂时不可用，请重试");
            ByteArrayOutputStream bytes=new ByteArrayOutputStream();try(InputStream input=incoming){byte[] block=new byte[2048];int count;while((count=input.read(block))!=-1){if(bytes.size()+count>262144)throw new Exception();bytes.write(block,0,count);}}
            JSONObject reply=new JSONObject(new String(bytes.toByteArray(),"UTF-8"));
            if(status!=200||reply.optInt("code",-1)!=0)throw new Exception(reply.optString("msg","反馈暂时无法提交，请重试"));
            JSONObject data=reply.optJSONObject("data");if(data==null)throw new Exception();return data;
        }catch(Exception failure){String text=failure.getMessage();throw new Exception(text!=null&&!text.isEmpty()&&text.length()<180?text:"反馈服务暂时不可用，请稍后重试");}
        finally{if(connection!=null)connection.disconnect();}
    }
    private String state(String key){switch(key){case "replied":return "客服已回复";case "processing":return "处理中";case "closed":return "已结束";default:return "待处理";}}
    private void list(int page){
        try{request("/game-feedback/list",new JSONObject().put("page",page).put("pageSize",20),data->{
            content.removeAllViews();JSONArray items=data.optJSONArray("items");int count=items==null?0:items.length();
            if(count==0)content.addView(text("暂无反馈，可点击下方“提交反馈”",16));
            for(int i=0;i<count;i++){final JSONObject item=items.getJSONObject(i);Button row=button(item.optString("subject")+"  ·  "+state(item.optString("state")),()->detail(item.optLong("id")));row.setAllCaps(false);content.addView(row,new LinearLayout.LayoutParams(-1,-2));}
            int total=data.optInt("total");content.addView(text("共 "+total+" 条反馈",13));
            if(page>1)content.addView(button("上一页",()->list(page-1)));
            if(page*20<total)content.addView(button("下一页",()->list(page+1)));
        });}catch(Exception ignored){message("反馈列表暂时无法读取");}
    }
    private void compose(){
        content.removeAllViews();content.addView(text("描述你遇到的问题，客服回复会保留在“我的反馈”中。",15));
        Spinner category=new Spinner(activity);category.setAdapter(new ArrayAdapter<String>(activity,android.R.layout.simple_spinner_dropdown_item,new String[]{"游戏问题","建议","账号问题","其他"}));content.addView(category);
        EditText subject=input("标题（最多80字）",false);EditText body=input("详细描述（最多2000字）",true);
        content.addView(button("发送反馈",()->{
            String title=subject.getText().toString().trim(),text=body.getText().toString().trim();if(title.isEmpty()||title.length()>80||text.isEmpty()||text.length()>2000){message("请填写标题和内容，并确认字数在限制内");return;}
            String kind=new String[]{"bug","suggestion","account","other"}[category.getSelectedItemPosition()];String fingerprint=kind+"\n"+title+"\n"+text;
            if(!fingerprint.equals(createText)){createText=fingerprint;createKey=UUID.randomUUID().toString();}
            try{request("/game-feedback/create",new JSONObject().put("requestId",createKey).put("category",kind).put("subject",title).put("content",text),data->{createText=null;createKey=null;renderDetail(data);message("反馈已提交");});}catch(Exception invalid){message("反馈暂时无法提交");}
        }));
    }
    private void detail(long id){try{request("/game-feedback/detail",new JSONObject().put("id",id),this::renderDetail);}catch(Exception invalid){message("反馈详情暂时无法读取");}}
    private void renderDetail(JSONObject data)throws Exception{
        content.removeAllViews();content.addView(text(data.optString("subject")+"  ·  "+state(data.optString("state")),18));
        JSONArray messages=data.optJSONArray("messages");if(messages!=null)for(int i=0;i<messages.length();i++){JSONObject item=messages.getJSONObject(i);content.addView(text(("support".equals(item.optString("sender"))?"客服：":"我：")+item.optString("content"),16));}
        EditText reply=input("继续补充问题（最多2000字）",true);final long id=data.optLong("id");
        content.addView(button("发送补充",()->{
            String text=reply.getText().toString().trim();if(text.isEmpty()||text.length()>2000){message("补充内容需为1–2000字");return;}
            String fingerprint=id+"\n"+text;if(!fingerprint.equals(replyText)){replyText=fingerprint;replyKey=UUID.randomUUID().toString();}
            try{request("/game-feedback/reply",new JSONObject().put("id",id).put("requestId",replyKey).put("content",text),result->{replyText=null;replyKey=null;renderDetail(result);message("补充已发送");});}catch(Exception invalid){message("补充暂时无法发送");}
        }));content.addView(button("刷新回复",()->detail(id)));
    }
}
