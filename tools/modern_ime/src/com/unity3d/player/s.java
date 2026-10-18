package com.unity3d.player;

import android.app.Activity;
import android.app.Dialog;
import android.content.Context;
import android.content.ContextWrapper;
import android.graphics.Color;
import android.graphics.Rect;
import android.os.Build;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.text.method.PasswordTransformationMethod;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewTreeObserver;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowManager;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputConnectionWrapper;
import android.view.inputmethod.InputMethodManager;
import android.widget.FrameLayout;
import android.widget.TextView;

import java.lang.ref.WeakReference;

/** Exact Unity5 keyboard ABI adapter. No dialog window, custom keyboard or persistence is used. */
public final class s extends Dialog implements TextWatcher, View.OnClickListener {
    private static WeakReference<s> active = new WeakReference<s>(null);
    private final UnityPlayer player;
    private final Activity activity;
    private final NativeEditor editor;
    private final boolean multiline;
    private final boolean secure;
    private ViewGroup root;
    private final InputState input = new InputState();
    private int savedSoftInputMode;
    private int installedSoftInputMode;
    private boolean seenKeyboard;
    private ViewTreeObserver.OnGlobalLayoutListener layoutListener;

    public s(Context context, UnityPlayer unity, String initial, int keyboardType,
             boolean autocorrect, boolean multi, boolean password, String placeholder) {
        super(context);
        player = unity;
        activity = findActivity(context);
        multiline = multi;
        secure = password;
        editor = new NativeEditor(context);
        editor.setSingleLine(!multi);
        editor.setInputType(originalInputType(keyboardType, autocorrect, multi, password));
        if (password) editor.setTransformationMethod(PasswordTransformationMethod.getInstance());
        editor.setImeOptions(imeOptions());
        editor.setHint(placeholder);
        // The game keeps rendering its own input field. This invisible native host supplies only InputConnection.
        editor.setBackgroundColor(Color.TRANSPARENT);
        editor.setTextColor(Color.TRANSPARENT);
        editor.setHintTextColor(Color.TRANSPARENT);
        editor.setCursorVisible(false);
        editor.setPadding(0, 0, 0, 0);
        editor.setTextSize(1);
        editor.setMinHeight(0);
        editor.setMinimumHeight(0);
        editor.setMinWidth(0);
        editor.setMinimumWidth(0);
        editor.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        editor.setFocusableInTouchMode(true);
        input.lastReported = initial == null ? "" : initial;
        editor.setText(input.lastReported);
        if (!multi) editor.selectAll();
        editor.addTextChangedListener(this);
        editor.setOnEditorActionListener(new TextView.OnEditorActionListener() {
            @Override public boolean onEditorAction(TextView view, int action, KeyEvent event) {
                if (action == EditorInfo.IME_ACTION_DONE ||
                    (!multiline && action == EditorInfo.IME_NULL && event != null &&
                     event.getKeyCode() == KeyEvent.KEYCODE_ENTER && event.getAction() == KeyEvent.ACTION_UP)) {
                    finish(false);
                    return true;
                }
                return false;
            }
        });
    }

    private static Activity findActivity(Context context) {
        Context cursor = context;
        while (cursor instanceof ContextWrapper) {
            if (cursor instanceof Activity) return (Activity) cursor;
            Context next = ((ContextWrapper) cursor).getBaseContext();
            if (next == cursor) break;
            cursor = next;
        }
        if (UnityPlayer.currentActivity != null) return UnityPlayer.currentActivity;
        throw new IllegalStateException("Unity input requires its existing Activity");
    }

    // Exact original s.a(I,Z,Z,Z) flags, including numeric decimal/signed and URL/email variants.
    static int originalInputType(int keyboard, boolean autocorrect, boolean multi, boolean password) {
        int flags = (autocorrect ? 32768 : 0) | (multi ? 131072 : 0) | (password ? 128 : 0);
        int[] original = {1, 16385, 12290, 17, 2, 3, 97, 33};
        return keyboard >= 0 && keyboard < original.length ? original[keyboard] | flags : flags;
    }

    private int imeOptions() {
        int options = EditorInfo.IME_ACTION_DONE | EditorInfo.IME_FLAG_NO_EXTRACT_UI | EditorInfo.IME_FLAG_NO_FULLSCREEN;
        if (secure && Build.VERSION.SDK_INT >= 26) options |= EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING;
        return options;
    }

    @Override public void show() {
        if (input.showing || activity.isFinishing()) return;
        s previous = active.get();
        if (previous != null && previous != this) previous.dismiss();
        active = new WeakReference<s>(this);
        root = (ViewGroup) activity.findViewById(android.R.id.content);
        if (root == null) throw new IllegalStateException("Unity content view missing");
        input.open();
        seenKeyboard = false;
        Window window = activity.getWindow();
        savedSoftInputMode = window.getAttributes().softInputMode;
        installedSoftInputMode = WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE |
                                 WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_VISIBLE;
        window.setSoftInputMode(installedSoftInputMode);
        FrameLayout.LayoutParams params = new FrameLayout.LayoutParams(1, 1, Gravity.BOTTOM | Gravity.LEFT);
        root.addView(editor, params);
        layoutListener = new ViewTreeObserver.OnGlobalLayoutListener() {
            @Override public void onGlobalLayout() { keyboardLayoutChanged(); }
        };
        root.getViewTreeObserver().addOnGlobalLayoutListener(layoutListener);
        editor.requestFocus();
        editor.post(new Runnable() {
            @Override public void run() {
                if (input.showing && !input.finishing && active.get() == s.this) {
                    editor.requestFocus();
                    InputMethodManager ime = (InputMethodManager) activity.getSystemService(Context.INPUT_METHOD_SERVICE);
                    if (ime != null) ime.showSoftInput(editor, InputMethodManager.SHOW_IMPLICIT);
                }
            }
        });
    }

    private void keyboardLayoutChanged() {
        if (!input.showing || input.finishing || root == null) return;
        boolean visible;
        if (Build.VERSION.SDK_INT >= 30 && root.getRootWindowInsets() != null) {
            WindowInsets insets = root.getRootWindowInsets();
            visible = insets.isVisible(WindowInsets.Type.ime());
        } else {
            Rect visibleFrame = new Rect();
            root.getWindowVisibleDisplayFrame(visibleFrame);
            int displayHeight = root.getResources().getDisplayMetrics().heightPixels;
            visible = displayHeight - visibleFrame.bottom > 120 * root.getResources().getDisplayMetrics().density;
        }
        if (visible) seenKeyboard = true;
        else if (seenKeyboard) finish(true);
    }

    @Override public boolean isShowing() { return input.showing; }

    @Override public void dismiss() {
        if (!input.showing) return;
        input.close();
        if (root != null && layoutListener != null && root.getViewTreeObserver().isAlive())
            root.getViewTreeObserver().removeOnGlobalLayoutListener(layoutListener);
        layoutListener = null;
        InputMethodManager ime = (InputMethodManager) activity.getSystemService(Context.INPUT_METHOD_SERVICE);
        if (ime != null) ime.hideSoftInputFromWindow(editor.getWindowToken(), 0);
        if (editor.getParent() instanceof ViewGroup) ((ViewGroup) editor.getParent()).removeView(editor);
        Window window = activity.getWindow();
        if (window.getAttributes().softInputMode == installedSoftInputMode) window.setSoftInputMode(savedSoftInputMode);
        // Password/plaintext stays only in the live platform editor and original Unity callback; no logs or storage.
        editor.setText("");
        input.lastReported = null;
        if (active.get() == this) {
            active.clear();
            player.requestFocus();
        }
        root = null;
    }

    @Override public void cancel() { finish(true); }
    @Override public void onBackPressed() { finish(true); }
    @Override public void onClick(View view) { finish(false); }
    protected final View createSoftInputView() { return editor; }

    // ABI called by UnityPlayer$9; an equal native echo must not reset IME composing spans or selection.
    public final void a(String value) {
        if (!input.showing || input.finishing || value == null || input.ignoresEcho(value, editor.getText().toString(), hasComposingText())) return;
        input.depth++;
        try { editor.setText(value); editor.setSelection(value.length()); }
        finally { input.depth--; }
        publishCommitted();
    }

    private boolean hasComposingText() {
        return BaseInputConnection.getComposingSpanStart(editor.getText()) >= 0 ||
               BaseInputConnection.getComposingSpanEnd(editor.getText()) >= 0;
    }

    private void publishCommitted() {
        if (active.get() != this) return;
        String value = editor.getText().toString();
        if (input.changed(value, hasComposingText())) player.reportSoftInputStr(value, 0, false);
    }

    private void finish(boolean canceled) {
        if (active.get() != this || !input.finishOnce()) return;
        BaseInputConnection.removeComposingSpans(editor.getText());
        // Original s trims only Done/Back; live text updates stay untrimmed.
        String result = editor.getText().toString().trim();
        player.reportSoftInputStr(result, 1, canceled);
    }

    @Override public final void afterTextChanged(Editable editable) { publishCommitted(); }
    @Override public final void beforeTextChanged(CharSequence value, int start, int count, int after) { }
    @Override public final void onTextChanged(CharSequence value, int start, int before, int count) { }

    // Platform-independent state used directly by the native adapter and its exact JVM lifecycle tests.
    static final class InputState {
        String lastReported;
        int depth;
        boolean showing;
        boolean finishing;
        void open() { showing = true; finishing = false; }
        void close() { showing = false; finishing = true; lastReported = null; }
        boolean changed(String value, boolean composing) {
            if (!showing || finishing || depth != 0 || composing || value.equals(lastReported)) return false;
            lastReported = value;
            return true;
        }
        boolean ignoresEcho(String value, String current, boolean composing) {
            return value.equals(current) || composing && value.equals(lastReported);
        }
        boolean finishOnce() {
            if (!showing || finishing) return false;
            finishing = true;
            return true;
        }
    }

    private final class NativeEditor extends android.widget.EditText {
        NativeEditor(Context context) { super(context); }

        @Override public boolean onKeyPreIme(int key, KeyEvent event) {
            if (key == KeyEvent.KEYCODE_BACK) {
                if (event.getAction() == KeyEvent.ACTION_UP) finish(true);
                return true;
            }
            return super.onKeyPreIme(key, event);
        }

        @Override public InputConnection onCreateInputConnection(EditorInfo out) {
            InputConnection connection = super.onCreateInputConnection(out);
            out.imeOptions = imeOptions();
            if (connection == null) return null;
            return new InputConnectionWrapper(connection, false) {
                @Override public boolean setComposingText(CharSequence text, int cursor) {
                    input.depth++;
                    try { return super.setComposingText(text, cursor); }
                    finally { input.depth--; }
                }
                @Override public boolean setComposingRegion(int start, int end) {
                    input.depth++;
                    try { return super.setComposingRegion(start, end); }
                    finally { input.depth--; }
                }
                @Override public boolean commitText(CharSequence text, int cursor) {
                    input.depth++;
                    try { return super.commitText(text, cursor); }
                    finally { input.depth--; publishCommitted(); }
                }
                @Override public boolean finishComposingText() {
                    input.depth++;
                    try { return super.finishComposingText(); }
                    finally { input.depth--; publishCommitted(); }
                }
                @Override public boolean deleteSurroundingText(int before, int after) {
                    input.depth++;
                    try { return super.deleteSurroundingText(before, after); }
                    finally { input.depth--; publishCommitted(); }
                }
                @Override public boolean performEditorAction(int action) {
                    if (action == EditorInfo.IME_ACTION_DONE) { finish(false); return true; }
                    return super.performEditorAction(action);
                }
            };
        }
    }
}
