// Package main is the generator wrapper
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
)

type (
	// Command are config commands that require a call and arguments
	Command struct {
		Call      string
		Arguments []string
	}
	// Config handles input build configurations
	Config struct {
		Definition struct {
			Name       string
			PreProcess []Command
			Arch       string
			Tag        string
		}
		Repository struct {
			URL          string
			Repositories []string
		}
		Source struct {
			Scripts   string
			Patches   []string
			Template  string
			Arguments []string
			Overlay   string
		}
		Variables map[string]Command
	}
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	inConfig := flag.String("config", "", "configuration file")
	debug := flag.Bool("debug", false, "enable debugging")
	output := flag.String("output", "", "output directory for artifacts")
	workdir := flag.String("workdir", "", "working directory")
	flag.Parse()
	b, err := os.ReadFile(*inConfig)
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
	isDebug := *debug
	work := *workdir
	to := filepath.Join(work, *output)
	if !pathExists(to) {
		if err := os.MkdirAll(to, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.MkdirTemp("", "alpine-image.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := cfg.run(isDebug, tmp, to, work); err != nil {
		return err
	}
	return nil
}

func simpleTemplate(in string, obj interface{}) (bytes.Buffer, error) {
	t, err := template.New("t").Parse(in)
	if err != nil {
		return bytes.Buffer{}, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, obj); err != nil {
		return bytes.Buffer{}, err
	}
	return buf, nil
}

func (cfg Config) run(debug bool, dir, to, workdir string) error {
	tag := cfg.Definition.Tag
	rawTag := tag
	var branch string
	if strings.Contains(rawTag, ".") {
		parts := strings.Split(tag, ".")
		if len(parts) != 3 {
			return fmt.Errorf("version must be X.Y.Z (have: %s)", parts)
		}
		for _, p := range parts {
			if _, err := strconv.Atoi(p); err != nil {
				return fmt.Errorf("invalid version, non-numeric? %v (%w)", parts, err)
			}
		}
		branch = strings.Join(parts[0:2], ".")
		tag = fmt.Sprintf("v%s", branch)
	} else {
		if rawTag != "edge" {
			return fmt.Errorf("unknown version/not edge: %s", rawTag)
		}
		branch = "master"
	}
	cmds := make(map[string][]string)
	if cfg.Variables != nil {
		for n, c := range cfg.Variables {
			t, err := exec.Command(c.Call, c.Arguments...).Output()
			if err != nil {
				return err
			}
			cmds[n] = strings.Split(string(t), "\n")
		}
	}
	type Definition struct {
		Name   string
		Arch   string
		Tag    string
		Branch string
	}
	base := Definition{cfg.Definition.Name, cfg.Definition.Arch, tag, branch}
	obj := struct {
		Definition
		Commands map[string][]string
	}{base, cmds}
	buf, err := simpleTemplate(cfg.Source.Template, obj)
	if err != nil {
		return err
	}
	if debug {
		fmt.Printf("profile: %s", buf.String())
	}
	var repositories []string
	url := cfg.Repository.URL
	for _, repo := range cfg.Repository.Repositories {
		r, err := template.New("t").Parse(fmt.Sprintf("%s/%s", url, repo))
		if err != nil {
			return err
		}
		var text bytes.Buffer
		if err := r.Execute(&text, obj); err != nil {
			return err
		}

		repositories = append(repositories, "--repository", text.String())
	}
	scriptFiles := filepath.Join(to, fmt.Sprintf("%s.scripts.tar.gz", cfg.Definition.Tag))
	if !pathExists(scriptFiles) {
		url, err := simpleTemplate(cfg.Source.Scripts, obj)
		if err != nil {
			return err
		}
		urlText := url.String()
		fmt.Printf("scripts downloading: %s\n", urlText)
		resp, err := http.Get(urlText)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		files, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if err := os.WriteFile(scriptFiles, files, 0o644); err != nil {
			return err
		}
	}

	if err := exec.Command("tar", "xf", scriptFiles, "--strip-components", "1", "-C", dir).Run(); err != nil {
		return err
	}
	copied := filepath.Join(dir, "scripts")
	var patches []string
	for _, p := range cfg.Source.Patches {
		full := filepath.Join(workdir, p)
		adding := []string{full}
		if strings.Contains(p, "*") {
			globbed, err := filepath.Glob(full)
			if err != nil {
				return err
			}
			if len(globbed) == 0 {
				return fmt.Errorf("no files matched for patches: %s", p)
			}
			adding = globbed
		}
		patches = append(patches, adding...)
	}
	for _, p := range patches {
		fmt.Printf("applying patch: %s\n", p)
		cmd := exec.Command("patch", "-p1", "-i", p)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	profile := filepath.Join(copied, fmt.Sprintf("mkimg.%s.sh", cfg.Definition.Name))
	if err := os.WriteFile(profile, buf.Bytes(), 0o755); err != nil {
		return err
	}
	if cfg.Source.Overlay != "" {
		ovl, err := simpleTemplate(cfg.Source.Overlay, obj)
		if err != nil {
			return err
		}
		if debug {
			fmt.Printf("overlay: %s\n", ovl.String())
		}
		if err := os.WriteFile(filepath.Join(copied, fmt.Sprintf("genapkovl-%s.sh", cfg.Definition.Name)), ovl.Bytes(), 0o755); err != nil {
			return err
		}
	}
	templating := struct {
		Definition
		Scripts string
	}{}
	templating.Definition = base
	templating.Scripts = copied
	for _, c := range cfg.Definition.PreProcess {
		var args []string
		for _, a := range c.Arguments {
			buf, err := simpleTemplate(a, templating)
			if err != nil {
				return err
			}
			args = append(args, buf.String())
		}
		cmd := exec.Command(c.Call, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	args := []string{
		filepath.Join(copied, "mkimage.sh"),
		"--outdir", to,
		"--arch", cfg.Definition.Arch,
		"--profile", cfg.Definition.Name,
		"--tag", rawTag,
	}
	args = append(args, repositories...)
	args = append(args, cfg.Source.Arguments...)
	if debug {
		fmt.Printf("mkimage arguments: %v\n", args)
	}
	cmd := exec.Command("sh", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}
