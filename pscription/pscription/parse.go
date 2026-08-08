package pscription

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/goodblaster/errors"
)

// ParseStep parses a single step token. Grammar (shared by the CLI and .rx
// files):
//
//	wait:user | wait:<duration>   a wait step
//	@name[:out]                   a built-in extraction script
//	path.js[:out]                 a user script file (resolved against baseDir)
//	<name>                        a built-in pipeline step
//
// The optional ":out" suffix on script steps captures the return value under
// that key in the report's Outputs.
func ParseStep(token, baseDir string) (Step, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Step{}, errors.New("empty step")
	}

	if strings.HasPrefix(token, "wait:") {
		return Step{Kind: KindWait, Ref: strings.TrimPrefix(token, "wait:")}, nil
	}

	ref, out := splitCapture(token)

	switch {
	case strings.HasPrefix(ref, "@"):
		name := strings.TrimPrefix(ref, "@")
		// Auto-name the output after the script (@article → "article") so
		// -only and jq have a predictable key even without an explicit :out.
		if out == "" {
			out = name
		}
		return Step{Kind: KindBuiltinScript, Ref: name, Out: out}, nil
	case strings.HasSuffix(ref, ".js"):
		if out == "" {
			out = defaultOutName(ref)
		}
		if baseDir != "" && !filepath.IsAbs(ref) {
			ref = filepath.Join(baseDir, ref)
		}
		return Step{Kind: KindScriptFile, Ref: ref, Out: out}, nil
	default:
		if out != "" {
			return Step{}, errors.Newf("step %q cannot capture output (only scripts can)", token)
		}
		return Step{Kind: KindBuiltin, Ref: ref}, nil
	}
}

// defaultOutName derives an output key from a script path: the base name
// without the .js extension, with unfriendly characters replaced.
func defaultOutName(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".js")
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, base)
}

// splitCapture separates a "ref:out" token. The capture name must be a bare
// identifier, so paths and other colons (rare in step tokens) aren't
// misread.
func splitCapture(token string) (ref, out string) {
	i := strings.LastIndexByte(token, ':')
	if i < 0 {
		return token, ""
	}
	candidate := token[i+1:]
	if candidate != "" && isIdentifier(candidate) {
		return token[:i], candidate
	}
	return token, ""
}

func isIdentifier(s string) bool {
	for _, r := range s {
		if !(r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// ParseTokens parses a list of step tokens (as given on the command line)
// into steps. Script paths resolve against baseDir (usually the cwd).
func ParseTokens(tokens []string, baseDir string) ([]Step, error) {
	steps := make([]Step, 0, len(tokens))
	for _, t := range tokens {
		s, err := ParseStep(t, baseDir)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, nil
}

// ParseFile reads a .rx pscription file. Format is line-oriented:
//
//	#!/usr/bin/env pscribe          shebang (ignored)
//	# comment                  ignored
//	prep: minimal              directive
//	headful: true              directive
//	vars: ./secrets.json       merge a JSON object of variables
//	var: key=value             set one string variable
//	@article:content           a step (any step token, one per line)
//
// Script paths and vars files resolve relative to the .rx file's directory,
// so a pscription is portable.
func ParseFile(path string) (*Pscription, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot open pscription %q", path)
	}
	defer f.Close()

	baseDir := filepath.Dir(path)
	p := &Pscription{Vars: map[string]any{}}

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // blank, comment, or shebang
		}

		if key, val, ok := directive(line); ok {
			if err := p.applyDirective(key, val, baseDir); err != nil {
				return nil, errors.Wrapf(err, "%s:%d", path, lineNo)
			}
			continue
		}

		step, err := ParseStep(line, baseDir)
		if err != nil {
			return nil, errors.Wrapf(err, "%s:%d", path, lineNo)
		}
		p.Steps = append(p.Steps, step)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrapf(err, "reading %q", path)
	}
	return p, nil
}

// directive recognizes "key: value" lines whose key is a known directive.
func directive(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:i])
	val = strings.TrimSpace(line[i+1:])
	switch key {
	case "prep", "headful", "vars", "var", "profile":
		return key, val, true
	}
	return "", "", false
}

func (p *Pscription) applyDirective(key, val, baseDir string) error {
	switch key {
	case "prep":
		p.Prep = val
	case "headful":
		p.Headful = val == "true" || val == "yes" || val == "1"
	case "profile":
		path := val
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		} else if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		p.Profile = path
	case "vars":
		path := val
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		return p.mergeVarsFile(path)
	case "var":
		k, v, ok := strings.Cut(val, "=")
		if !ok {
			return errors.Newf("bad var %q (want key=value)", val)
		}
		p.Vars[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return nil
}

// mergeVarsFile loads a JSON object of variables and merges it into Vars.
// Keeping credentials in a separate (gitignored) file, referenced by the
// pscription, is the intended way to apply logins without embedding secrets
// in the script logic.
func (p *Pscription) mergeVarsFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "cannot read vars file %q", path)
	}
	var vars map[string]any
	if err := json.Unmarshal(b, &vars); err != nil {
		return errors.Wrapf(err, "vars file %q is not a JSON object", path)
	}
	if p.Vars == nil {
		p.Vars = map[string]any{}
	}
	for k, v := range vars {
		p.Vars[k] = v
	}
	return nil
}
