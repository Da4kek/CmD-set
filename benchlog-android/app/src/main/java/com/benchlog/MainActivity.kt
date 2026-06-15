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

    private val benchlogBin  by lazy { File(applicationInfo.nativeLibraryDir, "libbenchlog.so") }
    private val homeDir      by lazy { File(filesDir, "home").also { it.mkdirs() } }
    private val benchlogDir  by lazy { File(homeDir, ".benchlog") }
    private val runLog       by lazy { File(benchlogDir, "run.log") }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        try {
            if (!benchlogBin.exists()) {
                showDiag("binary not found",
                    "nativeLibraryDir = ${applicationInfo.nativeLibraryDir}\n" +
                    "exists           = ${File(applicationInfo.nativeLibraryDir).exists()}\n" +
                    "contents         = ${File(applicationInfo.nativeLibraryDir).listFiles()?.joinToString { it.name }}")
                return
            }
            runLog.delete()          // clear previous run's log
            startTerminal()
        } catch (e: Throwable) {
            showDiag("Java crash in onCreate", e.stackTraceToString())
        }
    }

    private fun startTerminal(args: Array<String> = emptyArray()) {
        val frame = FrameLayout(this).apply { setBackgroundColor(0xFF000000.toInt()) }
        termView = TerminalView(this, null)
        frame.addView(termView,
            ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        setContentView(frame)

        session = TerminalSession(
            benchlogBin.absolutePath,
            homeDir.absolutePath,
            args,
            arrayOf(
                "HOME=${homeDir.absolutePath}",
                "TERM=xterm-256color",
                "COLORTERM=truecolor",
                "LANG=en_US.UTF-8",
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
        if (exit == 0) {
            // clean exit — just restart
            runLog.delete()
            startTerminal()
            return
        }
        // non-zero: show diagnostics
        val log = if (runLog.exists()) runLog.readText() else "(run.log not created — binary may not have executed at all)"
        showDiag(
            "benchlog exited (code $exit)",
            buildString {
                appendLine("binary:  ${benchlogBin.absolutePath}")
                appendLine("exists:  ${benchlogBin.exists()}")
                appendLine("HOME:    ${homeDir.absolutePath}")
                appendLine()
                appendLine("=== run.log ===")
                appendLine(log)
                appendLine()
                appendLine("Tap anywhere to run diagnostics mode.")
            },
            onTap = { startTerminal(arrayOf("--diag")) }
        )
    }

    private fun showDiag(title: String, body: String, onTap: (() -> Unit)? = null) {
        val tv = TextView(this).apply {
            text = "▶ $title\n\n$body"
            setTextColor(0xFFFF9500.toInt())
            textSize = 11f
            setPadding(32, 60, 32, 32)
            setBackgroundColor(0xFF000000.toInt())
            typeface = android.graphics.Typeface.MONOSPACE
            if (onTap != null) setOnClickListener { onTap() }
        }
        val scroll = ScrollView(this).apply {
            addView(tv)
            setBackgroundColor(0xFF000000.toInt())
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
