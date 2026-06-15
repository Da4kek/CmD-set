package com.benchlog

import android.app.Activity
import android.os.Bundle
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.FrameLayout
import android.widget.ScrollView
import android.widget.TextView
import com.termux.terminal.TerminalEmulator
import com.termux.terminal.TerminalSession
import com.termux.terminal.TerminalSessionClient
import com.termux.view.TerminalView
import com.termux.view.TerminalViewClient
import java.io.File
import java.util.concurrent.TimeUnit

class MainActivity : Activity() {

    private lateinit var termView: TerminalView
    private lateinit var session: TerminalSession

    // The binary lives in nativeLibraryDir (apk_data_file SELinux type).
    // Android allows exec from here even on API 29+ W^X policy because
    // the package manager owns this directory — apps cannot write to it.
    // filesDir (app_data_file) is writable by the app, so exec is blocked.
    private val binary  by lazy { File(applicationInfo.nativeLibraryDir, "libbenchlog.so") }
    private val homeDir by lazy { File(filesDir, "home").also { it.mkdirs() } }
    private val runLog  by lazy { File(File(homeDir, ".benchlog"), "run.log") }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        Thread.setDefaultUncaughtExceptionHandler { thread, t ->
            runOnUiThread { showDiag("Uncaught exception (${thread.name})", t.stackTraceToString()) }
        }

        try {
            setup()
        } catch (e: Throwable) {
            showDiag("Java crash in setup()", e.stackTraceToString())
        }
    }

    private fun setup() {
        // ── Step 1: verify binary was extracted from APK ──────────────────────
        if (!binary.exists()) {
            val dir = File(applicationInfo.nativeLibraryDir)
            showDiag(
                "libbenchlog.so not found",
                "nativeLibraryDir = ${dir.absolutePath}\n" +
                "dir exists        = ${dir.exists()}\n" +
                "contents          = ${dir.listFiles()?.joinToString { it.name } ?: "(empty)"}\n\n" +
                "Check useLegacyPackaging=true in build.gradle and " +
                "android:extractNativeLibs=\"true\" in the manifest."
            )
            return
        }

        // ── Step 2: pre-flight exec test ──────────────────────────────────────
        // nativeLibraryDir has SELinux type apk_data_file — exec is allowed.
        // If this fails, something unusual is blocking exec on this ROM.
        val execErr = testExec()
        if (execErr != null) {
            showDiag(
                "Binary cannot be executed from nativeLibraryDir",
                "path:  ${binary.absolutePath}\n" +
                "size:  ${binary.length()} bytes\n\n" +
                "Error: $execErr\n\n" +
                "apk_data_file exec is normally allowed on all Android versions.\n" +
                "This ROM may have a non-standard SELinux policy. Check adb logcat\n" +
                "for an 'avc: denied' line to confirm."
            )
            return
        }

        runLog.delete()
        startTerminal()
    }

    // Test whether the binary can actually be exec'd before handing control to
    // Termux's JNI layer. Uses ProcessBuilder (plain fork+exec, no PTY).
    private fun testExec(): String? {
        return try {
            val p = ProcessBuilder(binary.absolutePath, "--ping")
                .directory(homeDir)
                .apply {
                    environment().apply {
                        put("HOME", homeDir.absolutePath)
                        put("TERM", "dumb")
                        put("TMPDIR", cacheDir.absolutePath)
                    }
                }
                .redirectErrorStream(true)
                .start()
            val finished = p.waitFor(5, TimeUnit.SECONDS)
            if (!finished) p.destroyForcibly()
            null
        } catch (e: Exception) {
            e.message ?: e.javaClass.name
        }
    }

    private fun startTerminal(args: Array<String> = emptyArray()) {
        val frame = FrameLayout(this).apply { setBackgroundColor(0xFF000000.toInt()) }
        termView = TerminalView(this, null)
        frame.addView(termView,
            ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        setContentView(frame)

        session = TerminalSession(
            binary.absolutePath,
            homeDir.absolutePath,
            args,
            arrayOf(
                "HOME=${homeDir.absolutePath}",
                "TERM=xterm-256color",
                "COLORTERM=truecolor",
                "LANG=en_US.UTF-8",
                "TMPDIR=${cacheDir.absolutePath}",
            ),
            TerminalEmulator.DEFAULT_TERMINAL_TRANSCRIPT_ROWS,
            object : TerminalSessionClient {
                // onTextChanged fires on a background reader thread.
                // invalidate() is silently dropped off the UI thread, so
                // we must dispatch onScreenUpdated() back to the main thread.
                override fun onTextChanged(s: TerminalSession) {
                    runOnUiThread { if (::termView.isInitialized) termView.onScreenUpdated() }
                }
                override fun onTitleChanged(s: TerminalSession) {}
                override fun onSessionFinished(s: TerminalSession) = runOnUiThread { onExit(s) }
                override fun onCopyTextToClipboard(s: TerminalSession, text: String) {}
                override fun onPasteTextFromClipboard(s: TerminalSession?) {}
                override fun onBell(s: TerminalSession) {}
                override fun onColorsChanged(s: TerminalSession) {}
                override fun onTerminalCursorStateChange(state: Boolean) {}
                override fun getTerminalCursorStyle() = TerminalEmulator.TERMINAL_CURSOR_STYLE_UNDERLINE
                override fun logError(tag: String, message: String) {}
                override fun logWarn(tag: String, message: String) {}
                override fun logInfo(tag: String, message: String) {}
                override fun logDebug(tag: String, message: String) {}
                override fun logVerbose(tag: String, message: String) {}
                override fun logStackTraceWithMessage(tag: String, message: String, e: Exception) {}
                override fun logStackTrace(tag: String, e: Exception) {}
            }
        )

        termView.setTerminalViewClient(object : TerminalViewClient {
            override fun onScale(scale: Float): Float = 1f
            override fun onSingleTapUp(e: MotionEvent) {}
            override fun shouldBackButtonBeMappedToEscape() = false
            override fun shouldEnforceCharBasedInput() = false
            override fun shouldUseCtrlSpaceWorkaround() = false
            override fun isTerminalViewSelected() = true
            override fun copyModeChanged(copy: Boolean) {}
            override fun onKeyDown(keyCode: Int, e: KeyEvent, s: TerminalSession) = false
            override fun onKeyUp(keyCode: Int, e: KeyEvent) = false
            override fun onLongPress(event: MotionEvent) = false
            override fun readControlKey() = false
            override fun readAltKey() = false
            override fun readShiftKey() = false
            override fun readFnKey() = false
            override fun onCodePoint(cp: Int, ctrl: Boolean, s: TerminalSession) = false
            override fun onEmulatorSet() {}
            override fun logError(tag: String, message: String) {}
            override fun logWarn(tag: String, message: String) {}
            override fun logInfo(tag: String, message: String) {}
            override fun logDebug(tag: String, message: String) {}
            override fun logVerbose(tag: String, message: String) {}
            override fun logStackTraceWithMessage(tag: String, message: String, e: Exception) {}
            override fun logStackTrace(tag: String, e: Exception) {}
        })

        termView.attachSession(session)
        termView.requestFocus()
        // Force a redraw — binary may have already rendered before attachSession was called
        termView.postInvalidate()
    }

    private fun onExit(s: TerminalSession) {
        val exit = s.exitStatus
        val log = runLog.takeIf { it.exists() }?.readText()
            ?: "run.log not created — Go main() was never reached.\n" +
               "execve() may have been denied, or the binary crashed before writing the log.\n" +
               "Check adb logcat for 'avc: denied' or signal crash info."

        showDiag(
            "exited with code $exit",
            buildString {
                appendLine("binary:  ${binary.absolutePath}")
                appendLine("size:    ${binary.length()} bytes")
                appendLine()
                appendLine("=== run.log ===")
                append(log)
            },
            onTap = {
                runLog.delete()
                startTerminal(arrayOf("--diag"))
            }
        )
    }

    private fun showDiag(title: String, body: String, onTap: (() -> Unit)? = null) {
        val tv = TextView(this).apply {
            text = "▶ $title\n\n$body\n\n${if (onTap != null) "[tap to run --diag mode]" else ""}"
            setTextColor(0xFFFF9500.toInt())
            textSize = 11f
            setPadding(32, 60, 32, 32)
            setBackgroundColor(0xFF000000.toInt())
            typeface = android.graphics.Typeface.MONOSPACE
            if (onTap != null) setOnClickListener { onTap() }
        }
        val scroll = ScrollView(this).apply {
            setBackgroundColor(0xFF000000.toInt())
            addView(tv)
        }
        setContentView(scroll)
    }

    override fun onResume() {
        super.onResume()
        if (::termView.isInitialized) termView.requestFocus()
    }

    override fun onDestroy() {
        super.onDestroy()
        if (::session.isInitialized) session.finishIfRunning()
    }
}
