package com.hyjs.compat;

/** No Android/Unity dependency. One in-flight preparation and one successful handoff. */
public final class StartupGate {
    public interface Action { void run() throws Exception; }
    private boolean busy, ready, launched, cancelled;
    public synchronized boolean begin() {
        if (busy || ready || launched || cancelled) return false;
        busy = true; return true;
    }
    public void prepare(Action synchronize) throws Exception {
        try { synchronize.run(); synchronized (this) { if (!cancelled) ready = true; } }
        finally { synchronized (this) { busy = false; } }
    }
    public synchronized boolean launch(Action destination) throws Exception {
        if (!ready || launched || cancelled) return false;
        ready = false;
        destination.run();
        launched = true;
        return true;
    }
    public synchronized void cancel() { cancelled = true; ready = false; }
    public synchronized boolean launched() { return launched; }
}
