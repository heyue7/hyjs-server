package com.hyjs.wallet;

import android.app.Activity;
import android.app.Dialog;
import android.graphics.Color;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.view.Gravity;
import android.view.View;
import android.view.Window;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import java.io.InputStream;

/** Native-resolution text and IME on a generated, game-style panel. */
public final class LoginStyle {
    public static final int INK=Color.rgb(35,65,81), BLUE=Color.rgb(16,130,177);
    private LoginStyle() { }
    public static int dp(Activity a,int value){return Math.round(value*a.getResources().getDisplayMetrics().density);}
    public static GradientDrawable surface(int color,int edge,int radius){
        GradientDrawable d=new GradientDrawable();d.setColor(color);d.setCornerRadius(radius);d.setStroke(1,edge);return d;
    }
    public static LinearLayout body(Activity a,String title){
        LinearLayout b=new LinearLayout(a);b.setOrientation(LinearLayout.VERTICAL);
        b.setPadding(dp(a,22),dp(a,10),dp(a,22),dp(a,12));
        TextView heading=new TextView(a);heading.setText(title);heading.setTextSize(20);heading.setTextColor(INK);
        heading.setGravity(Gravity.CENTER);heading.setTypeface(null,android.graphics.Typeface.BOLD);
        b.addView(heading,new LinearLayout.LayoutParams(-1,dp(a,38)));
        return b;
    }
    public static FrameLayout panel(Activity a,LinearLayout body){
        FrameLayout frame=new FrameLayout(a);
        frame.setBackground(surface(Color.rgb(240,248,250),Color.rgb(75,173,199),dp(a,10)));
        ImageView art=new ImageView(a);art.setScaleType(ImageView.ScaleType.FIT_XY);
        try(InputStream in=a.getAssets().open("login-ui/panel.png")){art.setImageDrawable(Drawable.createFromStream(in,null));}
        catch(Exception unavailable){/* Retain a readable native fallback if an asset cannot load. */}
        art.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        frame.addView(art,new FrameLayout.LayoutParams(-1,-1));
        ScrollView scroll=new ScrollView(a);scroll.setFillViewport(false);scroll.setClipToPadding(false);
        scroll.addView(body,new ScrollView.LayoutParams(-1,-2));
        frame.addView(scroll,new FrameLayout.LayoutParams(-1,-2));return frame;
    }
    public static EditText field(Activity a,LinearLayout body,String hint,boolean password){
        EditText f=new EditText(a);
        f.setSingleLine(true);f.setTextSize(16);f.setTextColor(INK);
        f.setHintTextColor(Color.rgb(94,116,127));f.setHint(hint);
        f.setInputType(android.text.InputType.TYPE_CLASS_TEXT|(password?android.text.InputType.TYPE_TEXT_VARIATION_PASSWORD:android.text.InputType.TYPE_TEXT_VARIATION_NORMAL));
        f.setImeOptions(EditorInfo.IME_ACTION_NEXT|EditorInfo.IME_FLAG_NO_EXTRACT_UI);
        f.setPadding(dp(a,13),0,dp(a,13),0);
        f.setBackground(surface(Color.argb(245,255,255,255),Color.rgb(142,185,201),dp(a,6)));
        LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,dp(a,44));lp.topMargin=dp(a,6);body.addView(f,lp);return f;
    }
    public static Button button(Activity a,String label,boolean primary){
        Button b=new Button(a);b.setText(label);b.setTextSize(16);b.setAllCaps(false);b.setMinimumHeight(0);b.setMinHeight(0);
        b.setTextColor(primary?Color.WHITE:BLUE);b.setPadding(dp(a,10),0,dp(a,10),0);
        b.setBackground(surface(primary?BLUE:Color.rgb(233,246,251),Color.rgb(92,183,211),dp(a,22)));return b;
    }
    public static void fit(Activity a,Dialog dialog){
        Window w=dialog.getWindow();if(w==null)return;
        w.setBackgroundDrawableResource(android.R.color.transparent);
        w.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE|WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);
        android.graphics.Rect r=new android.graphics.Rect();a.getWindow().getDecorView().getWindowVisibleDisplayFrame(r);
        int width=r.width()>0?r.width():a.getResources().getDisplayMetrics().widthPixels;
        int height=r.height()>0?r.height():a.getResources().getDisplayMetrics().heightPixels;
        w.setLayout(Math.min(dp(a,540),Math.round(width*.90f)),Math.min(dp(a,360),Math.round(height*.94f)));
        w.setGravity(Gravity.CENTER);
    }
}
