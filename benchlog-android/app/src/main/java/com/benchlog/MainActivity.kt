package com.benchlog

import android.app.Activity
import android.os.Bundle
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.FrameLayout
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

    // benchlog binary — extracted by Android from jniLibs at install time
    private val benchlogBin by lazy {
        File(applicationInfo.nativeLibraryDir, "libbenchlog.so")
    }
    private val homeDir by lazy { File(filesDir, "home").also { it.mkdirs() } }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        try {
            if (!benchlogBin.exists()) {
                showError(
                    "benchlog binary not found.\n\n" +
                    "nativeLibraryDir: ${applicationInfo.nativeLibraryDir}\n" +
                    "exists: ${File(applicationInfo.nativeLibraryDir).exists()}\n" +
                    "contents: ${File(applicationInfo.nativeLibraryDir).listFiles()?.map { it.name }}"
                )
                return
            }
            startTerminal()
        } catch (e: Throwable) {
            showError("Startup error:\n${e.javaClass.simpleName}: ${e.message}")
        }
    }

    private fun startTerminal() {
        val frame = FrameLayout(this).apply { setBackgroundColor(0xFF000000.toInt()) }
        termView = TerminalView(this, null)
        frame.addView(
            termView,
            ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
        )
        setContentView(frame)

        session = TerminalSession(
            benchlogBin.absolutePath,
            homeDir.absolutePath,
            emptyArray(),
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
                override fun onSessionFinished(s: TerminalSession) = runOnUiThread { restartOrShowError(s) }
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

    private fun restartOrShowError(s: TerminalSession) {
        // Keep the terminal visible so any crash output is readable.
        // Overlay a dim restart banner at the bottom.
        val exitCode = s.exitStatus
        val banner = android.widget.TextView(this).apply {
            text = if (exitCode == 0) "benchlog exited — tap to restart"
                   else "benchlog crashed (exit $exitCode) — tap to restart"
            setTextColor(if (exitCode == 0) 0xFF00FF9F.toInt() else 0xFFFF453A.toInt())
            textSize = 13f
            setPadding(32, 20, 32, 20)
            setBackgroundColor(0xDD000000.toInt())
            gravity = android.view.Gravity.CENTER
            setOnClickListener { startTerminal() }
        }
        val root = window.decorView.findViewById<android.widget.FrameLayout>(android.R.id.content)
        root.addView(banner, android.widget.FrameLayout.LayoutParams(
            android.widget.FrameLayout.LayoutParams.MATCH_PARENT,
            android.widget.FrameLayout.LayoutParams.WRAP_CONTENT,
            android.view.Gravity.BOTTOM
        ))
    }

    private fun showError(msg: String) {
        setContentView(TextView(this).apply {
            text = msg
            setTextColor(0xFFff6666.toInt())
            textSize = 12f
            setPadding(32, 60, 32, 32)
            setBackgroundColor(0xFF000000.toInt())
        })
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
