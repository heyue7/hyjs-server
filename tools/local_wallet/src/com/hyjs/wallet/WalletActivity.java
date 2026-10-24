package com.hyjs.wallet;

import android.app.Dialog;
import android.app.AlertDialog;
import android.animation.ValueAnimator;
import android.content.DialogInterface;
import android.content.SharedPreferences;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Color;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.Rect;
import android.graphics.PixelFormat;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.BitmapDrawable;
import android.graphics.drawable.StateListDrawable;
import android.graphics.drawable.RippleDrawable;
import android.os.Bundle;
import android.os.Build;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewConfiguration;
import android.view.WindowInsets;
import android.view.Window;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.PopupMenu;
import android.text.InputFilter;
import com.unity3d.player.UnityPlayerActivity;
import com.unity3d.player.UnityPlayer;
import org.json.JSONArray;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.math.BigDecimal;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URL;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.ArrayList;
import java.util.Calendar;
import java.util.Collections;
import java.util.Locale;
import java.util.TimeZone;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** A native wallet layered on the original Unity activity; the engine remains original. */
public class WalletActivity extends UnityPlayerActivity {
    private static final int BLUE=Color.rgb(22,119,255), INK=Color.rgb(17,17,17), GRAY=Color.rgb(128,128,128), LINE=Color.rgb(240,240,240);
    private static final int HOME=0, BILLS=4, ORDERS=5, CASHIER=6, ACCOUNT=7, NICKNAME=8, PASSWORD=9, ACCOUNT_HOME=10;
    private static volatile WalletActivity current;
    private static volatile Credentials credentials=new Credentials(0,"",0);

    private static final class Credentials {
        final long player, generation; final String session;
        Credentials(long p,String s,long g){player=p;session=s;generation=g;}
        boolean valid(){return player>0 && session.matches("[a-fA-F0-9]{32}");}
    }

    /** Called only with the real game's current login/reconnect identity. */
    public static synchronized void bindSession(long playerId,String sessionHex){
        String session=sessionHex==null?"":sessionHex.toLowerCase(Locale.US);
        if(playerId<=0 || !session.matches("[a-f0-9]{32}")){playerId=0;session="";}
        Credentials old=credentials;
        if(old.player==playerId && old.session.equals(session))return;
        credentials=new Credentials(playerId,session,old.generation+1);
        final WalletActivity a=current;
        if(a!=null)a.runOnUiThread(new Runnable(){public void run(){a.sessionChanged();}});
    }

    /** Read-only preflight for the original recharge entry; payment stays server-authoritative. */
    public static void checkRechargeBalance(final String attemptId,final long amountCents){
        if(attemptId==null || !attemptId.matches("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}"))return;
        final WalletActivity activity=current;
        if(activity==null){sendRechargeBalance(attemptId,"error");return;}
        activity.runOnUiThread(()->activity.startRechargeBalance(attemptId,amountCents));
    }
    private static void sendRechargeBalance(String attemptId,String status){
        try{UnityPlayer.UnitySendMessage("HYJSPaymentFeedback","OnRechargeBalance",attemptId+"|"+status);}catch(Exception ignored){}
    }
    private volatile HttpURLConnection rechargeConnection;
    private long rechargeGeneration;
    private void startRechargeBalance(final String attemptId,final long amountCents){
        final Credentials auth=credentials;
        if(destroyed || !ready() || amountCents<1 || amountCents>9007199254740991L){sendRechargeBalance(attemptId,"error");return;}
        final long requestGeneration=++rechargeGeneration;
        final String address=origin+"/local-wallet/state";
        HttpURLConnection previous=rechargeConnection;if(previous!=null)previous.disconnect();
        try{network.execute(()->{
            String result="error";HttpURLConnection connection=null;
            try{
                JSONObject payload=new JSONObject();payload.put("playerId",auth.player);payload.put("sessionHex",auth.session);
                connection=(HttpURLConnection)new URL(address).openConnection();rechargeConnection=connection;
                connection.setRequestMethod("POST");connection.setConnectTimeout(8000);connection.setReadTimeout(15000);
                connection.setInstanceFollowRedirects(false);connection.setDoOutput(true);connection.setRequestProperty("Content-Type","application/json; charset=utf-8");connection.setRequestProperty("Cache-Control","no-store");
                byte[] body=payload.toString().getBytes("UTF-8");connection.setFixedLengthStreamingMode(body.length);
                OutputStream out=connection.getOutputStream();out.write(body);out.close();
                int status=connection.getResponseCode();InputStream in=status>=400?connection.getErrorStream():connection.getInputStream();
                if(in==null)throw new Exception();byte[] raw;try{raw=readBounded(in,1048576);}finally{in.close();}
                JSONObject reply=new JSONObject(new String(raw,"UTF-8"));JSONObject data=reply.optJSONObject("data");
                if(status!=200 || reply.optInt("code",-1)!=0 || data==null || !data.optBoolean("testOnly") || !data.optBoolean("enabled") || !"CNY".equals(data.optString("currency")) || !data.has("balanceCents") || data.getLong("balanceCents")<0)throw new Exception();
                result=data.getLong("balanceCents")<amountCents?"insufficient":"ready";
            }catch(Exception ignored){result="error";}
            finally{if(connection!=null)connection.disconnect();if(rechargeConnection==connection)rechargeConnection=null;}
            final String response=result;
            runOnUiThread(()->{if(!destroyed && credentials.generation==auth.generation && requestGeneration==rechargeGeneration)sendRechargeBalance(attemptId,response);});
        });}catch(java.util.concurrent.RejectedExecutionException unavailable){sendRechargeBalance(attemptId,"error");}
    }

    private volatile HttpURLConnection activeConnection;
    private final ExecutorService network=Executors.newSingleThreadExecutor();
    private FrameLayout overlay;
    private View bubble;
    private FrameLayout bubbleWindow;
    private WindowManager bubbleWindowManager;
    private View bubbleWindowAnchor;
    private final ArrayList<View> bubbleWindowAnchors=new ArrayList<View>();
    private float bubbleX,bubbleY;
    private ValueAnimator bubbleDockAnimation;
    private Dialog panel;
    private LinearLayout shell;
    private TextView unreadBadge;
    private TextView feedback;
    private final Bitmap[] accountButtonTiles=new Bitmap[4];
    private boolean accountArtLoaded;
    private Bitmap accountBrandIcon;
    private JSONObject wallet, ledger, account;
    private EditText nicknameInput, oldPasswordInput, newPasswordInput, confirmationInput;
    private String nicknameDraft="";
    private JSONArray ledgerEntries=new JSONArray();
    private String origin, burstRequestId, selectedOrderId;
    private String message="";
    private boolean messageError, busy, destroyed, dockRight, dragging, keyboardBack, refreshQueued;
    private int page=ACCOUNT_HOME;
    private int ledgerRange=2, ledgerKind=0, ledgerPage=1, ledgerScrollY=0;
    private ScrollView ledgerScroll, homeScroll;
    private int homeScrollY;
    private boolean foreground;
    private long seenConfirmedRevision;
    private JSONArray incomeEntries=new JSONArray();
    private final android.os.Handler pollHandler=new android.os.Handler(android.os.Looper.getMainLooper());
    private final Runnable statePoll=new Runnable(){public void run(){
        if(destroyed || !foreground)return;
        if(ready() && !busy)request("/local-wallet/state",new JSONObject(),false,true);
        pollHandler.postDelayed(this,30000);
    }};
    private final Runnable dimBubble=new Runnable(){public void run(){
        if(!destroyed && bubble!=null && !dragging && (panel==null || !panel.isShowing())){
            bubble.animate().cancel();bubble.animate().alpha(.40f).setDuration(180).start();
        }
    }};

    // Preserve the complete branded asset inside the existing 48dp touch window.
    private static final class CircularWalletIcon extends ImageView {
        CircularWalletIcon(android.content.Context context){super(context);}
    }

    // Clip the hidden half at the usable game edge, including immersive bars
    // and cutouts. The remaining half stays inside the app's touchable area.
    private final class WalletOverlay extends FrameLayout {
        private int pointer=-1;
        private float downX,downY,startX,startY;
        private boolean moved, longDragTraced;
        private final int[] screenPosition=new int[2];
        WalletOverlay(){super(WalletActivity.this);}
        // This host supplies stable activity coordinates only. The actual icon
        // and pointer window are a separate 48dp application sub-window.
        @Override protected void dispatchDraw(Canvas canvas) { }
        boolean dispatchWalletTouch(MotionEvent event){
            int action=event.getActionMasked();
            getLocationOnScreen(screenPosition);
            // Window coordinates stay stable when Unity relays a gesture or the
            // icon crosses a child hit rectangle. Keep ownership until UP/CANCEL.
            float x=event.getRawX()-screenPosition[0],y=event.getRawY()-screenPosition[1];
            if(action==MotionEvent.ACTION_DOWN){
                if(bubble==null || !bubbleTouchBounds().contains((int)x,(int)y))return false;
                // The parent owns the sequence while the half-hidden child moves.
                // Stable parent coordinates keep the drag independent of child
                // translation and give the narrow edge icon a full 48dp handle.
                pointer=event.getPointerId(0);dragging=true;moved=false;longDragTraced=false;
                if(getParent()!=null)getParent().requestDisallowInterceptTouchEvent(true);
                revealBubble();bubble.setPressed(true);
                downX=x;downY=y;startX=bubbleX;startY=bubbleY;
                traceBubbleGeometry("down");requestUnbufferedDispatch(event);
                return true;
            }
            if(pointer!=-1){
                int index=event.findPointerIndex(pointer);
                if(action==MotionEvent.ACTION_CANCEL || index<0){
                    finish(false);return true;
                }
                if(action==MotionEvent.ACTION_POINTER_UP && event.getPointerId(event.getActionIndex())==pointer){
                    int next=event.getActionIndex()==0?1:0;
                    pointer=event.getPointerId(next);
                    downX=x+event.getX(next)-event.getX(0);downY=y+event.getY(next)-event.getY(0);
                    startX=bubbleX;startY=bubbleY;moved=true;return true;
                }
                if(action==MotionEvent.ACTION_MOVE || action==MotionEvent.ACTION_UP){
                    float dx=x+event.getX(index)-event.getX(0)-downX,dy=y+event.getY(index)-event.getY(0)-downY;
                    int slop=ViewConfiguration.get(WalletActivity.this).getScaledTouchSlop();
                    if(Math.hypot(dx,dy)>slop)moved=true;
                    if(moved){
                        Rect bounds=bubbleBounds();
                        setBubblePosition(Math.max(bounds.left,Math.min(bounds.right-dp(48),startX+dx)),clampBubbleY(startY+dy,bounds));
                        updateBubbleGestureExclusion();invalidate();
                        if(!longDragTraced && Math.hypot(dx,dy)>dp(160)){longDragTraced=true;traceBubbleGeometry("long-move");}
                    }
                    if(action==MotionEvent.ACTION_UP)finish(!moved);
                    return true;
                }
                return true;
            }
            return false;
        }
        @Override public boolean dispatchTouchEvent(MotionEvent event){
            return dispatchWalletTouch(event);
        }
        private void finish(boolean click){
            pointer=-1;dragging=false;
            if(getParent()!=null)getParent().requestDisallowInterceptTouchEvent(false);
            bubble.setPressed(false);traceBubbleGeometry(click?"click":"finish");
            if(click){bubble.performClick();return;}
            Rect bounds=bubbleBounds();dockRight=bubbleX+dp(48)/2f>=(bounds.left+bounds.right)/2f;
            foldBubble(true);scheduleDim();
        }
    }

    @Override public boolean dispatchTouchEvent(MotionEvent event){return super.dispatchTouchEvent(event);}

    @Override protected void onCreate(Bundle state){
        try { LocalAssetOverlay.synchronize(this); } catch(java.io.IOException failure) { throw new IllegalStateException("本地游戏资源刷新失败", failure); }
        super.onCreate(state);current=this;
        try{
            JSONObject config=new JSONObject(readAsset("local-wallet.json"));URI uri=new URI(config.getString("origin"));
            if(!("http".equals(uri.getScheme()) || "https".equals(uri.getScheme())) || uri.getHost()==null || uri.getUserInfo()!=null || uri.getQuery()!=null || uri.getFragment()!=null || !(uri.getPath().isEmpty() || "/".equals(uri.getPath())))throw new Exception();
            origin=config.getString("origin").replaceAll("/+$","");
        }catch(Exception error){origin=null;}
        loadPendingRequests();
        getWindow().getDecorView().post(new Runnable(){public void run(){attachBubble();}});
    }
    @Override protected void onResume(){super.onResume();current=this;foreground=true;pollHandler.removeCallbacks(statePoll);pollHandler.post(statePoll);if(panel!=null && panel.isShowing())refreshPage();}
    @Override protected void onPause(){foreground=false;pollHandler.removeCallbacks(statePoll);super.onPause();}
    @Override protected void onDestroy(){
        destroyed=true;foreground=false;pollHandler.removeCallbacksAndMessages(null);if(current==this)current=null;
        if(bubble!=null){bubble.removeCallbacks(dimBubble);bubble.animate().cancel();}
        if(bubbleDockAnimation!=null)bubbleDockAnimation.cancel();
        removeBubbleWindow();bubbleWindowAnchors.clear();
        if(panel!=null)panel.dismiss();HttpURLConnection connection=activeConnection;if(connection!=null)connection.disconnect();HttpURLConnection preflight=rechargeConnection;if(preflight!=null)preflight.disconnect();rechargeGeneration++;network.shutdownNow();super.onDestroy();
    }
    private int dp(float n){return Math.round(n*getResources().getDisplayMetrics().density);}
    private GradientDrawable shape(int color,float radius){GradientDrawable d=new GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));return d;}
    private TextView text(String value,int size,int color){TextView t=new TextView(this);t.setText(value);t.setTextSize(size);t.setTextColor(color);t.setFontFeatureSettings("tnum");t.setIncludeFontPadding(false);return t;}
    private TextView center(String value,int size,int color){TextView t=text(value,size,color);t.setGravity(Gravity.CENTER);return t;}
    private LinearLayout column(){LinearLayout v=new LinearLayout(this);v.setOrientation(LinearLayout.VERTICAL);return v;}
    private TextView action(String label,boolean primary,boolean enabled,final Runnable click){
        TextView t=center(label,17,primary?Color.WHITE:BLUE);t.setTypeface(Typeface.DEFAULT,Typeface.NORMAL);t.setPadding(dp(16),0,dp(16),0);t.setMinHeight(dp(46));
        t.setBackground(new RippleDrawable(android.content.res.ColorStateList.valueOf(Color.argb(35,22,119,255)),shape(primary?BLUE:Color.WHITE,6),null));
        t.setEnabled(enabled);t.setAlpha(enabled?1f:.4f);t.setFocusable(true);t.setOnClickListener(new View.OnClickListener(){public void onClick(View v){click.run();}});return t;
    }
    private boolean accountPage(){return page==ACCOUNT_HOME || page==ACCOUNT || page==NICKNAME || page==PASSWORD;}
    private boolean accountLanding(){return page==ACCOUNT_HOME || page==ACCOUNT;}
    private void loadAccountArt(){
        if(accountArtLoaded)return;accountArtLoaded=true;
        try(InputStream in=getAssets().open("brain-burst/button-atlas.png")){
            Bitmap atlas=BitmapFactory.decodeStream(in);
            if(atlas!=null){
                if(atlas.getWidth()>=1428 && atlas.getHeight()>=738){
                    accountButtonTiles[0]=Bitmap.createBitmap(atlas,110,280,636,170);
                    accountButtonTiles[1]=Bitmap.createBitmap(atlas,110,568,636,170);
                    accountButtonTiles[2]=Bitmap.createBitmap(atlas,792,280,636,170);
                    accountButtonTiles[3]=Bitmap.createBitmap(atlas,792,568,636,170);
                }
                atlas.recycle();
            }
        }catch(Exception unavailable){/* Same-theme native fallback keeps every action available. */}
        try(InputStream in=getAssets().open("local-wallet/icon.png")){accountBrandIcon=BitmapFactory.decodeStream(in);}catch(Exception unavailable){}
    }
    private Drawable accountButtonSurface(boolean primary,boolean pressed){
        loadAccountArt();Bitmap tile=accountButtonTiles[(primary?0:2)+(pressed?1:0)];
        if(tile!=null){BitmapDrawable surface=new BitmapDrawable(getResources(),tile);surface.setGravity(Gravity.FILL);return surface;}
        GradientDrawable surface=new GradientDrawable(GradientDrawable.Orientation.TOP_BOTTOM,
            primary?(pressed?new int[]{0xff079bb9,0xff08728e}:new int[]{0xff46d8e6,0xff0796b6})
                :(pressed?new int[]{0xffd1dadd,0xffe2e7e8}:new int[]{0xfffafcfc,0xffdde4e6}));
        surface.setCornerRadius(dp(2));surface.setStroke(dp(1),primary?0xff4dcddd:0xffaab9bf);return surface;
    }
    private TextView accountAction(String label,boolean primary,boolean enabled,Runnable click){
        TextView button=center(label,15,primary?Color.WHITE:0xff34464d);button.setMinHeight(dp(44));button.setPadding(dp(8),0,dp(8),0);
        StateListDrawable states=new StateListDrawable();states.addState(new int[]{android.R.attr.state_pressed},accountButtonSurface(primary,true));states.addState(new int[]{android.R.attr.state_focused},accountButtonSurface(primary,true));states.addState(new int[]{},accountButtonSurface(primary,false));button.setBackground(states);
        button.setEnabled(enabled);button.setAlpha(enabled?1f:.45f);button.setFocusable(true);button.setOnClickListener(v->click.run());return button;
    }
    private void accountLine(LinearLayout parent){View line=new View(this);line.setBackgroundColor(0xffbcc9ce);parent.addView(line,new LinearLayout.LayoutParams(-1,dp(1)));}
    private void addFull(LinearLayout parent,View child,int height){parent.addView(child,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(height)));}
    private void space(LinearLayout parent,float weight){parent.addView(new View(this),new LinearLayout.LayoutParams(1,0,weight));}
    private void divider(LinearLayout parent){View line=new View(this);line.setBackgroundColor(LINE);parent.addView(line,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(1)));}
    private boolean ready(){return credentials.valid() && origin!=null;}
    private boolean active(){return ready() && !busy;}
    private static String money(long cents){return BigDecimal.valueOf(cents,2).stripTrailingZeros().toPlainString();}
    private static JSONArray array(JSONObject data,String key){JSONArray a=data==null?null:data.optJSONArray(key);return a==null?new JSONArray():a;}
    private static String date(long ms,String pattern){SimpleDateFormat f=new SimpleDateFormat(pattern,Locale.CHINA);f.setTimeZone(TimeZone.getTimeZone("Asia/Shanghai"));return f.format(new Date(ms));}

    // Only the visible half remains inside the inset-safe game area when docked.
    private void attachBubble(){
        if(destroyed || overlay!=null)return;
        ViewGroup decor=(ViewGroup)getWindow().getDecorView();decor.setClipChildren(false);decor.setClipToPadding(false);
        overlay=new WalletOverlay();overlay.setClipChildren(false);overlay.setClipToPadding(false);overlay.setClickable(false);
        decor.addView(overlay,new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT,Gravity.FILL));
        ImageView icon=new CircularWalletIcon(this);icon.setContentDescription("Brain Burst 2039，点击展开，拖动位置");icon.setScaleType(ImageView.ScaleType.FIT_CENTER);
        try{InputStream in=getAssets().open("local-wallet/icon.png");BitmapFactory.Options options=new BitmapFactory.Options();options.inSampleSize=4;Bitmap bitmap=BitmapFactory.decodeStream(in,null,options);in.close();if(bitmap!=null)icon.setImageBitmap(bitmap);}catch(Exception unavailable){icon.setImageResource(android.R.drawable.ic_menu_save);icon.setColorFilter(BLUE);}
        bubble=icon;bubble.setAlpha(.40f);
        bubbleX=0;bubbleY=dp(70);
        bubbleWindow=new FrameLayout(this){
            @Override public boolean dispatchTouchEvent(MotionEvent event){return ((WalletOverlay)overlay).dispatchWalletTouch(event);}
        };
        // The 48dp surface moves as a Window; the child never travels through
        // Unity's main Decor/RenderNode hierarchy and never takes keyboard focus.
        bubbleWindow.setClipChildren(true);bubbleWindow.setClipToPadding(true);
        bubbleWindow.addView(icon,new FrameLayout.LayoutParams(dp(48),dp(48),Gravity.TOP|Gravity.LEFT));
        unreadBadge=center("",10,Color.WHITE);unreadBadge.setBackground(shape(Color.rgb(239,67,67),9));unreadBadge.setVisibility(View.GONE);
        FrameLayout.LayoutParams badgeLayout=new FrameLayout.LayoutParams(dp(18),dp(18),Gravity.TOP|Gravity.LEFT);bubbleWindow.addView(unreadBadge,badgeLayout);updateUnreadBadge();
        bubbleWindowManager=(WindowManager)getSystemService(WINDOW_SERVICE);
        attachWalletWindow(getWindow().getDecorView());
        icon.addOnLayoutChangeListener(new View.OnLayoutChangeListener(){public void onLayoutChange(View v,int l,int t,int r,int b,int ol,int ot,int or,int ob){updateBubbleGestureExclusion();overlay.invalidate();}});
        // Opacity remains on the real small-window child; docking moves its
        // application Window rather than translating across the Unity hierarchy.
        icon.animate().setUpdateListener(new ValueAnimator.AnimatorUpdateListener(){public void onAnimationUpdate(ValueAnimator animation){updateBubbleGestureExclusion();}});
        icon.setOnClickListener(new View.OnClickListener(){public void onClick(View v){revealBubble();openPanel();}});
        overlay.addOnLayoutChangeListener(new View.OnLayoutChangeListener(){public void onLayoutChange(View v,int l,int t,int r,int b,int ol,int ot,int or,int ob){
            repositionBubble();
        }});
        overlay.setOnApplyWindowInsetsListener(new View.OnApplyWindowInsetsListener(){public WindowInsets onApplyWindowInsets(View view,WindowInsets insets){
            view.post(new Runnable(){public void run(){repositionBubble();}});return insets;
        }});
        overlay.requestApplyInsets();icon.post(new Runnable(){public void run(){repositionBubble();}});
    }
    /** Keep the icon above the current app-owned dialog without overlay permission. */
    public static void attachToNativeWindow(Window window){
        WalletActivity a=current;if(a!=null && window!=null)a.attachWalletWindow(window.getDecorView());
    }
    private void attachWalletWindow(final View decor){
        if(destroyed || decor==null || bubbleWindowAnchors.contains(decor))return;
        if(decor==getWindow().getDecorView())bubbleWindowAnchors.add(0,decor);else bubbleWindowAnchors.add(decor);
        decor.addOnAttachStateChangeListener(new View.OnAttachStateChangeListener(){
            public void onViewAttachedToWindow(View view){updateBubbleWindow();}
            public void onViewDetachedFromWindow(View view){bubbleWindowAnchors.remove(view);updateBubbleWindow();}
        });
        updateBubbleWindow();
    }
    private void removeBubbleWindow(){
        if(bubbleWindowAnchor==null || bubbleWindowManager==null)return;
        try{bubbleWindowManager.removeViewImmediate(bubbleWindow);}catch(IllegalArgumentException detached){/* Parent dialog already removed its children. */}
        bubbleWindowAnchor=null;
    }
    private void setBubblePosition(float x,float y){bubbleX=x;bubbleY=y;updateBubbleWindow();}
    private void updateBubbleWindow(){
        if(destroyed || bubbleWindow==null || overlay==null || overlay.getWidth()==0)return;
        View anchor=null;
        for(int i=bubbleWindowAnchors.size()-1;i>=0;i--){View candidate=bubbleWindowAnchors.get(i);if(candidate.isAttachedToWindow() && candidate.getWindowToken()!=null){anchor=candidate;break;}}
        if(anchor==null){removeBubbleWindow();return;}
        Rect handle=bubbleTouchBounds();if(handle.isEmpty())return;
        int[] origin=new int[2];overlay.getLocationOnScreen(origin);
        WindowManager.LayoutParams lp=new WindowManager.LayoutParams(dp(48),dp(48),
            WindowManager.LayoutParams.TYPE_APPLICATION_SUB_PANEL,
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE|WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL|WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN|WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS,
            PixelFormat.TRANSLUCENT);
        lp.token=anchor.getWindowToken();lp.gravity=Gravity.TOP|Gravity.LEFT;
        lp.x=origin[0]+handle.left;lp.y=origin[1]+handle.top;
        lp.softInputMode=WindowManager.LayoutParams.SOFT_INPUT_ADJUST_NOTHING;
        lp.setTitle("HYJSWalletBubble");
        if(Build.VERSION.SDK_INT>=30)lp.setFitInsetsTypes(0);
        // Docked half stays clipped inside the safe 48dp handle; the handle itself
        // remains usable for a new DOWN over either the game or a native dialog.
        bubble.setTranslationX(bubbleX-handle.left);bubble.setTranslationY(bubbleY-handle.top);updateUnreadBadge();
        if(anchor!=bubbleWindowAnchor){
            removeBubbleWindow();
            try{bubbleWindowManager.addView(bubbleWindow,lp);bubbleWindowAnchor=anchor;}
            catch(WindowManager.BadTokenException detached){/* A closing dialog's attach callback selects its predecessor. */}
        }else{
            try{bubbleWindowManager.updateViewLayout(bubbleWindow,lp);}
            catch(IllegalArgumentException detached){bubbleWindowAnchor=null;}
        }
        updateBubbleGestureExclusion();
    }
    private JSONObject terminal(){return wallet==null?null:wallet.optJSONObject("terminal");}
    private void updateUnreadBadge(){
        if(unreadBadge==null)return;JSONObject terminal=terminal();int count=terminal==null?0:terminal.optInt("unreadCount");
        unreadBadge.setText(count>99?"99+":String.valueOf(count));unreadBadge.setVisibility(count>0?View.VISIBLE:View.GONE);
        // Clamp the top-right marker to the visible icon half inside its real 48dp window.
        Rect handle=bubbleTouchBounds();float x=bubbleX-handle.left+dp(30);unreadBadge.setTranslationX(Math.max(0,Math.min(dp(30),x)));unreadBadge.setTranslationY(dp(1));
        unreadBadge.setContentDescription("未读到账 "+count+" 条");
    }
    private Rect bubbleBounds(){
        if(overlay==null)return new Rect();
        View decor=getWindow().getDecorView();WindowInsets insets=decor.getRootWindowInsets();
        int left=0,top=0,right=0,bottom=0;
        if(insets!=null){
            if(Build.VERSION.SDK_INT>=30){
                android.graphics.Insets safe=insets.getInsetsIgnoringVisibility(WindowInsets.Type.systemBars()|WindowInsets.Type.displayCutout());
                android.graphics.Insets gestures=insets.getInsets(WindowInsets.Type.mandatorySystemGestures());
                left=Math.max(safe.left,gestures.left);top=Math.max(safe.top,gestures.top);right=Math.max(safe.right,gestures.right);bottom=Math.max(safe.bottom,gestures.bottom);
            }else{
                left=Math.max(insets.getStableInsetLeft(),insets.getSystemWindowInsetLeft());top=Math.max(insets.getStableInsetTop(),insets.getSystemWindowInsetTop());
                right=Math.max(insets.getStableInsetRight(),insets.getSystemWindowInsetRight());bottom=Math.max(insets.getStableInsetBottom(),insets.getSystemWindowInsetBottom());
                if(Build.VERSION.SDK_INT>=28 && insets.getDisplayCutout()!=null){
                    left=Math.max(left,insets.getDisplayCutout().getSafeInsetLeft());top=Math.max(top,insets.getDisplayCutout().getSafeInsetTop());
                    right=Math.max(right,insets.getDisplayCutout().getSafeInsetRight());bottom=Math.max(bottom,insets.getDisplayCutout().getSafeInsetBottom());
                }
            }
        }
        int[] rootPosition=new int[2],overlayPosition=new int[2];decor.getLocationInWindow(rootPosition);overlay.getLocationInWindow(overlayPosition);
        int x=overlayPosition[0]-rootPosition[0],y=overlayPosition[1]-rootPosition[1];
        return new Rect(Math.max(0,left-x),Math.max(0,top-y),Math.min(overlay.getWidth(),decor.getWidth()-right-x),Math.min(overlay.getHeight(),decor.getHeight()-bottom-y));
    }
    private void traceBubbleGeometry(String stage){
        if(overlay==null || bubble==null || !android.util.Log.isLoggable("HYJSWalletDrag",android.util.Log.DEBUG))return;
        Rect visible=new Rect();overlay.getGlobalVisibleRect(visible);
        // Opt-in geometry only: disabled by default; never include login, wallet or session data.
        android.util.Log.d("HYJSWalletDrag",stage+" host="+overlay.getWidth()+"x"+overlay.getHeight()+" visible="+visible+" safe="+bubbleBounds()+" icon="+bubbleX+","+bubbleY);
    }
    private Rect bubbleTouchBounds(){
        Rect safe=bubbleBounds();
        if(bubble==null || safe.isEmpty())return new Rect();
        int size=dp(48),height=dp(48);
        int left=Math.max(safe.left,Math.min(safe.right-size,Math.round(bubbleX)));
        int top=Math.max(safe.top,Math.min(safe.bottom-height,Math.round(bubbleY)));
        return new Rect(left,top,Math.min(safe.right,left+size),Math.min(safe.bottom,top+height));
    }
    private float clampBubbleY(float y,Rect bounds){return Math.max(bounds.top+dp(6),Math.min(bounds.bottom-dp(48)-dp(6),y));}
    private void updateBubbleGestureExclusion(){
        // Only the actual small input window excludes the system Back gesture.
        if(Build.VERSION.SDK_INT>=29 && bubbleWindow!=null)
            bubbleWindow.setSystemGestureExclusionRects(Collections.singletonList(new Rect(0,0,dp(48),dp(48))));
    }
    private void repositionBubble(){
        if(bubble==null || overlay.getWidth()==0)return;setBubblePosition(bubbleX,clampBubbleY(bubbleY,bubbleBounds()));
        if(!dragging){if(panel!=null && panel.isShowing())revealBubble();else foldBubble(false);}updateBubbleGestureExclusion();overlay.invalidate();
    }
    private void revealBubble(){if(bubble==null)return;bubble.removeCallbacks(dimBubble);bubble.animate().cancel();if(bubbleDockAnimation!=null)bubbleDockAnimation.cancel();bubble.setAlpha(1);Rect bounds=bubbleBounds();setBubblePosition(Math.max(bounds.left,Math.min(bounds.right-dp(48),bubbleX)),bubbleY);updateBubbleGestureExclusion();}
    private void foldBubble(boolean animate){
        if(bubble==null || overlay.getWidth()==0)return;bubble.animate().cancel();
        Rect bounds=bubbleBounds();float x=dockRight?bounds.right-dp(48)/2f:bounds.left-dp(48)/2f;
        if(bubbleDockAnimation!=null)bubbleDockAnimation.cancel();
        if(animate){
            bubbleDockAnimation=ValueAnimator.ofFloat(bubbleX,x);bubbleDockAnimation.setDuration(160);
            bubbleDockAnimation.addUpdateListener(new ValueAnimator.AnimatorUpdateListener(){public void onAnimationUpdate(ValueAnimator animation){setBubblePosition((Float)animation.getAnimatedValue(),bubbleY);}});
            bubbleDockAnimation.start();
        }else setBubblePosition(x,bubbleY);
        updateBubbleGestureExclusion();
    }
    private void scheduleDim(){if(bubble!=null && !destroyed){bubble.removeCallbacks(dimBubble);bubble.postDelayed(dimBubble,1500);}}

    private void openPanel(){
        if(destroyed || (panel!=null && panel.isShowing()))return;
        revealBubble();homeScroll=null;homeScrollY=0;page=ACCOUNT_HOME;message="";messageError=false;
        panel=new Dialog(this);panel.requestWindowFeature(Window.FEATURE_NO_TITLE);
        shell=column();shell.setBackgroundColor(Color.WHITE);shell.setFocusableInTouchMode(true);panel.setContentView(shell);
        Window window=panel.getWindow();window.setBackgroundDrawableResource(android.R.color.white);window.addFlags(WindowManager.LayoutParams.FLAG_DIM_BEHIND);window.setDimAmount(.24f);
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE|WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);
        panel.setOnDismissListener(new DialogInterface.OnDismissListener(){public void onDismiss(DialogInterface d){rememberLedgerScroll();hideKeyboard();foldBubble(true);scheduleDim();}});
        panel.setOnKeyListener(new DialogInterface.OnKeyListener(){public boolean onKey(DialogInterface d,int code,KeyEvent event){
            if(code!=KeyEvent.KEYCODE_BACK)return false;
            if(event.getAction()==KeyEvent.ACTION_DOWN){keyboardBack=keyboardVisible();if(keyboardBack)hideKeyboard();return true;}
            if(event.getAction()==KeyEvent.ACTION_UP){if(keyboardBack){keyboardBack=false;return true;}back();return true;}
            return true;
        }});
        panel.show();window.setGravity(Gravity.CENTER);attachWalletWindow(window.getDecorView());
        render();refreshPage();
    }
    private void fitPanel(){
        if(panel==null || !panel.isShowing())return;
        android.util.DisplayMetrics metrics=getResources().getDisplayMetrics();
        boolean home=page==HOME;
        int width=Math.min(dp(home?392:440),metrics.widthPixels-dp(32));
        int height=Math.min(dp(home?440:600),Math.round(metrics.heightPixels*(home?.78f:.90f)));
        if(accountPage()){
            Rect usable=bubbleBounds();int availableWidth=usable.width()>0?usable.width():metrics.widthPixels,availableHeight=usable.height()>0?usable.height():metrics.heightPixels;
            width=Math.min(dp(availableWidth>availableHeight?640:440),Math.max(dp(1),availableWidth-dp(32)));
            height=Math.min(dp(accountLanding()?310:page==PASSWORD?360:270),Math.round(availableHeight*.90f));
        }
        panel.getWindow().setLayout(width,height);
    }
    private void navigate(int destination){
        rememberLedgerScroll();hideKeyboard();if(destination==NICKNAME)nicknameDraft=account==null?"":account.optString("nickname");page=destination;message="";messageError=false;render();refreshPage();
    }
    private void back(){
        hideKeyboard();
        if(page==ACCOUNT_HOME){panel.dismiss();return;}
        if(page==HOME || page==ACCOUNT){navigate(ACCOUNT_HOME);return;}
        if(page==NICKNAME || page==PASSWORD){navigate(ACCOUNT_HOME);return;}
        if(page==CASHIER && array(wallet,"pendingOrders").length()>1){navigate(ORDERS);return;}
        navigate(HOME);
    }
    private boolean keyboardVisible(){
        if(panel==null)return false;Rect rect=new Rect();panel.getWindow().getDecorView().getWindowVisibleDisplayFrame(rect);return getResources().getDisplayMetrics().heightPixels-rect.bottom>dp(100);
    }
    private void hideKeyboard(){
        if(panel==null)return;View focus=panel.getCurrentFocus();if(focus!=null){InputMethodManager ime=(InputMethodManager)getSystemService(INPUT_METHOD_SERVICE);if(ime!=null)ime.hideSoftInputFromWindow(focus.getWindowToken(),0);focus.clearFocus();}if(shell!=null)shell.requestFocus();
    }
    private void setMessage(String value,boolean error){message=value;messageError=error;if(feedback!=null){feedback.setText(value);feedback.setVisibility(value.isEmpty()?View.GONE:View.VISIBLE);feedback.setTextColor(error?Color.rgb(200,65,50):GRAY);}}
    private String title(){switch(page){case BILLS:return "账单";case CASHIER:return "付款";case ORDERS:return "待支付订单";case ACCOUNT_HOME:case ACCOUNT:return "Brain Burst 2039";case NICKNAME:return "修改名称";case PASSWORD:return "修改密码";default:return "蓝绿终端";}}
    private void render(){
        if(panel==null || !panel.isShowing())return;
        fitPanel();
        rememberLedgerScroll();ledgerScroll=null;
        if(homeScroll!=null && page==HOME)homeScrollY=homeScroll.getScrollY();homeScroll=null;shell.removeAllViews();
        boolean accountStyle=accountPage();
        shell.setPadding(accountStyle?dp(1):0,accountStyle?dp(1):0,accountStyle?dp(1):0,accountStyle?dp(1):0);
        if(accountStyle){GradientDrawable frame=shape(0xfff2f4f4,2);frame.setStroke(dp(1),0xffaab9bf);shell.setBackground(frame);}else shell.setBackgroundColor(Color.WHITE);
        if(page!=PASSWORD){oldPasswordInput=null;newPasswordInput=null;confirmationInput=null;}
        FrameLayout nav=new FrameLayout(this);
        if(accountStyle){
            nav.setBackground(new GradientDrawable(GradientDrawable.Orientation.TOP_BOTTOM,new int[]{0xfffafafa,0xffe1e5e6}));
            TextView heading=center(title(),17,0xff26383f);heading.setPadding(dp(48),0,dp(48),0);heading.setMaxLines(2);
            if(Build.VERSION.SDK_INT>=26)heading.setAutoSizeTextTypeUniformWithConfiguration(12,17,1,android.util.TypedValue.COMPLEX_UNIT_SP);
            nav.addView(heading,new FrameLayout.LayoutParams(-1,dp(44)));
            if(!accountLanding()){TextView back=center("‹",30,0xff52636a);back.setContentDescription("返回账号首页");nav.addView(back,new FrameLayout.LayoutParams(dp(44),dp(44),Gravity.LEFT));back.setOnClickListener(v->back());}
            TextView close=center("×",27,0xff52636a);close.setContentDescription("关闭 Brain Burst 2039");nav.addView(close,new FrameLayout.LayoutParams(dp(44),dp(44),Gravity.RIGHT));close.setOnClickListener(v->{hideKeyboard();panel.dismiss();});
            addFull(shell,nav,44);accountLine(shell);
        }else{
        TextView title=center(title(),page==ACCOUNT_HOME?17:18,INK);title.setPadding(dp(58),0,dp(58),0);title.setMaxLines(2);if(Build.VERSION.SDK_INT>=26)title.setAutoSizeTextTypeUniformWithConfiguration(12,18,1,android.util.TypedValue.COMPLEX_UNIT_SP);nav.addView(title,new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(52)));
        TextView back=center("‹",34,INK);back.setContentDescription(page==ACCOUNT_HOME?"收起 Brain Burst 2039":"返回");FrameLayout.LayoutParams left=new FrameLayout.LayoutParams(dp(56),dp(52),Gravity.LEFT);nav.addView(back,left);back.setOnClickListener(new View.OnClickListener(){public void onClick(View v){back();}});
        if(page==HOME){TextView menu=center("⋯",28,INK);menu.setContentDescription("终端菜单");FrameLayout.LayoutParams right=new FrameLayout.LayoutParams(dp(56),dp(52),Gravity.RIGHT);nav.addView(menu,right);menu.setOnClickListener(v->{PopupMenu popup=new PopupMenu(this,menu);popup.getMenu().add(0,1,0,"账单");popup.getMenu().add(0,2,1,"待支付订单").setEnabled(ready());popup.getMenu().add(0,3,2,"Burst Link！规则").setEnabled(terminal()!=null);popup.setOnMenuItemClickListener(item->{if(item.getItemId()==3)showBurstRules();else navigate(item.getItemId()==1?BILLS:ORDERS);return true;});popup.show();});}
        else if(page==BILLS){TextView reload=center("刷新",14,BLUE);reload.setEnabled(!busy);nav.addView(reload,new FrameLayout.LayoutParams(dp(64),dp(52),Gravity.RIGHT));reload.setOnClickListener(new View.OnClickListener(){public void onClick(View v){refreshPage();}});}
        addFull(shell,nav,52);divider(shell);
        }
        LinearLayout body=column();shell.addView(body,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,0,1));
        switch(page){case BILLS:renderBills(body);break;case ORDERS:renderOrders(body);break;case CASHIER:renderCashier(body);break;case ACCOUNT_HOME:case ACCOUNT:renderAccount(body);break;case NICKNAME:case PASSWORD:renderAccountForm(body);break;default:renderHome(body);}
        feedback=center("",12,GRAY);feedback.setPadding(dp(20),dp(6),dp(20),dp(6));feedback.setMaxLines(2);shell.addView(feedback);
        if(!credentials.valid())setMessage("请先登录 Brain Burst 2039",false);else if(origin==null)setMessage("蓝绿终端连接未配置",true);else setMessage(message,messageError);
    }
    private void renderHome(LinearLayout body){
        LinearLayout content=column();
        LinearLayout summary=column();summary.setPadding(dp(20),dp(16),dp(20),dp(16));
        JSONObject role=currentRole();TextView owner=text(role==null?"当前角色 · ID "+credentials.player:roleLabel(role),12,GRAY);owner.setPadding(0,0,0,dp(12));summary.addView(owner);
        JSONObject terminal=terminal();boolean enabled=terminal!=null && terminal.optBoolean("burstEnabled");boolean can=enabled && terminal.optBoolean("canBurst");
        boolean insufficient=terminal!=null && wallet.optLong("balanceCents")<terminal.optDouble("burstCostPoints")*100;
        LinearLayout row=new LinearLayout(this);row.setGravity(Gravity.CENTER_VERTICAL);
        LinearLayout funds=column();addFull(funds,text("超频点数",12,GRAY),24);
        TextView balance=text(wallet==null?"—":money(wallet.optLong("balanceCents")),32,INK);balance.setSingleLine(true);
        if(Build.VERSION.SDK_INT>=26)balance.setAutoSizeTextTypeUniformWithConfiguration(14,32,1,android.util.TypedValue.COMPLEX_UNIT_SP);
        addFull(funds,balance,46);LinearLayout.LayoutParams fundsLayout=new LinearLayout.LayoutParams(0,-2,1);fundsLayout.rightMargin=dp(12);row.addView(funds,fundsLayout);
        LinearLayout controls=column();String label=busy?"处理中…":burstRequestId!=null?"重试 Burst Link！":"Burst Link！";
        TextView burstButton=action(label,true,active() && (can || burstRequestId!=null),()->burst());burstButton.setTextSize(16);burstButton.setSingleLine(true);burstButton.setMinHeight(dp(42));burstButton.setPadding(dp(4),0,dp(4),0);burstButton.setBackground(new RippleDrawable(android.content.res.ColorStateList.valueOf(Color.argb(35,22,119,255)),shape(BLUE,4),null));
        if(Build.VERSION.SDK_INT>=26)burstButton.setAutoSizeTextTypeUniformWithConfiguration(11,16,1,android.util.TypedValue.COMPLEX_UNIT_SP);
        addFull(controls,burstButton,42);
        String cost=terminal==null?"读取中…":terminal.optDouble("burstCostPoints")==0?"免费":"消耗 "+pointNumber(terminal.optDouble("burstCostPoints"))+" 点";
        TextView costLabel=text(cost,11,GRAY);costLabel.setGravity(Gravity.RIGHT);costLabel.setPadding(0,dp(7),0,0);controls.addView(costLabel);
        String state=terminal==null?"":!enabled?"暂未开放":!can && burstRequestId==null?(insufficient?"超频点数不足":"今日已使用"):"";
        if(!state.isEmpty()){TextView status=text(state,11,GRAY);status.setGravity(Gravity.RIGHT);status.setPadding(0,dp(5),0,0);controls.addView(status);}
        row.addView(controls,new LinearLayout.LayoutParams(dp(120),-2));summary.addView(row);
        final JSONArray orders=array(wallet,"pendingOrders");
        if(orders.length()>0){TextView entry=text("待支付订单  "+orders.length()+"  ›",14,BLUE);entry.setGravity(Gravity.CENTER_VERTICAL);addFull(summary,entry,42);entry.setOnClickListener(v->{if(orders.length()==1){selectedOrderId=orders.optJSONObject(0).optString("orderId");navigate(CASHIER);}else navigate(ORDERS);});}
        content.addView(summary);
        if(incomeEntries.length()>0){
            LinearLayout notices=column();notices.setPadding(dp(12),dp(12),dp(12),dp(12));notices.setBackgroundColor(Color.rgb(245,245,245));
            addFull(notices,text("到账通知",14,INK),30);
            for(int i=0;i<incomeEntries.length();i++){final JSONObject entry=incomeEntries.optJSONObject(i);if(entry==null)continue;
                LinearLayout card=column();card.setPadding(dp(14),dp(12),dp(14),dp(12));card.setBackground(shape(Color.WHITE,4));
                LinearLayout header=new LinearLayout(this);header.setGravity(Gravity.CENTER_VERTICAL);header.addView(text("蓝绿终端",12,GRAY),new LinearLayout.LayoutParams(0,-2,1));header.addView(text(date(entry.optLong("createdAt"),"MM月dd日 HH:mm"),11,GRAY));card.addView(header);
                LinearLayout amountRow=new LinearLayout(this);amountRow.setGravity(Gravity.BOTTOM);amountRow.setPadding(0,dp(14),0,dp(10));
                TextView number=text("+"+unsignedMoney(entry.optLong("amountCents")),24,INK);number.setTypeface(Typeface.DEFAULT,Typeface.BOLD);number.setSingleLine(true);
                if(Build.VERSION.SDK_INT>=26)number.setAutoSizeTextTypeUniformWithConfiguration(12,24,1,android.util.TypedValue.COMPLEX_UNIT_SP);
                amountRow.addView(number,new LinearLayout.LayoutParams(0,dp(32),1));TextView unit=text("超频点数",11,GRAY);unit.setPadding(dp(8),0,0,dp(3));amountRow.addView(unit);card.addView(amountRow);
                TextView source=text(incomeSource(entry),13,GRAY);source.setMaxLines(2);source.setEllipsize(android.text.TextUtils.TruncateAt.END);source.setLineSpacing(dp(3),1);source.setPadding(0,0,0,dp(12));card.addView(source);divider(card);
                TextView detail=text("查看详情  ›",14,BLUE);detail.setGravity(Gravity.CENTER_VERTICAL);detail.setPadding(0,dp(12),0,0);detail.setMinHeight(dp(42));detail.setOnClickListener(v->showIncomeDetail(entry));card.addView(detail);
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(8);notices.addView(card,lp);
            }
            content.addView(notices);
        }
        homeScroll=scrolling(body,content);final ScrollView viewport=homeScroll;final int restore=homeScrollY;viewport.post(()->{if(homeScroll==viewport)viewport.scrollTo(0,restore);});
    }
    private String incomeSource(JSONObject entry){
        String source=entry.optString("source");return "signup".equals(source)?"注册赠送":"burst".equals(source)?"Burst Link！":"admin_manual".equals(source)||"admin_weekly".equals(source)?entry.optString("label","运营投放"):"终端收入";
    }
    private void showIncomeDetail(JSONObject entry){
        LinearLayout content=column();
        addFull(content,center("已到账",13,GRAY),32);
        TextView amount=center("+"+unsignedMoney(entry.optLong("amountCents")),30,INK);amount.setSingleLine(true);if(Build.VERSION.SDK_INT>=26)amount.setAutoSizeTextTypeUniformWithConfiguration(14,30,1,android.util.TypedValue.COMPLEX_UNIT_SP);addFull(content,amount,48);addFull(content,center("超频点数",12,GRAY),24);
        ledgerDetailField(content,"入账来源",incomeSource(entry));ledgerDetailField(content,"到账时间",date(entry.optLong("createdAt"),"yyyy年MM月dd日 HH:mm:ss"));ledgerDetailField(content,"流水号",entry.optString("id"));
        showTerminalDetail("到账详情",content);
    }
    private void showBurstRules(){
        JSONObject terminal=terminal();if(terminal==null)return;LinearLayout content=column();
        ledgerDetailField(content,"消耗",terminal.optDouble("burstCostPoints")==0?"免费":pointNumber(terminal.optDouble("burstCostPoints"))+" 点");
        ledgerDetailField(content,"随机获得",pointNumber(terminal.optDouble("burstMinPoints"))+"–"+pointNumber(terminal.optDouble("burstMaxPoints"))+" 点");
        ledgerDetailField(content,"次数","每日一次，北京时间 0 点重置");showTerminalDetail("Burst Link！规则",content);
    }
    private void showTerminalDetail(String title,LinearLayout content){
        final Dialog detail=new Dialog(this);detail.requestWindowFeature(Window.FEATURE_NO_TITLE);LinearLayout outer=column();outer.setPadding(dp(24),dp(20),dp(24),dp(16));outer.setBackgroundColor(Color.WHITE);
        addFull(outer,center(title,18,INK),40);ScrollView scroll=new ScrollView(this);content.setPadding(0,0,0,dp(18));scroll.addView(content);outer.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));addFull(outer,action("完成",true,true,()->detail.dismiss()),46);detail.setContentView(outer);
        Window window=detail.getWindow();if(window!=null){window.setBackgroundDrawableResource(android.R.color.white);window.setDimAmount(.30f);}detail.show();window=detail.getWindow();if(window!=null){attachWalletWindow(window.getDecorView());window.setLayout(Math.min(dp(430),getResources().getDisplayMetrics().widthPixels-dp(36)),Math.min(dp(560),getResources().getDisplayMetrics().heightPixels-dp(48)));window.setGravity(Gravity.CENTER);}
    }
    private static String pointNumber(double points){return BigDecimal.valueOf(points).stripTrailingZeros().toPlainString();}
    private boolean isCurrentRole(JSONObject role){
        return account!=null && role!=null && account.optLong("currentServerId")>0
            && role.optLong("serverId")==account.optLong("currentServerId")
            && role.optLong("playerId")==credentials.player;
    }
    private JSONObject currentRole(){
        if(!credentials.valid())return null;
        JSONArray roles=array(account,"players");
        for(int i=0;i<roles.length();i++){JSONObject role=roles.optJSONObject(i);if(role!=null && isCurrentRole(role) && !role.optString("name").trim().isEmpty())return role;}
        return null;
    }
    private String roleLabel(JSONObject role){return "区服 "+role.optLong("serverId")+" · "+role.optString("name")+" · Lv."+role.optLong("level");}
    private void renderAccount(LinearLayout body){
        LinearLayout content=column();content.setPadding(dp(18),dp(10),dp(18),dp(8));
        ScrollView viewport=scrolling(body,content);viewport.setLayoutParams(new LinearLayout.LayoutParams(-1,0,1));
        if(!credentials.valid()){addFull(content,text("请先登录 Brain Burst 2039",16,INK),60);return;}
        if(account==null){addFull(content,text(busy?"正在读取账号…":"账号信息尚未读取",16,INK),48);addFull(content,accountAction("重试",false,active(),()->refreshPage()),44);return;}
        loadAccountArt();LinearLayout identity=new LinearLayout(this);identity.setGravity(Gravity.CENTER_VERTICAL);identity.setPadding(0,0,0,dp(8));identity.setMinimumHeight(dp(60));
        ImageView icon=new ImageView(this);icon.setScaleType(ImageView.ScaleType.FIT_CENTER);if(accountBrandIcon!=null)icon.setImageBitmap(accountBrandIcon);else icon.setImageResource(android.R.drawable.ic_menu_myplaces);icon.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);identity.addView(icon,new LinearLayout.LayoutParams(dp(40),dp(40)));
        LinearLayout names=column();names.setPadding(dp(12),0,0,0);TextView name=text(account.optString("nickname"),20,0xff26383f);name.setTextIsSelectable(true);name.setLineSpacing(dp(2),1);names.addView(name);
        TextView username=text(account.optString("username"),12,0xff596b73);username.setContentDescription("登录账号 "+account.optString("username"));username.setTextIsSelectable(true);username.setPadding(0,dp(4),0,0);names.addView(username);identity.addView(names,new LinearLayout.LayoutParams(0,-2,1));content.addView(identity);accountLine(content);
        TextView caption=text("区服角色",12,0xff596b73);caption.setPadding(0,dp(9),0,0);content.addView(caption);
        JSONArray roles=array(account,"players");
        if(roles.length()==0){TextView empty=text("尚未创建角色，进入游戏区服后创建。",14,GRAY);empty.setPadding(0,dp(12),0,dp(12));content.addView(empty);}
        for(int i=0;i<roles.length();i++){
            final JSONObject role=roles.optJSONObject(i);if(role==null)continue;
            final boolean created=!role.optString("name").trim().isEmpty(),selected=created && isCurrentRole(role);
            LinearLayout item=new LinearLayout(this);item.setGravity(Gravity.CENTER_VERTICAL);item.setMinimumHeight(dp(64));item.setPadding(0,dp(8),0,dp(8));
            LinearLayout words=column();TextView label=text(created?role.optString("name"):"未创建角色",16,0xff26383f);label.setLineSpacing(dp(2),1);words.addView(label);
            TextView detail=text("区服 "+role.optLong("serverId")+(created?" · Lv."+role.optLong("level"):"")+(selected?" · 当前角色":""),12,0xff596b73);detail.setPadding(0,dp(4),0,0);words.addView(detail);
            if(!selected){TextView hint=text(created?"进入该区服后可打开终端":"进入该区服后创建角色",12,0xff596b73);hint.setPadding(0,dp(4),0,0);words.addView(hint);}
            item.addView(words,new LinearLayout.LayoutParams(0,-2,1));
            if(selected){TextView open=accountAction("蓝绿终端",true,active(),()->{if(active())navigate(HOME);});LinearLayout.LayoutParams entry=new LinearLayout.LayoutParams(dp(112),dp(44));entry.leftMargin=dp(12);item.addView(open,entry);}
            content.addView(item);accountLine(content);
        }
        accountLine(body);LinearLayout footer=new LinearLayout(this);footer.setPadding(dp(12),dp(8),dp(12),dp(8));
        TextView[] actions={accountAction("修改名称",false,active(),()->navigate(NICKNAME)),accountAction("修改密码",false,active(),()->navigate(PASSWORD)),accountAction(busy?"处理中…":"退出登录",false,active(),()->confirmLogout())};actions[2].setTextColor(0xffa23d36);
        for(int i=0;i<actions.length;i++){LinearLayout.LayoutParams cell=new LinearLayout.LayoutParams(0,dp(44),1);if(i>0)cell.leftMargin=dp(8);footer.addView(actions[i],cell);}body.addView(footer,new LinearLayout.LayoutParams(-1,-2));
    }
    private void confirmLogout(){
        if(!active())return;final Credentials auth=credentials;
        AlertDialog dialog=new AlertDialog.Builder(this).setTitle("退出 Brain Burst 2039？").setMessage("退出后需重新输入账号和密码登录。").setNegativeButton("取消",null).setPositiveButton("退出登录",(d,which)->{if(credentials.generation==auth.generation && active())request("/game-account/logout",new JSONObject(),true);}).create();
        dialog.show();attachToNativeWindow(dialog.getWindow());
    }
    private void completeLogout(){
        LoginCache.logout();bindSession(0,"");
        if(panel!=null)panel.dismiss();
        UnityPlayer.UnitySendMessage("LocalWalletSession","LogoutAccount","");
    }
    private EditText accountField(LinearLayout parent,String label,String hint,boolean secret){
        TextView caption=text(label,13,GRAY);caption.setPadding(0,dp(6),0,0);addFull(parent,caption,25);
        EditText field=new EditText(this);field.setSingleLine(true);field.setTextSize(16);field.setTextColor(INK);field.setHint(hint);field.setHintTextColor(GRAY);field.setPadding(dp(12),0,dp(12),0);GradientDrawable input=shape(0xfffbfcfc,1);input.setStroke(dp(1),0xffa8bac2);field.setBackground(input);field.setHighlightColor(0xffabe2ed);
        field.setInputType(InputType.TYPE_CLASS_TEXT|(secret?InputType.TYPE_TEXT_VARIATION_PASSWORD:InputType.TYPE_TEXT_FLAG_CAP_SENTENCES));
        field.setImeOptions(EditorInfo.IME_FLAG_NO_EXTRACT_UI|EditorInfo.IME_FLAG_NO_FULLSCREEN|EditorInfo.IME_ACTION_NEXT);field.setEnabled(!busy);addFull(parent,field,44);return field;
    }
    private void renderAccountForm(LinearLayout body){
        LinearLayout content=column();content.setPadding(dp(20),dp(8),dp(20),dp(12));scrolling(body,content);
        if(page==NICKNAME){
            nicknameInput=accountField(content,"账号名称","1–24个字",false);nicknameInput.setFilters(new InputFilter[]{new InputFilter.LengthFilter(24)});nicknameInput.setText(nicknameDraft);
            nicknameInput.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence t,int start,int count,int after){}public void onTextChanged(CharSequence t,int start,int before,int count){nicknameDraft=t.toString();}public void afterTextChanged(Editable t){}});
            nicknameInput.setImeOptions(EditorInfo.IME_FLAG_NO_EXTRACT_UI|EditorInfo.IME_FLAG_NO_FULLSCREEN|EditorInfo.IME_ACTION_DONE);
        }else{
            String oldDraft=oldPasswordInput==null?"":oldPasswordInput.getText().toString(),newDraft=newPasswordInput==null?"":newPasswordInput.getText().toString(),confirmationDraft=confirmationInput==null?"":confirmationInput.getText().toString();
            oldPasswordInput=accountField(content,"原密码","输入当前密码",true);
            boolean wide=panel.getWindow().getDecorView().getWidth()>=dp(520);
            if(wide){LinearLayout pair=new LinearLayout(this);LinearLayout first=column(),second=column();LinearLayout.LayoutParams half=new LinearLayout.LayoutParams(0,-2,1);half.rightMargin=dp(12);pair.addView(first,half);pair.addView(second,new LinearLayout.LayoutParams(0,-2,1));content.addView(pair);newPasswordInput=accountField(first,"新密码","至少6个字符",true);confirmationInput=accountField(second,"确认新密码","再次输入新密码",true);}
            else{newPasswordInput=accountField(content,"新密码","至少6个字符",true);confirmationInput=accountField(content,"确认新密码","再次输入新密码",true);}
            oldPasswordInput.setText(oldDraft);newPasswordInput.setText(newDraft);confirmationInput.setText(confirmationDraft);
            confirmationInput.setImeOptions(EditorInfo.IME_FLAG_NO_EXTRACT_UI|EditorInfo.IME_FLAG_NO_FULLSCREEN|EditorInfo.IME_ACTION_DONE);
        }
        TextView note=text(page==PASSWORD?"其他设备需用新密码重新登录":"不会更改角色名",12,GRAY);note.setPadding(0,dp(12),0,dp(10));content.addView(note);
        addFull(content,accountAction(busy?"保存中…":"保存",true,active(),()->saveAccount()),44);
    }
    private void saveAccount(){
        if(!active())return;
        try{
            JSONObject payload=new JSONObject();
            if(page==NICKNAME){
                String name=nicknameDraft;
                if(name.trim().isEmpty() || !name.equals(name.trim()) || name.codePointCount(0,name.length())>24 || name.matches("(?s).*[\\p{Cc}\\p{Cf}].*")){setMessage("请填写1–24个字的账号名称",true);return;}
                payload.put("nickname",name);hideKeyboard();request("/game-account/nickname",payload,true);
            }else if(page==PASSWORD){
                String old=oldPasswordInput.getText().toString(),value=newPasswordInput.getText().toString(),confirmation=confirmationInput.getText().toString();
                if(old.isEmpty()){setMessage("请输入原密码",true);return;}
                if(value.codePointCount(0,value.length())<6 || value.getBytes("UTF-8").length>72){setMessage("新密码至少6个字符，最多72字节",true);return;}
                if(!value.equals(confirmation)){setMessage("两次新密码不一致",true);return;}
                payload.put("oldPassword",old).put("password",value).put("confirmPassword",confirmation);hideKeyboard();request("/game-account/password",payload,true);
            }
        }catch(Exception invalid){setMessage("填写内容无效，请重试",true);}
    }
    private ScrollView scrolling(LinearLayout body,LinearLayout content){ScrollView scroll=new ScrollView(this);scroll.setFillViewport(false);scroll.addView(content);body.addView(scroll,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));return scroll;}
    private void changeLedgerFilter(int range,int kind){
        if(!active())return;ledgerRange=range;ledgerKind=kind;ledgerPage=1;ledgerScrollY=0;ledger=null;ledgerEntries=new JSONArray();message="";render();requestLedger(1);
    }
    private TextView ledgerChip(String label,boolean selected,final Runnable click){
        TextView chip=center(label,13,selected?BLUE:GRAY);chip.setPadding(dp(5),0,dp(5),0);chip.setBackground(shape(selected?Color.rgb(235,244,255):Color.WHITE,14));chip.setEnabled(active());chip.setAlpha(active()?1f:.5f);chip.setOnClickListener(new View.OnClickListener(){public void onClick(View v){click.run();}});return chip;
    }
    private String unsignedMoney(long cents){return BigDecimal.valueOf(cents,2).abs().stripTrailingZeros().toPlainString();}
    private String signedMoney(long cents){return (cents<0?"−":cents>0?"+":"")+unsignedMoney(cents);}
    private void renderBills(LinearLayout body){
        LinearLayout list=column();list.setPadding(dp(22),dp(10),dp(22),dp(14));
        LinearLayout ranges=new LinearLayout(this);final String[] rangeLabels={"今天","近7天","本月","全部"};
        for(int i=0;i<rangeLabels.length;i++){final int range=i;ranges.addView(ledgerChip(rangeLabels[i],ledgerRange==i,new Runnable(){public void run(){changeLedgerFilter(range,ledgerKind);}}),new LinearLayout.LayoutParams(0,dp(34),1));}addFull(list,ranges,34);
        JSONObject summary=ledger==null?null:ledger.optJSONObject("summary");
        TextView rangeTitle=text(summary==null?"读取本期收支…":rangeLabels[ledgerRange]+" · "+summary.optLong("transactionCount")+"笔收支",12,GRAY);rangeTitle.setPadding(0,dp(20),0,dp(12));list.addView(rangeTitle);
        LinearLayout totals=new LinearLayout(this);
        LinearLayout income=column();income.addView(text("收入（超频点数）",12,GRAY));TextView incomeAmount=text(summary==null?"—":money(summary.optLong("incomeCents")),21,INK);incomeAmount.setPadding(0,dp(8),0,0);income.addView(incomeAmount);totals.addView(income,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));
        LinearLayout expense=column();expense.addView(text("支出（超频点数）",12,GRAY));TextView expenseAmount=text(summary==null?"—":money(summary.optLong("expenseCents")),21,INK);expenseAmount.setPadding(0,dp(8),0,0);expense.addView(expenseAmount);totals.addView(expense,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));list.addView(totals);
        TextView net=text(summary==null?"净变化 —":"净变化 "+signedMoney(summary.optLong("netCents")),12,GRAY);net.setPadding(0,dp(10),0,dp(18));list.addView(net);divider(list);
        LinearLayout kinds=new LinearLayout(this);final String[] kindLabels={"全部收支","收入","支出"};
        for(int i=0;i<kindLabels.length;i++){final int kind=i;kinds.addView(ledgerChip(kindLabels[i],ledgerKind==i,new Runnable(){public void run(){changeLedgerFilter(ledgerRange,kind);}}),new LinearLayout.LayoutParams(0,dp(38),1));}addFull(list,kinds,38);
        if(ledgerEntries.length()==0)addFull(list,center(busy?"正在读取账单…":"这段时间还没有收支记录",14,GRAY),100);
        for(int i=0;i<ledgerEntries.length();i++){final JSONObject item=ledgerEntries.optJSONObject(i);if(item==null)continue;
            LinearLayout row=new LinearLayout(this);row.setGravity(Gravity.CENTER_VERTICAL);row.setPadding(0,dp(16),0,dp(16));LinearLayout words=column();TextView label=text(item.optString("label","收支"),15,INK);label.setMaxLines(2);words.addView(label);TextView when=text(date(item.optLong("createdAt"),"MM月dd日 HH:mm"),12,GRAY);when.setPadding(0,dp(7),0,0);words.addView(when);row.addView(words,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));
            long cents=item.optLong("amountCents");TextView delta=text((cents>=0?"+":"−")+unsignedMoney(cents),18,"credit".equals(item.optString("kind"))?BLUE:INK);delta.setPadding(dp(12),0,0,0);row.addView(delta);TextView arrow=text(" ›",16,GRAY);row.addView(arrow);row.setBackground(new RippleDrawable(android.content.res.ColorStateList.valueOf(Color.argb(25,22,119,255)),null,null));row.setOnClickListener(new View.OnClickListener(){public void onClick(View v){showLedgerDetail(item);}});list.addView(row);divider(list);
        }
        long total=ledger==null?0:ledger.optLong("total");
        if(ledgerEntries.length()<total)addFull(list,action(busy?"正在加载…":"查看更多记录",false,active(),new Runnable(){public void run(){requestLedger(ledgerPage+1);}}),46);
        else if(ledgerEntries.length()>0){TextView end=center("已展示全部 "+total+" 条记录",11,GRAY);end.setPadding(0,dp(16),0,0);list.addView(end);}
        final ScrollView viewport=scrolling(body,list);ledgerScroll=viewport;final int restore=ledgerScrollY;viewport.post(new Runnable(){public void run(){if(ledgerScroll==viewport)viewport.scrollTo(0,restore);}});
    }
    private void ledgerDetailField(LinearLayout parent,String label,String value){
        if(value==null || value.isEmpty())return;TextView name=text(label,12,GRAY);name.setPadding(0,dp(16),0,dp(6));parent.addView(name);TextView content=text(value,15,INK);content.setTextIsSelectable(true);content.setLineSpacing(dp(3),1);parent.addView(content);
    }
    private void showLedgerDetail(JSONObject item){
        final Dialog detail=new Dialog(this);detail.requestWindowFeature(Window.FEATURE_NO_TITLE);LinearLayout outer=column();outer.setPadding(dp(24),dp(20),dp(24),dp(16));outer.setBackgroundColor(Color.WHITE);
        addFull(outer,center("账单详情",18,INK),38);TextView title=center(item.optString("label","收支记录"),16,INK);title.setMaxLines(3);title.setLineSpacing(dp(4),1);outer.addView(title);
        addFull(outer,center(signedMoney(item.optLong("amountCents"))+" 点",30,INK),56);
        LinearLayout content=column();ledgerDetailField(content,"交易时间",date(item.optLong("createdAt"),"yyyy年MM月dd日 HH:mm:ss"));ledgerDetailField(content,"流水号",item.optString("id"));ledgerDetailField(content,"关联订单",item.optString("orderId"));ledgerDetailField(content,"入账原因","credit".equals(item.optString("kind"))?item.optString("label"):"");ledgerDetailField(content,"原因编号",item.optString("reasonId"));ledgerDetailField(content,"交易后余额（超频点数）",money(item.optLong("balanceCents")));
        ScrollView scroll=new ScrollView(this);scroll.addView(content);outer.addView(scroll,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,0,1));addFull(outer,action("完成",true,true,new Runnable(){public void run(){detail.dismiss();}}),46);detail.setContentView(outer);
        Window window=detail.getWindow();if(window!=null){window.setBackgroundDrawableResource(android.R.color.white);window.setDimAmount(.30f);}detail.show();window=detail.getWindow();if(window!=null)attachWalletWindow(window.getDecorView());if(window!=null){window.setLayout(Math.min(dp(430),getResources().getDisplayMetrics().widthPixels-dp(36)),Math.min(dp(560),getResources().getDisplayMetrics().heightPixels-dp(48)));window.setGravity(Gravity.CENTER);}
    }
    private JSONObject currentOrder(){JSONArray orders=array(wallet,"pendingOrders");for(int i=0;i<orders.length();i++){JSONObject order=orders.optJSONObject(i);if(order!=null && order.optString("orderId").equals(selectedOrderId))return order;}return null;}
    private void renderOrders(LinearLayout body){
        LinearLayout list=column();list.setPadding(dp(22),dp(8),dp(22),dp(8));JSONArray orders=array(wallet,"pendingOrders");
        if(orders.length()==0)addFull(list,center("暂无待支付订单",15,GRAY),100);
        for(int i=0;i<orders.length();i++){final JSONObject order=orders.optJSONObject(i);if(order==null)continue;TextView row=text(order.optString("productName","游戏商品")+"\n"+money(order.optLong("amountCents"))+" 超频点数    ›",16,INK);row.setLineSpacing(dp(7),1);row.setPadding(0,dp(18),0,dp(18));list.addView(row);divider(list);row.setOnClickListener(new View.OnClickListener(){public void onClick(View v){selectedOrderId=order.optString("orderId");navigate(CASHIER);}});}scrolling(body,list);
    }
    private void renderCashier(LinearLayout body){
        body.setPadding(dp(26),dp(10),dp(26),dp(18));final JSONObject order=currentOrder();
        if(order==null){space(body,1);addFull(body,center("暂无待支付订单",15,GRAY),50);space(body,1);addFull(body,action("返回终端",true,true,new Runnable(){public void run(){navigate(HOME);}}),48);return;}
        space(body,.5f);TextView product=center(order.optString("productName","游戏商品"),15,GRAY);product.setMaxLines(2);body.addView(product);addFull(body,center(money(order.optLong("amountCents"))+" 点",36,INK),68);space(body,.7f);
        LinearLayout method=new LinearLayout(this);method.setGravity(Gravity.CENTER_VERTICAL);method.addView(text("付款方式",14,GRAY),new LinearLayout.LayoutParams(0,dp(36),1));TextView value=text("超频点数 "+(wallet==null?"—":money(wallet.optLong("balanceCents"))),14,INK);value.setGravity(Gravity.CENTER_VERTICAL);method.addView(value);addFull(body,method,36);divider(body);
        final boolean enough=wallet!=null && wallet.optLong("balanceCents")>=order.optLong("amountCents");
        if(!enough){TextView hint=text("超频点数不足",12,Color.rgb(210,90,30));hint.setPadding(0,dp(8),0,dp(8));body.addView(hint);}
        LinearLayout.LayoutParams buttonSpace=new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(48));buttonSpace.topMargin=dp(14);
        body.addView(action(busy?"处理中…":enough?"确认付款":"超频点数不足",true,active() && enough,new Runnable(){public void run(){pay(order);}}),buttonSpace);
    }

    private void renderRequestState(){render();}
    private void rememberLedgerScroll(){if(page==BILLS && ledgerScroll!=null)ledgerScrollY=ledgerScroll.getScrollY();}

    private void sessionChanged(){HttpURLConnection preflight=rechargeConnection;if(preflight!=null)preflight.disconnect();rechargeGeneration++;HttpURLConnection connection=activeConnection;if(connection!=null)connection.disconnect();selectedOrderId=null;nicknameInput=null;oldPasswordInput=null;newPasswordInput=null;confirmationInput=null;wallet=null;incomeEntries=new JSONArray();seenConfirmedRevision=0;homeScrollY=0;updateUnreadBadge();ledger=null;account=null;nicknameDraft="";ledgerEntries=new JSONArray();ledgerPage=1;ledgerScrollY=0;busy=false;refreshQueued=false;page=ACCOUNT_HOME;message="";loadPendingRequests();if(panel!=null && panel.isShowing()){render();refreshPage();}pollHandler.removeCallbacks(statePoll);if(foreground)pollHandler.post(statePoll);}
    private SharedPreferences prefs(){return getSharedPreferences("local-wallet-pending",MODE_PRIVATE);}
    private String storageOrigin(){
        if(origin==null)return "";
        try{URI uri=new URI(origin);String scheme=uri.getScheme().toLowerCase(Locale.US),host=uri.getHost().toLowerCase(Locale.US);int port=uri.getPort();
            if(("http".equals(scheme) && port==80) || ("https".equals(scheme) && port==443))port=-1;
            return new URI(scheme,null,host,port,null,null,null).toASCIIString();
        }catch(Exception invalid){return "";}
    }
    private String burstStorageKey(long player){return "burst.v2."+storageOrigin()+"."+player;}
    private void loadPendingRequests(){
        burstRequestId=null;SharedPreferences preferences=prefs();SharedPreferences.Editor retired=preferences.edit();
        for(String key:preferences.getAll().keySet())if(key.startsWith("tool.") || key.startsWith("add."))retired.remove(key);retired.commit();
        if(!credentials.valid() || storageOrigin().isEmpty())return;
        // Bind all legacy single-server keys to one origin exactly once. Other
        // origins may never consume a legacy player-only pending request.
        String legacyOrigin=preferences.getString("burstLegacyOrigin",null);
        if(legacyOrigin==null){if(!preferences.edit().putString("burstLegacyOrigin",storageOrigin()).commit())return;legacyOrigin=storageOrigin();}
        String key=burstStorageKey(credentials.player),id=preferences.getString(key,null);
        String legacyKey="burst."+credentials.player;
        if(storageOrigin().equals(legacyOrigin) && preferences.contains(legacyKey)){
            String legacy=preferences.getString(legacyKey,null);SharedPreferences.Editor migration=preferences.edit().remove(legacyKey);
            if(id==null && legacy!=null && legacy.matches("[a-fA-F0-9-]{36}")){id=legacy;migration.putString(key,id);}
            if(!migration.commit())return;
        }
        if(id!=null && id.matches("[a-fA-F0-9-]{36}"))burstRequestId=id;
    }
    private void burst(){
        if(!active())return;
        try{if(burstRequestId==null)burstRequestId=UUID.randomUUID().toString();
            if(!prefs().edit().putString(burstStorageKey(credentials.player),burstRequestId).commit())throw new Exception("保存请求失败，请重试");
            JSONObject payload=new JSONObject();payload.put("requestId",burstRequestId);request("/local-wallet/burst",payload,true);
        }catch(Exception failure){setMessage("保存请求失败，请重试",true);}
    }
    private void acceptTerminal(JSONObject result){
        JSONObject terminal=result.optJSONObject("terminal");if(terminal==null)return;
        JSONArray incoming=array(terminal,"incomeEntries");
        // Keep cards received in this opening even if seen returns an empty unread list.
        java.util.LinkedHashMap<String,JSONObject> retained=new java.util.LinkedHashMap<String,JSONObject>();
        for(int i=0;i<incoming.length();i++){JSONObject e=incoming.optJSONObject(i);if(e!=null)retained.put(e.optString("id"),e);}
        for(int i=0;i<incomeEntries.length();i++){JSONObject e=incomeEntries.optJSONObject(i);if(e!=null && !retained.containsKey(e.optString("id")))retained.put(e.optString("id"),e);}
        incomeEntries=new JSONArray();for(JSONObject e:retained.values()){if(incomeEntries.length()>=20)break;incomeEntries.put(e);}
    }
    private void confirmSeen(){
        if(panel==null || !panel.isShowing() || page!=HOME || !active())return;
        long visibleRevision=0;for(int i=0;i<incomeEntries.length();i++){JSONObject e=incomeEntries.optJSONObject(i);if(e!=null)visibleRevision=Math.max(visibleRevision,e.optLong("revision"));}
        if(visibleRevision<=seenConfirmedRevision)return;
        try{JSONObject payload=new JSONObject();payload.put("seenRevision",visibleRevision);request("/local-wallet/seen",payload,false,true);}catch(Exception ignored){}
    }
    private void refreshPage(){
        if(busy){refreshQueued=true;return;}
        if(!active())return;
        if(page==ACCOUNT_HOME || page==ACCOUNT || (page==NICKNAME || page==PASSWORD) && account==null)request("/game-account/profile",new JSONObject(),false);else if(page==NICKNAME || page==PASSWORD)return;else if(page==BILLS)requestLedger(1);else request("/local-wallet/state",new JSONObject(),false);
    }
    private void requestLedger(int number){
        if(!active())return;
        try{
            JSONObject payload=new JSONObject();payload.put("page",number);payload.put("pageSize",30);payload.put("kind",ledgerKind==1?"credit":ledgerKind==2?"payment":"");
            if(ledgerRange!=3){
                Calendar start=Calendar.getInstance(TimeZone.getTimeZone("Asia/Shanghai"),Locale.CHINA);start.set(Calendar.HOUR_OF_DAY,0);start.set(Calendar.MINUTE,0);start.set(Calendar.SECOND,0);start.set(Calendar.MILLISECOND,0);
                if(ledgerRange==1)start.add(Calendar.DAY_OF_MONTH,-6);else if(ledgerRange==2)start.set(Calendar.DAY_OF_MONTH,1);
                payload.put("startAt",start.getTimeInMillis());
                Calendar end=Calendar.getInstance(TimeZone.getTimeZone("Asia/Shanghai"),Locale.CHINA);end.set(Calendar.HOUR_OF_DAY,0);end.set(Calendar.MINUTE,0);end.set(Calendar.SECOND,0);end.set(Calendar.MILLISECOND,0);end.add(Calendar.DAY_OF_MONTH,1);payload.put("endAt",end.getTimeInMillis());
            }else{payload.put("startAt",0);payload.put("endAt",0);}
            request("/local-wallet/ledger",payload,false);
        }catch(Exception invalid){setMessage("账单筛选无效，请重试",true);}
    }
    private void pay(JSONObject order){
        if(!active())return;
        try{JSONObject payload=new JSONObject();payload.put("orderId",order.getString("orderId"));request("/local-wallet/pay",payload,true);}catch(Exception invalid){setMessage("订单信息无效，请返回后重试",true);}
    }

    // Only authenticated server results update money, summaries and receipts.
    // Sessions remain memory-only and old-generation replies drop.
    private void request(final String route,final JSONObject payload,final boolean mutation){request(route,payload,mutation,false);}
    private void request(final String route,final JSONObject payload,final boolean mutation,final boolean quiet){
        if(busy || destroyed || !ready())return;final Credentials auth=credentials;final String address=origin+route;final boolean accountRoute=route.startsWith("/game-account/");final String accountUsername=account==null?"":account.optString("username");busy=true;if(mutation)setMessage("处理中…",false);if(!quiet)renderRequestState();
        network.execute(new Runnable(){public void run(){if(destroyed || credentials.generation!=auth.generation){payload.remove("oldPassword");payload.remove("password");payload.remove("confirmPassword");return;}JSONObject data=null;String error=null;HttpURLConnection connection=null;
            try{
                payload.put("playerId",auth.player);payload.put("sessionHex",auth.session);
                connection=(HttpURLConnection)new URL(address).openConnection();activeConnection=connection;connection.setRequestMethod("POST");connection.setConnectTimeout(8000);connection.setReadTimeout(15000);connection.setInstanceFollowRedirects(false);connection.setDoOutput(true);connection.setRequestProperty("Content-Type","application/json; charset=utf-8");connection.setRequestProperty("Cache-Control","no-store");
                byte[] body=payload.toString().getBytes("UTF-8");connection.setFixedLengthStreamingMode(body.length);OutputStream out=connection.getOutputStream();out.write(body);out.close();
                int status=connection.getResponseCode();InputStream in=status>=400?connection.getErrorStream():connection.getInputStream();if(in==null)throw new Exception("连接失败，请重试");byte[] raw=readBounded(in,1048576);in.close();JSONObject reply=new JSONObject(new String(raw,"UTF-8"));
                if(status!=200 || reply.optInt("code",-1)!=0)throw new Exception(reply.optString("msg","操作未完成，请重试"));data=reply.getJSONObject("data");
                if("/game-account/logout".equals(route)){
                    if(!data.optBoolean("loggedOut",false))throw new Exception("退出登录未确认，请重试");
                }else if(accountRoute){
                    if(data.optLong("id",0)<1 || data.optString("username").isEmpty() || data.optString("nickname").isEmpty())throw new Exception("账号信息无效，请重试");
                    if("/game-account/password".equals(route) && !data.optBoolean("passwordChanged",false))throw new Exception("密码修改未确认，请重试");
                }else{
                if(!data.optBoolean("testOnly",false))throw new Exception("返回数据无效，请重试");
                if(!"CNY".equals(data.optString("currency")) || !data.has("balanceCents") || data.getLong("balanceCents")<0)throw new Exception("余额数据无效，请重试");
                if(("/local-wallet/state".equals(route) || "/local-wallet/burst".equals(route) || "/local-wallet/seen".equals(route)) && (data.optJSONObject("terminal")==null || data.getJSONObject("terminal").optJSONArray("incomeEntries")==null))throw new Exception("终端数据无效，请重试");
                if("/local-wallet/burst".equals(route) && (!data.has("burstAwardPoints") || data.getDouble("burstAwardPoints")<0))throw new Exception("到账结果未确认，请重试");
                if("/local-wallet/ledger".equals(route) && (data.optJSONObject("summary")==null || data.optJSONArray("entries")==null || data.optInt("page",0)<1))throw new Exception("账单数据无效，请重试");
                }
            }catch(Exception failure){error=failure.getMessage();if(error==null || error.length()>120 || error.contains(auth.session))error="连接中断，请重试";}
            finally{if(connection!=null)connection.disconnect();if(activeConnection==connection)activeConnection=null;}
            final JSONObject result=data;final String failure=error;
            runOnUiThread(new Runnable(){public void run(){
                if(destroyed || credentials.generation!=auth.generation){payload.remove("oldPassword");payload.remove("password");payload.remove("confirmPassword");return;}busy=false;
                if(failure==null){
                    if("/game-account/logout".equals(route)){completeLogout();return;}
                    if(accountRoute){
                        account=result;
                        if("/game-account/password".equals(route)){LoginCache.passwordChanged(accountUsername,payload.optString("password"));page=ACCOUNT_HOME;setMessage("密码已修改",false);}
                        else if("/game-account/nickname".equals(route)){nicknameDraft=result.optString("nickname");page=ACCOUNT_HOME;setMessage("账号名称已更新",false);}
                    }else{
                    if(result.optJSONObject("terminal")==null && terminal()!=null)try{result.put("terminal",terminal());}catch(Exception ignored){}
                    wallet=result;acceptTerminal(result);updateUnreadBadge();
                    if("/local-wallet/ledger".equals(route)){
                        if(result.optInt("page",1)==1)ledgerEntries=new JSONArray();
                        JSONArray incoming=array(result,"entries");for(int i=0;i<incoming.length();i++)ledgerEntries.put(incoming.optJSONObject(i));
                        ledger=result;ledgerPage=result.optInt("page",1);
                    }
                    if("/local-wallet/seen".equals(route)){seenConfirmedRevision=Math.max(seenConfirmedRevision,payload.optLong("seenRevision"));}
                    else if("/local-wallet/burst".equals(route)){
                        prefs().edit().remove(burstStorageKey(auth.player)).commit();burstRequestId=null;ledger=null;ledgerEntries=new JSONArray();setMessage("已到账 "+pointNumber(result.optDouble("burstAwardPoints"))+" 点",false);
                    }else if("/local-wallet/pay".equals(route)){ledger=null;ledgerEntries=new JSONArray();UnityPlayer.UnitySendMessage("LocalWalletSession","RefreshAfterPayment","");page=HOME;setMessage("支付成功，去享受这一刻",false);}
                    }
                }else if(!quiet)setMessage(failure+(mutation?"；可重试原操作":""),true);
                payload.remove("oldPassword");payload.remove("password");payload.remove("confirmPassword");
                if(!quiet || (page==HOME && panel!=null && panel.isShowing()))renderRequestState();
                if(failure==null && !quiet && !accountRoute && !"/local-wallet/seen".equals(route) && panel!=null && panel.isShowing() && page==HOME)shell.post(()->confirmSeen());
                if(refreshQueued){refreshQueued=false;refreshPage();}
            }});
        }});
    }
    private String readAsset(String name)throws Exception{InputStream in=getAssets().open(name);try{return new String(readBounded(in,16384),"UTF-8");}finally{in.close();}}
    private static byte[] readBounded(InputStream in,int limit)throws Exception{ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] buffer=new byte[4096];int n;while((n=in.read(buffer))!=-1){if(out.size()+n>limit)throw new Exception("响应过长");out.write(buffer,0,n);}return out.toByteArray();}
}
