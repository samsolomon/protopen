// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "deploy":
		cmdDeploy(args)
	case "list":
		cmdList(args)
	case "deploys":
		cmdDeploys(args)
	case "rollback":
		cmdRollback(args)
	case "token":
		cmdToken(args)
	case "visibility":
		cmdVisibility(args)
	case "comments":
		cmdComments(args)
	case "login":
		cmdLogin(args)
	case "logout":
		cmdLogout(args)
	case "config":
		cmdConfig(args)
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: protopen <command> [options]

Commands:
  deploy <path>                Deploy a folder or zip to Protopen
  list                         List your sites
  deploys <name>               Show deploy history for a site
  rollback <name> <deploy-id>  Roll back to a previous deploy
  visibility <name> <public|private>  Set site visibility
  comments <name>              Read comments left on a site
  token                        Show current token info
  login                        Authenticate and save your API token
  logout                       Remove saved token
  config                       View and update CLI configuration
  version                      Print the CLI version

Configuration:
  Config file: ~/.protopen/config.json
  Priority:    --flag > PROTOPEN_TOKEN/PROTOPEN_URL > config file > default`)
}

func cmdDeploy(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	name := fs.String("name", "", "Site name (defaults to directory/zip name)")
	label := fs.String("label", "", "Deploy label (e.g. 'v2 with new header')")
	private := fs.Bool("private", false, "Make the site private after deploy")
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides PROTOPEN_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	jsonOutput := fs.Bool("json", false, "Output JSON response")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: protopen deploy <path> [--name NAME] [--private] [--token TOKEN] [--url URL] [--json]")
		os.Exit(1)
	}

	path := fs.Arg(0)
	client := newClient(requireToken(*token), resolveURL(*url))

	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	siteName := *name
	if siteName == "" {
		siteName = info.Name()
		if !info.IsDir() {
			siteName = stripZipExt(siteName)
		}
	}

	resolvedOrg := resolveOrg(*org)
	git := detectGitMeta(path)

	var result deployResult
	if info.IsDir() {
		result, err = client.deployDirectory(path, siteName, *label, resolvedOrg, git)
	} else {
		result, err = client.deployZip(path, siteName, *label, resolvedOrg, git)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *private {
		if err := client.updateVisibility(result.siteID, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: deployed but could not set private: %v\n", err)
		}
	}

	if *jsonOutput {
		fmt.Println(result.raw)
	} else {
		fmt.Println(result.liveURL)
		if git != nil {
			dirty := ""
			if git.Dirty {
				dirty = " (dirty)"
			}
			branch := git.Branch
			if branch == "" {
				branch = "detached"
			}
			fmt.Fprintf(os.Stderr, "  git: %s %s%s\n", branch, git.CommitHash[:8], dirty)
		}
	}
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides PROTOPEN_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	fs.Parse(args)

	client := newClient(requireToken(*token), resolveURL(*url))
	sites, err := client.listSites(resolveOrg(*org))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(sites) == 0 {
		fmt.Fprintln(os.Stderr, "no sites")
		return
	}

	for _, p := range sites {
		vis := "public"
		if !p.IsPublic {
			vis = "private"
		}
		fmt.Printf("%-24s %3d deploys  %-7s  %s\n", p.Name, p.DeployCount, vis, p.LiveURL)
	}
}

func cmdDeploys(args []string) {
	fs := flag.NewFlagSet("deploys", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides PROTOPEN_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: protopen deploys <site-name>")
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findSite(fs.Arg(0), resolveOrg(*org))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	deploys, err := client.listDeploys(proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(deploys) == 0 {
		fmt.Fprintln(os.Stderr, "no deploys")
		return
	}

	for _, d := range deploys {
		current := "  "
		if d.IsCurrent {
			current = "* "
		}
		label := ""
		if d.Label != nil && *d.Label != "" {
			label = "  " + *d.Label
		}
		gitInfo := ""
		if d.GitCommitHash != nil && *d.GitCommitHash != "" {
			short := *d.GitCommitHash
			if len(short) > 8 {
				short = short[:8]
			}
			gitInfo = "  " + short
			if d.GitBranch != nil && *d.GitBranch != "" {
				gitInfo += " (" + *d.GitBranch + ")"
			}
			if d.GitDirty != nil && *d.GitDirty {
				gitInfo += "*"
			}
		}
		fmt.Printf("%s%-20s %4d files  %s%s%s\n", current, d.ID, d.FileCount, d.CreatedAt, label, gitInfo)
	}
}

func cmdComments(args []string) {
	fs := flag.NewFlagSet("comments", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides PROTOPEN_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	deploy := fs.String("deploy", "", "Deploy ID to scope comments to (defaults to current)")
	status := fs.String("status", "open", "Filter: open | resolved | all")
	asJSON := fs.Bool("json", false, "Print raw JSON")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: protopen comments <site-name> [--status open|resolved|all] [--deploy <id>] [--json]")
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findSite(fs.Arg(0), resolveOrg(*org))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	comments, err := client.listComments(proj.ID, struct{ DeployID, Status string }{*deploy, *status})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"site": proj.Slug, "comments": comments})
		return
	}

	if len(comments) == 0 {
		fmt.Fprintf(os.Stderr, "no %s comments on %s\n", *status, proj.Slug)
		return
	}

	// Group: root -> replies.
	byParent := map[string][]commentInfo{}
	roots := []commentInfo{}
	for _, c := range comments {
		if c.ParentID == nil {
			roots = append(roots, c)
		} else {
			byParent[*c.ParentID] = append(byParent[*c.ParentID], c)
		}
	}

	for i, root := range roots {
		marker := " "
		if root.ResolvedAt != nil {
			marker = "✓"
		}
		author := "(unknown)"
		if root.Author != nil {
			author = root.Author.Name
			if author == "" {
				author = "@" + root.Author.Username
			}
		}
		page := root.PagePath
		if page == "" {
			page = "/"
		}
		fmt.Printf("%s [%d] %s — %s · %s\n", marker, i+1, author, page, abbrevTime(root.CreatedAt))
		printIndented(root.Body, "   ")
		for _, reply := range byParent[root.ID] {
			rauthor := "(unknown)"
			if reply.Author != nil {
				rauthor = reply.Author.Name
				if rauthor == "" {
					rauthor = "@" + reply.Author.Username
				}
			}
			fmt.Printf("   ↪ %s · %s\n", rauthor, abbrevTime(reply.CreatedAt))
			printIndented(reply.Body, "     ")
		}
		fmt.Println()
	}
}

func printIndented(text, prefix string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Println(prefix + line)
	}
}

func abbrevTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func cmdRollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides PROTOPEN_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: protopen rollback <site-name> <deploy-id>")
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findSite(fs.Arg(0), resolveOrg(*org))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = client.rollback(proj.ID, fs.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Rolled back %s to %s\n", proj.Name, fs.Arg(1))
}

func cmdToken(args []string) {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	token := fs.String("token", "", "API token (overrides PROTOPEN_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	fs.Parse(args)

	t := requireToken(*token)
	apiURL := resolveURL(*url)
	client := newClient(t, apiURL)
	user, err := client.getSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Token:    %s\n", maskToken(t))
	fmt.Printf("User:     %s (%s)\n", user.Name, user.Email)
	fmt.Printf("API:      %s\n", apiURL)
}

func resolveToken(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("PROTOPEN_TOKEN"); env != "" {
		return env
	}
	return loadConfig().Token
}

func resolveURL(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("PROTOPEN_URL"); env != "" {
		return env
	}
	if cfg := loadConfig().URL; cfg != "" {
		return cfg
	}
	return "http://localhost:8080"
}

func resolveOrg(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("PROTOPEN_ORG"); env != "" {
		return env
	}
	return loadConfig().Org
}

func maskToken(t string) string {
	if len(t) <= 12 {
		return t
	}
	return t[:8] + "..." + t[len(t)-4:]
}

func requireToken(flag string) string {
	t := resolveToken(flag)
	if t == "" {
		fmt.Fprintln(os.Stderr, `Error: no API token configured.

Set up authentication:
  protopen login

Or provide a token directly:
  protopen deploy --token ptk_your_token_here

Create tokens in the Protopen dashboard under Settings > API Tokens.`)
		os.Exit(1)
	}
	return t
}

func cmdVisibility(args []string) {
	fs := flag.NewFlagSet("visibility", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides PROTOPEN_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: protopen visibility <site-name> <public|private>")
		os.Exit(1)
	}

	siteName := fs.Arg(0)
	visibility := fs.Arg(1)

	var isPublic bool
	switch visibility {
	case "public":
		isPublic = true
	case "private":
		isPublic = false
	default:
		fmt.Fprintf(os.Stderr, "invalid visibility %q: must be \"public\" or \"private\"\n", visibility)
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findSite(siteName, resolveOrg(*org))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := client.updateVisibility(proj.ID, isPublic); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s is now %s\n", proj.Name, visibility)
}

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	token := fs.String("token", "", "API token")
	url := fs.String("url", "", "API base URL (overrides PROTOPEN_URL)")
	fs.Parse(args)

	t := *token
	if t == "" {
		t = loginWithDeviceCode(resolveURL(*url))
	}

	if t == "" {
		fmt.Fprintln(os.Stderr, "no token provided")
		os.Exit(1)
	}

	client := newClient(t, resolveURL(*url))
	user, err := client.getSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg := loadConfig()
	cfg.Token = t
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nAuthenticated as %s (%s)\n", user.Name, user.Email)
	fmt.Printf("Token saved to %s\n", configPath())
	fmt.Println("\nYou're ready to deploy:")
	fmt.Println("  protopen deploy ./my-site")
}

func loginWithDeviceCode(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")

	// Create device code
	resp, err := http.Post(baseURL+"/api/auth/device", "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not connect to %s\n", baseURL)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		fmt.Fprintf(os.Stderr, "error: could not create device code (HTTP %d)\n", resp.StatusCode)
		os.Exit(1)
	}

	var device struct {
		DeviceCode string `json:"deviceCode"`
		VerifyURL  string `json:"verifyUrl"`
		ExpiresIn  int    `json:"expiresIn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&device); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid response\n")
		os.Exit(1)
	}

	// Open browser
	fmt.Printf("Opening browser to approve access...\n")
	if err := openBrowser(device.VerifyURL); err != nil {
		fmt.Printf("\nOpen this URL in your browser:\n  %s\n", device.VerifyURL)
	}
	fmt.Printf("\nWaiting for approval... (press Ctrl+C to cancel)\n")

	// Poll for result
	httpClient := &http.Client{}
	deadline := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		pollResp, err := httpClient.Get(baseURL + "/api/auth/device/" + device.DeviceCode)
		if err != nil {
			continue
		}

		var result struct {
			Status string `json:"status"`
			Token  string `json:"token"`
		}
		json.NewDecoder(pollResp.Body).Decode(&result)
		pollResp.Body.Close()

		if result.Status == "complete" && result.Token != "" {
			return result.Token
		}

		if pollResp.StatusCode == 404 {
			fmt.Fprintln(os.Stderr, "\nDevice code expired.")
			os.Exit(1)
		}
	}

	fmt.Fprintln(os.Stderr, "\nTimed out waiting for approval.")
	os.Exit(1)
	return ""
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func cmdLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	fs.Parse(args)

	cfg := loadConfig()
	cfg.Token = ""
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Token removed from", configPath())
}

func cmdConfig(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `Usage: protopen config <subcommand>

Subcommands:
  show               Show current configuration
  set token <value>  Save API token
  set url <value>    Save API base URL`)
		os.Exit(1)
	}

	switch args[0] {
	case "show":
		cfg := loadConfig()
		url := resolveURL("")
		org := resolveOrg("")
		fmt.Printf("Token:   %s\n", maskToken(cfg.Token))
		fmt.Printf("URL:     %s\n", url)
		if org != "" {
			fmt.Printf("Org:     %s\n", org)
		}
		fmt.Printf("Config:  %s\n", configPath())

	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: protopen config set <key> <value>")
			os.Exit(1)
		}
		cfg := loadConfig()
		switch args[1] {
		case "token":
			cfg.Token = args[2]
		case "url":
			cfg.URL = args[2]
		case "org":
			cfg.Org = args[2]
		default:
			fmt.Fprintf(os.Stderr, "unknown config key: %s\n", args[1])
			os.Exit(1)
		}
		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Saved %s to %s\n", args[1], configPath())

	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func stripZipExt(name string) string {
	if len(name) > 4 && name[len(name)-4:] == ".zip" {
		return name[:len(name)-4]
	}
	return name
}
