package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mewsync"

	"github.com/arshnah/detsim/rt"
)

func main() {
	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "run":
		runForeground(true)
	case "web":
		runForeground(false)
	case "background":
		runBackground()
	case "setup":
		setupWizard()
	case "settings":
		editSettings()
	case "stop":
		stopInstance(pidFilePath("mewsync.pid"))
	case "kill":
		handleKill(args)
	case "update":
		checkUpdate()
	case "uninstall":
		uninstall()
	case "doctor":
		mewsync.RunDoctor()
	case "history":
		mewsync.PrintHistory()
	case "version":
		fmt.Printf("mewsync %s\n", mewsync.Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `usage: mewsync <command>

commands:
  run                 run the dashboard + engine (default)
  web                 run the engine with the web panel enabled
  background          run detached
  setup               interactive first-time setup
  settings            edit settings interactively
  stop                stop the running foreground instance
  kill background     stop the background instance
  kill autostart      disable autostart
  doctor              diagnose configuration problems
  history             print recently played songs
  update              download and install the latest release in place
  uninstall           disable autostart, remove config
  version             print version
`)
}

func handleKill(args []string) {
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "background":
		stopInstance(pidFilePath("mewsync.background.pid"))
	case "autostart":
		if err := mewsync.NewAutostart().Disable(); err != nil {
			fmt.Fprintln(os.Stderr, "error disabling autostart:", err)
			os.Exit(1)
		}
		fmt.Println("autostart disabled")
	default:
		printUsage()
		os.Exit(1)
	}
}

func pidFilePath(name string) string {
	return filepath.Join(mewsync.ConfigDir(), name)
}

func writePidFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func stopInstance(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("no running instance found")
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "bad pid file:", err)
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "process not found:", err)
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintln(os.Stderr, "error signaling process:", err)
		return
	}
	_ = os.Remove(path)
	fmt.Println("stopped")
}

func buildEngine() (*mewsync.Engine, *mewsync.Shared, *mewsync.SettingsBox, string) {
	dir := mewsync.ConfigDir()
	mewsync.SetLogDir(dir)

	settings, err := mewsync.LoadSettings(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading settings:", err)
		os.Exit(1)
	}
	if settings.AutoOffsetLimitMs == 0 {
		settings.AutoOffsetLimitMs = 2000
	}

	sched := rt.NewSched(time.Now().UnixNano())
	box := mewsync.NewSettingsBox(sched, settings)
	shared := mewsync.NewShared(sched)
	conn := mewsync.NewRealConnector(box, dir)
	conn.SetShared(shared)

	engine := mewsync.NewEngine(sched, box, shared, conn, 64)
	engine.SpawnPoller(2000)

	go func() {
		last := time.Now()
		for {
			time.Sleep(16 * time.Millisecond)
			now := time.Now()
			delta := uint64(now.Sub(last).Milliseconds())
			last = now
			engine.Tick(delta)
		}
	}()

	return engine, shared, box, dir
}

func runForeground(useTUI bool) {
	engine, shared, box, dir := buildEngine()

	pidPath := pidFilePath("mewsync.pid")
	_ = writePidFile(pidPath)
	defer os.Remove(pidPath)

	if useTUI {
		if err := mewsync.RunTUI(shared, box, engine); err != nil {
			fmt.Fprintln(os.Stderr, "tui error:", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("web panel: http://127.0.0.1:8999\n")
	if err := mewsync.RunWebServer(shared, box, dir); err != nil {
		fmt.Fprintln(os.Stderr, "web server error:", err)
		os.Exit(1)
	}
}

func runBackground() {
	if len(os.Args) > 0 && os.Getenv("MEWSYNC_DETACHED") != "1" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error locating executable:", err)
			os.Exit(1)
		}
		cmd := exec.Command(exe, "background")
		cmd.Env = append(os.Environ(), "MEWSYNC_DETACHED=1")
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Stdin = nil
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "error starting background process:", err)
			os.Exit(1)
		}
		fmt.Printf("started background process (pid %d)\n", cmd.Process.Pid)
		return
	}

	engine, shared, box, dir := buildEngine()
	_ = engine
	_ = shared
	_ = box

	pidPath := pidFilePath("mewsync.background.pid")
	if err := writePidFile(pidPath); err != nil {
		fmt.Fprintln(os.Stderr, "error writing pid file:", err)
		os.Exit(1)
	}
	defer os.Remove(pidPath)

	if err := mewsync.RunWebServer(shared, box, dir); err != nil {
		fmt.Fprintln(os.Stderr, "web server error:", err)
		os.Exit(1)
	}
}

func setupWizard() {
	reader := bufio.NewReader(os.Stdin)
	dir := mewsync.ConfigDir()

	fmt.Println("mewsync setup")
	fmt.Println("Choose a playback source:")
	fmt.Println("  1) Spotify (via your Discord account's Spotify connection)")
	fmt.Println("  2) Last.fm (also covers YouTube Music via WebScrobbler, or any other scrobbling player)")
	fmt.Print("> ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	settings, _ := mewsync.LoadSettings(dir)

	switch choice {
	case "2", "lastfm", "last.fm":
		settings.Source = mewsync.SourceLastfm
		fmt.Print("Last.fm API key: ")
		key, _ := reader.ReadString('\n')
		settings.LastfmAPIKey = strings.TrimSpace(key)
		fmt.Print("Last.fm username: ")
		user, _ := reader.ReadString('\n')
		settings.LastfmUsername = strings.TrimSpace(user)
	default:
		settings.Source = mewsync.SourceSpotify
		fmt.Print("Discord user token: ")
		tok, _ := reader.ReadString('\n')
		settings.Token = strings.TrimSpace(tok)
	}

	if settings.ViewTimestamp == false && settings.ViewLabel == false {
		settings.ViewTimestamp = true
		settings.ViewLabel = true
	}

	if err := mewsync.SaveSettings(dir, settings); err != nil {
		fmt.Fprintln(os.Stderr, "error saving settings:", err)
		os.Exit(1)
	}
	fmt.Println("Saved. Run `mewsync` to start.")
}

func editSettings() {
	reader := bufio.NewReader(os.Stdin)
	dir := mewsync.ConfigDir()
	settings, _ := mewsync.LoadSettings(dir)

	fmt.Printf("Auto-clear on song change [%v] (y/n, enter to keep): ", settings.AutoClear)
	if v := promptBool(reader); v != nil {
		settings.AutoClear = *v
	}

	fmt.Printf("Show timestamp prefix [%v] (y/n, enter to keep): ", settings.ViewTimestamp)
	if v := promptBool(reader); v != nil {
		settings.ViewTimestamp = *v
	}

	fmt.Printf("Show label prefix [%v] (y/n, enter to keep): ", settings.ViewLabel)
	if v := promptBool(reader); v != nil {
		settings.ViewLabel = *v
	}

	fmt.Printf("Auto-start on login [%v] (y/n, enter to keep): ", settings.AutoStart)
	if v := promptBool(reader); v != nil {
		settings.AutoStart = *v
		if *v {
			_ = mewsync.NewAutostart().Enable()
		} else {
			_ = mewsync.NewAutostart().Disable()
		}
	}

	if err := mewsync.SaveSettings(dir, settings); err != nil {
		fmt.Fprintln(os.Stderr, "error saving settings:", err)
		os.Exit(1)
	}
	fmt.Println("Saved.")
}

func promptBool(reader *bufio.Reader) *bool {
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	switch line {
	case "y", "yes":
		v := true
		return &v
	case "n", "no":
		v := false
		return &v
	default:
		return nil
	}
}

func checkUpdate() {
	msg, err := mewsync.ApplyUpdate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "update failed:", err)
		os.Exit(1)
	}
	fmt.Println(msg)
}

func uninstall() {
	fmt.Print("This will disable autostart and delete your mewsync config. Continue? (y/n): ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(line)) != "y" {
		fmt.Println("aborted")
		return
	}
	_ = mewsync.NewAutostart().Disable()
	dir := mewsync.ConfigDir()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "error removing config dir:", err)
		os.Exit(1)
	}
	fmt.Println("uninstalled")
}
