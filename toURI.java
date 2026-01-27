public String toUri(@UriFlags int flags) {
        StringBuilder uri = new StringBuilder(128);
        if ((flags&URI_ANDROID_APP_SCHEME) != 0) {
            if (mPackage == null) {
                throw new IllegalArgumentException(
                        "Intent must include an explicit package name to build an android-app: "
                        + this);
            }
            uri.append("android-app://");
            uri.append(mPackage);
            String scheme = null;
            if (mData != null) {
                scheme = mData.getScheme();
                if (scheme != null) {
                    uri.append('/');
                    uri.append(scheme);
                    String authority = mData.getEncodedAuthority();
                    if (authority != null) {
                        uri.append('/');
                        uri.append(authority);
                        String path = mData.getEncodedPath();
                        if (path != null) {
                            uri.append(path);
                        }
                        String queryParams = mData.getEncodedQuery();
                        if (queryParams != null) {
                            uri.append('?');
                            uri.append(queryParams);
                        }
                        String fragment = mData.getEncodedFragment();
                        if (fragment != null) {
                            uri.append('#');
                            uri.append(fragment);
                        }
                    }
                }
            }
            toUriFragment(uri, null, scheme == null ? Intent.ACTION_MAIN : Intent.ACTION_VIEW,
                    mPackage, flags);
            return uri.toString();
        }
        String scheme = null;
        if (mData != null) {
            String data = mData.toString();
            if ((flags&URI_INTENT_SCHEME) != 0) {
                final int N = data.length();
                for (int i=0; i<N; i++) {
                    char c = data.charAt(i);
                    if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
                            || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+') {
                        continue;
                    }
                    if (c == ':' && i > 0) {
                        // Valid scheme.
                        scheme = data.substring(0, i);
                        uri.append("intent:");
                        data = data.substring(i+1);
                        break;
                    }

                    // No scheme.
                    break;
                }
            }
            uri.append(data);

        } else if ((flags&URI_INTENT_SCHEME) != 0) {
            uri.append("intent:");
        }

        toUriFragment(uri, scheme, Intent.ACTION_VIEW, null, flags);

        return uri.toString();
    }