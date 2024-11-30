// Package main handles overlay backup utilities
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	curDir        = "cur"
	diffCommand   = "diff"
	statusCommand = "status"
	commitCommand = "commit"
	mountCommand  = "mount"
	envBase       = "OBU_"
	configEnv     = envBase + "CONFIG"
	hookEnv       = envBase + "HOOKS"
)

var pathSep = string(filepath.Separator)

type (
	fileSet struct {
		base   string
		offset string
	}
	// Config is the definition for setting up overlays
	Config struct {
		OBU struct {
			Modules     []string
			Directories []string
			RootBind    string
			Tracked     []string
		}
		Device Device
	}
	// Device is a device structure for overlay source/mount
	Device struct {
		Source string
		Mount  string
	}
)

func (f fileSet) String() string {
	return filepath.Join(f.base, f.offset)
}

func (d Device) upper(path string) string {
	return d.dir(path, curDir, "upperdir")
}

func (d Device) work(path string) string {
	return d.dir(path, curDir, "workdir")
}

func (d Device) root(path string) string {
	return d.dir(path, "rootfs")
}

func (d Device) dir(paths ...string) string {
	adding := []string{d.Mount}
	adding = append(adding, paths...)
	return filepath.Join(adding...)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootDir(path string) string {
	return fmt.Sprintf("%c%s", filepath.Separator, path)
}

func envOr(key, input string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return input
}

func run() error {
	if os.Geteuid() != 0 {
		return errors.New("must be run as root")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	name := filepath.Base(exe)
	etc := filepath.Join(newRootDir("etc"), name)
	hooks := filepath.Join(etc, "hooks")
	config := filepath.Join(etc, "config.toml")
	help := func(err error) {
		msg := ""
		exit := 0
		if err != nil {
			exit = 1
			msg = fmt.Sprintf("error: %v\n\n", err)
		}
		var commands []string
		type helpCommand struct {
			name string
			desc string
		}
		for _, item := range []helpCommand{
			{commitCommand, "commit files to persist in the overlay"},
			{diffCommand, "diff files from the overlay against lower directories"},
			{mountCommand, "mount the overlay system"},
			{statusCommand, "list files that are different"},
		} {
			commands = append(commands, fmt.Sprintf("  %-10s %s", item.name, item.desc))
		}
		sort.Strings(commands)
		var env []string
		for k, v := range map[string]string{
			configEnv: config,
			hookEnv:   hooks,
		} {
			env = append(env, fmt.Sprintf("use the '%s' environment to override the path for:\n  %s", k, v))
		}
		sort.Strings(env)
		fmt.Fprintf(os.Stderr, `%s%s:
 %s [command] (files...)

commands:
%s

%s
`, msg, name, name, strings.Join(commands, "\n"), strings.Join(env, "\n"))
		os.Exit(exit)
	}
	args := os.Args
	if len(args) <= 1 {
		help(errors.New("arguments required"))
	}
	config = envOr(configEnv, config)
	hooks = envOr(hookEnv, hooks)

	if config == "" || !pathExists(config) {
		help(errors.New("invalid configuration file"))
	}
	b, err := os.ReadFile(config)
	if err != nil {
		return err
	}
	cfg := Config{}
	decoder := toml.NewDecoder(bytes.NewReader(b))
	md, err := decoder.Decode(&cfg)
	if err != nil {
		return err
	}
	undecoded := md.Undecoded()
	if len(undecoded) > 0 {
		return fmt.Errorf("undecoded fields: %v", undecoded)
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	cmd := args[1]
	var sub []string
	if len(args) > 1 {
		sub = args[2:]
	}
	switch cmd {
	case commitCommand, diffCommand, statusCommand:
		files, err := cfg.calculateFileList(sub...)
		if err != nil {
			return err
		}
		switch cmd {
		case commitCommand:
			return cfg.Device.commit(files)
		case diffCommand, statusCommand:
			return cfg.diff(cmd == statusCommand, files)
		}
	case mountCommand:
		if len(sub) > 0 {
			return fmt.Errorf("%s does not take arguments", cmd)
		}
		return cfg.mount(hooks)
	default:
		help(fmt.Errorf("unknown command: %s", cmd))
	}

	return nil
}

func (c Config) diff(nameOnly bool, files []fileSet) error {
	for _, f := range files {
		has := filepath.Join(c.Device.root(f.base), f.offset)
		if !pathExists(has) {
			has = filepath.Join(c.OBU.RootBind, f.base, f.offset)
		}
		if !pathExists(has) {
			fmt.Printf("%s does not exist in any lower directory\n", f.String())
			continue
		}
		from := filepath.Join(c.Device.upper(f.base), f.offset)
		cmd := pipedCommand("diff", "-U", "0", has, from)
		if nameOnly {
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			if err := cmd.Run(); err != nil {
				fmt.Printf("+/- %s\n", f.String())
			}
			continue
		}
		cmd.Run()
	}
	return nil
}

func (c Config) validate() error {
	if c.OBU.RootBind == "" {
		return errors.New("no rootbind")
	}
	if len(c.OBU.Directories) == 0 {
		return errors.New("no directories given")
	}
	if c.Device.Source == "" {
		return errors.New("no source device given")
	}
	if c.Device.Mount == "" {
		return errors.New("no device mount given")
	}
	return nil
}

func copyDir(src, dst string) error {
	if pathExists(dst) {
		return nil
	}
	parent := filepath.Dir(dst)
	if !pathExists(parent) {
		if err := copyDir(filepath.Dir(src), parent); err != nil {
			return err
		}
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Mkdir(dst, info.Mode())
}

func copyFile(src, dst string) error {
	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer from.Close()

	info, err := from.Stat()
	if err != nil {
		return err
	}
	if err := copyDir(filepath.Dir(src), filepath.Dir(dst)); err != nil {
		return err
	}
	to, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer to.Close()
	if _, err := io.Copy(to, from); err != nil {
		return err
	}
	return nil
}

func (d Device) commit(files []fileSet) error {
	for _, f := range files {
		to := filepath.Join(d.root(f.base), f.offset)
		from := filepath.Join(d.upper(f.base), f.offset)
		if err := copyFile(from, to); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) calculateFileList(filters ...string) ([]fileSet, error) {
	if len(c.OBU.Tracked) == 0 {
		return nil, errors.New("no tracked paths set")
	}
	filter := func(string) bool {
		return true
	}
	if len(filters) > 0 {
		filter = func(path string) bool {
			for _, f := range filters {
				if path == f {
					return true
				}
			}
			return false
		}
	}
	var matched []fileSet
	for _, t := range c.OBU.Tracked {
		dir := ""
		offset := ""
		for _, d := range c.OBU.Directories {
			rooted := fmt.Sprintf("%s%c", d, filepath.Separator)
			isRooted := strings.HasPrefix(t, rooted)
			if t == d || isRooted {
				dir = d
				if isRooted && t != d {
					offset = strings.TrimPrefix(t, rooted)
				}
				break
			}
		}
		if dir == "" {
			return nil, fmt.Errorf("unable to determine how %s is tracked", t)
		}
		walk := c.Device.upper(dir)
		if offset != "" {
			walk = filepath.Join(walk, offset)
		}
		if !pathExists(walk) {
			continue
		}
		err := filepath.Walk(walk, func(p string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			use := strings.TrimPrefix(p, walk)
			if offset != "" {
				use = filepath.Join(use, offset)
			}
			use = strings.TrimPrefix(use, pathSep)
			if filter(filepath.Join(dir, use)) {
				matched = append(matched, fileSet{offset: use, base: dir})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(matched) == 0 {
		return nil, errors.New("no files matched")
	}
	return matched, nil
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func (c Config) mount(hookDir string) error {
	for _, dir := range []string{c.Device.Mount, c.OBU.RootBind} {
		if !pathExists(dir) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
	}
	for _, m := range c.OBU.Modules {
		if err := pipedCommand("modprobe", m).Run(); err != nil {
			return err
		}
	}
	for k, v := range map[string]string{
		c.Device.Source: c.Device.Mount,
		"/":             c.OBU.RootBind,
	} {
		if err := pipedCommand("mount", k, v).Run(); err != nil {
			return err
		}
	}
	for _, d := range c.OBU.Directories {
		upper := c.Device.upper(d)
		work := c.Device.work(d)
		rootfs := c.Device.root(d)
		for k, v := range map[string]bool{
			upper:  true,
			work:   true,
			rootfs: false,
		} {
			exists := pathExists(k)
			if exists && v {
				if err := os.RemoveAll(k); err != nil {
					return err
				}
				exists = false
			}
			if !exists {
				if err := os.MkdirAll(k, 0o755); err != nil {
					return err
				}
			}
		}
		name := fmt.Sprintf("overlayfs-rootfs-%s", d)
		options := fmt.Sprintf("lowerdir=%s:%s,upperdir=%s,workdir=%s", rootfs, filepath.Join(c.OBU.RootBind, d), upper, work)
		if err := pipedCommand("mount", "-t", "overlay", "-o", options, name, fmt.Sprintf("%s%s", pathSep, d)).Run(); err != nil {
			return err
		}
	}
	if !pathExists(hookDir) {
		return nil
	}
	files, err := os.ReadDir(hookDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() {
			return fmt.Errorf("unexpected directory in hooks area: %s", name)
		}
		if err := pipedCommand(filepath.Join(hookDir, name)).Run(); err != nil {
			return err
		}
	}
	return nil
}

func pipedCommand(cmd string, args ...string) *exec.Cmd {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}
