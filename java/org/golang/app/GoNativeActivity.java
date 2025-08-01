package org.golang.app;

import android.content.Intent;
import android.os.Bundle;

public class GoNativeActivity extends org.golang.app.GoNativeActivityBase {
    private long lastIntentTime = 0;
    private static final long INTENT_THRESHOLD_MS = 300; // 300ms threshold

    static {
        System.loadLibrary("croc");
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        handleIntent(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        handleIntent(intent);
    }

    private void handleIntent(Intent intent) {
        if (intent == null) {
            return;
        }
        
        long currentTime = System.currentTimeMillis();
        if (currentTime - lastIntentTime < INTENT_THRESHOLD_MS) {
            return;
        }
        lastIntentTime = currentTime;

        String action = intent.getAction();
        if (Intent.ACTION_VIEW.equals(action) || 
            Intent.ACTION_SEND.equals(action) ||
            Intent.ACTION_SEND_MULTIPLE.equals(action)) {
            processIntent(intent);
        }
    }
}