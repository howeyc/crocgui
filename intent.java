//https://android.googlesource.com/platform/frameworks/base/+/refs/heads/master/core/java/android/content/Intent.java

/**
     * If set, the recipient of this Intent will be granted permission to
     * perform read operations on the URI in the Intent's data and any URIs
     * specified in its ClipData.  When applying to an Intent's ClipData,
     * all URIs as well as recursive traversals through data or other ClipData
     * in Intent items will be granted; only the grant flags of the top-level
     * Intent are used.
     */
    public static final int FLAG_GRANT_READ_URI_PERMISSION = 0x00000001;
    /**
     * If set, the recipient of this Intent will be granted permission to
     * perform write operations on the URI in the Intent's data and any URIs
     * specified in its ClipData.  When applying to an Intent's ClipData,
     * all URIs as well as recursive traversals through data or other ClipData
     * in Intent items will be granted; only the grant flags of the top-level
     * Intent are used.
     */
    public static final int FLAG_GRANT_WRITE_URI_PERMISSION = 0x00000002;
    /**
     * Can be set by the caller to indicate that this Intent is coming from
     * a background operation, not from direct user interaction.
     */
    public static final int FLAG_FROM_BACKGROUND = 0x00000004;
    /**
     * A flag you can enable for debugging: when set, log messages will be
     * printed during the resolution of this intent to show you what has
     * been found to create the final resolved list.
     */
    public static final int FLAG_DEBUG_LOG_RESOLUTION = 0x00000008;
    /**
     * If set, this intent will not match any components in packages that
     * are currently
     * {@linkplain android.content.pm.ApplicationInfo#FLAG_STOPPED stopped}.
     * If this is not set, then the default behavior is to include such
     * applications in the result.
     */
    public static final int FLAG_EXCLUDE_STOPPED_PACKAGES = 0x00000010;
    /**
     * If set, this intent will always match any components in packages that
     * are currently
     * {@linkplain android.content.pm.ApplicationInfo#FLAG_STOPPED stopped}.
     * This is the default behavior when
     * {@link #FLAG_EXCLUDE_STOPPED_PACKAGES} is not set.  If both of these
     * flags are set, this one wins (it allows overriding of exclude for
     * places where the framework may automatically set the exclude flag,
     * such as broadcasts).
     */
    public static final int FLAG_INCLUDE_STOPPED_PACKAGES = 0x00000020;
    /**
     * When combined with {@link #FLAG_GRANT_READ_URI_PERMISSION} and/or
     * {@link #FLAG_GRANT_WRITE_URI_PERMISSION}, the URI permission grant can be
     * persisted across device reboots until explicitly revoked with
     * {@link Context#revokeUriPermission(Uri, int)}. This flag only offers the
     * grant for possible persisting; the receiving application must call
     * {@link ContentResolver#takePersistableUriPermission(Uri, int)} to
     * actually persist.
     *
     * @see ContentResolver#takePersistableUriPermission(Uri, int)
     * @see ContentResolver#releasePersistableUriPermission(Uri, int)
     * @see ContentResolver#getPersistedUriPermissions()
     * @see ContentResolver#getOutgoingPersistedUriPermissions()
     */
    public static final int FLAG_GRANT_PERSISTABLE_URI_PERMISSION = 0x00000040;
    /**
     * When combined with {@link #FLAG_GRANT_READ_URI_PERMISSION} and/or
     * {@link #FLAG_GRANT_WRITE_URI_PERMISSION}, the URI permission grant
     * applies to any URI that is a prefix match against the original granted
     * URI. (Without this flag, the URI must match exactly for access to be
     * granted.) Another URI is considered a prefix match only when scheme,
     * authority, and all path segments defined by the prefix are an exact
     * match.
     */
    public static final int FLAG_GRANT_PREFIX_URI_PERMISSION = 0x00000080;
    /**
     * Flag used to automatically match intents based on their Direct Boot
     * awareness and the current user state.
     * <p>
     * Since the default behavior is to automatically apply the current user
     * state, this is effectively a sentinel value that doesn't change the
     * output of any queries based on its presence or absence.
     * <p>
     * Instead, this value can be useful in conjunction with
     * {@link android.os.StrictMode.VmPolicy.Builder#detectImplicitDirectBoot()}
     * to detect when a caller is relying on implicit automatic matching,
     * instead of confirming the explicit behavior they want.
     */
    public static final int FLAG_DIRECT_BOOT_AUTO = 0x00000100;
    /** {@hide} */
    @Deprecated
    public static final int FLAG_DEBUG_TRIAGED_MISSING = FLAG_DIRECT_BOOT_AUTO;
    /**
     * Internal flag used to indicate ephemeral applications should not be
     * considered when resolving the intent.
     *
     * @hide
     */
    public static final int FLAG_IGNORE_EPHEMERAL = 0x80000000;
    /**
     * If set, the new activity is not kept in the history stack.  As soon as
     * the user navigates away from it, the activity is finished.  This may also
     * be set with the {@link android.R.styleable#AndroidManifestActivity_noHistory
     * noHistory} attribute.
     *
     * <p>If set, {@link android.app.Activity#onActivityResult onActivityResult()}
     * is never invoked when the current activity starts a new activity which
     * sets a result and finishes.
     */
    public static final int FLAG_ACTIVITY_NO_HISTORY = 0x40000000;
    /**
     * If set, the activity will not be launched if it is already running
     * at the top of the history stack.  See
     * <a href="{@docRoot}guide/topics/fundamentals/tasks-and-back-stack.html#TaskLaunchModes">
     * Tasks and Back Stack</a> for more information.
     */
    public static final int FLAG_ACTIVITY_SINGLE_TOP = 0x20000000;
    /**
     * If set, this activity will become the start of a new task on this
     * history stack.  A task (from the activity that started it to the
     * next task activity) defines an atomic group of activities that the
     * user can move to.  Tasks can be moved to the foreground and background;
     * all of the activities inside of a particular task always remain in
     * the same order.  See
     * <a href="{@docRoot}guide/topics/fundamentals/tasks-and-back-stack.html">Tasks and Back
     * Stack</a> for more information about tasks.
     *
     * <p>This flag is generally used by activities that want
     * to present a "launcher" style behavior: they give the user a list of
     * separate things that can be done, which otherwise run completely
     * independently of the activity launching them.
     *
     * <p>When using this flag, if a task is already running for the activity
     * you are now starting, then a new activity will not be started; instead,
     * the current task will simply be brought to the front of the screen with
     * the state it was last in.  See {@link #FLAG_ACTIVITY_MULTIPLE_TASK} for a flag
     * to disable this behavior.
     *
     * <p>This flag can not be used when the caller is requesting a result from
     * the activity being launched.
     */
    public static final int FLAG_ACTIVITY_NEW_TASK = 0x10000000;
    /**
     * This flag is used to create a new task and launch an activity into it.
     * This flag is always paired with either {@link #FLAG_ACTIVITY_NEW_DOCUMENT}
     * or {@link #FLAG_ACTIVITY_NEW_TASK}. In both cases these flags alone would
     * search through existing tasks for ones matching this Intent. Only if no such
     * task is found would a new task be created. When paired with
     * FLAG_ACTIVITY_MULTIPLE_TASK both of these behaviors are modified to skip
     * the search for a matching task and unconditionally start a new task.
     *
     * <strong>When used with {@link #FLAG_ACTIVITY_NEW_TASK} do not use this
     * flag unless you are implementing your own
     * top-level application launcher.</strong>  Used in conjunction with
     * {@link #FLAG_ACTIVITY_NEW_TASK} to disable the
     * behavior of bringing an existing task to the foreground.  When set,
     * a new task is <em>always</em> started to host the Activity for the
     * Intent, regardless of whether there is already an existing task running
     * the same thing.
     *
     * <p><strong>Because the default system does not include graphical task management,
     * you should not use this flag unless you provide some way for a user to
     * return back to the tasks you have launched.</strong>
     *
     * See {@link #FLAG_ACTIVITY_NEW_DOCUMENT} for details of this flag's use for
     * creating new document tasks.
     *
     * <p>This flag is ignored if one of {@link #FLAG_ACTIVITY_NEW_TASK} or
     * {@link #FLAG_ACTIVITY_NEW_DOCUMENT} is not also set.
     *
     * <p>See
     * <a href="{@docRoot}guide/topics/fundamentals/tasks-and-back-stack.html">Tasks and Back
     * Stack</a> for more information about tasks.
     *
     * @see #FLAG_ACTIVITY_NEW_DOCUMENT
     * @see #FLAG_ACTIVITY_NEW_TASK
     */
    public static final int FLAG_ACTIVITY_MULTIPLE_TASK = 0x08000000;
    /**
     * If set, and the activity being launched is already running in the
     * current task, then instead of launching a new instance of that activity,
     * all of the other activities on top of it will be closed and this Intent
     * will be delivered to the (now on top) old activity as a new Intent.
     *
     * <p>For example, consider a task consisting of the activities: A, B, C, D.
     * If D calls startActivity() with an Intent that resolves to the component
     * of activity B, then C and D will be finished and B receive the given
     * Intent, resulting in the stack now being: A, B.
     *
     * <p>The currently running instance of activity B in the above example will
     * either receive the new intent you are starting here in its
     * onNewIntent() method, or be itself finished and restarted with the
     * new intent.  If it has declared its launch mode to be "multiple" (the
     * default) and you have not set {@link #FLAG_ACTIVITY_SINGLE_TOP} in
     * the same intent, then it will be finished and re-created; for all other
     * launch modes or if {@link #FLAG_ACTIVITY_SINGLE_TOP} is set then this
     * Intent will be delivered to the current instance's onNewIntent().
     *
     * <p>This launch mode can also be used to good effect in conjunction with
     * {@link #FLAG_ACTIVITY_NEW_TASK}: if used to start the root activity
     * of a task, it will bring any currently running instance of that task
     * to the foreground, and then clear it to its root state.  This is
     * especially useful, for example, when launching an activity from the
     * notification manager.
     *
     * <p>See
     * <a href="{@docRoot}guide/topics/fundamentals/tasks-and-back-stack.html">Tasks and Back
     * Stack</a> for more information about tasks.
     */
    public static final int FLAG_ACTIVITY_CLEAR_TOP = 0x04000000;
    /**
     * If set and this intent is being used to launch a new activity from an
     * existing one, then the reply target of the existing activity will be
     * transferred to the new activity.  This way, the new activity can call
     * {@link android.app.Activity#setResult} and have that result sent back to
     * the reply target of the original activity.
     */
    public static final int FLAG_ACTIVITY_FORWARD_RESULT = 0x02000000;
    /**
     * If set and this intent is being used to launch a new activity from an
     * existing one, the current activity will not be counted as the top
     * activity for deciding whether the new intent should be delivered to
     * the top instead of starting a new one.  The previous activity will
     * be used as the top, with the assumption being that the current activity
     * will finish itself immediately.
     */
    public static final int FLAG_ACTIVITY_PREVIOUS_IS_TOP = 0x01000000;
    /**
     * If set, the new activity is not kept in the list of recently launched
     * activities.
     */
    public static final int FLAG_ACTIVITY_EXCLUDE_FROM_RECENTS = 0x00800000;
    /**
     * This flag is not normally set by application code, but set for you by
     * the system as described in the
     * {@link android.R.styleable#AndroidManifestActivity_launchMode
     * launchMode} documentation for the singleTask mode.
     */
    public static final int FLAG_ACTIVITY_BROUGHT_TO_FRONT = 0x00400000;
    /**
     * If set, and this activity is either being started in a new task or
     * bringing to the top an existing task, then it will be launched as
     * the front door of the task.  This will result in the application of
     * any affinities needed to have that task in the proper state (either
     * moving activities to or from it), or simply resetting that task to
     * its initial state if needed.
     *
     * @see android.R.attr#allowTaskReparenting
     * @see android.R.attr#clearTaskOnLaunch
     * @see android.R.attr#finishOnTaskLaunch
     */
    public static final int FLAG_ACTIVITY_RESET_TASK_IF_NEEDED = 0x00200000;
    /**
     * This flag is not normally set by application code, but set for you by
     * the system if this activity is being launched from history.
     */
    public static final int FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY = 0x00100000;
    /**
     * @deprecated As of API 21 this performs identically to
     * {@link #FLAG_ACTIVITY_NEW_DOCUMENT} which should be used instead of this.
     */
    @Deprecated
    public static final int FLAG_ACTIVITY_CLEAR_WHEN_TASK_RESET = 0x00080000;
    /**
     * This flag is used to open a document into a new task rooted at the activity launched
     * by this Intent. Through the use of this flag, or its equivalent attribute,
     * {@link android.R.attr#documentLaunchMode} multiple instances of the same activity
     * containing different documents will appear in the recent tasks list.
     *
     * <p>The use of the activity attribute form of this,
     * {@link android.R.attr#documentLaunchMode}, is
     * preferred over the Intent flag described here. The attribute form allows the
     * Activity to specify multiple document behavior for all launchers of the Activity
     * whereas using this flag requires each Intent that launches the Activity to specify it.
     *
     * <p>Note that the default semantics of this flag w.r.t. whether the recents entry for
     * it is kept after the activity is finished is different than the use of
     * {@link #FLAG_ACTIVITY_NEW_TASK} and {@link android.R.attr#documentLaunchMode} -- if
     * this flag is being used to create a new recents entry, then by default that entry
     * will be removed once the activity is finished.  You can modify this behavior with
     * {@link #FLAG_ACTIVITY_RETAIN_IN_RECENTS}.
     *
     * <p>FLAG_ACTIVITY_NEW_DOCUMENT may be used in conjunction with {@link
     * #FLAG_ACTIVITY_MULTIPLE_TASK}. When used alone it is the
     * equivalent of the Activity manifest specifying {@link
     * android.R.attr#documentLaunchMode}="intoExisting". When used with
     * FLAG_ACTIVITY_MULTIPLE_TASK it is the equivalent of the Activity manifest specifying
     * {@link android.R.attr#documentLaunchMode}="always". The flag is ignored even in
     * conjunction with {@link #FLAG_ACTIVITY_MULTIPLE_TASK} when the Activity manifest specifies
     * {@link android.R.attr#documentLaunchMode}="never".
     *
     * Refer to {@link android.R.attr#documentLaunchMode} for more information.
     *
     * @see android.R.attr#documentLaunchMode
     * @see #FLAG_ACTIVITY_MULTIPLE_TASK
     */
    public static final int FLAG_ACTIVITY_NEW_DOCUMENT = FLAG_ACTIVITY_CLEAR_WHEN_TASK_RESET;
    /**
     * If set, this flag will prevent the normal {@link android.app.Activity#onUserLeaveHint}
     * callback from occurring on the current frontmost activity before it is
     * paused as the newly-started activity is brought to the front.
     *
     * <p>Typically, an activity can rely on that callback to indicate that an
     * explicit user action has caused their activity to be moved out of the
     * foreground. The callback marks an appropriate point in the activity's
     * lifecycle for it to dismiss any notifications that it intends to display
     * "until the user has seen them," such as a blinking LED.
     *
     * <p>If an activity is ever started via any non-user-driven events such as
     * phone-call receipt or an alarm handler, this flag should be passed to {@link
     * Context#startActivity Context.startActivity}, ensuring that the pausing
     * activity does not think the user has acknowledged its notification.
     */
    public static final int FLAG_ACTIVITY_NO_USER_ACTION = 0x00040000;
    /**
     * If set in an Intent passed to {@link Context#startActivity Context.startActivity()},
     * this flag will cause the launched activity to be brought to the front of its
     * task's history stack if it is already running.
     *
     * <p>For example, consider a task consisting of four activities: A, B, C, D.
     * If D calls startActivity() with an Intent that resolves to the component
     * of activity B, then B will be brought to the front of the history stack,
     * with this resulting order:  A, C, D, B.
     *
     * This flag will be ignored if {@link #FLAG_ACTIVITY_CLEAR_TOP} is also
     * specified.
     */
    public static final int FLAG_ACTIVITY_REORDER_TO_FRONT = 0X00020000;
    /**
     * If set in an Intent passed to {@link Context#startActivity Context.startActivity()},
     * this flag will prevent the system from applying an activity transition
     * animation to go to the next activity state.  This doesn't mean an
     * animation will never run -- if another activity change happens that doesn't
     * specify this flag before the activity started here is displayed, then
     * that transition will be used.  This flag can be put to good use
     * when you are going to do a series of activity operations but the
     * animation seen by the user shouldn't be driven by the first activity
     * change but rather a later one.
     */
    public static final int FLAG_ACTIVITY_NO_ANIMATION = 0X00010000;
    /**
     * If set in an Intent passed to {@link Context#startActivity Context.startActivity()},
     * this flag will cause any existing task that would be associated with the
     * activity to be cleared before the activity is started.  That is, the activity
     * becomes the new root of an otherwise empty task, and any old activities
     * are finished.  This can only be used in conjunction with {@link #FLAG_ACTIVITY_NEW_TASK}.
     */
    public static final int FLAG_ACTIVITY_CLEAR_TASK = 0X00008000;
    /**
     * If set in an Intent passed to {@link Context#startActivity Context.startActivity()},
     * this flag will cause a newly launching task to be placed on top of the current
     * home activity task (if there is one).  That is, pressing back from the task
     * will always return the user to home even if that was not the last activity they
     * saw.   This can only be used in conjunction with {@link #FLAG_ACTIVITY_NEW_TASK}.
     */
    public static final int FLAG_ACTIVITY_TASK_ON_HOME = 0X00004000;
    /**
     * By default a document created by {@link #FLAG_ACTIVITY_NEW_DOCUMENT} will
     * have its entry in recent tasks removed when the user closes it (with back
     * or however else it may finish()). If you would like to instead allow the
     * document to be kept in recents so that it can be re-launched, you can use
     * this flag. When set and the task's activity is finished, the recents
     * entry will remain in the interface for the user to re-launch it, like a
     * recents entry for a top-level application.
     * <p>
     * The receiving activity can override this request with
     * {@link android.R.attr#autoRemoveFromRecents} or by explcitly calling
     * {@link android.app.Activity#finishAndRemoveTask()
     * Activity.finishAndRemoveTask()}.
     */
    public static final int FLAG_ACTIVITY_RETAIN_IN_RECENTS = 0x00002000;
    /**
     * This flag is only used for split-screen multi-window mode. The new activity will be displayed
     * adjacent to the one launching it if possible. This can only be used in conjunction with
     * {@link #FLAG_ACTIVITY_NEW_TASK}. Also, setting {@link #FLAG_ACTIVITY_MULTIPLE_TASK} is
     * required if you want a new instance of an existing activity to be created.
     */
    public static final int FLAG_ACTIVITY_LAUNCH_ADJACENT = 0x00001000;
    /**
     * If set in an Intent passed to {@link Context#startActivity Context.startActivity()},
     * this flag will attempt to launch an instant app if no full app on the device can already
     * handle the intent.
     * <p>
     * When attempting to resolve instant apps externally, the following {@link Intent} properties
     * are supported:
     * <ul>
     *     <li>{@link Intent#setAction(String)}</li>
     *     <li>{@link Intent#addCategory(String)}</li>
     *     <li>{@link Intent#setData(Uri)}</li>
     *     <li>{@link Intent#setType(String)}</li>
     *     <li>{@link Intent#setPackage(String)}</li>
     *     <li>{@link Intent#addFlags(int)}</li>
     * </ul>
     * <p>
     * In the case that no instant app can be found, the installer will be launched to notify the
     * user that the intent could not be resolved. On devices that do not support instant apps,
     * the flag will be ignored.
     */
    public static final int FLAG_ACTIVITY_MATCH_EXTERNAL = 0x00000800;
    /**
     * If set in an intent passed to {@link Context#startActivity Context.startActivity()}, this
     * flag will only launch the intent if it resolves to a result that is not a browser. If no such
     * result exists, an {@link ActivityNotFoundException} will be thrown.
     */
    public static final int FLAG_ACTIVITY_REQUIRE_NON_BROWSER = 0x00000400;
    /**
     * If set in an intent passed to {@link Context#startActivity Context.startActivity()}, this
     * flag will only launch the intent if it resolves to a single result. If no such result exists
     * or if the system chooser would otherwise be displayed, an {@link ActivityNotFoundException}
     * will be thrown.
     */
    public static final int FLAG_ACTIVITY_REQUIRE_DEFAULT = 0x00000200;
