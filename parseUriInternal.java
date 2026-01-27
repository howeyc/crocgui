//https://cs.android.com/android/platform/superproject/+/master:frameworks/base/core/java/android/content/Intent.java
private static Intent parseUriInternal(String uri, @UriFlags int flags)
            throws URISyntaxException {
        int i = 0;
        try {
            final boolean androidApp = uri.startsWith("android-app:");

            // Validate intent scheme if requested.
            if ((flags&(URI_INTENT_SCHEME|URI_ANDROID_APP_SCHEME)) != 0) {
                if (!uri.startsWith("intent:") && !androidApp) {
                    Intent intent = new Intent(ACTION_VIEW);
                    try {
                        intent.setData(Uri.parse(uri));
                    } catch (IllegalArgumentException e) {
                        throw new URISyntaxException(uri, e.getMessage());
                    }
                    return intent;
                }
            }

            i = uri.lastIndexOf("#");
            // simple case
            if (i == -1) {
                if (!androidApp) {
                    return new Intent(ACTION_VIEW, Uri.parse(uri));
                }

            // old format Intent URI
            } else if (!uri.startsWith("#Intent;", i)) {
                if (!androidApp) {
                    return getIntentOld(uri, flags);
                } else {
                    i = -1;
                }
            }

            // new format
            Intent intent = new Intent(ACTION_VIEW);
            Intent baseIntent = intent;
            boolean explicitAction = false;
            boolean inSelector = false;

            // fetch data part, if present
            String scheme = null;
            String data;
            if (i >= 0) {
                data = uri.substring(0, i);
                i += 8; // length of "#Intent;"
            } else {
                data = uri;
            }

            // loop over contents of Intent, all name=value;
            while (i >= 0 && !uri.startsWith("end", i)) {
                int eq = uri.indexOf('=', i);
                if (eq < 0) eq = i-1;
                int semi = uri.indexOf(';', i);
                String value = eq < semi ? Uri.decode(uri.substring(eq + 1, semi)) : "";

                // action
                if (uri.startsWith("action=", i)) {
                    intent.setAction(value);
                    if (!inSelector) {
                        explicitAction = true;
                    }
                }

                // categories
                else if (uri.startsWith("category=", i)) {
                    intent.addCategory(value);
                }

                // type
                else if (uri.startsWith("type=", i)) {
                    intent.mType = value;
                }

                // identifier
                else if (uri.startsWith("identifier=", i)) {
                    intent.mIdentifier = value;
                }

                // launch flags
                else if (uri.startsWith("launchFlags=", i)) {
                    intent.mFlags = Integer.decode(value).intValue();
                    if ((flags& URI_ALLOW_UNSAFE) == 0) {
                        intent.mFlags &= ~IMMUTABLE_FLAGS;
                    }
                }

                // package
                else if (uri.startsWith("package=", i)) {
                    intent.mPackage = value;
                }

                // component
                else if (uri.startsWith("component=", i)) {
                    intent.mComponent = ComponentName.unflattenFromString(value);
                }

                // scheme
                else if (uri.startsWith("scheme=", i)) {
                    if (inSelector) {
                        intent.mData = Uri.parse(value + ":");
                    } else {
                        scheme = value;
                    }
                }

                // source bounds
                else if (uri.startsWith("sourceBounds=", i)) {
                    intent.mSourceBounds = Rect.unflattenFromString(value);
                }

                // selector
                else if (semi == (i+3) && uri.startsWith("SEL", i)) {
                    intent = new Intent();
                    inSelector = true;
                }

                // extra
                else {
                    String key = Uri.decode(uri.substring(i + 2, eq));
                    // create Bundle if it doesn't already exist
                    if (intent.mExtras == null) intent.mExtras = new Bundle();
                    Bundle b = intent.mExtras;
                    // add EXTRA
                    if      (uri.startsWith("S.", i)) b.putString(key, value);
                    else if (uri.startsWith("B.", i)) b.putBoolean(key, Boolean.parseBoolean(value));
                    else if (uri.startsWith("b.", i)) b.putByte(key, Byte.parseByte(value));
                    else if (uri.startsWith("c.", i)) b.putChar(key, value.charAt(0));
                    else if (uri.startsWith("d.", i)) b.putDouble(key, Double.parseDouble(value));
                    else if (uri.startsWith("f.", i)) b.putFloat(key, Float.parseFloat(value));
                    else if (uri.startsWith("i.", i)) b.putInt(key, Integer.parseInt(value));
                    else if (uri.startsWith("l.", i)) b.putLong(key, Long.parseLong(value));
                    else if (uri.startsWith("s.", i)) b.putShort(key, Short.parseShort(value));
                    else throw new URISyntaxException(uri, "unknown EXTRA type", i);
                }

                // move to the next item
                i = semi + 1;
            }

            if (inSelector) {
                // The Intent had a selector; fix it up.
                if (baseIntent.mPackage == null) {
                    baseIntent.setSelector(intent);
                }
                intent = baseIntent;
            }

            if (data != null) {
                if (data.startsWith("intent:")) {
                    data = data.substring(7);
                    if (scheme != null) {
                        data = scheme + ':' + data;
                    }
                } else if (data.startsWith("android-app:")) {
                    if (data.charAt(12) == '/' && data.charAt(13) == '/') {
                        // Correctly formed android-app, first part is package name.
                        int end = data.indexOf('/', 14);
                        if (end < 0) {
                            // All we have is a package name.
                            intent.mPackage = data.substring(14);
                            if (!explicitAction) {
                                intent.setAction(ACTION_MAIN);
                            }
                            data = "";
                        } else {
                            // Target the Intent at the given package name always.
                            String authority = null;
                            intent.mPackage = data.substring(14, end);
                            int newEnd;
                            if ((end+1) < data.length()) {
                                if ((newEnd=data.indexOf('/', end+1)) >= 0) {
                                    // Found a scheme, remember it.
                                    scheme = data.substring(end+1, newEnd);
                                    end = newEnd;
                                    if (end < data.length() && (newEnd=data.indexOf('/', end+1)) >= 0) {
                                        // Found a authority, remember it.
                                        authority = data.substring(end+1, newEnd);
                                        end = newEnd;
                                    }
                                } else {
                                    // All we have is a scheme.
                                    scheme = data.substring(end+1);
                                }
                            }
                            if (scheme == null) {
                                // If there was no scheme, then this just targets the package.
                                if (!explicitAction) {
                                    intent.setAction(ACTION_MAIN);
                                }
                                data = "";
                            } else if (authority == null) {
                                data = scheme + ":";
                            } else {
                                data = scheme + "://" + authority + data.substring(end);
                            }
                        }
                    } else {
                        data = "";
                    }
                }

                if (data.length() > 0) {
                    try {
                        intent.mData = Uri.parse(data);
                    } catch (IllegalArgumentException e) {
                        throw new URISyntaxException(uri, e.getMessage());
                    }
                }
            }

            return intent;

        } catch (IndexOutOfBoundsException e) {
            throw new URISyntaxException(uri, "illegal Intent URI format", i);
        }
    }