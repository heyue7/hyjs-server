package com.hyjs.compat;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.os.Bundle;
import android.util.Log;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import com.hyjs.wallet.LocalAssetOverlay;

/** Plain Android bootstrap: no Unity superclass, class literal, native load, or game mutation. */
public final class StartupActivity extends Activity {
    private final StartupGate gate = new StartupGate();
    private volatile boolean destroyed;
    private boolean resumed, pending;
    private TextView title, message;
    private Button retry, exit;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        Intent intent = getIntent();
        if ((!isTaskRoot() && Intent.ACTION_MAIN.equals(intent.getAction())
                && intent.hasCategory(Intent.CATEGORY_LAUNCHER))
                || (state != null && state.getBoolean("hyjs.compat.handoff", false))) {
            finish(); return;
        }
        buildView();
        prepare();
    }
    private int dp(int n) { return Math.round(n * getResources().getDisplayMetrics().density); }
    private TextView text(int size, int color) {
        TextView v = new TextView(this); v.setTextSize(size); v.setTextColor(color);
        v.setGravity(Gravity.CENTER); return v;
    }
    private void buildView() {
        getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_FULLSCREEN);
        ScrollView scroll = new ScrollView(this); scroll.setFillViewport(true);
        scroll.setBackgroundColor(Color.rgb(20, 26, 36));
        LinearLayout content = new LinearLayout(this); content.setOrientation(LinearLayout.VERTICAL);
        content.setGravity(Gravity.CENTER); content.setPadding(dp(24), dp(20), dp(24), dp(20));
        title = text(22, Color.WHITE); title.setText("正在准备游戏资源");
        message = text(16, Color.rgb(220, 226, 232)); message.setPadding(0, dp(16), 0, dp(20));
        message.setText("请稍候");
        content.addView(title, new LinearLayout.LayoutParams(-1, -2));
        content.addView(message, new LinearLayout.LayoutParams(-1, -2));
        LinearLayout actions = new LinearLayout(this); actions.setGravity(Gravity.CENTER);
        retry = new Button(this); retry.setText("重试"); retry.setMinHeight(dp(48));
        retry.setOnClickListener(new View.OnClickListener() { public void onClick(View v) { prepare(); } });
        exit = new Button(this); exit.setText("退出"); exit.setMinHeight(dp(48));
        exit.setOnClickListener(new View.OnClickListener() { public void onClick(View v) { gate.cancel(); finish(); } });
        actions.addView(retry, new LinearLayout.LayoutParams(dp(136), -2));
        LinearLayout.LayoutParams second = new LinearLayout.LayoutParams(dp(136), -2); second.leftMargin = dp(16);
        actions.addView(exit, second); content.addView(actions);
        scroll.addView(content, new ScrollView.LayoutParams(-1, -1)); setContentView(scroll);
    }
    private void prepare() {
        if (destroyed || !gate.begin()) return;
        title.setText("正在准备游戏资源"); message.setText("请稍候");
        retry.setVisibility(View.GONE); exit.setVisibility(View.GONE);
        new Thread(new Runnable() { public void run() {
            try {
                gate.prepare(new StartupGate.Action() { public void run() throws Exception {
                    LocalAssetOverlay.synchronize(getApplicationContext());
                } });
                runOnUiThread(new Runnable() { public void run() {
                    if (!destroyed) { pending = true; handoff(); }
                } });
            } catch (Exception failure) { diagnose("prepare", failure); failLater(); }
              catch (LinkageError failure) { diagnose("prepare", failure); failLater(); }
        } }, "hyjs-resource-preflight").start();
    }
    private static void diagnose(String phase, Throwable failure) {
        Log.e("HYJSStartup", FailureDiagnostic.format(phase, failure));
    }
    private void failLater() { runOnUiThread(new Runnable() { public void run() { showFailure(); } }); }
    private void showFailure() {
        if (destroyed) return;
        pending = false;
        title.setText("游戏资源准备失败");
        message.setText("本地游戏资源校验或存储读写未完成。请确认存储空间充足后重试；若仍失败，请重新安装完整安装包。\n重新安装前请确认已保存账号信息。");
        retry.setVisibility(View.VISIBLE); exit.setVisibility(View.VISIBLE);
    }
    private void handoff() {
        if (!resumed || !pending || destroyed) return;
        pending = false;
        try {
            if (gate.launch(new StartupGate.Action() { public void run() {
                Intent next = new Intent();
                next.setClassName(getPackageName(), "com.hyjs.wallet.ClientUpdateActivity");
                startActivity(next);
            } })) finish();
        } catch (Exception failure) { diagnose("handoff", failure); showFailure(); }
          catch (LinkageError failure) { diagnose("handoff", failure); showFailure(); }
    }
    @Override protected void onResume() { super.onResume(); resumed = true; handoff(); }
    @Override protected void onPause() { resumed = false; super.onPause(); }
    @Override protected void onSaveInstanceState(Bundle state) {
        state.putBoolean("hyjs.compat.handoff", gate.launched()); super.onSaveInstanceState(state);
    }
    @Override protected void onDestroy() { destroyed = true; gate.cancel(); super.onDestroy(); }
}
