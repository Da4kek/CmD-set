package com.benchlog

import android.app.Activity
import android.os.Bundle
import android.os.Handler
import android.os.Looper
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
import java.nio.file.Files

class MainActivity : Activity() {

    private lateinit var termView: TerminalView
    private lateinit var session: TerminalSession

    // Android extracts jniLibs/*.so to nativeLibraryDir — always executable
    private val nativeLibDir by lazy { File(applicationInfo.nativeLibraryDir) }
    private val busyboxLib  by lazy { File(nativeLibDir, "libbusybox.so") }
    private val benchlogLib by lazy { File(nativeLibDir, "libbenchlog.so") }

    private val binDir   by lazy { File(filesDir, "bin") }
    private val homeDir  by lazy { File(filesDir, "home") }
    private val setupTag by lazy { File(filesDir, ".setup_done") }
    private val shBin    by lazy { File(binDir, "sh") }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        if (!setupTag.exists()) {
            runSetupThenStart()
        } else {
            startTerminal()
        }
    }

    // ── First-launch setup ────────────────────────────────────────────────────

    private fun runSetupThenStart() {
        val tv = TextView(this).apply {
            text = "Setting up Linux environment…"
            setTextColor(0xFF88ff88.toInt())
            textSize = 15f
            setPadding(48, 80, 48, 40)
            setBackgroundColor(0xFF000000.toInt())
        }
        setContentView(tv)

        val handler = Handler(Looper.getMainLooper())

        Thread {
            fun progress(msg: String) = handler.post { tv.text = msg }
            try {
                progress("Creating directories…")
                binDir.mkdirs()
                homeDir.mkdirs()

                progress("Registering Linux commands…")
                createBusyboxSymlinks()

                progress("Writing shell profile…")
                writeProfile()

                setupTag.createNewFile()
                handler.post { startTerminal() }

            } catch (e: Exception) {
                handler.post {
                    tv.text = "Setup failed: ${e.message}\n\nReinstall the app to retry."
                }
            }
        }.start()
    }

    private fun createBusyboxSymlinks() {
        val applets = listOf(
            "sh", "ash", "ls", "cat", "grep", "egrep", "fgrep", "find",
            "sed", "awk", "vi", "wget", "tar", "gzip", "gunzip",
            "bzip2", "bunzip2", "xz", "unxz", "mkdir", "cp", "mv", "rm",
            "rmdir", "chmod", "chown", "echo", "printf", "pwd", "env",
            "date", "time", "which", "head", "tail", "cut", "sort", "uniq",
            "wc", "diff", "patch", "xargs", "test", "true", "false",
            "ps", "kill", "top", "du", "df", "free", "uname", "id",
            "whoami", "hostname", "ping", "nc", "tee", "yes", "sleep",
            "seq", "expr", "basename", "dirname", "realpath", "readlink",
            "ln", "stat", "touch", "less", "more", "hexdump", "strings",
            "base64", "md5sum", "sha256sum", "tr", "nl", "tac", "rev",
            "fold", "expand", "od", "bc", "timeout", "nohup", "stty"
        )

        val target = busyboxLib.toPath()
        for (applet in applets) {
            val link = File(binDir, applet)
            if (!link.exists()) {
                runCatching { Files.createSymbolicLink(link.toPath(), target) }
            }
        }

        val benchlogLink = File(binDir, "benchlog")
        if (!benchlogLink.exists()) {
            runCatching { Files.createSymbolicLink(benchlogLink.toPath(), benchlogLib.toPath()) }
        }
    }

    private fun writeProfile() {
        File(homeDir, ".profile").writeText(
            """
            export HOME=${homeDir.absolutePath}
            export PATH=${binDir.absolutePath}
            export TERM=xterm-256color
            export COLORTERM=truecolor
            export LANG=en_US.UTF-8

            benchlog

            PS1='$ '
            """.trimIndent()
        )
    }

    // ── Terminal ──────────────────────────────────────────────────────────────

    private fun startTerminal() {
        val frame = FrameLayout(this).apply { setBackgroundColor(0xFF000000.toInt()) }
        termView = TerminalView(this, null)
        frame.addView(
            termView,
            ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
        )
        setContentView(frame)

        val env = arrayOf(
            "HOME=${homeDir.absolutePath}",
            "PATH=${binDir.absolutePath}",
            "TERM=xterm-256color",
            "COLORTERM=truecolor",
            "LANG=en_US.UTF-8",
        )

        session = TerminalSession(
            shBin.absolutePath,
            homeDir.absolutePath,
            arrayOf("-l"),
            env,
            TerminalEmulator.DEFAULT_TERMINAL_TRANSCRIPT_ROWS,
            object : TerminalSessionClient {
                override fun onTextChanged(s: TerminalSession) = termView.onScreenUpdated()
                override fun onTitleChanged(s: TerminalSession) {}
                override fun onSessionFinished(s: TerminalSession) = finish()
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

    override fun onResume() {
        super.onResume()
        if (::termView.isInitialized) termView.requestFocus()
    }

    override fun onDestroy() {
        super.onDestroy()
        if (::session.isInitialized) session.finishIfRunning()
    }
}
