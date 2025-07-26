package org.golang.app;

import android.content.Intent;
import android.os.Bundle;

public class GoNativeActivity extends org.golang.app.GoNativeActivityBase {

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
        handleIntent(Intent);
        super.onNewIntent(intent);
    }

    private void handleIntent(Intent intent) {
        if (intent == null) {
            return;
        }
        String action = intent.getAction();
        if (Intent.ACTION_VIEW.equals(action) || 
            Intent.ACTION_SEND.equals(action) ||
            Intent.ACTION_SEND_MULTIPLE.equals(action)) {
            processIntent(intent);
        }
    }
}