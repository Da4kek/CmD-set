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

class MainActivity : Activity() {

    private lateinit var termView: TerminalView
    private lateinit var session: TerminalSession

    // Source binary — installed by Android's package manager
    private val nativeLib by lazy { File(applicationInfo.nativeLibraryDir, "libbenchlog.so") }
    // Executable copy in filesDir — Android guarantees apps can exec from here
    private val execBin  by lazy { File(filesDir, "benchlog") }
    private val homeDir  by lazy { File(filesDir, "home").also { it.mkdirs() } }
    private val runLog   by lazy { File(homeDir, ".benchlog", "run.log") }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        try {
            setup()
        } catch (e: Throwable) {
            showDiag("Java crash", e.stackTraceToString())
        }
    }

    private fun setup() {
        // ── Step 1: verify source binary exists ───────────────────────────────
        if (!nativeLib.exists()) {
            val dir = File(applicationInfo.nativeLibraryDir)
            showDiag("libbenchlog.so not found",
                "nativeLibraryDir = ${dir.absolutePath}\n" +
                "dir exists        = ${dir.exists()}\n" +
                "contents          = ${dir.listFiles()?.joinToString { it.name } ?: "(empty)"}")
            return
        }

        // ── Step 2: copy to filesDir + chmod +x ───────────────────────────────
        val copyErr = tryCopy()
        if (copyErr != null) {
            showDiag("Copy failed", copyErr)
            return
        }

        // ── Step 3: sanity-check file permissions ─────────────────────────────
        val permInfo = buildString {
            appendLine("source: ${nativeLib.absolutePath}")
            appendLine("  exists=${nativeLib.exists()} readable=${nativeLib.canRead()} exec=${nativeLib.canExecute()} size=${nativeLib.length()}")
            appendLine("exec copy: ${execBin.absolutePath}")
            appendLine("  exists=${execBin.exists()} readable=${execBin.canRead()} exec=${execBin.canExecute()} size=${execBin.length()}")
        }
        android.util.Log.d("benchlog", permInfo)

        runLog.delete()
        startTerminal()
    }

    private fun tryCopy(): String? {
        return try {
            // Only re-copy if binary changed (reinstall)
            if (!execBin.exists() || execBin.length() != nativeLib.length()) {
                nativeLib.copyTo(execBin, overwrite = true)
            }
            execBin.setExecutable(true, false)
            execBin.setReadable(true, false)
            if (!execBin.canExecute()) "setExecutable returned false — filesystem may not support exec" else null
        } catch (e: Exception) {
            "Exception during copy: ${e.message}"
        }
    }

    private fun startTerminal(args: Array<String> = emptyArray()) {
        // Remove any previous diagnostic view
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
            ?: "run.log not created — Go main() was never reached.\nThis means execve() failed or the binary crashed before main()."

        showDiag(
            "exit code $exit",
            buildString {
                appendLine("binary:     ${execBin.absolutePath}")
                appendLine("can exec:   ${execBin.canExecute()}")
                appendLine("size:       ${execBin.length()} bytes")
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
