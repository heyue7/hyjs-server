package com.hyjs.wallet;

import android.app.Dialog;
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
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
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
import java.util.Calendar;
import java.util.Collections;
import java.util.Locale;
import java.util.TimeZone;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** A native wallet layered on the original Unity activity; the engine remains original. */
public final class WalletActivity extends UnityPlayerActivity {
    private static final int BLUE=Color.rgb(22,119,255), INK=Color.rgb(17,17,17), GRAY=Color.rgb(128,128,128), LINE=Color.rgb(240,240,240);
    private static final int HOME=0, ADD=3, BILLS=4, ORDERS=5, CASHIER=6;
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

    private final ExecutorService network=Executors.newSingleThreadExecutor();
    private FrameLayout overlay;
    private View bubble;
    private Dialog panel;
    private LinearLayout shell;
    private EditText amount;
    private TextView feedback;
    private JSONObject wallet, ledger;
    private JSONArray ledgerEntries=new JSONArray();
    private String origin, creditRequestId, selectedOrderId;
    private long creditAmount;
    private String amountDraft="100", message="";
    private boolean messageError, busy, destroyed, dockRight, dragging, keyboardBack, refreshQueued;
    private int page=HOME, addReturn=HOME;
    private int ledgerRange=2, ledgerKind=0, ledgerPage=1, ledgerScrollY=0;
    private ScrollView ledgerScroll;
    private final Runnable dimBubble=new Runnable(){public void run(){
        if(!destroyed && bubble!=null && !dragging && (panel==null || !panel.isShowing())){
            bubble.animate().cancel();bubble.animate().alpha(.40f).setDuration(180).start();
        }
    }};

    // Clip the drawable itself; Unity's window can disable outline clipping.
    private static final class CircularWalletIcon extends ImageView {
        private final Path circle=new Path();
        private final Paint surface=new Paint(Paint.ANTI_ALIAS_FLAG);
        CircularWalletIcon(android.content.Context context){super(context);surface.setColor(Color.WHITE);}
        @Override protected void onSizeChanged(int w,int h,int oldw,int oldh){
            super.onSizeChanged(w,h,oldw,oldh);circle.reset();
            circle.addCircle(w/2f,h/2f,Math.min(w,h)/2f,Path.Direction.CW);
        }
        @Override protected void onDraw(Canvas canvas){
            canvas.drawCircle(getWidth()/2f,getHeight()/2f,Math.min(getWidth(),getHeight())/2f,surface);
            int saved=canvas.save();canvas.clipPath(circle);super.onDraw(canvas);canvas.restoreToCount(saved);
        }
    }

    // Clip the hidden half at the usable game edge, including immersive bars
    // and cutouts. The remaining half stays inside the app's touchable area.
    private final class WalletOverlay extends FrameLayout {
        private int pointer=-1;
        private float downX,downY,startX,startY;
        private boolean moved;
        private final int[] screenPosition=new int[2];
        WalletOverlay(){super(WalletActivity.this);}
        @Override protected void dispatchDraw(Canvas canvas){
            int saved=canvas.save();canvas.clipRect(bubbleBounds());super.dispatchDraw(canvas);canvas.restoreToCount(saved);
        }
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
                pointer=event.getPointerId(0);dragging=true;moved=false;
                if(getParent()!=null)getParent().requestDisallowInterceptTouchEvent(true);
                revealBubble();bubble.setPressed(true);
                downX=x;downY=y;startX=bubble.getX();startY=bubble.getY();
                requestUnbufferedDispatch(event);
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
                    startX=bubble.getX();startY=bubble.getY();moved=true;return true;
                }
                if(action==MotionEvent.ACTION_MOVE || action==MotionEvent.ACTION_UP){
                    float dx=x+event.getX(index)-event.getX(0)-downX,dy=y+event.getY(index)-event.getY(0)-downY;
                    int slop=ViewConfiguration.get(WalletActivity.this).getScaledTouchSlop();
                    if(Math.hypot(dx,dy)>slop)moved=true;
                    if(moved){
                        Rect bounds=bubbleBounds();
                        bubble.setX(Math.max(bounds.left,Math.min(bounds.right-bubble.getWidth(),startX+dx)));
                        bubble.setY(clampBubbleY(startY+dy,bounds));updateBubbleGestureExclusion();invalidate();
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
            bubble.setPressed(false);
            if(click){bubble.performClick();return;}
            Rect bounds=bubbleBounds();dockRight=bubble.getX()+bubble.getWidth()/2f>=(bounds.left+bounds.right)/2f;
            foldBubble(true);scheduleDim();
        }
    }

    @Override public boolean dispatchTouchEvent(MotionEvent event){
        // Capture the wallet at the activity boundary before Unity/decor dispatch.
        // Touches starting anywhere else keep their original game routing.
        if(overlay instanceof WalletOverlay && ((WalletOverlay)overlay).dispatchWalletTouch(event))return true;
        return super.dispatchTouchEvent(event);
    }

    @Override protected void onCreate(Bundle state){
        try { LocalAssetOverlay.synchronize(this); } catch(java.io.IOException failure) { throw new IllegalStateException("本地游戏资源刷新失败", failure); }
        super.onCreate(state);current=this;loadPendingRequests();
        try{
            JSONObject config=new JSONObject(readAsset("local-wallet.json"));URI uri=new URI(config.getString("origin"));
            if(!("http".equals(uri.getScheme()) || "https".equals(uri.getScheme())) || uri.getHost()==null || uri.getUserInfo()!=null || uri.getQuery()!=null || uri.getFragment()!=null || !(uri.getPath().isEmpty() || "/".equals(uri.getPath())))throw new Exception();
            origin=config.getString("origin").replaceAll("/+$","");
        }catch(Exception error){origin=null;}
        getWindow().getDecorView().post(new Runnable(){public void run(){attachBubble();}});
    }
    @Override protected void onResume(){super.onResume();current=this;if(panel!=null && panel.isShowing())refreshPage();}
    @Override protected void onDestroy(){
        destroyed=true;if(current==this)current=null;
        if(bubble!=null){bubble.removeCallbacks(dimBubble);bubble.animate().cancel();}
        if(panel!=null)panel.dismiss();network.shutdownNow();super.onDestroy();
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
    private void addFull(LinearLayout parent,View child,int height){parent.addView(child,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(height)));}
    private void space(LinearLayout parent,float weight){parent.addView(new View(this),new LinearLayout.LayoutParams(1,0,weight));}
    private void divider(LinearLayout parent){View line=new View(this);line.setBackgroundColor(LINE);parent.addView(line,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(1)));}
    private boolean ready(){return credentials.valid() && origin!=null;}
    private boolean active(){return ready() && !busy;}
    private static String money(long cents){return BigDecimal.valueOf(cents,2).setScale(2).toPlainString();}
    private static JSONArray array(JSONObject data,String key){JSONArray a=data==null?null:data.optJSONArray(key);return a==null?new JSONArray():a;}
    private static String date(long ms,String pattern){SimpleDateFormat f=new SimpleDateFormat(pattern,Locale.CHINA);f.setTimeZone(TimeZone.getTimeZone("Asia/Shanghai"));return f.format(new Date(ms));}

    // Only the visible half remains inside the inset-safe game area when docked.
    private void attachBubble(){
        if(destroyed || overlay!=null)return;
        ViewGroup decor=(ViewGroup)getWindow().getDecorView();decor.setClipChildren(false);decor.setClipToPadding(false);
        overlay=new WalletOverlay();overlay.setClipChildren(false);overlay.setClipToPadding(false);overlay.setClickable(false);
        decor.addView(overlay,new ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));
        ImageView icon=new CircularWalletIcon(this);icon.setContentDescription("蓝绿钱包，点击展开，拖动位置");icon.setScaleType(ImageView.ScaleType.CENTER_CROP);
        try{InputStream in=getAssets().open("local-wallet/icon.png");BitmapFactory.Options options=new BitmapFactory.Options();options.inSampleSize=4;Bitmap bitmap=BitmapFactory.decodeStream(in,null,options);in.close();if(bitmap!=null)icon.setImageBitmap(bitmap);}catch(Exception unavailable){icon.setImageResource(android.R.drawable.ic_menu_save);icon.setColorFilter(BLUE);}
        bubble=icon;bubble.setAlpha(.40f);
        FrameLayout.LayoutParams lp=new FrameLayout.LayoutParams(dp(48),dp(48));lp.gravity=Gravity.TOP|Gravity.LEFT;lp.topMargin=dp(70);overlay.addView(icon,lp);
        if(Build.VERSION.SDK_INT>=29){
            icon.addOnLayoutChangeListener(new View.OnLayoutChangeListener(){public void onLayoutChange(View v,int l,int t,int r,int b,int ol,int ot,int or,int ob){updateBubbleGestureExclusion();}});
            icon.animate().setUpdateListener(new ValueAnimator.AnimatorUpdateListener(){public void onAnimationUpdate(ValueAnimator animation){updateBubbleGestureExclusion();}});
        }
        icon.setOnClickListener(new View.OnClickListener(){public void onClick(View v){revealBubble();openPanel();}});
        overlay.addOnLayoutChangeListener(new View.OnLayoutChangeListener(){public void onLayoutChange(View v,int l,int t,int r,int b,int ol,int ot,int or,int ob){
            repositionBubble();
        }});
        overlay.setOnApplyWindowInsetsListener(new View.OnApplyWindowInsetsListener(){public WindowInsets onApplyWindowInsets(View view,WindowInsets insets){
            view.post(new Runnable(){public void run(){repositionBubble();}});return insets;
        }});
        overlay.requestApplyInsets();icon.post(new Runnable(){public void run(){repositionBubble();}});
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
    private Rect bubbleTouchBounds(){
        Rect safe=bubbleBounds();
        if(bubble==null || safe.isEmpty())return new Rect();
        int size=Math.max(dp(48),bubble.getWidth()),height=Math.max(dp(48),bubble.getHeight());
        int left=Math.max(safe.left,Math.min(safe.right-size,Math.round(bubble.getX())));
        int top=Math.max(safe.top,Math.min(safe.bottom-height,Math.round(bubble.getY())));
        return new Rect(left,top,Math.min(safe.right,left+size),Math.min(safe.bottom,top+height));
    }
    private float clampBubbleY(float y,Rect bounds){return Math.max(bounds.top+dp(6),Math.min(bounds.bottom-bubble.getHeight()-dp(6),y));}
    private void updateBubbleGestureExclusion(){
        // Keep the visible handle in the overlay's stable coordinate space.
        // All remaining screen edges retain the system's normal Back gesture.
        if(Build.VERSION.SDK_INT>=29 && overlay!=null && bubble!=null && bubble.getWidth()>0 && bubble.getHeight()>0)
            overlay.setSystemGestureExclusionRects(Collections.singletonList(bubbleTouchBounds()));
    }
    private void repositionBubble(){
        if(bubble==null || overlay.getWidth()==0)return;bubble.setY(clampBubbleY(bubble.getY(),bubbleBounds()));
        if(!dragging){if(panel!=null && panel.isShowing())revealBubble();else foldBubble(false);}updateBubbleGestureExclusion();overlay.invalidate();
    }
    private void revealBubble(){if(bubble==null)return;bubble.removeCallbacks(dimBubble);bubble.animate().cancel();bubble.setAlpha(1);Rect bounds=bubbleBounds();bubble.setX(Math.max(bounds.left,Math.min(bounds.right-bubble.getWidth(),bubble.getX())));updateBubbleGestureExclusion();}
    private void foldBubble(boolean animate){
        if(bubble==null || overlay.getWidth()==0)return;bubble.animate().cancel();
        Rect bounds=bubbleBounds();float x=dockRight?bounds.right-bubble.getWidth()/2f:bounds.left-bubble.getWidth()/2f;
        if(animate)bubble.animate().x(x).setDuration(160).start();else bubble.setX(x);
        updateBubbleGestureExclusion();
    }
    private void scheduleDim(){if(bubble!=null && !destroyed){bubble.removeCallbacks(dimBubble);bubble.postDelayed(dimBubble,1500);}}

    private void openPanel(){
        if(destroyed || (panel!=null && panel.isShowing()))return;
        revealBubble();page=HOME;message="";messageError=false;
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
        panel.show();window.setGravity(Gravity.CENTER);
        render();refreshPage();
    }
    private void fitPanel(){
        if(panel==null || !panel.isShowing())return;
        android.util.DisplayMetrics metrics=getResources().getDisplayMetrics();
        boolean home=page==HOME;
        int width=Math.min(dp(home?392:440),metrics.widthPixels-dp(32));
        int height=Math.min(dp(home?440:600),Math.round(metrics.heightPixels*(home?.78f:.90f)));
        panel.getWindow().setLayout(width,height);
    }
    private void navigate(int destination){
        rememberLedgerScroll();hideKeyboard();page=destination;message="";messageError=false;render();refreshPage();
    }
    private void back(){
        hideKeyboard();
        if(page==HOME){panel.dismiss();return;}
        if(page==ADD){navigate(addReturn==CASHIER && currentOrder()!=null?CASHIER:HOME);return;}
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
    private String title(){switch(page){case ADD:return "增加余额";case BILLS:return "账单";case CASHIER:return "付款";case ORDERS:return "待支付订单";default:return "蓝绿钱包";}}
    private void render(){
        if(panel==null || !panel.isShowing())return;
        fitPanel();
        rememberLedgerScroll();ledgerScroll=null;
        boolean wasEditing=amount!=null && amount.hasFocus();amount=null;shell.removeAllViews();
        FrameLayout nav=new FrameLayout(this);
        TextView title=center(title(),18,INK);nav.addView(title,new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(52)));
        TextView back=center("‹",34,INK);back.setContentDescription(page==HOME?"收起蓝绿钱包":"返回");FrameLayout.LayoutParams left=new FrameLayout.LayoutParams(dp(56),dp(52),Gravity.LEFT);nav.addView(back,left);back.setOnClickListener(new View.OnClickListener(){public void onClick(View v){back();}});
        if(page==HOME){TextView bill=center("账单",14,INK);FrameLayout.LayoutParams right=new FrameLayout.LayoutParams(dp(64),dp(52),Gravity.RIGHT);nav.addView(bill,right);bill.setOnClickListener(new View.OnClickListener(){public void onClick(View v){navigate(BILLS);}});}
        else if(page==BILLS){TextView reload=center("刷新",14,BLUE);reload.setEnabled(!busy);nav.addView(reload,new FrameLayout.LayoutParams(dp(64),dp(52),Gravity.RIGHT));reload.setOnClickListener(new View.OnClickListener(){public void onClick(View v){refreshPage();}});}
        addFull(shell,nav,52);divider(shell);
        LinearLayout body=column();shell.addView(body,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,0,1));
        switch(page){case ADD:renderAdd(body);break;case BILLS:renderBills(body);break;case ORDERS:renderOrders(body);break;case CASHIER:renderCashier(body);break;default:renderHome(body);}
        feedback=center("",12,GRAY);feedback.setPadding(dp(20),dp(6),dp(20),dp(6));feedback.setMaxLines(2);shell.addView(feedback);
        if(!credentials.valid())setMessage("登录游戏后可用",false);else if(origin==null)setMessage("蓝绿钱包连接未配置",true);else setMessage(message,messageError);
        if(wasEditing && page==ADD && amount!=null && !busy){amount.requestFocus();amount.setSelection(amount.length());}
    }
    private void renderHome(LinearLayout body){
        body.setPadding(dp(24),0,dp(24),dp(14));space(body,.8f);
        addFull(body,center("账户余额（元）",13,GRAY),24);
        TextView balance=center(wallet==null?"—":money(wallet.optLong("balanceCents")),36,INK);
        balance.setSingleLine(true);if(Build.VERSION.SDK_INT>=26)balance.setAutoSizeTextTypeUniformWithConfiguration(18,36,1,android.util.TypedValue.COMPLEX_UNIT_SP);
        addFull(body,balance,54);
        space(body,1f);
        final JSONArray orders=array(wallet,"pendingOrders");
        if(orders.length()>0){TextView entry=text("待支付订单  "+orders.length()+"  ›",14,BLUE);entry.setGravity(Gravity.CENTER_VERTICAL);addFull(body,entry,42);entry.setOnClickListener(new View.OnClickListener(){public void onClick(View v){if(orders.length()==1){selectedOrderId=orders.optJSONObject(0).optString("orderId");navigate(CASHIER);}else navigate(ORDERS);}});}
        addFull(body,action("增加余额",true,active(),new Runnable(){public void run(){addReturn=HOME;navigate(ADD);}}),48);
        TextView virtual=center("本地虚拟余额",11,GRAY);virtual.setPadding(0,dp(10),0,0);body.addView(virtual);
    }
    private void renderAdd(LinearLayout body){
        LinearLayout form=column();ScrollView viewport=new ScrollView(this);viewport.setFillViewport(true);viewport.addView(form);body.addView(viewport,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));body=form;
        body.setPadding(dp(26),dp(16),dp(26),dp(18));
        addFull(body,text("增加金额",14,GRAY),28);
        LinearLayout row=new LinearLayout(this);row.setGravity(Gravity.CENTER_VERTICAL);
        amount=new EditText(this);amount.setSingleLine(true);amount.setText(creditRequestId==null?amountDraft:money(creditAmount));amount.setTextSize(36);amount.setTextColor(INK);amount.setSelectAllOnFocus(true);amount.setPadding(0,0,0,0);amount.setBackgroundColor(Color.TRANSPARENT);amount.setInputType(InputType.TYPE_CLASS_NUMBER|InputType.TYPE_NUMBER_FLAG_DECIMAL);
        amount.setImeOptions(EditorInfo.IME_FLAG_NO_EXTRACT_UI|EditorInfo.IME_FLAG_NO_FULLSCREEN|EditorInfo.IME_ACTION_DONE);amount.setContentDescription("增加金额，单位元");amount.setEnabled(ready() && creditRequestId==null);
        amount.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence s,int a,int c,int f){}public void onTextChanged(CharSequence s,int start,int before,int count){amountDraft=s.toString();}public void afterTextChanged(Editable e){}});
        amount.setOnEditorActionListener(new TextView.OnEditorActionListener(){public boolean onEditorAction(TextView v,int id,KeyEvent event){if(id==EditorInfo.IME_ACTION_DONE){hideKeyboard();return true;}return false;}});
        row.addView(amount,new LinearLayout.LayoutParams(0,dp(58),1));row.addView(center("元",18,INK),new LinearLayout.LayoutParams(dp(36),dp(58)));addFull(body,row,58);divider(body);
        TextView help=text(creditRequestId==null?"添一点底气，去享受游戏 · 本地虚拟余额":"上次增加结果待确认，重试会沿用原请求",12,GRAY);help.setPadding(0,dp(10),0,0);body.addView(help);
        space(body,1);addFull(body,action(busy?"处理中…":creditRequestId==null?"确认增加":"重试增加",true,active(),new Runnable(){public void run(){addMoney();}}),48);
    }
    private ScrollView scrolling(LinearLayout body,LinearLayout content){ScrollView scroll=new ScrollView(this);scroll.setFillViewport(false);scroll.addView(content);body.addView(scroll,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,ViewGroup.LayoutParams.MATCH_PARENT));return scroll;}
    private void changeLedgerFilter(int range,int kind){
        if(!active())return;ledgerRange=range;ledgerKind=kind;ledgerPage=1;ledgerScrollY=0;ledger=null;ledgerEntries=new JSONArray();message="";render();requestLedger(1);
    }
    private TextView ledgerChip(String label,boolean selected,final Runnable click){
        TextView chip=center(label,13,selected?BLUE:GRAY);chip.setPadding(dp(5),0,dp(5),0);chip.setBackground(shape(selected?Color.rgb(235,244,255):Color.WHITE,14));chip.setEnabled(active());chip.setAlpha(active()?1f:.5f);chip.setOnClickListener(new View.OnClickListener(){public void onClick(View v){click.run();}});return chip;
    }
    private String unsignedMoney(long cents){return BigDecimal.valueOf(cents,2).abs().setScale(2).toPlainString();}
    private String signedMoney(long cents){return (cents<0?"−":cents>0?"+":"")+unsignedMoney(cents);}
    private void renderBills(LinearLayout body){
        LinearLayout list=column();list.setPadding(dp(22),dp(10),dp(22),dp(14));
        LinearLayout ranges=new LinearLayout(this);final String[] rangeLabels={"今天","近7天","本月","全部"};
        for(int i=0;i<rangeLabels.length;i++){final int range=i;ranges.addView(ledgerChip(rangeLabels[i],ledgerRange==i,new Runnable(){public void run(){changeLedgerFilter(range,ledgerKind);}}),new LinearLayout.LayoutParams(0,dp(34),1));}addFull(list,ranges,34);
        JSONObject summary=ledger==null?null:ledger.optJSONObject("summary");
        TextView rangeTitle=text(summary==null?"读取本期收支…":rangeLabels[ledgerRange]+" · "+summary.optLong("transactionCount")+"笔收支",12,GRAY);rangeTitle.setPadding(0,dp(20),0,dp(12));list.addView(rangeTitle);
        LinearLayout totals=new LinearLayout(this);
        LinearLayout income=column();income.addView(text("收入",12,GRAY));TextView incomeAmount=text(summary==null?"—":money(summary.optLong("incomeCents")),21,INK);incomeAmount.setPadding(0,dp(8),0,0);income.addView(incomeAmount);totals.addView(income,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));
        LinearLayout expense=column();expense.addView(text("支出",12,GRAY));TextView expenseAmount=text(summary==null?"—":money(summary.optLong("expenseCents")),21,INK);expenseAmount.setPadding(0,dp(8),0,0);expense.addView(expenseAmount);totals.addView(expense,new LinearLayout.LayoutParams(0,ViewGroup.LayoutParams.WRAP_CONTENT,1));list.addView(totals);
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
        addFull(outer,center(signedMoney(item.optLong("amountCents")),30,INK),56);
        LinearLayout content=column();ledgerDetailField(content,"交易时间",date(item.optLong("createdAt"),"yyyy年MM月dd日 HH:mm:ss"));ledgerDetailField(content,"流水号",item.optString("id"));ledgerDetailField(content,"关联订单",item.optString("orderId"));ledgerDetailField(content,"入账原因","credit".equals(item.optString("kind"))?item.optString("label"):"");ledgerDetailField(content,"原因编号",item.optString("reasonId"));ledgerDetailField(content,"交易后余额（元）",money(item.optLong("balanceCents")));
        ScrollView scroll=new ScrollView(this);scroll.addView(content);outer.addView(scroll,new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,0,1));addFull(outer,action("完成",true,true,new Runnable(){public void run(){detail.dismiss();}}),46);detail.setContentView(outer);
        Window window=detail.getWindow();if(window!=null){window.setBackgroundDrawableResource(android.R.color.white);window.setDimAmount(.30f);}detail.show();window=detail.getWindow();if(window!=null){window.setLayout(Math.min(dp(430),getResources().getDisplayMetrics().widthPixels-dp(36)),Math.min(dp(560),getResources().getDisplayMetrics().heightPixels-dp(48)));window.setGravity(Gravity.CENTER);}
    }
    private JSONObject currentOrder(){JSONArray orders=array(wallet,"pendingOrders");for(int i=0;i<orders.length();i++){JSONObject order=orders.optJSONObject(i);if(order!=null && order.optString("orderId").equals(selectedOrderId))return order;}return null;}
    private void renderOrders(LinearLayout body){
        LinearLayout list=column();list.setPadding(dp(22),dp(8),dp(22),dp(8));JSONArray orders=array(wallet,"pendingOrders");
        if(orders.length()==0)addFull(list,center("暂无待支付订单",15,GRAY),100);
        for(int i=0;i<orders.length();i++){final JSONObject order=orders.optJSONObject(i);if(order==null)continue;TextView row=text(order.optString("productName","游戏商品")+"\n¥ "+money(order.optLong("amountCents"))+"    ›",16,INK);row.setLineSpacing(dp(7),1);row.setPadding(0,dp(18),0,dp(18));list.addView(row);divider(list);row.setOnClickListener(new View.OnClickListener(){public void onClick(View v){selectedOrderId=order.optString("orderId");navigate(CASHIER);}});}scrolling(body,list);
    }
    private void renderCashier(LinearLayout body){
        body.setPadding(dp(26),dp(10),dp(26),dp(18));final JSONObject order=currentOrder();
        if(order==null){space(body,1);addFull(body,center("暂无待支付订单",15,GRAY),50);space(body,1);addFull(body,action("返回钱包",true,true,new Runnable(){public void run(){navigate(HOME);}}),48);return;}
        space(body,.5f);TextView product=center(order.optString("productName","游戏商品"),15,GRAY);product.setMaxLines(2);body.addView(product);addFull(body,center("¥ "+money(order.optLong("amountCents")),42,INK),68);space(body,.7f);
        LinearLayout method=new LinearLayout(this);method.setGravity(Gravity.CENTER_VERTICAL);method.addView(text("付款方式",14,GRAY),new LinearLayout.LayoutParams(0,dp(36),1));TextView value=text("余额 ¥ "+(wallet==null?"—":money(wallet.optLong("balanceCents"))),14,INK);value.setGravity(Gravity.CENTER_VERTICAL);method.addView(value);addFull(body,method,36);divider(body);
        final boolean enough=wallet!=null && wallet.optLong("balanceCents")>=order.optLong("amountCents");
        if(!enough){TextView hint=text("余额不足，请先增加余额",12,Color.rgb(210,90,30));hint.setPadding(0,dp(8),0,dp(8));body.addView(hint);}
        LinearLayout.LayoutParams buttonSpace=new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,dp(48));buttonSpace.topMargin=dp(14);
        body.addView(action(busy?"处理中…":enough?"确认付款":"增加余额",true,active(),new Runnable(){public void run(){if(enough)pay(order);else{addReturn=CASHIER;navigate(ADD);}}}),buttonSpace);
    }

    private void renderRequestState(){render();}
    private void rememberLedgerScroll(){if(page==BILLS && ledgerScroll!=null)ledgerScrollY=ledgerScroll.getScrollY();}

    private void sessionChanged(){wallet=null;ledger=null;ledgerEntries=new JSONArray();ledgerPage=1;ledgerScrollY=0;busy=false;refreshQueued=false;page=HOME;message="";loadPendingRequests();if(panel!=null && panel.isShowing()){render();refreshPage();}}
    private SharedPreferences prefs(){return getSharedPreferences("local-wallet-pending",MODE_PRIVATE);}
    private void loadPendingRequests(){
        creditRequestId=null;creditAmount=0;amountDraft="100";
        // Retired operational requests never resume after this wallet update.
        SharedPreferences.Editor retired=prefs().edit();
        for(String key:prefs().getAll().keySet())if(key.startsWith("tool."))retired.remove(key);
        retired.commit();
        if(!credentials.valid())return;String key="add."+credentials.player;String id=prefs().getString(key+".id",null);long cents=prefs().getLong(key+".cents",0);
        if(id!=null && id.matches("[a-fA-F0-9-]{36}") && cents>0 && cents<=100000000000000L){creditRequestId=id;creditAmount=cents;amountDraft=money(cents);}
    }
    private void persistCreditRequest()throws Exception{String key="add."+credentials.player;if(!prefs().edit().putString(key+".id",creditRequestId).putLong(key+".cents",creditAmount).commit())throw new Exception("保存请求失败，请重试");}
    private void clearCreditRequest(long player){String key="add."+player;prefs().edit().remove(key+".id").remove(key+".cents").commit();creditRequestId=null;creditAmount=0;amountDraft="100";}
    private void refreshPage(){
        if(busy){refreshQueued=true;return;}
        if(!active())return;
        if(page==BILLS)requestLedger(1);else request("/local-wallet/state",new JSONObject(),false);
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
    private void addMoney(){
        if(!active())return;
        try{
            if(creditRequestId==null){BigDecimal value=new BigDecimal(amountDraft.trim());long cents=value.movePointRight(2).longValueExact();if(cents<=0 || cents>100000000000000L)throw new Exception();creditAmount=cents;creditRequestId=UUID.randomUUID().toString();}
            persistCreditRequest();JSONObject payload=new JSONObject();payload.put("requestId",creditRequestId);payload.put("amountCents",creditAmount);hideKeyboard();request("/local-wallet/add",payload,true);
        }catch(Exception invalid){setMessage("请输入 0.01 至 1,000,000,000,000 元，最多两位小数",true);}
    }
    private void pay(JSONObject order){
        if(!active())return;
        try{JSONObject payload=new JSONObject();payload.put("orderId",order.getString("orderId"));request("/local-wallet/pay",payload,true);}catch(Exception invalid){setMessage("订单信息无效，请返回后重试",true);}
    }

    // Only authenticated server results update money, summaries and receipts.
    // Sessions remain memory-only and old-generation replies drop.
    private void request(final String route,final JSONObject payload,final boolean mutation){
        if(busy || destroyed || !ready())return;final Credentials auth=credentials;final String address=origin+route;busy=true;if(mutation)setMessage("处理中…",false);renderRequestState();
        network.execute(new Runnable(){public void run(){JSONObject data=null;String error=null;HttpURLConnection connection=null;
            try{
                payload.put("playerId",auth.player);payload.put("sessionHex",auth.session);
                connection=(HttpURLConnection)new URL(address).openConnection();connection.setRequestMethod("POST");connection.setConnectTimeout(8000);connection.setReadTimeout(15000);connection.setInstanceFollowRedirects(false);connection.setDoOutput(true);connection.setRequestProperty("Content-Type","application/json; charset=utf-8");connection.setRequestProperty("Cache-Control","no-store");
                byte[] body=payload.toString().getBytes("UTF-8");connection.setFixedLengthStreamingMode(body.length);OutputStream out=connection.getOutputStream();out.write(body);out.close();
                if(connection.getResponseCode()!=200)throw new Exception("连接失败，请重试");InputStream in=connection.getInputStream();byte[] raw=readBounded(in,1048576);in.close();JSONObject reply=new JSONObject(new String(raw,"UTF-8"));
                if(reply.optInt("code",-1)!=0)throw new Exception(reply.optString("msg","操作未完成，请重试"));data=reply.getJSONObject("data");
                if(!data.optBoolean("testOnly",false))throw new Exception("返回数据无效，请重试");
                if(!"CNY".equals(data.optString("currency")) || !data.has("balanceCents") || data.getLong("balanceCents")<0)throw new Exception("余额数据无效，请重试");
                if("/local-wallet/ledger".equals(route) && (data.optJSONObject("summary")==null || data.optJSONArray("entries")==null || data.optInt("page",0)<1))throw new Exception("账单数据无效，请重试");
            }catch(Exception failure){error=failure.getMessage();if(error==null || error.length()>120 || error.contains(auth.session))error="连接中断，请重试";}
            finally{if(connection!=null)connection.disconnect();}
            final JSONObject result=data;final String failure=error;
            runOnUiThread(new Runnable(){public void run(){
                if(destroyed || credentials.generation!=auth.generation)return;busy=false;
                if(failure==null){
                    wallet=result;
                    if("/local-wallet/ledger".equals(route)){
                        if(result.optInt("page",1)==1)ledgerEntries=new JSONArray();
                        JSONArray incoming=array(result,"entries");for(int i=0;i<incoming.length();i++)ledgerEntries.put(incoming.optJSONObject(i));
                        ledger=result;ledgerPage=result.optInt("page",1);
                    }
                    if("/local-wallet/add".equals(route)){
                        String label=result.optString("creditLabel","");
                        if(label.isEmpty()){JSONArray rows=array(result,"entries");for(int i=0;i<rows.length();i++){JSONObject row=rows.optJSONObject(i);if(row!=null && ("add:"+creditRequestId).equals(row.optString("id"))){label=row.optString("label","");break;}}}
                        clearCreditRequest(auth.player);ledger=null;ledgerEntries=new JSONArray();page=addReturn==CASHIER && currentOrder()!=null?CASHIER:HOME;setMessage(label.isEmpty()?"余额已增加":label,false);
                    }else if("/local-wallet/pay".equals(route)){ledger=null;ledgerEntries=new JSONArray();UnityPlayer.UnitySendMessage("LocalWalletSession","RefreshAfterPayment","");page=HOME;setMessage("支付成功，去享受这一刻",false);}
                }else setMessage(failure+(mutation?"；可重试原操作":""),true);
                renderRequestState();
                if(refreshQueued){refreshQueued=false;refreshPage();}
            }});
        }});
    }
    private String readAsset(String name)throws Exception{InputStream in=getAssets().open(name);try{return new String(readBounded(in,16384),"UTF-8");}finally{in.close();}}
    private static byte[] readBounded(InputStream in,int limit)throws Exception{ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] buffer=new byte[4096];int n;while((n=in.read(buffer))!=-1){if(out.size()+n>limit)throw new Exception("响应过长");out.write(buffer,0,n);}return out.toByteArray();}
}
