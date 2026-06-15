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

    // Source binary — installed by Android's package manager into nativeLibraryDir
    private val nativeLib by lazy { File(applicationInfo.nativeLibraryDir, "libbenchlog.so") }
    // Executable copy in filesDir — Android allows exec from here (Termux pattern)
    private val execBin  by lazy { File(filesDir, "benchlog") }
    private val homeDir  by lazy { File(filesDir, "home").also { it.mkdirs() } }
    private val runLog   by lazy { File(File(homeDir, ".benchlog"), "run.log") }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        // Catch any exception that escapes the UI thread (background threads, etc.)
        Thread.setDefaultUncaughtExceptionHandler { thread, t ->
            runOnUiThread {
                showDiag("Uncaught exception (${thread.name})", t.stackTraceToString())
            }
        }

        try {
            setup()
        } catch (e: Throwable) {
            showDiag("Java crash in setup()", e.stackTraceToString())
        }
    }

    private fun setup() {
        // ── Step 1: verify source binary exists ───────────────────────────────
        if (!nativeLib.exists()) {
            val dir = File(applicationInfo.nativeLibraryDir)
            showDiag(
                "libbenchlog.so not found in nativeLibraryDir",
                "nativeLibraryDir = ${dir.absolutePath}\n" +
                "dir exists        = ${dir.exists()}\n" +
                "contents          = ${dir.listFiles()?.joinToString { it.name } ?: "(empty)"}\n\n" +
                "The .so was not extracted from the APK. Check useLegacyPackaging=true " +
                "in build.gradle and android:extractNativeLibs=\"true\" in the manifest."
            )
            return
        }

        // ── Step 2: copy to filesDir + chmod +x ───────────────────────────────
        val copyErr = tryCopy()
        if (copyErr != null) {
            showDiag("Copy to filesDir failed", copyErr)
            return
        }

        // ── Step 3: pre-flight exec test (ProcessBuilder, no PTY) ─────────────
        // Separates "binary can't exec" from "Termux terminal fails to launch"
        val execErr = testExec()
        if (execErr != null) {
            showDiag(
                "Binary cannot be executed",
                "exec path: ${execBin.absolutePath}\n" +
                "canExecute: ${execBin.canExecute()}\n" +
                "size: ${execBin.length()} bytes\n\n" +
                "Error: $execErr\n\n" +
                "This usually means SELinux or the filesystem noexec flag is " +
                "blocking exec from filesDir on this ROM."
            )
            return
        }

        runLog.delete()
        startTerminal()
    }

    private fun tryCopy(): String? {
        return try {
            if (!execBin.exists() || execBin.length() != nativeLib.length()) {
                nativeLib.copyTo(execBin, overwrite = true)
            }
            execBin.setExecutable(true, false)
            execBin.setReadable(true, false)
            if (!execBin.canExecute()) "setExecutable returned false — filesystem may not support +x" else null
        } catch (e: Exception) {
            "Exception during copy: ${e.message}"
        }
    }

    // Runs the binary once via ProcessBuilder (no PTY) to confirm execve() works.
    // Returns null on success, error string on failure.
    private fun testExec(): String? {
        return try {
            val p = ProcessBuilder(execBin.absolutePath, "--ping")
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
            // --ping exits immediately with code 0; give it 5s to be safe
            val finished = p.waitFor(5, TimeUnit.SECONDS)
            if (!finished) p.destroyForcibly()
            null // exec succeeded (any exit code is fine)
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
            execBin.absolutePath,
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
                override fun onTextChanged(s: TerminalSession) = termView.onScreenUpdated()
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
    }

    private fun onExit(s: TerminalSession) {
        val exit = s.exitStatus
        val log = runLog.takeIf { it.exists() }?.readText()
            ?: "run.log not created — Go main() was never reached.\n" +
               "execve() succeeded (pre-flight passed) but the process exited before writing the log.\n" +
               "Possible causes: signal crash (SIGSEGV/SIGBUS), missing syscall on this Android version."

        showDiag(
            "exited with code $exit",
            buildString {
                appendLine("binary:   ${execBin.absolutePath}")
                appendLine("size:     ${execBin.length()} bytes")
                appendLine("canExec:  ${execBin.canExecute()}")
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
