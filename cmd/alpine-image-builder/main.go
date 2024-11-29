// Package main is the generator wrapper
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
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
		Tags       []string
		Repository struct {
			URL          string
			Repositories []string
		}
		Architecture string
		Name         string
		Source       struct {
			Template  string
			Arguments []string
			Overlay   string
		}
		Variables  map[string]Command
		PreProcess []Command
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
	scripts := flag.String("scripts", "", "path to generation scripts")
	flag.Parse()
	b, err := os.ReadFile(*inConfig)
	if err != nil {
		return err
	}
	scriptDir := *scripts
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
	did := false
	isDebug := *debug
	to := *output
	tmp, err := os.MkdirTemp("", "alpine-image.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for idx := range cfg.Tags {
		did = true
		if err := cfg.run(idx, isDebug, tmp, to, scriptDir); err != nil {
			return err
		}
	}
	if !did {
		return errors.New("no tags processed")
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

func (cfg Config) run(idx int, debug bool, dir, to, scripts string) error {
	var tag string
	tag = cfg.Tags[idx]
	rawTag := tag
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
		tag = fmt.Sprintf("v%s", strings.Join(parts[0:2], "."))
	} else {
		if rawTag != "edge" {
			return fmt.Errorf("unknown version/not edge: %s", rawTag)
		}
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
		Name string
		Arch string
		Tag  string
	}
	base := Definition{cfg.Name, cfg.Architecture, tag}
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

	copied := filepath.Join(dir, "scripts")
	if err := exec.Command("cp", "-r", scripts, copied).Run(); err != nil {
		return err
	}
	profile := filepath.Join(copied, fmt.Sprintf("mkimg.%s.sh", cfg.Name))
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
		if err := os.WriteFile(filepath.Join(copied, fmt.Sprintf("genapkovl-%s.sh", cfg.Name)), ovl.Bytes(), 0o755); err != nil {
			return err
		}
	}
	templating := struct {
		Definition
		Scripts string
	}{}
	templating.Definition = base
	templating.Scripts = copied
	for _, c := range cfg.PreProcess {
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
		"--arch", cfg.Architecture,
		"--profile", cfg.Name,
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
