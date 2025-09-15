package org.golang.app;

import android.content.Intent;
import android.os.Bundle;
import android.net.Uri;
import android.content.ContentResolver;

public class GoNativeActivity extends org.golang.app.GoNativeActivityBase {

    static {
        System.loadLibrary("croc");
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        if (savedInstanceState == null) {
            handleIntent(getIntent());
        }
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
        if ((intent.getFlags() & Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY) != 0) {
            return; 
        }

        // takePersistableUriPermissions(intent);

        String action = intent.getAction();
        if (Intent.ACTION_VIEW.equals(action) || 
            Intent.ACTION_SEND.equals(action) ||
            Intent.ACTION_SEND_MULTIPLE.equals(action)) {
            
            processIntent(intent);
            
            if (Intent.ACTION_SEND.equals(action) || 
                Intent.ACTION_SEND_MULTIPLE.equals(action)) {
                setResult(android.app.Activity.RESULT_OK);
            }
        }
    }

    private void takePersistableUriPermissions(Intent intent) {
        try {
            Uri data = intent.getData();
            if (data != null && "content".equals(data.getScheme())) {
                getContentResolver().takePersistableUriPermission(
                    data,
                    Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION
                );
            }

            if (Intent.ACTION_SEND.equals(intent.getAction())) {
                Uri streamUri = intent.getParcelableExtra(Intent.EXTRA_STREAM);
                if (streamUri != null && "content".equals(streamUri.getScheme())) {
                    getContentResolver().takePersistableUriPermission(
                        streamUri,
                        Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION
                    );
                }
            }

            if (Intent.ACTION_SEND_MULTIPLE.equals(intent.getAction())) {
                java.util.ArrayList<Uri> streamUris = intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM);
                if (streamUris != null) {
                    for (Uri uri : streamUris) {
                        if (uri != null && "content".equals(uri.getScheme())) {
                            getContentResolver().takePersistableUriPermission(
                                uri,
                                Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION
                            );
                        }
                    }
                }
            }

            android.content.ClipData clipData = intent.getClipData();
            if (clipData != null) {
                for (int i = 0; i < clipData.getItemCount(); i++) {
                    Uri uri = clipData.getItemAt(i).getUri();
                    if (uri != null && "content".equals(uri.getScheme())) {
                        getContentResolver().takePersistableUriPermission(
                            uri,
                            Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION
                        );
                    }
                }
            }

        } catch (Exception e) {
        }
    }

    private native void processIntent(Intent intent);
}