package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/backup"
	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/engine"
	"github.com/Anonyfox/lfr/internal/events"
)

func runSetup(c *call) error {
	data := c.data
	res, err := engine.Setup(c.ctx, engine.SetupOptions{Data: data, Hostname: c.str("hostname"), IPv4: c.str("ipv4"), IPv6: c.str("ipv6"),
		ACMEEmail: c.str("acme-email"), Now: func() int64 { return time.Now().Unix() }})
	if err != nil {
		return err
	}
	if res.Hostname == "" {
		return usage("--hostname is required the first time, e.g. --hostname mail.example.com")
	}
	var service string
	if c.has("systemd") {
		if service, err = installService(c, data); err != nil {
			return err
		}
	}
	return c.result(res, func() {
		verb := "Created"
		if res.Existing {
			verb = "Updated"
		}
		c.printf("%s %s\n", verb, data)
		kv(c.env.Stdout, "hostname", res.Hostname, "ipv4", res.IPv4, "ipv6", res.IPv6, "service", service)
		if res.IPv4 == "" {
			c.next("lfr config ipv4 <public IPv4> (could not detect one)")
			return
		}
		if service == "" {
			c.next("start the server: sudo lfr setup --systemd (or lfr serve --data %s), then lfr domain add <domain>", data)
			return
		}
		c.next("lfr domain add <domain>, then publish the DNS records it prints")
	})
}

func installService(c *call, data string) (string, error) {
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("--systemd needs root: sudo lfr setup --systemd")
	}
	name := c.str("user")
	if name == "" {
		name = "lfr"
	}
	if _, err := user.Lookup(name); err != nil {
		if out, err := exec.Command("useradd", "--system", "--home-dir", data, "--shell", "/usr/sbin/nologin", name).CombinedOutput(); err != nil {
			return "", fmt.Errorf("useradd: %v: %s", err, out)
		}
	}
	if out, err := exec.Command("chown", "-R", name+":"+name, data).CombinedOutput(); err != nil {
		return "", fmt.Errorf("chown: %v: %s", err, out)
	}
	os.Chmod(data, 0o750)
	bin, err := os.Executable()
	if err != nil {
		return "", err
	}
	bin, _ = filepath.EvalSymlinks(bin)
	unit := "/etc/systemd/system/lfr.service"
	if err := os.WriteFile(unit, []byte(engine.SystemdUnit(bin, data, name)), 0o644); err != nil {
		return "", err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", "lfr"}, {"restart", "lfr"}} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	return unit + " (running as " + name + "; add your agent's user to group " + name + " to use the CLI without sudo)", nil
}

func runServe(c *call) error {
	level := slog.LevelInfo
	if c.env.getenv("LFR_LOG") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(c.env.Stderr, &slog.HandlerOptions{Level: level}))
	addr := func(name, def string) string {
		if c.has(name) {
			return c.str(name)
		}
		return def
	}
	e, err := engine.Open(c.ctx, engine.Options{Data: c.data, SMTPAddr: addr("smtp", ":25"), HTTPAddr: addr("http", ":80"), HTTPSAddr: addr("https", ":443"),
		Version: c.env.Build.Version, Log: log})
	if err != nil {
		return err
	}
	defer e.Close()
	return e.Run(c.ctx)
}

func runStatus(c *call) error {
	var st api.Status
	if err := c.do(api.OpStatus, nil, &st); err != nil {
		return err
	}
	return c.result(st, func() {
		q := st.Queue
		out := st.Transport
		if q.Down != "" {
			out += " (DOWN since " + q.DownSince + ": " + q.Down + ")"
		}
		kv(c.env.Stdout, "version", st.Version, "uptime", st.Uptime, "hostname", st.Hostname, "ipv4", st.IPv4, "outbound", out,
			"listening", strings.Join(st.Listening, ", "),
			"domains", strconv.Itoa(st.Domains), "mailboxes", fmt.Sprintf("%d (%d unread)", st.Mailboxes, st.Unread),
			"lists", fmt.Sprintf("%d (%s)", st.Lists, count(st.Members, "confirmed reader", "confirmed readers")),
			"queue", fmt.Sprintf("%d queued, %d retrying, %d sent / %d failed / %d bounced in 24h", q.Queued, q.Retrying, q.Sent24h, q.Failed24, q.Bounce24),
			"paused", strings.Join(st.Paused, "; "))
		for _, n := range st.Next {
			c.next("%s", n)
		}
	})
}

func runDoctor(c *call) error {
	var checks []api.Check
	if err := c.do(api.OpDoctor, nil, &checks); err != nil {
		return err
	}
	failed := 0
	for _, ch := range checks {
		if ch.Status == "fail" {
			failed++
		}
	}
	err := c.result(checks, func() {
		for _, ch := range checks {
			c.printf("%-5s %-28s %s\n", paint(c.env.Stdout, ch.Status), ch.Name, ch.Detail)
			if ch.Fix != "" && ch.Status != "ok" {
				c.printf("      %-28s fix: %s\n", "", ch.Fix)
			}
		}
		if failed == 0 {
			c.next("mail works. For a newsletter on one of these domains: lfr init <dir> --list <slug> (skill lfr-concept)")
		} else {
			c.next("fix the %d failing checks above, then run lfr doctor again", failed)
		}
	})
	if err == nil && failed > 0 {
		return &silentErr{}
	}
	return err
}

// silentErr fails the exit code without another message.
type silentErr struct{}

func (silentErr) Error() string { return "" }

func runConfig(c *call) error {
	switch len(c.args) {
	case 0:
		var all []config.Entry
		if err := c.do(api.OpConfig, nil, &all); err != nil {
			return err
		}
		return c.result(all, func() {
			var rows [][]string
			for _, e := range all {
				v := e.Value
				if e.Default {
					v += " (default)"
				}
				rows = append(rows, []string{e.Name, v})
			}
			table(c.env.Stdout, []string{"setting", "value"}, rows)
			c.next("lfr help config; lfr config <name> explains one setting")
		})
	case 1:
		var e config.Entry
		if err := c.do(api.OpConfigGet, api.Name{Name: c.args[0]}, &e); err != nil {
			return err
		}
		return c.result(e, func() { c.printf("%s\n\n%s\n", e.Value, e.Help) })
	case 2:
		c.args = append([]string{}, c.args...)
		return runConfigSet(c)
	}
	return usage("too many arguments")
}

func runConfigSet(c *call) error {
	if err := exactly(c, 2); err != nil {
		return err
	}
	var all []config.Entry
	if err := c.do(api.OpConfigSet, api.ConfigSet{Name: c.args[0], Value: c.args[1]}, &all); err != nil {
		return err
	}
	return c.result(map[string]string{"name": c.args[0], "value": c.args[1]}, func() { c.printf("%s = %s\n", c.args[0], c.args[1]) })
}

// --- domains ---

func runWatch(c *call) error {
	enc := json.NewEncoder(c.env.Stdout)
	return c.client().Stream(c.ctx, api.OpWatch, api.Watch{Kinds: c.all("kind")}, func(m json.RawMessage) error {
		if c.json {
			return enc.Encode(m)
		}
		var e events.Event
		json.Unmarshal(m, &e)
		c.printf("%s  %-13s %s\n", e.At.Local().Format("15:04:05"), e.Kind, e.Detail)
		return nil
	})
}

func runEvents(c *call) error {
	if c.has("follow") {
		return runWatch(c)
	}
	a := api.Events{Since: c.str("since"), Kinds: c.all("kind"), List: c.str("list")}
	var err error
	if a.Limit, err = intFlag(c, "limit"); err != nil {
		return err
	}
	var evs []events.Event
	if err := c.do(api.OpEvents, a, &evs); err != nil {
		return err
	}
	return c.result(evs, func() {
		for _, e := range evs {
			c.printf("%s  %-13s %s\n", when(e.At), e.Kind, e.Detail)
		}
	})
}

// --- lists ---

func runBackup(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	dir, err := filepath.Abs(c.args[0])
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(c.ctx, time.Hour)
	defer cancel()
	res, err := backup.Receive(ctx, c.client(), dir)
	if err != nil {
		return err
	}
	return c.result(res, func() {
		c.printf("Backed up the database and %d messages (%d MB) to %s\n", res.Blobs, res.Bytes>>20, res.Dir)
	})
}
