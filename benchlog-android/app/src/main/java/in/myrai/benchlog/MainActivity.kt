package in.myrai.benchlog

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

    private val binDir   by lazy { File(filesDir, "bin") }
    private val homeDir  by lazy { File(filesDir, "home") }
    private val busybox  by lazy { File(binDir, "busybox") }
    private val shBin    by lazy { File(binDir, "sh") }
    private val setupTag by lazy { File(filesDir, ".setup_done") }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        if (!setupTag.exists()) {
            runSetupThenStart()
        } else {
            // Refresh benchlog binary on every launch so APK updates take effect
            copyAsset("benchlog", File(binDir, "benchlog"))
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

                progress("Installing BusyBox (Linux commands)…")
                copyAsset("busybox", busybox)

                // Install all BusyBox applets as symlinks: ls, grep, vi, wget, tar, awk, sed…
                progress("Registering Linux commands…")
                ProcessBuilder(busybox.absolutePath, "--install", "-s", binDir.absolutePath)
                    .directory(filesDir)
                    .start()
                    .waitFor()

                // Fallback: create sh symlink manually if --install didn't get it
                if (!shBin.exists()) {
                    Files.createSymbolicLink(shBin.toPath(), busybox.toPath())
                }

                progress("Installing benchlog…")
                copyAsset("benchlog", File(binDir, "benchlog"))

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

    private fun writeProfile() {
        // .profile is sourced by 'sh -l' — auto-launches benchlog, then drops to shell
        File(homeDir, ".profile").writeText(
            """
            export HOME=${homeDir.absolutePath}
            export PATH=${binDir.absolutePath}
            export TERM=xterm-256color
            export COLORTERM=truecolor
            export LANG=en_US.UTF-8

            # Auto-launch benchlog; pressing q drops you back here
            benchlog

            # Shell prompt after benchlog exits
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

        // Run BusyBox sh as a login shell — reads $HOME/.profile on startup
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
            override fun onScroll() = false
            override fun onLongPress(event: MotionEvent) = false
            override fun readControlKey() = false
            override fun readAltKey() = false
            override fun readFnKey() = false
            override fun readShiftKey() = false
            override fun onKeyDown(keyCode: Int, e: KeyEvent, s: TerminalSession) = false
            override fun onKeyUp(keyCode: Int, e: KeyEvent) = false
            override fun onCodePoint(cp: Int, ctrl: Boolean, s: TerminalSession) = false
            override fun copyModeChanged(copy: Boolean) {}
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

    // ── Helpers ───────────────────────────────────────────────────────────────

    private fun copyAsset(name: String, dest: File) {
        assets.open(name).use { src -> dest.outputStream().use { src.copyTo(it) } }
        dest.setExecutable(true, false)
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
